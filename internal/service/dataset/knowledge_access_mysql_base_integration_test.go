//go:build integration && linux

package dataset

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/storage"
)

const mysqlBaseOldELFSHA = "a703ffc0fe68301145e581b1faeec9dbe1bd90b8e596a6eee34de051284f1649"

var errMySQLBaseScope = errors.New("mysql_base_scope_refused")

// These private transport fields match provisioner01's identity/seed RAM output.
// They are never a product API or a source of existing account credentials.
type mysqlBaseSession struct {
	Raw           string `json:"raw"`
	IssuedAt      int64  `json:"issuedAt"`
	ExpiresAt     int64  `json:"expiresAt"`
	Authorization string `json:"authorization"`
	ID            string `json:"id"`
	Email         string `json:"email"`
	Nickname      string `json:"nickname"`
}

type mysqlBaseIdentity struct {
	DomainID              string           `json:"domainID"`
	RunID                 string           `json:"runID"`
	TargetTenant          string           `json:"targetTenant"`
	Owner                 mysqlBaseSession `json:"owner"`
	Recipient             mysqlBaseSession `json:"recipient"`
	Attempts              int              `json:"attempts"`
	Bytes                 int              `json:"bytes"`
	Origin                string           `json:"origin"`
	MainPersonalTenants   int              `json:"mainPersonalTenants"`
	IsolatedTargetTenants int              `json:"isolatedTargetTenants"`
}

type mysqlBaseSeed struct {
	Version         string `json:"version"`
	Phase           string `json:"phase"`
	RunID           string `json:"run_id"`
	Schema          string `json:"schema"`
	Ownership       string `json:"ownership"`
	Completed       bool   `json:"completed"`
	Calls           int    `json:"calls"`
	Rows            int    `json:"rows"`
	TriggerSHA      string `json:"trigger_action_sha256"`
	CatalogSHA      string `json:"catalog_sha256"`
	FixturePrepared bool   `json:"actual_fixture_prepared"`
}

type mysqlBaseLease struct {
	mysqlAccessLease
	MainSchema        string    `json:"main_schema"`
	Ownership         string    `json:"ownership"`
	IdentitySHA       string    `json:"identity_sha256"`
	SeedSHA           string    `json:"seed_sha256"`
	IdentityNotBefore time.Time `json:"identity_not_before"`
	CatalogSHA        string    `json:"catalog_sha256"`
	OutputPipe        string    `json:"output_ram_pipe"`
	RegistrationKey   string    `json:"registration_key"`
	CommitKey         string    `json:"commit_key"`
	BodySHA           string    `json:"body_sha256"`
	BodyBytes         int       `json:"body_bytes"`
	GrantExpires      time.Time `json:"grant_expires_at"`
}

type mysqlBaseInput struct {
	Lease    json.RawMessage `json:"lease"`
	Identity json.RawMessage `json:"identity"`
	Seed     json.RawMessage `json:"seed"`
	DSN      string          `json:"dsn"`
	Text     string          `json:"text"`
}

var mysqlBaseTables = []string{
	"user", "tenant", "user_tenant", "knowledgebase", "knowledge_access_tenant_state", "knowledge_access_domain_policy", "knowledge_access_module", "knowledge_access_grant", "knowledge_access_view", "knowledge_access_audit", "document", "file", "file2document", "file_commit", "file_commit_item", "knowledge_source_revision", "knowledge_node", "knowledge_source_lifecycle", "knowledge_source_registration", "knowledge_source_binding", "knowledge_local_lexical_policy", "knowledge_content_policy", "knowledge_local_lexical_job", "knowledge_local_lexical_operation", "knowledge_local_lexical_publication", "knowledge_import_preview", "knowledge_import", "knowledge_source_fence", "knowledge_retired_position", "knowledge_structure_operation",
}

func validateMySQLBaseLease(lease mysqlBaseLease, now, bootstrapExpires time.Time) error {
	if lease.Purpose != "ROOT_ACCESS02_S1_MYSQL_CREATE_BASE" || !lease.Dedicated || !lease.Primary || !lease.MySQL8 || lease.Prepared || !lease.ResourcesVerified ||
		!isKnowledgeID(lease.RunID, 16) || lease.Schema != "lfen_s1_mysql_"+lease.RunID || !isMySQLPreparationSchema(lease.Schema) || lease.MainSchema == "" || lease.MainSchema == lease.Schema ||
		!isKnowledgeID(lease.Ownership, 32) || lease.ServerUUID == "" || lease.FixtureSHA != "" ||
		lease.NotBefore.IsZero() || now.Before(lease.NotBefore) || !lease.Expires.After(now.Add(5*time.Second)) || !lease.Expires.Equal(bootstrapExpires) || lease.Expires.Sub(lease.NotBefore) > 10*time.Minute ||
		lease.IdentityNotBefore.IsZero() || lease.IdentityNotBefore.After(lease.NotBefore) || now.Sub(lease.IdentityNotBefore) > 10*time.Minute {
		return errMySQLBaseScope
	}
	host, port, err := net.SplitHostPort(lease.Address)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || net.ParseIP(host) == nil || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return errMySQLBaseScope
	}
	for _, hash := range []string{lease.IdentitySHA, lease.SeedSHA, lease.BodySHA, lease.TriggerSHA, lease.ResourceManifestSHA, lease.TestELFSHA, lease.CatalogSHA} {
		if !entity.ValidKnowledgeLexicalHash(hash) {
			return errMySQLBaseScope
		}
	}
	if lease.TestELFSHA == mysqlBaseOldELFSHA || lease.TestELFBytes < 1 || lease.TestELFBytes > 1<<30 || lease.BodyBytes < 1 || lease.BodyBytes > 8192 ||
		!entity.ValidKnowledgeLexicalKey(lease.RegistrationKey) || len(lease.RegistrationKey) > 64 || !entity.ValidKnowledgeLexicalKey(lease.CommitKey) || len(lease.CommitKey) > 64 || lease.CommitKey == lease.RegistrationKey ||
		!lease.GrantExpires.Equal(lease.GrantExpires.Truncate(time.Millisecond)) || !lease.GrantExpires.After(lease.Expires.Add(15*time.Minute)) || !lease.GrantExpires.Before(now.Add(24*time.Hour)) {
		return errMySQLBaseScope
	}
	return nil
}

