//go:build integration && linux

package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

// This test owns no schema DDL or fixture construction. Root must separately
// provision an isolated schema, a real current publication/module/user Grant,
// and the narrowly scoped audit-fault trigger described in the proposal.
type mysqlAccessLease struct {
	Purpose             string    `json:"purpose"`
	RunID               string    `json:"run_id"`
	Schema              string    `json:"schema"`
	Address             string    `json:"address"`
	ServerUUID          string    `json:"server_uuid"`
	FixtureSHA          string    `json:"fixture_sha256"`
	TriggerSHA          string    `json:"audit_trigger_sha256"`
	NotBefore           time.Time `json:"not_before"`
	Expires             time.Time `json:"expires_at"`
	Dedicated           bool      `json:"dedicated_synthetic_schema"`
	Primary             bool      `json:"primary_endpoint"`
	MySQL8              bool      `json:"mysql_8"`
	Prepared            bool      `json:"fixture_prepared"`
	DictPath            string    `json:"tokenizer_dict_path"`
	ResourceManifestSHA string    `json:"resource_manifest_sha256"`
	ResourcesVerified   bool      `json:"local_resources_verified"`
	TestELFSHA          string    `json:"test_elf_sha256"`
	TestELFBytes        int64     `json:"test_elf_bytes"`
}

type mysqlAccessObject struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	Body   string `json:"body"`
}

type mysqlAccessFixture struct {
	Owner          string                       `json:"owner_user_id"`
	Recipient      string                       `json:"recipient_user_id"`
	Tenant         string                       `json:"tenant_id"`
	Domain         string                       `json:"domain_id"`
	Module         string                       `json:"module_id"`
	ModuleVersion  int64                        `json:"module_version"`
	Grant          string                       `json:"grant_id"`
	GrantExpires   time.Time                    `json:"grant_expires_at"`
	SessionExpires time.Time                    `json:"recipient_session_expires_at"`
	ProducerSHA    string                       `json:"producer_sha256"`
	Target         entity.KnowledgeAccessTarget `json:"exact_target"`
	ExpectedSHA    string                       `json:"expected_unit_sha256"`
	ExpectedBytes  int64                        `json:"expected_unit_bytes"`
	Objects        []mysqlAccessObject          `json:"synthetic_storage_objects"`
}

type mysqlAccessInput struct {
	Lease           json.RawMessage `json:"lease"`
	Fixture         json.RawMessage `json:"fixture"`
	DSN             string          `json:"dsn"`
	SessionSHA      string          `json:"recipient_session_sha256"`
	OwnerSessionSHA string          `json:"owner_session_sha256,omitempty"`
}

type mysqlPermitKey struct{}

type mysqlPermitGate struct {
	phase     string
	entered   chan struct{}
	release   chan struct{}
	committed chan struct{}
	once      sync.Once
}

func newMySQLPermitGate(phase string) *mysqlPermitGate {
	return &mysqlPermitGate{phase: phase, entered: make(chan struct{}), release: make(chan struct{}), committed: make(chan struct{})}
}

func (g *mysqlPermitGate) wait(ctx context.Context, phase string) error {
	if g.phase != phase {
		return nil
	}
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConnPoolBeginner is an existing GORM injection surface. The stack anchor is
// deliberately the actual production final-permit owner, not a transaction
// count or guessed SQL timing. If that frame is unavailable, no barrier is
// observed and the test fails rather than claiming concurrency evidence.
type mysqlAccessConn struct{ *sql.Conn }

func (c *mysqlAccessConn) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	gate, _ := ctx.Value(mysqlPermitKey{}).(*mysqlPermitGate)
	final := false
	if gate != nil {
		var pcs [64]uintptr
		frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs[:])])
		for {
			frame, more := frames.Next()
			if frame.Function == "ragflow/internal/service/dataset.(*DatasetService).accessFinalPermit" {
				final = true
			}
			if !more {
				break
			}
		}
	}
	if final {
		if opts == nil || opts.Isolation != sql.LevelReadCommitted {
			return nil, errors.New("mysql_final_permit_isolation_refused")
		}
		if err := gate.wait(ctx, "before_begin"); err != nil {
			return nil, err
		}
	}
	tx, err := c.Conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	if final {
		return &mysqlAccessTx{Tx: tx, gate: gate, ctx: ctx}, nil
	}
	return tx, nil
}

