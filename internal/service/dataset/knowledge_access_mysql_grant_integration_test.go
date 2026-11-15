//go:build integration && linux

package dataset

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

// Only this test context selects the second real connection. Production
// command/session/CAS/replay logic remains unchanged and uses dao.DB.
type mysqlGrantWriterKey struct{}
type mysqlGrantPool struct {
	*mysqlAccessConn
	writer *sql.Conn
}

func (p *mysqlGrantPool) selectConnection(ctx context.Context) *sql.Conn {
	if writer, _ := ctx.Value(mysqlGrantWriterKey{}).(bool); writer {
		return p.writer
	}
	return p.Conn
}
func (p *mysqlGrantPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.selectConnection(ctx).PrepareContext(ctx, query)
}
func (p *mysqlGrantPool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.selectConnection(ctx).ExecContext(ctx, query, args...)
}
func (p *mysqlGrantPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.selectConnection(ctx).QueryContext(ctx, query, args...)
}
func (p *mysqlGrantPool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.selectConnection(ctx).QueryRowContext(ctx, query, args...)
}
func (p *mysqlGrantPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	if writer, _ := ctx.Value(mysqlGrantWriterKey{}).(bool); writer {
		return p.writer.BeginTx(ctx, opts)
	}
	return p.mysqlAccessConn.BeginTx(ctx, opts)
}

type mysqlGrantEngine struct {
	*accessTestEngine
	returned chan struct{}
	once     sync.Once
}

func (e *mysqlGrantEngine) Search(ctx context.Context, request *types.SearchRequest) (*types.SearchResult, error) {
	result, err := e.accessTestEngine.Search(ctx, request)
	if err == nil {
		e.once.Do(func() { close(e.returned) })
	}
	return result, err
}