func validateMySQLBaseIdentity(lease mysqlBaseLease, identity mysqlBaseIdentity, seed mysqlBaseSeed, now time.Time) error {
	if identity.Origin != "actual-new-native-register" || identity.RunID != lease.RunID || identity.DomainID == identity.Owner.ID || identity.TargetTenant != identity.Owner.ID ||
		identity.Owner.ID == identity.Recipient.ID || identity.Owner.Raw == identity.Recipient.Raw || !isKnowledgeID(identity.DomainID, 32) || identity.Attempts != 7 || identity.Bytes < 1 || identity.Bytes > 1<<20 || identity.MainPersonalTenants != 2 || identity.IsolatedTargetTenants != 1 {
		return errMySQLBaseScope
	}
	for i, session := range []mysqlBaseSession{identity.Owner, identity.Recipient} {
		role := []string{"owner", "recipient"}[i]
		issued, expires := time.UnixMilli(session.IssuedAt), time.UnixMilli(session.ExpiresAt)
		if !isKnowledgeID(session.ID, 32) || !isKnowledgeID(session.Raw, 32) || session.Email != "mysql-"+lease.RunID+"-"+role+"@example.invalid" || session.Nickname != "Synthetic "+lease.RunID+" "+strconv.Itoa(i) ||
			len(session.Authorization) < 1 || len(session.Authorization) > 2048 || issued.Before(lease.IdentityNotBefore.Truncate(time.Second)) || issued.After(now) || expires.Sub(issued) != 24*time.Hour || !expires.After(lease.GrantExpires) {
			return errMySQLBaseScope
		}
	}
	if seed.Version != "mysql-provisioner-result/1" || seed.Phase != "seed" || seed.RunID != lease.RunID || seed.Schema != lease.Schema || seed.Ownership != lease.Ownership || !seed.Completed || seed.FixturePrepared || seed.CatalogSHA != lease.CatalogSHA || seed.TriggerSHA != lease.TriggerSHA || seed.Calls < 1 || seed.Calls > 8 || seed.Rows < 0 || seed.Rows > 512 {
		return errMySQLBaseScope
	}
	return nil
}

func readMySQLBaseInput(t *testing.T, started time.Time) (mysqlBaseLease, mysqlBaseIdentity, mysqlBaseInput, context.Context, context.CancelFunc) {
	t.Helper()
	expires, err := time.Parse(time.RFC3339Nano, os.Getenv("LFEN_ACCESS_MYSQL_BASE_EXPIRES_AT"))
	requireMySQLAccess(t, err == nil && expires.After(time.Now().Add(6*time.Second)) && expires.Sub(started) <= 10*time.Minute, "BASE_BOOTSTRAP_ABSOLUTE_EXPIRY")
	raw, err := mysqlAccessSystemRAM().read(os.Getenv("LFEN_ACCESS_MYSQL_BASE_RAM_PIPE"), os.Getenv("LFEN_ACCESS_MYSQL_BASE_SCOPE_SHA256"), expires)
	requireMySQLAccess(t, err == nil, "BASE_RAM_INPUT")
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	var wire mysqlBaseInput
	var lease mysqlBaseLease
	var identity mysqlBaseIdentity
	var seed mysqlBaseSeed
	requireMySQLAccess(t, entity.KnowledgeParsedDecode(raw, &wire) == nil && lexicalSHA(wire.Lease) == os.Getenv("LFEN_ACCESS_MYSQL_BASE_SCOPE_SHA256") && entity.KnowledgeParsedDecode(wire.Lease, &lease) == nil, "BASE_CLOSED_ROOT_SCOPE")
	requireMySQLAccess(t, validateMySQLBaseLease(lease, time.Now(), expires) == nil && lexicalSHA(wire.Identity) == lease.IdentitySHA && lexicalSHA(wire.Seed) == lease.SeedSHA && entity.KnowledgeParsedDecode(wire.Identity, &identity) == nil && entity.KnowledgeParsedDecode(wire.Seed, &seed) == nil && validateMySQLBaseIdentity(lease, identity, seed, time.Now()) == nil, "BASE_NEW_NATIVE_IDENTITY_BINDING")
	requireMySQLAccess(t, utf8.ValidString(wire.Text) && strings.TrimSpace(wire.Text) != "" && len(wire.Text) == lease.BodyBytes && lexicalSHA([]byte(wire.Text)) == lease.BodySHA && !strings.Contains(wire.Text, "-----BEGIN "), "BASE_SYNTHETIC_UTF8_BODY")
	requireMySQLAccess(t, lease.OutputPipe != os.Getenv("LFEN_ACCESS_MYSQL_BASE_RAM_PIPE"), "BASE_DISTINCT_RAM_PIPES")
	checkMySQLPreparationPipe(t, lease.OutputPipe, lease.Expires)
	deadline := started.Add(120 * time.Second)
	if lease.Expires.Before(deadline) {
		deadline = lease.Expires
	}
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	return lease, identity, wire, ctx, cancel
}

func mysqlBaseForbiddenWrite(table string) bool {
	switch table {
	case "user", "tenant", "user_tenant", "knowledge_local_lexical_job", "knowledge_local_lexical_publication", "knowledge_access_domain_policy", "knowledge_access_module", "knowledge_access_grant", "knowledge_access_view":
		return true
	}
	return false
}