type mysqlAccessTx struct {
	*sql.Tx
	gate *mysqlPermitGate
	ctx  context.Context
}

func (tx *mysqlAccessTx) Commit() error {
	if err := tx.gate.wait(tx.ctx, "before_commit"); err != nil {
		return err
	}
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	close(tx.gate.committed)
	return nil
}

func requireMySQLAccess(t *testing.T, condition bool, label string) {
	t.Helper()
	if !condition {
		t.Fatalf("mysql_access_assertion:%s", label)
	}
}

func waitMySQLAccess(t *testing.T, ctx context.Context, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("mysql_access_barrier:%s", label)
	}
}

// No credential may be read from env, argv, a regular file or a DSN file.
// Only the public gate/fingerprint/FIFO path are environment values.
func readMySQLAccessInput(t *testing.T) (mysqlAccessLease, mysqlAccessFixture, mysqlAccessInput) {
	t.Helper()
	if os.Getenv("LFEN_ACCESS_MYSQL_RUNTIME") != "root-approved-isolated-schema" {
		t.Skip("mysql_runtime_scope_not_granted")
	}
	expected := os.Getenv("LFEN_ACCESS_MYSQL_SCOPE_SHA256")
	name := os.Getenv("LFEN_ACCESS_MYSQL_RAM_PIPE")
	expires, err := time.Parse(time.RFC3339Nano, os.Getenv("LFEN_ACCESS_MYSQL_EXPIRES_AT"))
	requireMySQLAccess(t, err == nil && expires.After(time.Now()), "BARRIER_BOOTSTRAP_ABSOLUTE_EXPIRY")
	raw, err := mysqlAccessSystemRAM().read(name, expected, expires)
	requireMySQLAccess(t, err == nil, "BARRIER_RAM_INPUT")
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	var input mysqlAccessInput
	var lease mysqlAccessLease
	var fixture mysqlAccessFixture
	requireMySQLAccess(t, entity.KnowledgeParsedDecode(raw, &input) == nil && lexicalSHA(input.Lease) == expected && entity.KnowledgeParsedDecode(input.Lease, &lease) == nil && entity.KnowledgeParsedDecode(input.Fixture, &fixture) == nil, "RAM_CLOSED_JSON")
	now := time.Now()
	requireMySQLAccess(t, lease.Purpose == "ROOT_ACCESS02_S1_MYSQL_BARRIERS" && lease.Dedicated && lease.Primary && lease.MySQL8 && lease.Prepared && lease.RunID != "" && lease.ServerUUID != "" && regexp.MustCompile(`^lfen_s1_mysql_[0-9a-f]{16}$`).MatchString(lease.Schema) && !lease.NotBefore.IsZero() && !now.Before(lease.NotBefore) && lease.Expires.Equal(expires) && lease.Expires.After(now.Add(110*time.Second)) && lease.Expires.Sub(lease.NotBefore) <= 10*time.Minute && lexicalSHA(input.Fixture) == lease.FixtureSHA && entity.ValidKnowledgeLexicalHash(lease.TriggerSHA), "ROOT_LEASE")
	host, port, err := net.SplitHostPort(lease.Address)
	portNumber, portErr := strconv.Atoi(port)
	requireMySQLAccess(t, err == nil && net.ParseIP(host) != nil && portErr == nil && portNumber > 0 && portNumber <= 65535 && entity.ValidKnowledgeLexicalHash(input.SessionSHA), "ENDPOINT_AND_SESSION")
	requireMySQLAccess(t, lease.ResourcesVerified && entity.ValidKnowledgeLexicalHash(lease.ResourceManifestSHA) && entity.ValidKnowledgeLexicalHash(lease.TestELFSHA) && lease.TestELFBytes > 0 && lease.TestELFBytes <= 1<<30 && filepath.IsAbs(lease.DictPath) && filepath.Clean(lease.DictPath) == lease.DictPath, "ROOT_LOCAL_RESOURCE_GATE")
	for _, id := range []string{fixture.Owner, fixture.Recipient, fixture.Tenant, fixture.Domain, fixture.Module, fixture.Grant} {
		requireMySQLAccess(t, isKnowledgeID(id, 32), "FIXTURE_ID")
	}
	requireMySQLAccess(t, fixture.Owner != fixture.Recipient && entity.ValidKnowledgeLexicalHash(fixture.ExpectedSHA) && fixture.ExpectedBytes > 0 && fixture.ExpectedBytes <= 8192 && len(fixture.Objects) > 0 && len(fixture.Objects) <= 4, "FIXTURE_BOUND")
	requireMySQLAccess(t, fixture.ModuleVersion == 1 && entity.ValidKnowledgeLexicalHash(fixture.ProducerSHA) && fixture.GrantExpires.After(lease.Expires) && fixture.SessionExpires.After(lease.Expires), "FIXTURE_GRANT_SESSION_LIFETIME")
	requireMySQLAccess(t, entity.KnowledgeAccessDecode([]byte(accessJSON(fixture.Target)), "Target", nil) == nil && fixture.Target.Source.DomainID == fixture.Domain && fixture.Target.Representation.Kind == "source_revision", "FROZEN_UNIQUE_EXACT_TARGET")
	return lease, fixture, input
}