func TestKnowledgeAccessMySQLGrantBarriersAndCommandConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("real MySQL integration excluded by short mode")
	}
	// A new finite RAM/lease binding is required. Old C08 cannot activate it.
	if os.Getenv("LFEN_ACCESS_MYSQL_GRANT_RUNTIME") != "root-approved-exact-grant" {
		t.Skip("exact grant MySQL scope not granted")
	}
	lease, fixture, input := readMySQLAccessInput(t)
	requireMySQLAccess(t, entity.ValidKnowledgeLexicalHash(input.OwnerSessionSHA), "GRANT_OWNER_RAM_SHA_REQUIRED")
	ctx, cancel := context.WithTimeout(t.Context(), 105*time.Second)
	defer cancel()
	deadline, ok := ctx.Deadline()
	requireMySQLAccess(t, ok && lease.Expires.After(deadline), "GRANT_ROOT_DEADLINE")
	verifyMySQLAccessResources(t, lease)
	requireMySQLAccess(t, tokenizer.Init(&tokenizer.PoolConfig{DictPath: lease.DictPath, MinSize: 1, MaxSize: 1, IdleTimeout: time.Minute, AcquireTimeout: time.Second}) == nil, "GRANT_TOKENIZER_INIT")
	defer tokenizer.Close()
	producer, err := tokenizer.ProducerSHA256()
	requireMySQLAccess(t, err == nil && producer == fixture.ProducerSHA, "GRANT_PRODUCER_EXACT")
	cfg, err := mysqlDriver.ParseDSN(input.DSN)
	requireMySQLAccess(t, err == nil && cfg.Net == "tcp" && cfg.Addr == lease.Address && cfg.DBName == lease.Schema, "GRANT_EXACT_DSN")
	cfg.ParseTime, cfg.MultiStatements, cfg.AllowAllFiles, cfg.AllowCleartextPasswords, cfg.InterpolateParams = true, false, false, false, false
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 3*time.Second, 5*time.Second, 5*time.Second
	connector, err := mysqlDriver.NewConnector(cfg)
	requireMySQLAccess(t, err == nil, "GRANT_CONNECTOR")
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(3)
	pool.SetMaxIdleConns(0)
	defer pool.Close()
	reader, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "GRANT_READER")
	defer reader.Close()
	writer, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "GRANT_WRITER")
	defer writer.Close()
	observer, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "GRANT_OBSERVER")
	defer observer.Close()
	_, err = observer.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	requireMySQLAccess(t, err == nil, "GRANT_OBSERVER_READ_ONLY")
	ids := make([]uint64, 3)
	for i, conn := range []*sql.Conn{reader, writer, observer} {
		var database string
		err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID(),DATABASE()").Scan(&ids[i], &database)
		requireMySQLAccess(t, err == nil && database == lease.Schema, "GRANT_CONNECTION_SCHEMA")
	}
	requireMySQLAccess(t, ids[0] != ids[1] && ids[0] != ids[2] && ids[1] != ids[2], "GRANT_DISTINCT_PHYSICAL_CONNECTIONS")
	db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: &mysqlGrantPool{mysqlAccessConn: &mysqlAccessConn{Conn: reader}, writer: writer}, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	requireMySQLAccess(t, err == nil, "GRANT_GORM")
	oldDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = oldDB }()
	oldStore := storage.GetStorageFactory().GetStorage()
	store := storage.NewMemoryStorage()
	for _, object := range fixture.Objects {
		requireMySQLAccess(t, store.Put(ctx, object.Bucket, object.Key, []byte(object.Body)) == nil, "GRANT_SYNTHETIC_OBJECT")
	}
	storage.GetStorageFactory().SetStorage(store)
	defer storage.GetStorageFactory().SetStorage(oldStore)
	service := NewDatasetService()
	recipient := context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: fixture.Recipient, SHA256: input.SessionSHA})
	owner := context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: fixture.Owner, SHA256: input.OwnerSessionSHA})
	ownerWriter := context.WithValue(owner, mysqlGrantWriterKey{}, true)
	callOwner := func(tb *testing.T, callCtx context.Context, operation string, body any, expected int) mysqlAccessReply {
		reply := callMySQLAccess(service, callCtx, fixture.Owner, operation, common.GenerateUUID(), body)
		requireMySQLAccess(tb, reply.status == expected && entity.KnowledgeAccessDecode(reply.raw, accessDefinition(operation), nil) == nil, "GRANT_REAL_OWNER_COMMAND")
		return reply
	}
	revokeBody := func(grant, key string) entity.KnowledgeAccessRevokeGrantRequest {
		return entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: fixture.Domain, GrantID: grant, Confirmed: true, IdempotencyKey: key}
	}
	callOwner(t, ownerWriter, "revokeGrant", revokeBody(fixture.Grant, common.GenerateUUID()), 200)
	var oldRefs []entity.KnowledgeAccessEvidenceRef
	epoch := func(tb *testing.T) int64 {
		var value int64
		requireMySQLAccess(tb, observer.QueryRowContext(ctx, "SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=?", fixture.Tenant).Scan(&value) == nil, "GRANT_EPOCH")
		return value
	}
	checkDeniedRef := func(tb *testing.T, ref entity.KnowledgeAccessEvidenceRef) {
		body := entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: json.RawMessage(accessJSON(entity.KnowledgeAccessEvidenceReadSelector{Kind: "evidence", EvidenceRef: ref}))}
		reply := callMySQLAccess(service, recipient, fixture.Recipient, "read", common.GenerateUUID(), body)
		var envelope accessTestReply
		requireMySQLAccess(tb, (reply.status == 404 || reply.status == 409) && json.Unmarshal(reply.raw, &envelope) == nil && bytes.Equal(envelope.Data, []byte("null")) && entity.KnowledgeAccessDecode(reply.raw, "ReadResponse", nil) == nil, "GRANT_OLD_REF_NEVER_REVIVED")
	}
	for _, phase := range []string{"before_begin", "before_commit"} {
		t.Run(phase, func(t *testing.T) {
			key := common.GenerateUUID()
			created := callOwner(t, ownerWriter, "createGrant", entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: fixture.Domain, ModuleID: fixture.Module, ExpectedModuleVersion: fixture.ModuleVersion, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: fixture.Recipient}, Mode: "read_search", Delivery: "web_only", ExpiresAt: accessTime(fixture.GrantExpires), Confirmed: true, IdempotencyKey: key}, 201)
			grant := accessTestData[entity.KnowledgeAccessGrant](t, decodeMySQLGrantReply(t, created))
			for _, ref := range oldRefs {
				checkDeniedRef(t, ref)
			}
			inspected := callMySQLAccess(service, recipient, fixture.Recipient, "inspect", common.GenerateUUID(), entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: fixture.Domain, ModuleID: fixture.Module, Mode: "read_search", Delivery: "web_only"})
			requireMySQLAccess(t, inspected.status == 200 && entity.KnowledgeAccessDecode(inspected.raw, "InspectResponse", nil) == nil, "GRANT_SEARCH_VIEW")
			view := accessTestData[entity.KnowledgeAccessInspectData](t, decodeMySQLGrantReply(t, inspected))
			viewExpires, err := time.Parse(time.RFC3339Nano, view.ExpiresAt)
			requireMySQLAccess(t, err == nil && viewExpires.After(deadline), "GRANT_VIEW_NOT_EXPIRING")
			baseline := callMySQLAccess(service, recipient, fixture.Recipient, "read", common.GenerateUUID(), mysqlAccessReadRequest(view.ViewID))
			requireMySQLAccess(t, baseline.status == 200, "GRANT_BASELINE_UNIT")
			unit := accessTestData[entity.KnowledgeAccessReadData](t, decodeMySQLGrantReply(t, baseline))
			oldRefs = append(oldRefs, unit.EvidenceRef)
			verified, err := service.accessOpenView(recipient, fixture.Recipient, view.ViewID, "search", common.GenerateUUID())
			requireMySQLAccess(t, err == nil && len(verified.Rows) == 1, "GRANT_ENGINE_FIXTURE_PROOF")
			row := verified.Rows[0]
			engine := &mysqlGrantEngine{accessTestEngine: &accessTestEngine{lexicalTestEngine: &lexicalTestEngine{rows: map[string]map[string]any{row.ID: {"id": row.ID, "kb_id": fixture.Domain, "doc_id": fixture.Target.Source.SourceID, "knowledge_binding_kwd": row.Binding, "knowledge_ref": accessJSON(row.Reference), "available_int": 0, "content_with_weight": unit.Text}}}}, returned: make(chan struct{})}
			service.docEngine = engine
			gate := newMySQLPermitGate(phase)
			callCtx, stop := context.WithTimeout(context.WithValue(recipient, mysqlPermitKey{}, gate), 10*time.Second)
			writeCtx := context.WithValue(context.WithValue(callCtx, lexicalSessionKey{}, lexicalSession{UserID: fixture.Owner, SHA256: input.OwnerSessionSHA}), mysqlGrantWriterKey{}, true)
			var workers sync.WaitGroup
			defer func() { stop(); workers.Wait() }()
			done := make(chan mysqlAccessReply, 1)
			requestID := common.GenerateUUID()
			beforeEpoch := epoch(t)
			workers.Add(1)
			go func() {
				defer workers.Done()
				done <- callMySQLAccess(service, callCtx, fixture.Recipient, "search", requestID, entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "synthetic", Limit: 1})
			}()
			waitMySQLAccess(t, callCtx, gate.entered, "GRANT_BUFFER_ENCODED_AT_FINAL_OWNER")
			waitMySQLAccess(t, callCtx, engine.returned, "GRANT_ENGINE_ALREADY_RETURNED")
			revokeKey := common.GenerateUUID()
			revoke := func() mysqlAccessReply {
				return callMySQLAccess(service, writeCtx, fixture.Owner, "revokeGrant", common.GenerateUUID(), revokeBody(grant.GrantID, revokeKey))
			}
			var revoked mysqlAccessReply
			if phase == "before_begin" {
				revoked = revoke()
				requireMySQLAccess(t, revoked.status == 200 && epoch(t) == beforeEpoch+1, "GRANT_REVOKE_COMMITTED_BEFORE_FINAL_BEGIN")
			} else {
				observeMySQLAccessIsolation(t, callCtx, observer, ids[0])
				var beforeOutcome string
				requireMySQLAccess(t, observer.QueryRowContext(callCtx, "SELECT outcome FROM knowledge_access_audit WHERE tenant_id=? AND actor_user_id=? AND action=? AND request_id=?", fixture.Tenant, fixture.Recipient, "search", requestID).Scan(&beforeOutcome) == nil && beforeOutcome == "denied", "GRANT_ALLOWED_AUDIT_NOT_DURABLE_BEFORE_COMMIT")
				writerDone := make(chan mysqlAccessReply, 1)
				workers.Add(1)
				go func() { defer workers.Done(); writerDone <- revoke() }()
				observeMySQLAccessWait(t, callCtx, observer, ids[1], ids[0], lease.Schema)
				observeMySQLAccessIsolation(t, callCtx, observer, ids[1])
				select {
				case <-done:
					t.Fatal("search released before permit commit")
				default:
				}
				select {
				case <-writerDone:
					t.Fatal("grant writer escaped current permit lock")
				default:
				}
				close(gate.release)
				waitMySQLAccess(t, callCtx, gate.committed, "GRANT_FINAL_ACTUAL_COMMIT")
				select {
				case revoked = <-writerDone:
				case <-callCtx.Done():
					t.Fatal("grant writer did not join")
				}
			}
			if phase == "before_begin" {
				close(gate.release)
			}
			var result mysqlAccessReply
			select {
			case result = <-done:
			case <-callCtx.Done():
				t.Fatal("search did not join")
			}
			requireMySQLAccess(t, revoked.status == 200 && entity.KnowledgeAccessDecode(revoked.raw, "RevokeGrantResponse", nil) == nil && epoch(t) == beforeEpoch+1, "GRANT_ONE_REAL_REVOKE")
			var revokedAt sql.NullTime
			var state, outcome string
			requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT revoked_at FROM knowledge_access_grant WHERE id=? AND tenant_id=?", grant.GrantID, fixture.Tenant).Scan(&revokedAt) == nil && revokedAt.Valid, "GRANT_ROW_DURABLY_REVOKED")
			requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT command_state FROM knowledge_access_audit WHERE actor_user_id=? AND command_operation='revokeGrant' AND command_key=?", fixture.Owner, revokeKey).Scan(&state) == nil && state == "completed", "GRANT_REVOKE_RECEIPT_COMPLETED")
			requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT outcome FROM knowledge_access_audit WHERE tenant_id=? AND actor_user_id=? AND action='search' AND request_id=?", fixture.Tenant, fixture.Recipient, requestID).Scan(&outcome) == nil, "GRANT_SEARCH_DURABLE_AUDIT")
			requireMySQLAccess(t, entity.KnowledgeAccessDecode(result.raw, "SearchResponse", nil) == nil && time.Now().Before(viewExpires) && time.Now().Before(fixture.GrantExpires), "GRANT_RESULT_NOT_EXPIRY")
			if phase == "before_begin" {
				var denied accessTestReply
				requireMySQLAccess(t, json.Unmarshal(result.raw, &denied) == nil && result.status == 409 && bytes.Equal(denied.Data, []byte("null")) && outcome == "denied", "GRANT_FIRST_NO_TEXT_METADATA_COUNT_REF")
			} else {
				data := accessTestData[entity.KnowledgeAccessSearchData](t, decodeMySQLGrantReply(t, result))
				requireMySQLAccess(t, result.status == 200 && outcome == "allowed" && data.ReturnedCount == 1 && data.TotalMatches == nil && len(data.Hits) == 1 && data.Hits[0].Text == unit.Text && data.Hits[0].EvidenceRef.ViewID == view.ViewID && lexicalSHA([]byte(data.Hits[0].Text)) == fixture.ExpectedSHA, "GRANT_PERMIT_FIRST_INFLIGHT_MAY_FINISH")
				oldRefs = append(oldRefs, data.Hits[0].EvidenceRef)
			}
			for _, ref := range oldRefs {
				checkDeniedRef(t, ref)
			}
		})
		if t.Failed() {
			return
		}
	}
	t.Run("MySQLCASAndNullableUnique", func(t *testing.T) {
		checkMySQLGrantCommandConstraints(t, ctx, db, observer, owner, ownerWriter, fixture)
	})
	t.Log("MYSQL_EXACT_GRANT_WRITER_TWO_ORDERS_NOT_NATIVE_FLAGS_OR_REAL_ES")
}