func installMySQLBaseBudget(t *testing.T, db *gorm.DB, ctx context.Context) *int {
	t.Helper()
	statements := 0
	allowed := map[string]bool{}
	for _, table := range mysqlBaseTables {
		allowed[table] = true
	}
	guard := func(tx *gorm.DB) {
		statements++
		if statements > 512 || ctx.Err() != nil {
			tx.AddError(errMySQLBaseScope)
		}
	}
	write := func(tx *gorm.DB) {
		guard(tx)
		if !allowed[tx.Statement.Table] || mysqlBaseForbiddenWrite(tx.Statement.Table) {
			tx.AddError(errMySQLBaseScope)
		}
		if tx.Statement.Table == "knowledge_local_lexical_operation" {
			row, ok := tx.Statement.Dest.(*entity.KnowledgeLexicalOperationRecord)
			if !ok || row.Kind != "policy" {
				tx.AddError(errMySQLBaseScope)
			}
		}
		if tx.Statement.Table == "knowledge_structure_operation" {
			row, ok := tx.Statement.Dest.(*entity.KnowledgeStructureOperationRecord)
			if !ok || row.Kind != "parsed_policy" {
				tx.AddError(errMySQLBaseScope)
			}
		}
		if row, ok := tx.Statement.Dest.(*entity.KnowledgeAccessAuditRecord); ok && row.CommandOperation != nil {
			tx.AddError(errMySQLBaseScope)
		}
	}
	raw := func(tx *gorm.DB) {
		guard(tx)
		text := strings.ToUpper(strings.TrimSpace(tx.Statement.SQL.String()))
		if text != "" && !strings.HasPrefix(text, "SELECT ") && !strings.HasPrefix(text, "WITH ") && !strings.HasPrefix(text, "SAVEPOINT ") && !strings.HasPrefix(text, "ROLLBACK TO SAVEPOINT ") {
			tx.AddError(errMySQLBaseScope)
		}
	}
	requireMySQLAccess(t, db.Callback().Query().Before("gorm:query").Register("mysql_base_scope", guard) == nil && db.Callback().Row().Before("gorm:row").Register("mysql_base_scope", raw) == nil && db.Callback().Raw().Before("gorm:raw").Register("mysql_base_scope", raw) == nil && db.Callback().Create().Before("gorm:create").Register("mysql_base_scope", write) == nil && db.Callback().Update().Before("gorm:update").Register("mysql_base_scope", write) == nil && db.Callback().Delete().Before("gorm:delete").Register("mysql_base_scope", func(tx *gorm.DB) { tx.AddError(errMySQLBaseScope) }) == nil, "BASE_SQL_SCOPE_BUDGET")
	return &statements
}

func verifyMySQLBaseSeed(t *testing.T, db *gorm.DB, ctx context.Context, lease mysqlBaseLease, identity mysqlBaseIdentity) entity.Knowledgebase {
	t.Helper()
	// Counts apply only to the dedicated fixture schema, not to the main system.
	for _, spec := range []struct {
		model any
		count int64
	}{{&entity.User{}, 2}, {&entity.Tenant{}, 1}, {&entity.UserTenant{}, 2}, {&entity.Knowledgebase{}, 1}} {
		var n int64
		requireMySQLAccess(t, db.WithContext(ctx).Model(spec.model).Count(&n).Error == nil && n == spec.count, "BASE_ISOLATED_IDENTITY_COUNTS")
	}
	state := mysqlBaseSeedState{}
	for _, session := range []mysqlBaseSession{identity.Owner, identity.Recipient} {
		var user entity.User
		requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND status = ?", session.ID, "1").Take(&user).Error == nil, "BASE_ACTUAL_SEEDED_SESSION")
		state.Users = append(state.Users, user)
	}
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND status = ?", identity.TargetTenant, "1").Take(&state.Tenant).Error == nil, "BASE_ACTUAL_TARGET_TENANT")
	requireMySQLAccess(t, db.WithContext(ctx).Where("tenant_id = ? AND status = ?", identity.TargetTenant, "1").Find(&state.Members).Error == nil, "BASE_MEMBERSHIP_PAIR")
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND created_by = ? AND status = ? AND permission = ?", identity.DomainID, identity.TargetTenant, identity.Owner.ID, "1", "me").Take(&state.KB).Error == nil && validateMySQLBaseSeedState(identity, state) == nil, "BASE_EXPLICIT_NATIVE_SEED_STATE")
	for _, table := range mysqlBaseTables {
		if table == "user" || table == "tenant" || table == "user_tenant" || table == "knowledgebase" {
			continue
		}
		var n int64
		requireMySQLAccess(t, db.WithContext(ctx).Table(table).Count(&n).Error == nil && n == 0, "BASE_FRESH_EMPTY_OWNER_TABLES")
	}
	return state.KB
}

type mysqlBaseSeedState struct {
	Users   []entity.User
	Tenant  entity.Tenant
	Members []entity.UserTenant
	KB      entity.Knowledgebase
}

func mysqlBaseValidStatus(status *string) bool { return status != nil && *status == "1" }

func validateMySQLBaseSeedState(identity mysqlBaseIdentity, state mysqlBaseSeedState) error {
	if len(state.Users) != 2 || len(state.Members) != 2 {
		return errMySQLBaseScope
	}
	users := map[string]entity.User{}
	for _, user := range state.Users {
		if _, exists := users[user.ID]; exists {
			return errMySQLBaseScope
		}
		users[user.ID] = user
	}
	for _, session := range []mysqlBaseSession{identity.Owner, identity.Recipient} {
		user, found := users[session.ID]
		if !found || !mysqlBaseValidStatus(user.Status) || user.Password != nil || user.AccessToken == nil || *user.AccessToken != session.Raw || user.Email != session.Email || user.Nickname != session.Nickname || user.IsAuthenticated != "1" || user.IsActive != "1" || user.IsAnonymous != "0" || user.IsSuperuser == nil || *user.IsSuperuser {
			return errMySQLBaseScope
		}
	}
	tenant := state.Tenant
	if tenant.ID != identity.TargetTenant || !mysqlBaseValidStatus(tenant.Status) || tenant.LLMID != "" || tenant.EmbdID != "" || tenant.ASRID != "" || tenant.Img2TxtID != "" || tenant.RerankID != "" || tenant.TTSID != nil && *tenant.TTSID != "" || tenant.OCRID != nil && *tenant.OCRID != "" || tenant.TenantLLMID != nil || tenant.TenantEmbdID != nil || tenant.TenantASRID != nil || tenant.TenantImg2TxtID != nil || tenant.TenantRerankID != nil || tenant.TenantTTSID != nil || tenant.TenantOCRID != nil {
		return errMySQLBaseScope
	}
	roles := map[string]string{}
	for _, member := range state.Members {
		if !isKnowledgeID(member.ID, 32) || member.TenantID != identity.TargetTenant || member.InvitedBy != identity.Owner.ID || !mysqlBaseValidStatus(member.Status) {
			return errMySQLBaseScope
		}
		if _, found := roles[member.UserID]; found {
			return errMySQLBaseScope
		}
		roles[member.UserID] = member.Role
	}
	if roles[identity.Owner.ID] != "owner" || roles[identity.Recipient.ID] != "normal" {
		return errMySQLBaseScope
	}
	kb := state.KB
	if kb.ID != identity.DomainID || kb.TenantID != identity.TargetTenant || kb.CreatedBy != identity.Owner.ID || !mysqlBaseValidStatus(kb.Status) || kb.Permission != "me" || !kb.KnowledgeEnabled || kb.KnowledgePolicyVersion != 1 || kb.KnowledgeFolderID != nil || kb.KnowledgeCategory != "custom" || kb.ParserID != "naive" || kb.PipelineID != nil || kb.EmbdID != "" || kb.TenantEmbdID != nil || len(kb.ParserConfig) != 0 || kb.DocNum != 0 || kb.ChunkNum != 0 || kb.TokenNum != 0 {
		return errMySQLBaseScope
	}
	return nil
}