type mysqlAccessReply struct {
	status   int
	raw      []byte
	started  time.Time
	finished time.Time
}

func callMySQLAccess(s *DatasetService, ctx context.Context, actor, op, requestID string, body any) mysqlAccessReply {
	started := time.Now()
	status, raw := s.KnowledgeAccess(ctx, actor, op, requestID, []byte(accessJSON(body)))
	return mysqlAccessReply{status: status, raw: raw, started: started, finished: time.Now()}
}

type mysqlAccessView struct {
	data    entity.KnowledgeAccessInspectData
	expires time.Time
}

func isMySQLAccessLive(view mysqlAccessView, fixture mysqlAccessFixture, deadline, at time.Time) bool {
	return !at.IsZero() && at.Before(deadline) && view.expires.After(deadline) && fixture.GrantExpires.After(deadline) && fixture.SessionExpires.After(deadline) && at.Before(view.expires) && at.Before(fixture.GrantExpires) && at.Before(fixture.SessionExpires)
}

func checkMySQLAccessRead(t *testing.T, reply mysqlAccessReply, fixture mysqlAccessFixture, view mysqlAccessView, requestID string, deadline time.Time, allowed bool) {
	t.Helper()
	// Both boundaries are recorded around the actual owning call, including
	// denied reads. A stale error alone cannot establish an epoch rejection.
	requireMySQLAccess(t, isMySQLAccessLive(view, fixture, deadline, reply.started) && isMySQLAccessLive(view, fixture, deadline, reply.finished) && isMySQLAccessLive(view, fixture, deadline, time.Now()), "FIXTURE_VIEW_GRANT_SESSION_NOT_EXPIRED")
	requireMySQLAccess(t, entity.KnowledgeAccessDecode(reply.raw, "ReadResponse", nil) == nil, "READ_CLOSED_RESPONSE")
	var envelope struct {
		Data      json.RawMessage `json:"data"`
		Code      string          `json:"code"`
		RequestID string          `json:"request_id"`
	}
	requireMySQLAccess(t, json.Unmarshal(reply.raw, &envelope) == nil && envelope.RequestID == requestID, "READ_REQUEST_BINDING")
	if !allowed {
		valid := reply.status == 401 && envelope.Code == "unauthenticated" || reply.status == 404 && envelope.Code == "not_found" || reply.status == 409 && (envelope.Code == "view_stale" || envelope.Code == "scope_changed")
		requireMySQLAccess(t, valid && bytes.Equal(envelope.Data, []byte("null")), "NO_BUFFERED_BODY_RELEASE")
		return
	}
	var data entity.KnowledgeAccessReadData
	requireMySQLAccess(t, reply.status == 200 && envelope.Code == "" && json.Unmarshal(envelope.Data, &data) == nil && data.Complete && data.Encoding == "utf-8" && utf8.ValidString(data.Text) && data.ByteLength == fixture.ExpectedBytes && int64(len(data.Text)) == fixture.ExpectedBytes && lexicalSHA([]byte(data.Text)) == fixture.ExpectedSHA, "EXACT_SYNTHETIC_UNIT")
	requireMySQLAccess(t, data.EvidenceRef.ViewID == view.data.ViewID && data.UnitLocator.ViewID == view.data.ViewID && data.UnitLocator.TargetIndex == 0 && data.UnitLocator.UnitOrdinal == 0 && data.UnitLocator.TargetUnitCount == 1 && data.UnitLocator.NextUnitOrdinal == nil && accessJSON(data.Citation.Target) == accessJSON(fixture.Target) && accessJSON(view.data.Targets[0].Target) == accessJSON(fixture.Target), "READ_VIEW_LOCATOR_EXACT_TARGET_BINDING")
}