func decodeMySQLGrantReply(t *testing.T, reply mysqlAccessReply) accessTestReply {
	t.Helper()
	var envelope accessTestReply
	if json.Unmarshal(reply.raw, &envelope) != nil {
		t.Fatal("grant reply JSON")
	}
	return envelope
}

func checkMySQLGrantCommandConstraints(t *testing.T, ctx context.Context, db *gorm.DB, observer *sql.Conn, owner, ownerWriter context.Context, fixture mysqlAccessFixture) {
	t.Helper()
	var version, beforeEpoch int64
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT version FROM knowledge_access_domain_policy WHERE domain_id=? AND tenant_id=?", fixture.Domain, fixture.Tenant).Scan(&version) == nil, "MYSQL_CAS_POLICY_BASELINE")
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=?", fixture.Tenant).Scan(&beforeEpoch) == nil, "MYSQL_CAS_EPOCH_BASELINE")
	keys := []string{"Synthetic_MySQL_CAS_A_" + common.GenerateUUID(), "Synthetic_MySQL_CAS_B_" + common.GenerateUUID()}
	start := make(chan struct{})
	results := make(chan mysqlAccessReply, 2)
	var workers sync.WaitGroup
	for i, callCtx := range []context.Context{owner, ownerWriter} {
		workers.Add(1)
		go func(i int, callCtx context.Context) {
			defer workers.Done()
			<-start
			request := entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: fixture.Domain, ExpectedVersion: version, Enabled: false, Confirmed: true, IdempotencyKey: keys[i]}
			results <- callMySQLAccess(NewDatasetService(), callCtx, fixture.Owner, "setPolicy", common.GenerateUUID(), request)
		}(i, callCtx)
	}
	close(start)
	workers.Wait()
	close(results)
	passed, conflicted := 0, 0
	for reply := range results {
		requireMySQLAccess(t, entity.KnowledgeAccessDecode(reply.raw, "SetPolicyResponse", nil) == nil, "MYSQL_CAS_CLOSED_RESULT")
		if reply.status == 200 {
			passed++
		} else {
			var denied accessTestReply
			requireMySQLAccess(t, reply.status == 409 && json.Unmarshal(reply.raw, &denied) == nil && denied.Code == "version_conflict" && bytes.Equal(denied.Data, []byte("null")), "MYSQL_CAS_LOSER_VERSION_CONFLICT")
			conflicted++
		}
	}
	requireMySQLAccess(t, passed == 1 && conflicted == 1, "MYSQL_CAS_EXACTLY_ONE_WINNER")
	var afterEpoch, afterVersion, commands, pending, completed int64
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=?", fixture.Tenant).Scan(&afterEpoch) == nil && afterEpoch == beforeEpoch+1, "MYSQL_CAS_SINGLE_MONOTONIC_EPOCH")
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT version FROM knowledge_access_domain_policy WHERE domain_id=?", fixture.Domain).Scan(&afterVersion) == nil && afterVersion == version+1, "MYSQL_CAS_SINGLE_ENTITY_VERSION")
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(command_state='pending'),0),COALESCE(SUM(command_state='completed'),0) FROM knowledge_access_audit WHERE actor_user_id=? AND command_operation='setPolicy' AND command_key IN (?,?)", fixture.Owner, keys[0], keys[1]).Scan(&commands, &pending, &completed) == nil && commands == 1 && pending == 0 && completed == 1, "MYSQL_CAS_RECEIPT_AND_ENTITY_ATOMIC")
	var winningKey string
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT command_key FROM knowledge_access_audit WHERE actor_user_id=? AND command_operation='setPolicy' AND command_key IN (?,?)", fixture.Owner, keys[0], keys[1]).Scan(&winningKey) == nil, "MYSQL_CAS_WINNER_KEY")
	// Check complete column sets, not an ORM's generated index spelling.
	indexRows, err := observer.QueryContext(ctx, "SELECT TABLE_NAME,INDEX_NAME,NON_UNIQUE,COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('knowledge_access_audit','knowledge_access_tenant_state') ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX", readMySQLGrantSchema(ctx, t, observer))
	requireMySQLAccess(t, err == nil, "MYSQL_INDEX_COLUMN_SETS")
	defer indexRows.Close()
	type indexDefinition struct {
		table, name string
		unique      bool
		columns     []string
	}
	indexes := map[string]*indexDefinition{}
	count := 0
	for indexRows.Next() {
		count++
		requireMySQLAccess(t, count <= 256, "MYSQL_INDEX_METADATA_BOUND")
		var table, name, column string
		var nonunique int
		requireMySQLAccess(t, indexRows.Scan(&table, &name, &nonunique, &column) == nil, "MYSQL_INDEX_COLUMN")
		id := table + ":" + name
		if indexes[id] == nil {
			indexes[id] = &indexDefinition{table: table, name: name, unique: nonunique == 0}
		}
		indexes[id].columns = append(indexes[id].columns, column)
	}
	requireMySQLAccess(t, indexRows.Err() == nil, "MYSQL_INDEX_ITERATION")
	indexRows.Close()
	matches := 0
	for _, index := range indexes {
		columns := append([]string(nil), index.columns...)
		sort.Strings(columns)
		if index.table == "knowledge_access_audit" && index.unique && strings.Join(columns, ",") == "actor_user_id,command_key,command_operation" {
			matches++
		}
		if index.table == "knowledge_access_tenant_state" && len(columns) == 1 && columns[0] == "tenant_id" {
			requireMySQLAccess(t, index.name == "PRIMARY", "MYSQL_NO_DUPLICATE_TENANT_PRIMARY_INDEX")
		}
	}
	requireMySQLAccess(t, matches == 1, "MYSQL_ONE_COMPLETE_COMMAND_UNIQUE_SET")
	tx := db.WithContext(ownerWriter).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	requireMySQLAccess(t, tx.Error == nil, "MYSQL_UNIQUE_PROBE_TRANSACTION")
	defer tx.Rollback()
	newCommand := func(key string) entity.KnowledgeAccessAuditRecord {
		operation, state, kind, result, status := "setPolicy", "completed", "domain", "{}", 200
		now := time.Now().UTC()
		return entity.KnowledgeAccessAuditRecord{ID: common.GenerateUUID(), TenantID: fixture.Tenant, ActorUserID: &fixture.Owner, DomainID: &fixture.Domain, Action: operation, Outcome: "changed", RequestID: common.GenerateUUID(), OccurredAt: time.Now().UTC(), CommandOperation: &operation, CommandKey: &key, CommandRequestSHA256: make([]byte, 32), CommandScopeKind: &kind, CommandScopeID: &fixture.Domain, CommandState: &state, CommandResourceID: &fixture.Domain, CommandHTTPStatus: &status, CommandResultJSON: &result, CommandCommittedAt: &now}
	}
	duplicate := newCommand(winningKey)
	var duplicateError *mysqlDriver.MySQLError
	requireMySQLAccess(t, errors.As(tx.Create(&duplicate).Error, &duplicateError) && duplicateError.Number == 1062, "MYSQL_COMMAND_DUPLICATE_ACTUALLY_REJECTED")
	differentCase := newCommand(strings.ToLower(winningKey))
	requireMySQLAccess(t, tx.Create(&differentCase).Error == nil, "MYSQL_COMMAND_KEY_CASE_SENSITIVE")
	ordinaryIDs := []string{common.GenerateUUID(), common.GenerateUUID()}
	for _, id := range ordinaryIDs {
		event := entity.KnowledgeAccessAuditRecord{ID: id, TenantID: fixture.Tenant, ActorUserID: &fixture.Owner, Action: "read", Outcome: "denied", RequestID: common.GenerateUUID(), OccurredAt: time.Now().UTC()}
		requireMySQLAccess(t, tx.Create(&event).Error == nil, "MYSQL_MULTIPLE_NONCOMMAND_NULL_ROWS")
	}
	requireMySQLAccess(t, tx.Rollback().Error == nil, "MYSQL_SYNTHETIC_INDEX_PROBES_ROLLED_BACK")
	var retained int64
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_access_audit WHERE id IN (?,?,?)", differentCase.ID, ordinaryIDs[0], ordinaryIDs[1]).Scan(&retained) == nil && retained == 0, "MYSQL_NO_INDEX_PROBE_EVENTS_COMMITTED")
}

func readMySQLGrantSchema(ctx context.Context, t *testing.T, observer *sql.Conn) string {
	t.Helper()
	var schema string
	requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema) == nil, "MYSQL_OBSERVER_SCHEMA")
	return schema
}