func mysqlBaseObjects(ctx context.Context, store storage.Storage) ([]mysqlAccessObject, error) {
	memory, ok := store.(*storage.MemoryStorage)
	if !ok {
		return nil, errMySQLBaseScope
	}
	entries := memory.Inspect()
	if len(entries) < 1 || len(entries) > 4 {
		return nil, errMySQLBaseScope
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Bucket != entries[j].Bucket {
			return entries[i].Bucket < entries[j].Bucket
		}
		return entries[i].Key < entries[j].Key
	})
	objects := make([]mysqlAccessObject, 0, len(entries))
	for _, entry := range entries {
		body, err := store.Get(ctx, entry.Bucket, entry.Key)
		if err != nil || len(body) < 1 || len(body) > 8192 || len(body) != entry.Size || !utf8.Valid(body) {
			return nil, errMySQLBaseScope
		}
		objects = append(objects, mysqlAccessObject{Bucket: entry.Bucket, Key: entry.Key, Body: string(body)})
	}
	return objects, nil
}

func TestKnowledgeAccessMySQLCreateBase(t *testing.T) {
	if os.Getenv("LFEN_ACCESS_MYSQL_BASE") != "root-approved-create-native-base" {
		t.Skip("mysql_create_base_scope_not_granted")
	}
	requireMySQLAccess(t, os.Getenv("LFEN_ACCESS_MYSQL_PREPARE") == "" && os.Getenv("LFEN_ACCESS_MYSQL_RUNTIME") == "", "BASE_SEPARATE_PHASE_GATES")
	started := time.Now()
	lease, identity, wire, ctx, cancel := readMySQLBaseInput(t, started)
	defer cancel()
	verifyMySQLAccessResources(t, lease.mysqlAccessLease)
	cfg, err := mysqlDriver.ParseDSN(wire.DSN)
	requireMySQLAccess(t, err == nil && len(wire.DSN) > 0 && len(wire.DSN) <= 8192 && cfg.Net == "tcp" && cfg.Addr == lease.Address && cfg.DBName == lease.Schema, "BASE_EXACT_NUMERIC_DSN")
	cfg.Params = nil
	cfg.ParseTime, cfg.MultiStatements, cfg.AllowAllFiles, cfg.AllowCleartextPasswords, cfg.InterpolateParams = true, false, false, false, false
	cfg.Loc = time.UTC
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 3*time.Second, 5*time.Second, 5*time.Second
	connector, err := mysqlDriver.NewConnector(cfg)
	requireMySQLAccess(t, err == nil, "BASE_CONNECTOR")
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(0)
	defer pool.Close()
	conn, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "BASE_ONE_CONNECTION")
	defer conn.Close()
	db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	requireMySQLAccess(t, err == nil, "BASE_GORM")
	statements := installMySQLBaseBudget(t, db, ctx)
	var database, server, version string
	var connectionID uint64
	var readOnly int
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT DATABASE(), CONNECTION_ID(), @@server_uuid, @@version, @@read_only").Row().Scan(&database, &connectionID, &server, &version, &readOnly) == nil && database == lease.Schema && connectionID > 0 && server == lease.ServerUUID && strings.HasPrefix(version, "8.") && readOnly == 0, "BASE_PRIMARY_SCHEMA_IDENTITY")
	var tables []struct {
		Name    string `gorm:"column:TABLE_NAME"`
		Engine  string `gorm:"column:ENGINE"`
		Comment string `gorm:"column:TABLE_COMMENT"`
	}
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT TABLE_NAME, ENGINE, TABLE_COMMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME LIMIT 31", lease.Schema).Scan(&tables).Error == nil && len(tables) == 30, "BASE_EXACT_30_TABLES")
	expected := append([]string(nil), mysqlBaseTables...)
	sort.Strings(expected)
	for i, table := range tables {
		requireMySQLAccess(t, table.Name == expected[i] && table.Engine == "InnoDB" && table.Comment == "LFEN_MYSQL01:"+lease.RunID+":"+lease.Ownership, "BASE_TABLE_OWNERSHIP")
	}
	var trigger string
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=? AND TRIGGER_NAME='lfen_s1_audit_fault' AND EVENT_OBJECT_TABLE='knowledge_access_audit' AND EVENT_MANIPULATION='INSERT' AND ACTION_TIMING='BEFORE'", lease.Schema).Row().Scan(&trigger) == nil && lexicalSHA([]byte(trigger)) == lease.TriggerSHA, "BASE_EXISTING_TRIGGER")
	kb := verifyMySQLBaseSeed(t, db, ctx, lease, identity)
	oldDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = oldDB }()
	oldStore := storage.GetStorageFactory().GetStorage()
	store := storage.NewMemoryStorage()
	storage.GetStorageFactory().SetStorage(store)
	defer storage.GetStorageFactory().SetStorage(oldStore)
	service := NewDatasetService()
	ownerSHA, recipientSHA := lexicalSHA([]byte(identity.Owner.Raw)), lexicalSHA([]byte(identity.Recipient.Raw))
	ownerCtx := context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: identity.Owner.ID, SHA256: ownerSHA})
	requireMySQLAccess(t, accessSession(ownerCtx, db, identity.Owner.ID, false) == nil, "BASE_OWNER_ACTUAL_SESSION")
	var folder *entity.File
	err = dao.WithKnowledgeAccessNativeMutation(ownerCtx, db, dao.KnowledgeAccessNativeMutation{ActorUserID: &identity.Owner.ID, Action: "native_domain", UserIDs: []string{identity.Owner.ID}, TenantIDs: []string{identity.TargetTenant}, DomainIDs: []string{identity.DomainID}}, func(tx *gorm.DB) error {
		if err := dao.CheckKnowledgeAccessNativeTenant(tx, identity.TargetTenant); err != nil {
			return err
		}
		if err := permission.NewLockingDatabaseChecker(tx).CheckResource(ownerCtx, permission.Subject{UserID: identity.Owner.ID}, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: identity.DomainID}, permission.OperationUpdate); err != nil {
			return err
		}
		var err error
		folder, err = dao.NewFileDAO().EnsureKnowledgeFolder(ownerCtx, tx, identity.DomainID)
		return err
	})
	requireMySQLAccess(t, err == nil && folder != nil && folder.TenantID == identity.TargetTenant && folder.Type == "folder" && folder.SourceType == "knowledgebase", "BASE_NATIVE_FOLDER_OWNER")
	verifyMySQLBaseFolderTree(t, db, ownerCtx, identity, *folder)
	local, err := service.ChangeKnowledgeLocalPolicy(ownerCtx, identity.Owner.ID, identity.DomainID, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "base-local-" + lease.RunID, ExpectedVersion: 0, ExpectedFoundationVersion: 1, Enabled: true})
	requireMySQLAccess(t, err == nil && local != nil && local.Policy.Enabled && local.Policy.Version == 1 && !local.Policy.ExternalModels && !local.Policy.LocalInference && !local.Policy.AutomaticIngestion && !local.Policy.RemoteFetches, "BASE_REAL_LOCAL_POLICY_OWNER")
	contentRequest := entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, ParseEnabled: false, LexicalEnabled: true, Confirmed: true, IdempotencyKey: "base-content-" + lease.RunID}
	content, changed, err := service.ChangeKnowledgeContentPolicy(ownerCtx, identity.Owner.ID, identity.DomainID, contentRequest)
	requireMySQLAccess(t, err == nil && changed && content != nil && content.LexicalEnabled && !content.ParseEnabled && content.ContentPolicyVersion == 2, "BASE_REAL_CONTENT_POLICY_OWNER")
	registration, created, err := service.CreateKnowledgeRegistration(ownerCtx, identity.Owner.ID, identity.DomainID, entity.CreateKnowledgeRegistrationRequest{RegistrationKey: lease.RegistrationKey, Name: "Synthetic MySQL base", SourceKind: "text_file", InputMode: "inline_text", IncludePaths: []string{"one.txt"}, ComponentSide: "non_code"})
	requireMySQLAccess(t, err == nil && created && registration != nil && registration.DomainID == identity.DomainID && registration.GroupID == nil, "BASE_REAL_NEW_REGISTRATION")
	preview, err := service.PreviewKnowledgeImport(ownerCtx, identity.Owner.ID, identity.DomainID, registration.ID, entity.PreviewKnowledgeImportRequest{RelativePath: "one.txt", Text: wire.Text, ComponentSide: "non_code"})
	requireMySQLAccess(t, err == nil && preview != nil && preview.PolicyVersion == kb.KnowledgePolicyVersion && preview.ExpiresAt.After(time.Now()) && preview.ConsumedTaskID == nil && len(preview.Items) == 1 && preview.Items[0].Action == "add" && preview.Items[0].SourceID == nil && preview.Items[0].ExpectedRevisionID == nil && preview.Items[0].SHA256 == lease.BodySHA && preview.Items[0].ByteLength == int64(lease.BodyBytes), "BASE_REAL_PREVIEW_FENCE")
	receipt, created, err := service.CommitKnowledgeImport(ownerCtx, identity.Owner.ID, identity.DomainID, registration.ID, entity.CommitKnowledgeImportRequest{PreviewID: preview.ID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: lease.CommitKey})
	requireMySQLAccess(t, err == nil && created && receipt != nil && receipt.Status == "succeeded" && receipt.ModelCalls == 0 && len(receipt.Items) == 1, "BASE_REAL_CREATED_SUCCEEDED")
	item := receipt.Items[0]
	requireMySQLAccess(t, item.SaveStatus == "saved" && item.SourceID != nil && item.SourceRevisionID != nil && item.RootNodeID != nil && item.SHA256 == lease.BodySHA && item.ByteLength == int64(lease.BodyBytes) && item.GroupID == nil && item.ComponentSide == "non_code" && item.VectorStatus == "disabled" && item.FullTextStatus == "disabled", "BASE_REAL_SAVED_ITEM")
	canonical, err := service.GetKnowledgeImport(ownerCtx, identity.Owner.ID, identity.DomainID, registration.ID, receipt.ID)
	requireMySQLAccess(t, err == nil && canonical != nil && accessJSON(canonical) == accessJSON(receipt), "BASE_CANONICAL_GET_IMPORT")
	source, err := service.GetKnowledgeSource(ownerCtx, identity.Owner.ID, identity.DomainID, *item.SourceRevisionID)
	requireMySQLAccess(t, err == nil && source != nil && source.DocumentID == *item.SourceID && source.ID == *item.SourceRevisionID && source.SHA256 == lease.BodySHA && source.ByteLength == int64(lease.BodyBytes) && source.RootNodeID == *item.RootNodeID && source.RepresentationKind == "text" && source.IsCurrent && source.GroupID == nil && source.ComponentSide == "non_code" && source.RelativePath == "one.txt" && source.FolderID == folder.ID && source.FileID != "" && source.CommitID != "", "BASE_CANONICAL_GET_SOURCE")
	verifyMySQLBaseGraph(t, db, ownerCtx, identity, *source, *item.RootNodeID, wire.Text)
	objects, err := mysqlBaseObjects(ctx, store)
	requireMySQLAccess(t, err == nil, "BASE_SAME_MEMORY_STORAGE_OBJECTS")
	policyBaseline := captureMySQLBasePolicyBaseline(t, db, ownerCtx, identity.Owner.ID, identity.DomainID, local, content, contentRequest)
	base := mysqlPreparationBase{PolicyBaseline: policyBaseline, Owner: identity.Owner.ID, Recipient: identity.Recipient.ID, Tenant: identity.TargetTenant, Domain: identity.DomainID, Registration: registration.ID, Import: receipt.ID, Root: *item.RootNodeID, ReceiptSHA: lexicalSHA([]byte(accessJSON(canonical))), SourceCreated: true, Target: entity.KnowledgeAccessTarget{Source: entity.KnowledgeAccessSource{DomainID: identity.DomainID, SourceID: source.DocumentID, SourceRevisionID: source.ID, SourceSHA256: source.SHA256}, Representation: entity.KnowledgeAccessRepresentation{Kind: "source_revision", ID: source.ID}}, Objects: objects, OwnerSessionExpires: time.UnixMilli(identity.Owner.ExpiresAt), RecipientSessionExpires: time.UnixMilli(identity.Recipient.ExpiresAt), GrantExpires: lease.GrantExpires}
	// Observe the actual committed Base state. This read never creates an epoch
	// row or derives its value from the seed/controller or an expected call count.
	err = dao.KnowledgeAccessTransaction(ownerCtx, db, func(tx *gorm.DB) error {
		generation, readErr := readMySQLPreparationGeneration(tx, ownerCtx, base)
		if readErr != nil {
			return readErr
		}
		base.GenerationBaseline = generation.mysqlPreparationGenerationBaseline
		return validateMySQLPreparationGeneration(base, generation, 0)
	})
	requireMySQLAccess(t, err == nil, "BASE_ACTUAL_COMMITTED_GENERATION")
	verifyMySQLBasePolicyBaseline(t, db, ownerCtx, base, "", "")
	baseRaw, err := json.Marshal(base)
	requireMySQLAccess(t, err == nil && validateMySQLBaseOutput(base, source.FolderID, wire.Text) == nil && *statements <= 512, "BASE_A03_OUTPUT_BINDING")
	for _, model := range []any{&entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{}} {
		var n int64
		requireMySQLAccess(t, db.WithContext(ctx).Model(model).Count(&n).Error == nil && n == 0, "BASE_NO_INDEX_OR_S1_OUTPUTS")
	}
	var commands int64
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.KnowledgeAccessAuditRecord{}).Where("command_operation IS NOT NULL").Count(&commands).Error == nil && commands == 0, "BASE_NO_S1_COMMANDS")
	var finalConnectionID uint64
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT CONNECTION_ID()").Row().Scan(&finalConnectionID) == nil && finalConnectionID == connectionID && pool.Stats().OpenConnections == 1, "BASE_SAME_SINGLE_CONNECTION")
	finalGeneration, err := readMySQLPreparationGeneration(db, ownerCtx, base)
	requireMySQLAccess(t, err == nil && validateMySQLPreparationGeneration(base, finalGeneration, 0) == nil, "BASE_GENERATION_BEFORE_PRIVATE_OUTPUT")
	output, err := json.Marshal(struct {
		Version      string          `json:"version"`
		RunID        string          `json:"run_id"`
		Schema       string          `json:"schema"`
		Ownership    string          `json:"ownership"`
		ELFSHA       string          `json:"test_elf_sha256"`
		Base         json.RawMessage `json:"base"`
		BaseSHA      string          `json:"base_sha256"`
		DSN          string          `json:"dsn"`
		OwnerSHA     string          `json:"owner_session_sha256"`
		RecipientSHA string          `json:"recipient_session_sha256"`
		Statements   int             `json:"gorm_statement_callbacks"`
	}{"mysql-native-base/1", lease.RunID, lease.Schema, lease.Ownership, lease.TestELFSHA, baseRaw, lexicalSHA(baseRaw), wire.DSN, ownerSHA, recipientSHA, *statements})
	requireMySQLAccess(t, err == nil && ctx.Err() == nil && time.Now().Before(lease.Expires) && !bytes.Contains(output, []byte(identity.Owner.Raw)) && !bytes.Contains(output, []byte(identity.Recipient.Raw)) && !bytes.Contains(output, []byte(identity.Owner.Authorization)) && !bytes.Contains(output, []byte(identity.Recipient.Authorization)), "BASE_PRIVATE_HANDOFF_NO_SESSION_TOKEN")
	defer func() {
		for i := range output {
			output[i] = 0
		}
		for i := range baseRaw {
			baseRaw[i] = 0
		}
	}()
	writeMySQLPreparationOutput(t, ctx, lease.OutputPipe, output)
	t.Log("MYSQL_NATIVE_BASE_CREATED_NOT_PUBLICATION_OR_BARRIER_ACCEPTANCE")
}