func verifyMySQLAccessResources(t *testing.T, lease mysqlAccessLease) {
	t.Helper()
	dict, err := os.Lstat(lease.DictPath)
	requireMySQLAccess(t, err == nil && dict.IsDir() && dict.Mode()&os.ModeSymlink == 0 && !tokenizer.IsInitialized(), "FRESH_PINNED_TOKENIZER_RESOURCES")
	exe, err := os.Open("/proc/self/exe")
	requireMySQLAccess(t, err == nil, "PINNED_TEST_ELF_OPEN")
	defer exe.Close()
	info, err := exe.Stat()
	requireMySQLAccess(t, err == nil && info.Mode().IsRegular() && info.Size() == lease.TestELFBytes, "PINNED_TEST_ELF_SIZE")
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(exe, lease.TestELFBytes+1))
	requireMySQLAccess(t, err == nil && n == lease.TestELFBytes && hex.EncodeToString(hash.Sum(nil)) == lease.TestELFSHA, "PINNED_TEST_ELF_SHA256")
}

func mysqlAccessReadRequest(view string) entity.KnowledgeAccessReadRequest {
	return entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: json.RawMessage(accessJSON(entity.KnowledgeAccessUnitReadSelector{Kind: "unit", ViewID: view, TargetIndex: 0, UnitOrdinal: 0}))}
}

const mysqlAccessWaitQuery = `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks r ON r.ENGINE=w.ENGINE AND r.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID JOIN performance_schema.data_locks b ON b.ENGINE=w.ENGINE AND b.ENGINE_LOCK_ID=w.BLOCKING_ENGINE_LOCK_ID JOIN performance_schema.threads rt ON rt.THREAD_ID=w.REQUESTING_THREAD_ID JOIN performance_schema.threads bt ON bt.THREAD_ID=w.BLOCKING_THREAD_ID WHERE rt.PROCESSLIST_ID=? AND bt.PROCESSLIST_ID=? AND r.OBJECT_SCHEMA=? AND b.OBJECT_SCHEMA=? AND r.OBJECT_NAME='knowledge_access_tenant_state' AND b.OBJECT_NAME='knowledge_access_tenant_state'`

// Poll only exact worker IDs and the approved schema. Timer ticks bound
// observation work; success requires a real database lock edge, never elapsed
// time or a goroutine merely having started.
func observeMySQLAccessWait(t *testing.T, ctx context.Context, observer *sql.Conn, requester, blocker uint64, schema string) {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		requireMySQLAccess(t, observer.QueryRowContext(deadline, mysqlAccessWaitQuery, requester, blocker, schema, schema).Scan(&count) == nil, "LOCK_OBSERVATION_AVAILABLE")
		if count > 0 {
			t.Logf("MYSQL_LOCK_EDGE requester=%d blocker=%d schema_bound=true", requester, blocker)
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.Done():
			t.Fatal("mysql_access_lock_edge_not_observed")
		}
	}
}

func observeMySQLAccessIsolation(t *testing.T, ctx context.Context, observer *sql.Conn, connectionID uint64) {
	t.Helper()
	var isolation string
	err := observer.QueryRowContext(ctx, `SELECT e.ISOLATION_LEVEL FROM performance_schema.events_transactions_current e JOIN performance_schema.threads t ON t.THREAD_ID=e.THREAD_ID WHERE t.PROCESSLIST_ID=? AND e.STATE='ACTIVE'`, connectionID).Scan(&isolation)
	requireMySQLAccess(t, err == nil && strings.ReplaceAll(strings.ToUpper(isolation), "-", " ") == "READ COMMITTED", "ACTUAL_READ_COMMITTED")
	t.Log("MYSQL_ACTIVE_TRANSACTION_READ_COMMITTED")
}