func verifyMySQLBaseGraph(t *testing.T, db *gorm.DB, ctx context.Context, identity mysqlBaseIdentity, source entity.KnowledgeSourceRevision, rootID, body string) {
	t.Helper()
	var root entity.KnowledgeNode
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND domain_id = ? AND source_revision_id = ?", rootID, identity.DomainID, source.ID).Take(&root).Error == nil && root.ParentID == nil && root.Kind == "file" && root.StartByte == 0 && root.EndByte == source.ByteLength, "BASE_ACTUAL_ROOT_GRAPH")
	var file entity.File
	var doc entity.Document
	var link entity.File2Document
	var commit entity.FileCommit
	var item entity.FileCommitItem
	var lifecycle entity.KnowledgeSourceLifecycle
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND parent_id = ? AND tenant_id = ?", source.FileID, source.FolderID, identity.TargetTenant).Take(&file).Error == nil && file.CreatedBy == identity.Owner.ID && file.Name == "one.txt" && file.Type == "txt" && file.SourceType == "knowledgebase" && file.Size == source.ByteLength && file.Location != nil && *file.Location == ".knowledge-import/"+source.DocumentID+"/"+source.SHA256, "BASE_ACTUAL_NATIVE_FILE")
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND kb_id = ? AND status = ?", source.DocumentID, identity.DomainID, "1").Take(&doc).Error == nil && doc.CreatedBy == identity.Owner.ID && doc.Location != nil && *doc.Location == *file.Location && doc.Size == source.ByteLength, "BASE_ACTUAL_NATIVE_DOCUMENT")
	requireMySQLAccess(t, db.WithContext(ctx).Where("file_id = ? AND document_id = ?", file.ID, doc.ID).Take(&link).Error == nil && link.FileID != nil && link.DocumentID != nil && *link.FileID == file.ID && *link.DocumentID == doc.ID, "BASE_ACTUAL_NATIVE_MAPPING")
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND folder_id = ? AND author_id = ?", source.CommitID, source.FolderID, identity.Owner.ID).Take(&commit).Error == nil && commit.FileCount == 1, "BASE_ACTUAL_COMMIT")
	requireMySQLAccess(t, db.WithContext(ctx).Where("commit_id = ? AND file_id = ?", commit.ID, file.ID).Take(&item).Error == nil && item.ID == source.ID && item.Operation == "add" && item.NewHash != nil && *item.NewHash == source.SHA256 && item.NewLocation != nil && *item.NewLocation == ".objects/"+source.SHA256, "BASE_ACTUAL_COMMIT_OBJECT")
	requireMySQLAccess(t, db.WithContext(ctx).Where("source_id = ? AND domain_id = ?", source.DocumentID, identity.DomainID).Take(&lifecycle).Error == nil && lifecycle.State == "active" && lifecycle.Version == 1, "BASE_ACTUAL_LIFECYCLE")
	stored, err := storage.GetStorageFactory().GetStorage().Get(ctx, source.FolderID, ".objects/"+source.SHA256)
	requireMySQLAccess(t, err == nil && string(stored) == body && lexicalSHA(stored) == source.SHA256, "BASE_ACTUAL_COMPLETE_IMMUTABLE_BYTES")
	for _, model := range []any{&entity.Document{}, &entity.File2Document{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeNode{}, &entity.KnowledgeSourceLifecycle{}, &entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeImportPreview{}, &entity.KnowledgeImport{}, &entity.KnowledgeSourceFence{}} {
		var count int64
		requireMySQLAccess(t, db.WithContext(ctx).Model(model).Count(&count).Error == nil && count == 1, "BASE_EXACT_SINGLE_NATIVE_C1_GRAPH")
	}
	var fileCount int64
	var kb entity.Knowledgebase
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.File{}).Count(&fileCount).Error == nil && fileCount == 4 && db.WithContext(ctx).Where("id = ? AND created_by = ?", identity.DomainID, identity.Owner.ID).Take(&kb).Error == nil && kb.KnowledgeFolderID != nil && *kb.KnowledgeFolderID == source.FolderID && kb.DocNum == 1 && kb.ChunkNum == 0 && kb.TokenNum == 0 && kb.KnowledgePolicyVersion == 1, "BASE_ACTUAL_NATIVE_COUNTERS_AND_FOLDER")
}

func validateMySQLBaseOutput(base mysqlPreparationBase, folderID, body string) error {
	for _, id := range []string{base.Owner, base.Recipient, base.Tenant, base.Domain, base.Registration, base.Import, base.Target.Source.SourceID, base.Target.Source.SourceRevisionID} {
		if !isKnowledgeID(id, 32) {
			return errMySQLBaseScope
		}
	}
	if !base.SourceCreated || base.Owner == base.Recipient || !entity.ValidKnowledgeLexicalHash(base.ReceiptSHA) || !isKnowledgeID(base.Root, 32) && !isKnowledgeID(base.Root, 64) || base.Target.Source.DomainID != base.Domain || base.Target.Representation.Kind != "source_revision" || base.Target.Representation.ID != base.Target.Source.SourceRevisionID || base.Target.Representation.ProducerSHA256 != nil || base.Target.Source.SourceSHA256 != lexicalSHA([]byte(body)) || len(body) < 1 || len(body) > 8192 || !utf8.ValidString(body) || len(base.Objects) < 1 || len(base.Objects) > 4 || validateMySQLBasePolicyBinding(base) != nil || !base.GenerationBaseline.TenantStatePresent || base.GenerationBaseline.TenantEpoch < 0 || base.GenerationBaseline.TenantEpoch > entity.KnowledgeAccessMaxGeneration || !entity.ValidKnowledgeLexicalHash(base.GenerationBaseline.SourceStateSHA256) {
		return errMySQLBaseScope
	}
	seen := map[string]bool{}
	immutable := false
	for _, object := range base.Objects {
		key := object.Bucket + "\x00" + object.Key
		if object.Bucket == "" || object.Key == "" || seen[key] || object.Body != body {
			return errMySQLBaseScope
		}
		seen[key] = true
		if object.Key == ".objects/"+base.Target.Source.SourceSHA256 && object.Bucket == folderID {
			immutable = true
		} else if object.Bucket != base.Domain || object.Key != ".knowledge-import/"+base.Target.Source.SourceID+"/"+base.Target.Source.SourceSHA256 {
			return errMySQLBaseScope
		}
	}
	if !immutable {
		return errMySQLBaseScope
	}
	return nil
}