func TestKnowledgeAccessMySQLBarriers(t *testing.T) {
	lease, fixture, input := readMySQLAccessInput(t)
	ctx, cancel := context.WithTimeout(t.Context(), 105*time.Second)
	defer cancel()
	deadline, ok := ctx.Deadline()
	requireMySQLAccess(t, ok && lease.Expires.After(deadline), "ROOT_RUN_DEADLINE")
	verifyMySQLAccessResources(t, lease)
	requireMySQLAccess(t, tokenizer.Init(&tokenizer.PoolConfig{DictPath: lease.DictPath, MinSize: 1, MaxSize: 1, IdleTimeout: time.Minute, AcquireTimeout: time.Second}) == nil, "TOKENIZER_INIT")
	defer tokenizer.Close()
	producer, err := tokenizer.ProducerSHA256()
	requireMySQLAccess(t, err == nil && producer == fixture.ProducerSHA, "SAME_TEST_ELF_PREPARED_PRODUCER")
	cfg, err := mysqlDriver.ParseDSN(input.DSN)
	requireMySQLAccess(t, err == nil && cfg.Net == "tcp" && cfg.Addr == lease.Address && cfg.DBName == lease.Schema, "DSN_EXACT_SCHEMA_ENDPOINT")
	cfg.ParseTime, cfg.MultiStatements, cfg.AllowAllFiles, cfg.AllowCleartextPasswords, cfg.InterpolateParams = true, false, false, false, false
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 3*time.Second, 5*time.Second, 5*time.Second
	connector, err := mysqlDriver.NewConnector(cfg)
	requireMySQLAccess(t, err == nil, "MYSQL_CONNECTOR")
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(3)
	pool.SetMaxIdleConns(0)
	defer pool.Close()
	connA, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "WORKER_A")
	defer connA.Close()
	connB, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "WORKER_B")
	defer connB.Close()
	observer, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "OBSERVER")
	defer observer.Close()
	_, err = observer.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
	requireMySQLAccess(t, err == nil, "OBSERVER_CONNECTION_READ_ONLY")
	ids := make([]uint64, 3)
	for i, conn := range []*sql.Conn{connA, connB, observer} {
		var database string
		err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID(),DATABASE()").Scan(&ids[i], &database)
		requireMySQLAccess(t, err == nil && database == lease.Schema, "EXACT_SCHEMA_CONNECTION")
	}
	requireMySQLAccess(t, ids[0] != ids[1] && ids[0] != ids[2] && ids[1] != ids[2], "DISTINCT_PHYSICAL_CONNECTIONS")
	t.Logf("MYSQL_CONNECTIONS worker_a=%d worker_b=%d observer=%d", ids[0], ids[1], ids[2])
	openGORM := func(conn gorm.ConnPool) *gorm.DB {
		db, openErr := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		requireMySQLAccess(t, openErr == nil, "GORM_MYSQL")
		return db
	}
	dbA, dbB := openGORM(&mysqlAccessConn{Conn: connA}), openGORM(connB)
	var grantExpires time.Time
	var grantRevoked sql.NullTime
	err = observer.QueryRowContext(ctx, "SELECT expires_at,revoked_at FROM knowledge_access_grant WHERE id=? AND tenant_id=? AND domain_id=? AND module_id=? AND module_version=? AND subject_kind='user' AND subject_id=? AND mode='read' AND delivery='web_only'", fixture.Grant, fixture.Tenant, fixture.Domain, fixture.Module, fixture.ModuleVersion, fixture.Recipient).Scan(&grantExpires, &grantRevoked)
	requireMySQLAccess(t, err == nil && !grantRevoked.Valid && grantExpires.Equal(fixture.GrantExpires) && grantExpires.After(deadline), "ACTUAL_GRANT_LIFETIME_PREFLIGHT")
	// Server version/UUID/primary ownership are Root's short-lived endpoint
	// attestation. This test never reads global server variables/configuration.
	var engines, waits int
	err = observer.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND ENGINE='InnoDB' AND TABLE_NAME IN ('user','tenant','user_tenant','knowledgebase','knowledge_access_tenant_state','knowledge_access_domain_policy','knowledge_access_module','knowledge_access_grant','knowledge_access_view','knowledge_access_audit')`, lease.Schema).Scan(&engines)
	requireMySQLAccess(t, err == nil && engines == 10, "OWNING_TABLES_TRANSACTIONAL")
	requireMySQLAccess(t, observer.QueryRowContext(ctx, mysqlAccessWaitQuery, ids[1], ids[0], lease.Schema, lease.Schema).Scan(&waits) == nil && waits == 0, "SCOPED_LOCK_OBSERVATION_PREFLIGHT")
	var trigger string
	err = observer.QueryRowContext(ctx, "SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=? AND TRIGGER_NAME='lfen_s1_audit_fault' AND EVENT_OBJECT_TABLE='knowledge_access_audit' AND EVENT_MANIPULATION='INSERT' AND ACTION_TIMING='BEFORE'", lease.Schema).Scan(&trigger)
	requireMySQLAccess(t, err == nil && lexicalSHA([]byte(trigger)) == lease.TriggerSHA, "EXACT_ROOT_AUDIT_FAULT_TRIGGER")
	requireMySQLAccess(t, dao.KnowledgeAccessTransaction(ctx, dbA, func(tx *gorm.DB) error {
		var epoch int64
		if queryErr := tx.Raw("SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=?", fixture.Tenant).Row().Scan(&epoch); queryErr != nil {
			return queryErr
		}
		if epoch < 0 || epoch > entity.KnowledgeAccessMaxGeneration {
			return errors.New("mysql_access_preflight_epoch")
		}
		observeMySQLAccessIsolation(t, ctx, observer, ids[0])
		return nil
	}) == nil, "PRODUCTION_TRANSACTION_PREFLIGHT")
	oldDB := dao.DB
	dao.DB = dbA
	defer func() { dao.DB = oldDB }()
	oldStore := storage.GetStorageFactory().GetStorage()
	store := storage.NewMemoryStorage()
	for _, object := range fixture.Objects {
		requireMySQLAccess(t, object.Bucket != "" && object.Key != "" && len(object.Body) <= 8192 && store.Put(ctx, object.Bucket, object.Key, []byte(object.Body)) == nil, "SYNTHETIC_LOCAL_OBJECT")
	}
	storage.GetStorageFactory().SetStorage(store)
	defer storage.GetStorageFactory().SetStorage(oldStore)
	service := NewDatasetService()
	recipient := context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: fixture.Recipient, SHA256: input.SessionSHA})
	read := func(readCtx context.Context, view mysqlAccessView, requestID string) mysqlAccessReply {
		if !isMySQLAccessLive(view, fixture, deadline, time.Now()) {
			return mysqlAccessReply{}
		}
		return callMySQLAccess(service, readCtx, fixture.Recipient, "read", requestID, mysqlAccessReadRequest(view.data.ViewID))
	}
	checkRead := func(tb *testing.T, readCtx context.Context, view mysqlAccessView, allowed bool) {
		requestID := common.GenerateUUID()
		checkMySQLAccessRead(tb, read(readCtx, view, requestID), fixture, view, requestID, deadline, allowed)
	}
	inspect := func(tb *testing.T) mysqlAccessView {
		requestID := common.GenerateUUID()
		reply := callMySQLAccess(service, recipient, fixture.Recipient, "inspect", requestID, entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: fixture.Domain, ModuleID: fixture.Module, Mode: "read", Delivery: "web_only"})
		var envelope struct {
			Data      entity.KnowledgeAccessInspectData `json:"data"`
			RequestID string                            `json:"request_id"`
		}
		requireMySQLAccess(tb, reply.status == 200 && entity.KnowledgeAccessDecode(reply.raw, "InspectResponse", nil) == nil && json.Unmarshal(reply.raw, &envelope) == nil && envelope.RequestID == requestID && envelope.Data.DomainID == fixture.Domain && envelope.Data.ModuleID == fixture.Module && envelope.Data.ModuleVersion == fixture.ModuleVersion && envelope.Data.Mode == "read" && envelope.Data.Delivery == "web_only" && envelope.Data.UnitCount == 1 && envelope.Data.SourceCount == 1 && len(envelope.Data.Targets) == 1 && envelope.Data.Targets[0].UnitCount == 1 && accessJSON(envelope.Data.Targets[0].Target) == accessJSON(fixture.Target), "ACTUAL_CURRENT_VIEW_EXACT_TARGET")
		expires, err := time.Parse(time.RFC3339Nano, envelope.Data.ExpiresAt)
		view := mysqlAccessView{data: envelope.Data, expires: expires}
		requireMySQLAccess(tb, err == nil && isMySQLAccessLive(view, fixture, deadline, time.Now()), "FIXTURE_VIEW_LIFETIME_PREFLIGHT")
		return view
	}
	mutate := func(writeCtx context.Context, active string) error {
		return dao.WithKnowledgeAccessNativeMutation(writeCtx, dbB, dao.KnowledgeAccessNativeMutation{ActorUserID: &fixture.Owner, Action: "native_user_flags", UserIDs: []string{fixture.Owner, fixture.Recipient}, TenantIDs: []string{fixture.Tenant}}, func(tx *gorm.DB) error {
			result := tx.Model(&entity.User{}).Where("id = ?", fixture.Recipient).UpdateColumn("is_active", active)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("mysql_synthetic_user_update_refused")
			}
			return nil
		})
	}
	auditOutcome := func(tb *testing.T, requestID string) string {
		var outcome string
		requireMySQLAccess(tb, observer.QueryRowContext(ctx, "SELECT outcome FROM knowledge_access_audit WHERE tenant_id=? AND request_id=? AND actor_user_id=? AND action='read'", fixture.Tenant, requestID, fixture.Recipient).Scan(&outcome) == nil, "AUDIT_OBSERVATION")
		return outcome
	}
	view := inspect(t)
	checkRead(t, recipient, view, true)

	t.Run("MutationCommitsBeforeFinalPermit", func(t *testing.T) {
		requireMySQLAccess(t, isMySQLAccessLive(view, fixture, deadline, time.Now()), "RACE_VIEW_LIFETIME_PREFLIGHT")
		gate := newMySQLPermitGate("before_begin")
		readCtx, stop := context.WithTimeout(context.WithValue(recipient, mysqlPermitKey{}, gate), 10*time.Second)
		var workers sync.WaitGroup
		defer func() { stop(); workers.Wait() }()
		done := make(chan mysqlAccessReply, 1)
		requestID := common.GenerateUUID()
		workers.Add(1)
		go func() { defer workers.Done(); done <- read(readCtx, view, requestID) }()
		waitMySQLAccess(t, readCtx, gate.entered, "BUFFER_ENCODED_BEFORE_FINAL_BEGIN")
		requireMySQLAccess(t, mutate(readCtx, "0") == nil, "NATIVE_MUTATION_COMMITTED_FIRST")
		close(gate.release)
		select {
		case result := <-done:
			checkMySQLAccessRead(t, result, fixture, view, requestID, deadline, false)
		case <-readCtx.Done():
			t.Fatal("mysql_access_read_did_not_finish")
		}
		requireMySQLAccess(t, auditOutcome(t, requestID) == "denied", "NO_ALLOWED_AUDIT_AFTER_DENIAL")
		requireMySQLAccess(t, mutate(readCtx, "1") == nil, "NATIVE_RESTORE_COMMITTED")
		checkRead(t, recipient, view, false)
		t.Log("MYSQL_NATIVE_FIRST_DENIED_NO_BODY_AND_OLD_VIEW_NOT_REVIVED")
	})
	if t.Failed() {
		return
	}

	t.Run("FinalPermitCommitsBeforeNativeWriter", func(t *testing.T) {
		view = inspect(t)
		gate := newMySQLPermitGate("before_commit")
		readCtx, stop := context.WithTimeout(context.WithValue(recipient, mysqlPermitKey{}, gate), 10*time.Second)
		var workers sync.WaitGroup
		defer func() { stop(); workers.Wait() }()
		done := make(chan mysqlAccessReply, 1)
		requestID := common.GenerateUUID()
		workers.Add(1)
		go func() { defer workers.Done(); done <- read(readCtx, view, requestID) }()
		waitMySQLAccess(t, readCtx, gate.entered, "REAL_FINAL_TRANSACTION_BEFORE_COMMIT")
		observeMySQLAccessIsolation(t, readCtx, observer, ids[0])
		requireMySQLAccess(t, auditOutcome(t, requestID) == "denied", "ALLOWED_UPDATE_NOT_YET_DURABLE")
		writerDone := make(chan error, 1)
		workers.Add(1)
		go func() { defer workers.Done(); writerDone <- mutate(readCtx, "0") }()
		observeMySQLAccessWait(t, readCtx, observer, ids[1], ids[0], lease.Schema)
		observeMySQLAccessIsolation(t, readCtx, observer, ids[1])
		select {
		case <-done:
			t.Fatal("mysql_access_body_released_before_commit")
		default:
		}
		select {
		case <-writerDone:
			t.Fatal("mysql_access_writer_finished_while_blocked")
		default:
		}
		close(gate.release)
		waitMySQLAccess(t, readCtx, gate.committed, "ACTUAL_SQL_COMMIT")
		select {
		case result := <-done:
			checkMySQLAccessRead(t, result, fixture, view, requestID, deadline, true)
		case <-readCtx.Done():
			t.Fatal("mysql_access_read_did_not_finish")
		}
		select {
		case writerErr := <-writerDone:
			requireMySQLAccess(t, writerErr == nil, "NATIVE_WRITER_AFTER_PERMIT")
		case <-readCtx.Done():
			t.Fatal("mysql_access_writer_did_not_finish")
		}
		requireMySQLAccess(t, auditOutcome(t, requestID) == "allowed", "DURABLE_ALLOWED_AUDIT")
		requireMySQLAccess(t, mutate(readCtx, "1") == nil, "NATIVE_RESTORE_AFTER_PERMIT")
		checkRead(t, recipient, view, false)
		t.Log("MYSQL_PERMIT_AUDIT_COMMIT_PRECEDES_WRITER_AND_BODY_RELEASE")
	})
	if t.Failed() {
		return
	}

	t.Run("ActualAuditInsertFailureRollsBackNativeMutation", func(t *testing.T) {
		view = inspect(t)
		var beforeEpoch, beforeCount, afterEpoch, afterCount int64
		var active string
		requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=?", fixture.Tenant).Scan(&beforeEpoch) == nil, "EPOCH_BEFORE_FAILURE")
		requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_access_audit WHERE tenant_id=?", fixture.Tenant).Scan(&beforeCount) == nil, "AUDIT_COUNT_BEFORE_FAILURE")
		_, err := connB.ExecContext(ctx, "SET @lfen_s1_audit_fault=1")
		requireMySQLAccess(t, err == nil, "ARM_ROOT_SQL_FAULT")
		writeErr := mutate(ctx, "0")
		_, clearErr := connB.ExecContext(ctx, "SET @lfen_s1_audit_fault=NULL")
		var mysqlErr *mysqlDriver.MySQLError
		requireMySQLAccess(t, clearErr == nil && errors.As(writeErr, &mysqlErr) && mysqlErr.Number == 1644 && string(mysqlErr.SQLState[:]) == "45000", "ACTUAL_MYSQL_AUDIT_INSERT_REJECTED")
		requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT is_active FROM user WHERE id=?", fixture.Recipient).Scan(&active) == nil && active == "1", "NATIVE_ROW_ROLLED_BACK")
		requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT epoch FROM knowledge_access_tenant_state WHERE tenant_id=?", fixture.Tenant).Scan(&afterEpoch) == nil && afterEpoch == beforeEpoch, "EPOCH_ROLLED_BACK")
		requireMySQLAccess(t, observer.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_access_audit WHERE tenant_id=?", fixture.Tenant).Scan(&afterCount) == nil && afterCount == beforeCount, "AUDIT_ROLLED_BACK")
		checkRead(t, recipient, view, true)
		t.Log("MYSQL_SQL_AUDIT_FAILURE_ROLLS_BACK_USER_EPOCH_AUDIT")
	})
}