func captureMySQLBasePolicyBaseline(t *testing.T, db *gorm.DB, ctx context.Context, owner, domain string, local *entity.KnowledgeLexicalPolicyReceipt, content *entity.KnowledgeContentPolicy, request entity.KnowledgeContentPolicyRequest) mysqlPreparationPolicyBaseline {
	t.Helper()
	var localRow entity.KnowledgeLexicalOperationRecord
	var contentRow entity.KnowledgeStructureOperationRecord
	localID := lexicalDigest("knowledge-local-operation/1", owner, domain, "policy", local.Key)
	contentID := parsedOperationID(owner, domain, "parsed_policy", request.IdempotencyKey)
	requestSHA, err := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "content-policy/0.2.0", "owner_id": owner, "domain_id": domain, "request": request})
	requireMySQLAccess(t, err == nil, "BASE_CONTENT_POLICY_REQUEST_BINDING")
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", localID, owner, domain, "policy").Take(&localRow).Error == nil && localRow.Key == local.Key && localRow.RequestSHA256 == local.RequestSHA256 && localRow.Payload == lexicalJSON(local), "BASE_ACTUAL_LOCAL_POLICY_OPERATION")
	encodedContent, err := structureJSON(content)
	requireMySQLAccess(t, err == nil && db.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", contentID, owner, domain, "parsed_policy").Take(&contentRow).Error == nil && contentRow.Key == request.IdempotencyKey && contentRow.RequestSHA256 == requestSHA && contentRow.OutcomeKind == "policy" && contentRow.Status == 200 && contentRow.TaskID == "" && contentRow.Payload == encodedContent, "BASE_ACTUAL_CONTENT_POLICY_OPERATION")
	return mysqlPreparationPolicyBaseline{LocalOperationID: localRow.ID, LocalKey: localRow.Key, LocalRequestSHA: localRow.RequestSHA256, LocalReceiptSHA: lexicalSHA([]byte(localRow.Payload)), ContentOperationID: contentRow.ID, ContentKey: contentRow.Key, ContentRequestSHA: contentRow.RequestSHA256, ContentReceiptSHA: lexicalSHA([]byte(contentRow.Payload)), FoundationVersion: local.Policy.FoundationVersion, LocalVersion: local.Policy.Version, ContentVersion: content.ContentPolicyVersion}
}

func verifyMySQLBasePolicyBaseline(t *testing.T, db *gorm.DB, ctx context.Context, base mysqlPreparationBase, indexOperationID, jobID string) {
	t.Helper()
	p := base.PolicyBaseline
	requireMySQLAccess(t, validateMySQLBasePolicyBinding(base) == nil, "BASE_POLICY_BASELINE_IDENTITIES")
	var localRow entity.KnowledgeLexicalOperationRecord
	var contentRow entity.KnowledgeStructureOperationRecord
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", p.LocalOperationID, base.Owner, base.Domain, "policy").Take(&localRow).Error == nil && localRow.Key == p.LocalKey && localRow.RequestSHA256 == p.LocalRequestSHA && lexicalSHA([]byte(localRow.Payload)) == p.LocalReceiptSHA, "BASE_LOCAL_POLICY_BASELINE_PRESERVED")
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", p.ContentOperationID, base.Owner, base.Domain, "parsed_policy").Take(&contentRow).Error == nil && contentRow.Key == p.ContentKey && contentRow.RequestSHA256 == p.ContentRequestSHA && contentRow.OutcomeKind == "policy" && contentRow.Status == 200 && contentRow.TaskID == "" && lexicalSHA([]byte(contentRow.Payload)) == p.ContentReceiptSHA, "BASE_CONTENT_POLICY_BASELINE_PRESERVED")
	var localReceipt entity.KnowledgeLexicalPolicyReceipt
	var contentReceipt entity.KnowledgeContentPolicy
	requireMySQLAccess(t, entity.KnowledgeParsedDecode([]byte(localRow.Payload), &localReceipt) == nil && localReceipt.Key == p.LocalKey && localReceipt.RequestSHA256 == p.LocalRequestSHA && localReceipt.Outcome == "changed" && localReceipt.Policy.DomainID == base.Domain && localReceipt.Policy.FoundationVersion == p.FoundationVersion && localReceipt.Policy.Version == p.LocalVersion && localReceipt.Policy.Enabled && !localReceipt.Policy.ExternalModels && !localReceipt.Policy.LocalInference && !localReceipt.Policy.RemoteFetches && !localReceipt.Policy.AutomaticIngestion && len(localReceipt.Policy.AllowedModelUses) == 0 && entity.KnowledgeParsedDecode([]byte(contentRow.Payload), &contentReceipt) == nil && contentReceipt.DomainID == base.Domain && contentReceipt.FoundationPolicyVersion == p.FoundationVersion && contentReceipt.ContentPolicyVersion == p.ContentVersion && !contentReceipt.ParseEnabled && contentReceipt.LexicalEnabled, "BASE_NATIVE_POLICY_RETURN_BINDING")
	var localPolicy entity.KnowledgeLexicalPolicyRecord
	var contentPolicy entity.KnowledgeContentPolicyRecord
	requireMySQLAccess(t, db.WithContext(ctx).Where("domain_id = ? AND owner_id = ?", base.Domain, base.Owner).Take(&localPolicy).Error == nil && localPolicy.Version == p.LocalVersion && localPolicy.Enabled && db.WithContext(ctx).Where("domain_id = ? AND owner_id = ?", base.Domain, base.Owner).Take(&contentPolicy).Error == nil && contentPolicy.Version == p.ContentVersion && !contentPolicy.ParseEnabled && contentPolicy.LexicalEnabled, "BASE_ACTUAL_POLICY_ROWS_MATCH_BASELINE")
	var localCount, contentCount int64
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.KnowledgeLexicalOperationRecord{}).Count(&localCount).Error == nil && db.WithContext(ctx).Model(&entity.KnowledgeStructureOperationRecord{}).Count(&contentCount).Error == nil && contentCount == 1, "BASE_EXACT_CONTENT_OPERATION_BASELINE")
	if indexOperationID == "" {
		requireMySQLAccess(t, jobID == "" && localCount == 1, "BASE_EXACT_LOCAL_OPERATION_BASELINE")
		return
	}
	var index entity.KnowledgeLexicalOperationRecord
	requireMySQLAccess(t, entity.ValidKnowledgeLexicalHash(indexOperationID) && indexOperationID != p.LocalOperationID && isKnowledgeID(jobID, 32) && localCount == 2 && db.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", indexOperationID, base.Owner, base.Domain, "content_index").Take(&index).Error == nil && index.Payload == jobID, "PREPARE_EXACT_ONE_ADDITIONAL_INDEX_OPERATION")
	var job entity.KnowledgeLexicalJobRecord
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ?", jobID, base.Owner, base.Domain).Take(&job).Error == nil && index.Key == job.Key && index.RequestSHA256 == job.RequestSHA256 && indexOperationID == lexicalDigest("knowledge-content-operation/0.2.0", base.Owner, base.Domain, "index", job.Key), "PREPARE_INDEX_OPERATION_JOB_BINDING")
}

func validateMySQLBasePolicyBinding(base mysqlPreparationBase) error {
	p := base.PolicyBaseline
	for _, hash := range []string{p.LocalOperationID, p.LocalRequestSHA, p.LocalReceiptSHA, p.ContentOperationID, p.ContentRequestSHA, p.ContentReceiptSHA} {
		if !entity.ValidKnowledgeLexicalHash(hash) {
			return errMySQLBaseScope
		}
	}
	if !entity.ValidKnowledgeLexicalKey(p.LocalKey) || !entity.ValidKnowledgeLexicalKey(p.ContentKey) || p.FoundationVersion != 1 || p.LocalVersion != 1 || p.ContentVersion != 2 || p.LocalOperationID != lexicalDigest("knowledge-local-operation/1", base.Owner, base.Domain, "policy", p.LocalKey) || p.ContentOperationID != parsedOperationID(base.Owner, base.Domain, "parsed_policy", p.ContentKey) {
		return errMySQLBaseScope
	}
	return nil
}

func verifyMySQLBaseFolderTree(t *testing.T, db *gorm.DB, ctx context.Context, identity mysqlBaseIdentity, folder entity.File) {
	t.Helper()
	var container, root entity.File
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND type = ?", folder.ParentID, identity.TargetTenant, "folder").Take(&container).Error == nil && container.Name == dao.DatasetFolderName && container.ID != folder.ID, "BASE_REAL_RESERVED_CONTAINER")
	requireMySQLAccess(t, db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND type = ?", container.ParentID, identity.TargetTenant, "folder").Take(&root).Error == nil && root.ParentID == root.ID && root.ID != container.ID && root.ID != folder.ID && root.Name == "/", "BASE_REAL_NATIVE_ROOT")
	var folders int64
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.File{}).Where("type = ?", "folder").Count(&folders).Error == nil && folders == 3 && folder.CreatedBy == identity.Owner.ID, "BASE_EXACT_NATIVE_FOLDER_TREE")
}
