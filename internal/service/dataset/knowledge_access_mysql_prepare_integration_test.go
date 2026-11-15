//go:build integration && linux

package dataset

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

// The limit counts GORM callbacks, including query construction; physical SQL is not measured.
const mysqlPreparationMaxCallbacks = 1280

func logMySQLPreparationDriver(t *testing.T, err error) bool {
	t.Helper()
	var driver *mysqlDriver.MySQLError
	if !errors.As(err, &driver) || driver == nil {
		return false
	}
	errno, state := "Unknown", "Unknown"
	if driver.Number >= 1 && driver.Number <= 65535 {
		errno = strconv.Itoa(int(driver.Number))
	}
	valid := true
	for _, value := range driver.SQLState {
		valid = valid && (value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z')
	}
	if valid {
		state = string(driver.SQLState[:])
	}
	t.Logf("mysql_owner_read_driver:errno=%s,sqlstate=%s", errno, state)
	return true
}

type mysqlAccessRAMStream interface {
	io.Reader
	Stat() (os.FileInfo, error)
	SetReadDeadline(time.Time) error
	Close() error
}

// One reader owns all three phases. The injectable syscall boundary lets guards
// advance the clock between bootstrap, metadata, open and read without sleeping.
type mysqlAccessRAMReader struct {
	now   func() time.Time
	lstat func(string) (os.FileInfo, error)
	open  func(string, int, os.FileMode) (mysqlAccessRAMStream, error)
}

func mysqlAccessSystemRAM() mysqlAccessRAMReader {
	return mysqlAccessRAMReader{now: time.Now, lstat: os.Lstat, open: func(name string, flag int, mode os.FileMode) (mysqlAccessRAMStream, error) {
		return os.OpenFile(name, flag, mode)
	}}
}

func mysqlAccessRAMDeadline(now, expires time.Time) (time.Time, error) {
	if expires.IsZero() || !now.Before(expires) {
		return time.Time{}, errMySQLBaseScope
	}
	deadline := now.Add(5 * time.Second)
	if expires.Before(deadline) {
		deadline = expires
	}
	return deadline, nil
}

func (r mysqlAccessRAMReader) pipe(name string, expires time.Time) (os.FileInfo, error) {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return nil, errMySQLBaseScope
	}
	if _, err := mysqlAccessRAMDeadline(r.now(), expires); err != nil {
		return nil, err
	}
	info, err := r.lstat(name)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0600 {
		return nil, errMySQLBaseScope
	}
	if _, err := mysqlAccessRAMDeadline(r.now(), expires); err != nil {
		return nil, err
	}
	parent, err := r.lstat(filepath.Dir(name))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm() != 0700 {
		return nil, errMySQLBaseScope
	}
	a, aok := info.Sys().(*syscall.Stat_t)
	b, bok := parent.Sys().(*syscall.Stat_t)
	if !aok || !bok || a.Uid != uint32(os.Geteuid()) || b.Uid != uint32(os.Geteuid()) {
		return nil, errMySQLBaseScope
	}
	return info, nil
}

// The producer must already be ready and close its writer FDs after transfer.
// O_NONBLOCK avoids a detached opener. Every IO boundary rechecks the absolute
// deadline; neither metadata nor scheduling delay grants another five seconds.
func (r mysqlAccessRAMReader) read(name, expected string, expires time.Time) ([]byte, error) {
	if !entity.ValidKnowledgeLexicalHash(expected) {
		return nil, errMySQLBaseScope
	}
	before, err := r.pipe(name, expires)
	if err != nil {
		return nil, err
	}
	if _, err = mysqlAccessRAMDeadline(r.now(), expires); err != nil {
		return nil, err
	}
	stream, err := r.open(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errMySQLBaseScope
	}
	defer stream.Close()
	if _, err = mysqlAccessRAMDeadline(r.now(), expires); err != nil {
		return nil, err
	}
	after, err := stream.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, errMySQLBaseScope
	}
	deadline, err := mysqlAccessRAMDeadline(r.now(), expires)
	if err != nil || stream.SetReadDeadline(deadline) != nil {
		return nil, errMySQLBaseScope
	}
	if !r.now().Before(deadline) {
		return nil, errMySQLBaseScope
	}
	raw, err := io.ReadAll(io.LimitReader(stream, (128<<10)+1))
	if err != nil || !r.now().Before(deadline) || len(raw) < 1 || len(raw) > 128<<10 {
		clear(raw)
		return nil, errMySQLBaseScope
	}
	return raw, nil
}

type mysqlPreparationLease struct {
	mysqlAccessLease
	BasePrepared bool   `json:"base_source_prepared"`
	OutputPipe   string `json:"output_ram_pipe"`
	ProducerSHA  string `json:"producer_sha256"`
}

type mysqlPreparationPolicyBaseline struct {
	LocalOperationID   string `json:"local_operation_id"`
	LocalKey           string `json:"local_policy_key"`
	LocalRequestSHA    string `json:"local_request_sha256"`
	LocalReceiptSHA    string `json:"local_receipt_sha256"`
	ContentOperationID string `json:"content_operation_id"`
	ContentKey         string `json:"content_policy_key"`
	ContentRequestSHA  string `json:"content_request_sha256"`
	ContentReceiptSHA  string `json:"content_receipt_sha256"`
	FoundationVersion  int64  `json:"foundation_policy_version"`
	LocalVersion       int64  `json:"local_policy_version"`
	ContentVersion     int64  `json:"content_policy_version"`
}

type mysqlPreparationBase struct {
	PolicyBaseline          mysqlPreparationPolicyBaseline     `json:"native_policy_baseline"`
	GenerationBaseline      mysqlPreparationGenerationBaseline `json:"native_generation_baseline"`
	Owner                   string                             `json:"owner_user_id"`
	Recipient               string                             `json:"recipient_user_id"`
	Tenant                  string                             `json:"tenant_id"`
	Domain                  string                             `json:"domain_id"`
	Registration            string                             `json:"registration_id"`
	Import                  string                             `json:"import_id"`
	Root                    string                             `json:"root_node_id"`
	ReceiptSHA              string                             `json:"base_receipt_sha256"`
	SourceCreated           bool                               `json:"source_created"`
	Target                  entity.KnowledgeAccessTarget       `json:"exact_target"`
	Objects                 []mysqlAccessObject                `json:"synthetic_storage_objects"`
	OwnerSessionExpires     time.Time                          `json:"owner_session_expires_at"`
	RecipientSessionExpires time.Time                          `json:"recipient_session_expires_at"`
	GrantExpires            time.Time                          `json:"grant_expires_at"`
}

type mysqlPreparationInput struct {
	Lease               json.RawMessage `json:"lease"`
	Base                json.RawMessage `json:"base"`
	DSN                 string          `json:"dsn"`
	OwnerSessionSHA     string          `json:"owner_session_sha256"`
	RecipientSessionSHA string          `json:"recipient_session_sha256"`
}

func readMySQLAccessPreparationInput(t *testing.T) (mysqlPreparationLease, mysqlPreparationBase, mysqlPreparationInput) {
	t.Helper()
	if os.Getenv("LFEN_ACCESS_MYSQL_PREPARE") != "root-approved-publication-prepare" {
		t.Skip("mysql_prepare_scope_not_granted")
	}
	expected, name := os.Getenv("LFEN_ACCESS_MYSQL_PREPARE_SCOPE_SHA256"), os.Getenv("LFEN_ACCESS_MYSQL_PREPARE_RAM_PIPE")
	expires, err := time.Parse(time.RFC3339Nano, os.Getenv("LFEN_ACCESS_MYSQL_PREPARE_EXPIRES_AT"))
	requireMySQLAccess(t, err == nil && expires.After(time.Now()), "PREPARE_BOOTSTRAP_ABSOLUTE_EXPIRY")
	raw, err := mysqlAccessSystemRAM().read(name, expected, expires)
	requireMySQLAccess(t, err == nil, "PREPARE_RAM_INPUT")
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	var wire mysqlPreparationInput
	var lease mysqlPreparationLease
	var base mysqlPreparationBase
	requireMySQLAccess(t, entity.KnowledgeParsedDecode(raw, &wire) == nil && lexicalSHA(wire.Lease) == expected && entity.KnowledgeParsedDecode(wire.Lease, &lease) == nil && entity.KnowledgeParsedDecode(wire.Base, &base) == nil && lexicalSHA(wire.Base) == lease.FixtureSHA, "PREPARE_CLOSED_RAM_BINDING")
	now := time.Now()
	requireMySQLAccess(t, lease.Purpose == "ROOT_ACCESS02_S1_MYSQL_PREPARE_FIXTURE" && !lease.Prepared && lease.BasePrepared && lease.Dedicated && lease.Primary && lease.MySQL8 && lease.ResourcesVerified && len(lease.RunID) >= 16 && len(lease.RunID) <= 64 && entity.ValidKnowledgeLexicalKey("mysql-prepare-"+lease.RunID) && lease.ServerUUID != "" && lease.NotBefore.Before(now) && lease.Expires.Equal(expires) && lease.Expires.After(now.Add(125*time.Second)) && lease.Expires.Sub(lease.NotBefore) <= 10*time.Minute, "PREPARE_ROOT_LEASE")
	host, port, err := net.SplitHostPort(lease.Address)
	portNumber, portErr := strconv.Atoi(port)
	requireMySQLAccess(t, err == nil && net.ParseIP(host) != nil && portErr == nil && portNumber > 0 && portNumber <= 65535, "PREPARE_NUMERIC_ENDPOINT")
	requireMySQLAccess(t, isMySQLPreparationSchema(lease.Schema) && entity.ValidKnowledgeLexicalHash(lease.TriggerSHA) && entity.ValidKnowledgeLexicalHash(lease.ResourceManifestSHA) && entity.ValidKnowledgeLexicalHash(lease.ProducerSHA) && entity.ValidKnowledgeLexicalHash(lease.TestELFSHA) && lease.TestELFBytes > 0 && lease.TestELFBytes <= 1<<30 && filepath.IsAbs(lease.DictPath) && filepath.Clean(lease.DictPath) == lease.DictPath, "PREPARE_LOCAL_RESOURCE_SCOPE")
	for _, id := range []string{base.Owner, base.Recipient, base.Tenant, base.Domain, base.Registration, base.Import} {
		requireMySQLAccess(t, isKnowledgeID(id, 32), "PREPARE_BASE_ID")
	}
	requireMySQLAccess(t, base.SourceCreated && base.Owner != base.Recipient && entity.ValidKnowledgeLexicalHash(base.ReceiptSHA) && (isKnowledgeID(base.Root, 32) || isKnowledgeID(base.Root, 64)) && entity.ValidKnowledgeLexicalHash(wire.OwnerSessionSHA) && entity.ValidKnowledgeLexicalHash(wire.RecipientSessionSHA) && wire.OwnerSessionSHA != wire.RecipientSessionSHA, "PREPARE_BASE_PROOF")
	requireMySQLAccess(t, entity.KnowledgeAccessDecode([]byte(accessJSON(base.Target)), "Target", nil) == nil && base.Target.Source.DomainID == base.Domain && base.Target.Representation.Kind == "source_revision" && base.Target.Representation.ID == base.Target.Source.SourceRevisionID && base.Target.Representation.ProducerSHA256 == nil, "PREPARE_C1_TARGET")
	requireMySQLAccess(t, base.GrantExpires.Equal(base.GrantExpires.Truncate(time.Millisecond)) && base.GrantExpires.Before(now.Add(30*24*time.Hour)) && base.GrantExpires.After(lease.Expires.Add(10*time.Minute)) && base.OwnerSessionExpires.After(base.GrantExpires) && base.RecipientSessionExpires.After(base.GrantExpires), "PREPARE_FUTURE_LIFETIMES")
	requireMySQLAccess(t, len(base.Objects) >= 1 && len(base.Objects) <= 4 && lease.OutputPipe != name, "PREPARE_OBJECT_OUTPUT_SCOPE")
	seen := map[string]bool{}
	for _, object := range base.Objects {
		key := object.Bucket + "\x00" + object.Key
		requireMySQLAccess(t, object.Bucket != "" && object.Key != "" && !seen[key] && len(object.Body) > 0 && len(object.Body) <= 8192 && utf8.ValidString(object.Body), "PREPARE_BOUNDED_OBJECT")
		seen[key] = true
	}
	checkMySQLPreparationPipe(t, lease.OutputPipe, lease.Expires)
	return lease, base, wire
}

func isMySQLPreparationSchema(name string) bool {
	if !strings.HasPrefix(name, "lfen_s1_mysql_") || len(name) != len("lfen_s1_mysql_")+16 {
		return false
	}
	for _, c := range strings.TrimPrefix(name, "lfen_s1_mysql_") {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func checkMySQLPreparationPipe(t *testing.T, name string, expires time.Time) os.FileInfo {
	t.Helper()
	info, err := mysqlAccessSystemRAM().pipe(name, expires)
	requireMySQLAccess(t, err == nil, "PREPARE_OUTPUT_PIPE")
	return info
}

func writeMySQLPreparationOutput(t *testing.T, ctx context.Context, name string, raw []byte) {
	t.Helper()
	requireMySQLAccess(t, len(raw) > 0 && len(raw) <= 128<<10, "PREPARE_OUTPUT_BOUND")
	expires, ok := ctx.Deadline()
	requireMySQLAccess(t, ok && time.Now().Before(expires), "PREPARE_OUTPUT_ABSOLUTE_EXPIRY")
	before := checkMySQLPreparationPipe(t, name, expires)
	requireMySQLAccess(t, time.Now().Before(expires), "PREPARE_OUTPUT_OPEN_EXPIRY")
	file, err := os.OpenFile(name, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	requireMySQLAccess(t, err == nil, "PREPARE_OUTPUT_READER_READY")
	defer file.Close()
	after, err := file.Stat()
	requireMySQLAccess(t, err == nil && os.SameFile(before, after), "PREPARE_OUTPUT_IDENTITY")
	deadline := time.Now().Add(5 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	requireMySQLAccess(t, file.SetWriteDeadline(deadline) == nil, "PREPARE_OUTPUT_DEADLINE")
	n, err := file.Write(raw)
	requireMySQLAccess(t, err == nil && n == len(raw) && ctx.Err() == nil, "PREPARE_OUTPUT_WRITE")
}

// The input contains no permission to construct identities, native folders,
// source rows or DDL. All publication/setup writes go through actual owners.
func TestKnowledgeAccessMySQLPrepareFixture(t *testing.T) {
	lease, base, wire := readMySQLAccessPreparationInput(t)
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	verifyMySQLAccessResources(t, lease.mysqlAccessLease)
	requireMySQLAccess(t, tokenizer.Init(&tokenizer.PoolConfig{DictPath: lease.DictPath, MinSize: 1, MaxSize: 1, IdleTimeout: time.Minute, AcquireTimeout: time.Second}) == nil, "PREPARE_TOKENIZER_INIT")
	defer tokenizer.Close()
	producer, err := tokenizer.ProducerSHA256()
	requireMySQLAccess(t, err == nil && producer == lease.ProducerSHA, "PREPARE_ACTUAL_ELF_PRODUCER")
	cfg, err := mysqlDriver.ParseDSN(wire.DSN)
	requireMySQLAccess(t, err == nil && cfg.Net == "tcp" && cfg.Addr == lease.Address && cfg.DBName == lease.Schema, "PREPARE_EXACT_DSN")
	cfg.Params = nil
	cfg.ParseTime, cfg.MultiStatements, cfg.AllowAllFiles, cfg.AllowCleartextPasswords, cfg.InterpolateParams = true, false, false, false, false
	cfg.Loc = time.UTC
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 3*time.Second, 5*time.Second, 5*time.Second
	connector, err := mysqlDriver.NewConnector(cfg)
	requireMySQLAccess(t, err == nil, "PREPARE_CONNECTOR")
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(0)
	defer pool.Close()
	conn, err := pool.Conn(ctx)
	requireMySQLAccess(t, err == nil, "PREPARE_ONE_CONNECTION")
	defer conn.Close()
	db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	requireMySQLAccess(t, err == nil, "PREPARE_GORM")
	statements := 0
	budget := func(tx *gorm.DB) {
		statements++
		if statements > mysqlPreparationMaxCallbacks {
			tx.AddError(errors.New("mysql_prepare_SQL_budget"))
		}
	}
	requireMySQLAccess(t, db.Callback().Query().Before("gorm:query").Register("mysql_prepare_budget", budget) == nil && db.Callback().Row().Before("gorm:row").Register("mysql_prepare_budget", budget) == nil && db.Callback().Raw().Before("gorm:raw").Register("mysql_prepare_budget", budget) == nil && db.Callback().Create().Before("gorm:create").Register("mysql_prepare_budget", budget) == nil && db.Callback().Update().Before("gorm:update").Register("mysql_prepare_budget", budget) == nil && db.Callback().Delete().Before("gorm:delete").Register("mysql_prepare_budget", budget) == nil, "PREPARE_SQL_BUDGET")
	// Keep the first command SQL error in private test RAM. No SQL formatter or
	// logger is invoked; public service error normalization remains unchanged.
	var commandError, commandContextError error
	captureCommand := false
	captureError := func(tx *gorm.DB) {
		if captureCommand && commandError == nil && tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			commandError = tx.Error
			if tx.Statement != nil && tx.Statement.Context != nil {
				commandContextError = tx.Statement.Context.Err()
			}
		}
	}
	requireMySQLAccess(t, db.Callback().Query().After("gorm:query").Register("mysql_prepare_command_error", captureError) == nil && db.Callback().Row().After("gorm:row").Register("mysql_prepare_command_error", captureError) == nil && db.Callback().Raw().After("gorm:raw").Register("mysql_prepare_command_error", captureError) == nil && db.Callback().Create().After("gorm:create").Register("mysql_prepare_command_error", captureError) == nil && db.Callback().Update().After("gorm:update").Register("mysql_prepare_command_error", captureError) == nil && db.Callback().Delete().After("gorm:delete").Register("mysql_prepare_command_error", captureError) == nil, "PREPARE_COMMAND_ERROR_CAPTURE")
	var database string
	var connectionID uint64
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT DATABASE(),CONNECTION_ID()").Row().Scan(&database, &connectionID) == nil && database == lease.Schema && connectionID > 0, "PREPARE_SCHEMA_IDENTITY")
	tables := append([]string(nil), mysqlBaseTables...)
	var engines int64
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ? AND ENGINE='InnoDB'", lease.Schema, tables).Row().Scan(&engines) == nil && engines == int64(len(tables)), "PREPARE_EXACT_30_INNODB")
	var trigger string
	requireMySQLAccess(t, db.WithContext(ctx).Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=? AND TRIGGER_NAME='lfen_s1_audit_fault' AND EVENT_OBJECT_TABLE='knowledge_access_audit' AND EVENT_MANIPULATION='INSERT' AND ACTION_TIMING='BEFORE'", lease.Schema).Row().Scan(&trigger) == nil && lexicalSHA([]byte(trigger)) == lease.TriggerSHA, "PREPARE_PREINSTALLED_TRIGGER")
	oldDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = oldDB }()
	oldStore := storage.GetStorageFactory().GetStorage()
	store := storage.NewMemoryStorage()
	for _, object := range base.Objects {
		requireMySQLAccess(t, store.Put(ctx, object.Bucket, object.Key, []byte(object.Body)) == nil, "PREPARE_OBJECT_COPY")
	}
	storage.GetStorageFactory().SetStorage(store)
	defer storage.GetStorageFactory().SetStorage(oldStore)
	service := NewDatasetService()
	ownerCtx := context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: base.Owner, SHA256: wire.OwnerSessionSHA})
	fence := mysqlPreparationFence{base: base, read: func(current *gorm.DB) (mysqlPreparationGeneration, error) {
		return readMySQLPreparationGeneration(current, ownerCtx, base)
	}}
	requireMySQLAccess(t, fence.check(db) == nil, "PREPARE_INITIAL_BASE_GENERATION")
	native := &mysqlPreparationEngine{lexicalTestEngine: &lexicalTestEngine{rows: map[string]map[string]any{}}, check: func() error { return fence.check(db) }}
	service.docEngine = native
	// Use the owning transaction's connection for these reads. Looking through
	// the global one-connection pool here would deadlock while its Tx is active.
	guardWrite := func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "knowledge_local_lexical_job", "knowledge_local_lexical_publication", "knowledge_access_domain_policy", "knowledge_access_module", "knowledge_access_grant":
			if err := fence.check(tx); err != nil {
				tx.AddError(err)
			}
		}
	}
	requireMySQLAccess(t, db.Callback().Create().Before("gorm:create").Register("mysql_prepare_generation", guardWrite) == nil && db.Callback().Update().Before("gorm:update").Register("mysql_prepare_generation", guardWrite) == nil, "PREPARE_OWNING_WRITE_GENERATION_FENCE")
	requireMySQLAccess(t, accessSession(ownerCtx, db, base.Owner, false) == nil, "PREPARE_OWNER_SESSION")
	recipientCtx := context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: base.Recipient, SHA256: wire.RecipientSessionSHA})
	requireMySQLAccess(t, accessSession(recipientCtx, db, base.Recipient, false) == nil, "PREPARE_RECIPIENT_SESSION")
	for _, spec := range []struct {
		model any
		want  int64
	}{{&entity.User{}, 2}, {&entity.Tenant{}, 1}, {&entity.UserTenant{}, 2}, {&entity.Knowledgebase{}, 1}, {&entity.KnowledgeSourceRevision{}, 1}, {&entity.KnowledgeNode{}, 1}} {
		var n int64
		requireMySQLAccess(t, db.WithContext(ctx).Model(spec.model).Count(&n).Error == nil && n == spec.want, "PREPARE_EXCLUSIVE_BASE_COUNT")
	}
	var members []entity.UserTenant
	requireMySQLAccess(t, db.WithContext(ctx).Where("tenant_id=? AND status=?", base.Tenant, "1").Find(&members).Error == nil && len(members) == 2, "PREPARE_ACTIVE_MEMBERS")
	roles := map[string]string{}
	for _, member := range members {
		roles[member.UserID] = member.Role
	}
	requireMySQLAccess(t, roles[base.Owner] == "owner" && roles[base.Recipient] == "normal", "PREPARE_EXACT_ROLES")
	var users []entity.User
	requireMySQLAccess(t, db.WithContext(ctx).Select("id", "is_superuser").Where("id IN ?", []string{base.Owner, base.Recipient}).Find(&users).Error == nil && len(users) == 2, "PREPARE_USER_FLAGS")
	for _, user := range users {
		requireMySQLAccess(t, user.IsSuperuser != nil && !*user.IsSuperuser, "PREPARE_NON_SUPERUSER")
	}
	var kb entity.Knowledgebase
	requireMySQLAccess(t, db.WithContext(ctx).Where("id=? AND tenant_id=? AND created_by=? AND status=? AND permission=?", base.Domain, base.Tenant, base.Owner, "1", "me").Take(&kb).Error == nil && kb.KnowledgeEnabled && kb.KnowledgePolicyVersion == base.PolicyBaseline.FoundationVersion && kb.KnowledgePolicyVersion == 1 && kb.KnowledgeFolderID != nil, "PREPARE_PRIVATE_ENABLED_DOMAIN")
	var activeTenant int64
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.Tenant{}).Where("id=? AND status=?", base.Tenant, "1").Count(&activeTenant).Error == nil && activeTenant == 1, "PREPARE_ACTIVE_TENANT")
	for _, model := range []any{&entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{}} {
		var n int64
		requireMySQLAccess(t, db.WithContext(ctx).Model(model).Count(&n).Error == nil && n == 0, "PREPARE_EMPTY_OUTPUT_TABLE")
	}
	verifyMySQLBasePolicyBaseline(t, db, ownerCtx, base, "", "")
	var priorCommands int64
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.KnowledgeAccessAuditRecord{}).Where("command_operation IS NOT NULL").Count(&priorCommands).Error == nil && priorCommands == 0, "PREPARE_NO_PRIOR_COMMANDS")
	receipt, err := service.GetKnowledgeImport(ownerCtx, base.Owner, base.Domain, base.Registration, base.Import)
	requireMySQLAccess(t, err == nil && receipt.Status == "succeeded" && lexicalSHA([]byte(accessJSON(receipt))) == base.ReceiptSHA && len(receipt.Items) == 1 && receipt.Items[0].SaveStatus == "saved" && receipt.Items[0].SourceID != nil && receipt.Items[0].SourceRevisionID != nil && receipt.Items[0].RootNodeID != nil, "PREPARE_ACTUAL_C1_RECEIPT")
	item := receipt.Items[0]
	requireMySQLAccess(t, *item.SourceID == base.Target.Source.SourceID && *item.SourceRevisionID == base.Target.Source.SourceRevisionID && *item.RootNodeID == base.Root && item.SHA256 == base.Target.Source.SourceSHA256, "PREPARE_C1_RECEIPT_BINDING")
	source, err := service.GetKnowledgeSource(ownerCtx, base.Owner, base.Domain, base.Target.Source.SourceRevisionID)
	requireMySQLAccess(t, err == nil && source.DocumentID == base.Target.Source.SourceID && source.SHA256 == base.Target.Source.SourceSHA256 && source.RepresentationKind == "text" && source.IsCurrent && source.GroupID == nil && source.ComponentSide == "non_code" && source.ByteLength > 0 && source.ByteLength <= 8192 && source.RootNodeID == base.Root, "PREPARE_ACTUAL_TEXT_SOURCE")
	requireMySQLAccess(t, source.FolderID == *kb.KnowledgeFolderID, "PREPARE_BOUND_NATIVE_FOLDER")
	var root entity.KnowledgeNode
	requireMySQLAccess(t, db.WithContext(ctx).Where("id=? AND domain_id=? AND source_revision_id=?", base.Root, base.Domain, source.ID).Take(&root).Error == nil && root.ParentID == nil && root.StartByte == 0 && root.EndByte == source.ByteLength, "PREPARE_ACTUAL_C1_ROOT")
	objectFound := false
	for _, object := range base.Objects {
		if object.Bucket == source.FolderID && object.Key == ".objects/"+source.SHA256 && int64(len(object.Body)) == source.ByteLength && lexicalSHA([]byte(object.Body)) == source.SHA256 {
			objectFound = true
		}
	}
	requireMySQLAccess(t, objectFound, "PREPARE_REAL_IMMUTABLE_OBJECT")
	selection := entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{accessContentTarget(base.Target)}}
	prepared, err := service.prepareKnowledgeContentLexical(ownerCtx, base.Owner, base.Domain, selection, true)
	requireMySQLAccess(t, err == nil && prepared.Policies.FoundationVersion == base.PolicyBaseline.FoundationVersion && prepared.Policies.LocalVersion == base.PolicyBaseline.LocalVersion && prepared.Policies.ContentVersion == base.PolicyBaseline.ContentVersion && prepared.ProducerSHA256 == producer && len(prepared.Rows) == 1 && len(prepared.Inputs) == 1 && len(prepared.Selection.Targets) == 1 && len(prepared.Coverage) == 1 && len(prepared.Inputs[0].Text) > 0 && len(prepared.Inputs[0].Text) <= 8192, "PREPARE_REAL_ONE_UNIT")
	coverage := prepared.Coverage[0]
	requireMySQLAccess(t, coverage.ReadingTextComplete && coverage.ExpectedBytes == source.ByteLength && coverage.CoveredBytes == source.ByteLength && coverage.IndexedUnits == 1 && int64(len(prepared.Inputs[0].Text)) == source.ByteLength && lexicalSHA([]byte(prepared.Inputs[0].Text)) == source.SHA256, "PREPARE_COMPLETE_C1_UNIT")
	requireMySQLAccess(t, fence.check(db) == nil, "PREPARE_GENERATION_BEFORE_INDEX_CREATE")
	jobView, created, err := service.CreateKnowledgeContentLexicalIndex(ownerCtx, base.Owner, base.Domain, entity.KnowledgeContentLexicalCreateRequest{Version: entity.KnowledgeContentVersion, Key: "mysql-prepare-" + lease.RunID, ExpectedLocalVersion: base.PolicyBaseline.LocalVersion, ExpectedContentVersion: base.PolicyBaseline.ContentVersion, ExpectedFoundationVersion: base.PolicyBaseline.FoundationVersion, ExpectedProfileSHA256: prepared.ProfileSHA256, Selection: prepared.Selection})
	requireMySQLAccess(t, err == nil && created && jobView.State == "queued", "PREPARE_CREATE_ONE_JOB")
	var job entity.KnowledgeLexicalJobRecord
	requireMySQLAccess(t, db.WithContext(ctx).Where("id=? AND owner_id=? AND domain_id=? AND protocol=?", jobView.ID, base.Owner, base.Domain, entity.KnowledgeContentLexicalProtocol).Take(&job).Error == nil, "PREPARE_EXACT_JOB")
	buildCtx, stopBuild := context.WithTimeout(ownerCtx, 90*time.Second)
	defer stopBuild()
	claimed, err := claimKnowledgeLocalLexical(buildCtx, job)
	requireMySQLAccess(t, err == nil && claimed != nil && claimed.AttemptCount == 1, "PREPARE_ACTUAL_CLAIM")
	requireMySQLAccess(t, fence.check(db) == nil, "PREPARE_GENERATION_BEFORE_NATIVE_PUBLICATION")
	requireMySQLAccess(t, service.buildKnowledgeContentLexical(buildCtx, *claimed) == nil, "PREPARE_ACTUAL_BUILD_PUBLICATION")
	requireMySQLAccess(t, fence.check(db) == nil, "PREPARE_PUBLICATION_HAS_ZERO_EPOCH_DELTA")
	published, err := service.GetKnowledgeContentLexicalJob(ownerCtx, base.Owner, base.Domain, job.ID)
	requireMySQLAccess(t, err == nil && published.State == "published" && published.Index != nil && published.ProducerSHA256 == producer && published.ProfileSHA256 == prepared.ProfileSHA256, "PREPARE_PUBLISHED_OWNER_GET")
	var record entity.KnowledgeLexicalPublicationRecord
	requireMySQLAccess(t, db.WithContext(ctx).Where("id=? AND owner_id=? AND domain_id=? AND protocol=?", published.Index.ID, base.Owner, base.Domain, entity.KnowledgeContentLexicalProtocol).Take(&record).Error == nil, "PREPARE_EXACT_PUBLICATION")
	manifest, err := contentLexicalPublication(record)
	requireMySQLAccess(t, err == nil && len(manifest.Rows) == 1 && len(manifest.Selection.Targets) == 1 && manifest.ProducerSHA256 == producer && manifest.ProfileSHA256 == prepared.ProfileSHA256 && accessJSON(accessTarget(manifest.Selection.Targets[0])) == accessJSON(base.Target), "PREPARE_ACTUAL_MANIFEST_TARGET")
	content, err := service.ReadKnowledgeContentLexical(ownerCtx, base.Owner, base.Domain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: contentLexicalEvidence(*published.Index, manifest.Rows[0])})
	if err != nil {
		hasMySQL := logMySQLPreparationDriver(t, err)
		if statements > mysqlPreparationMaxCallbacks {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_WITH_SQL_BUDGET")
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_WITH_DEADLINE")
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_WITH_CANCEL")
		}
		if hasMySQL {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_MYSQL")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_RECORD_NOT_FOUND")
		}
		var lexical *KnowledgeLexicalError
		if errors.As(err, &lexical) && lexical != nil {
			switch lexical.Code {
			case "unauthenticated":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_UNAUTHENTICATED")
			case "not_found":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_NOT_FOUND")
			case "invalid_request":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_INVALID_REQUEST")
			case "contract_mismatch":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_CONTRACT")
			case "integrity_error":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_INTEGRITY")
			case "index_stale":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_INDEX_STALE")
			case "source_changed":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_SOURCE_CHANGED")
			case "policy_changed", "policy_disabled":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_POLICY")
			case "capacity_exceeded":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_CAPACITY")
			case "unsupported_representation":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_UNSUPPORTED")
			case "coverage_gap":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_COVERAGE")
			case "unavailable":
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_UNAVAILABLE")
			default:
				requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_LEXICAL_OTHER")
			}
		}
		var knowledge *KnowledgeError
		if errors.As(err, &knowledge) && knowledge != nil {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_KNOWLEDGE")
		}
		var document *DocumentOperationError
		if errors.As(err, &document) && document != nil {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_DOCUMENT")
		}
		var network net.Error
		if errors.As(err, &network) && network != nil && network.Timeout() {
			requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_TIMEOUT")
		}
		requireMySQLAccess(t, false, "PREPARE_OWNER_READ_ERROR_OTHER")
	}
	requireMySQLAccess(t, content != nil, "PREPARE_OWNER_READ_NON_NIL")
	requireMySQLAccess(t, content.Complete, "PREPARE_OWNER_READ_COMPLETE")
	requireMySQLAccess(t, content.Encoding == "utf-8", "PREPARE_OWNER_READ_ENCODING")
	requireMySQLAccess(t, utf8.ValidString(content.Text), "PREPARE_OWNER_READ_UTF8")
	requireMySQLAccess(t, content.Text == prepared.Inputs[0].Text, "PREPARE_OWNER_READ_TEXT_EQUAL")
	requireMySQLAccess(t, content.ByteLength == len(content.Text), "PREPARE_OWNER_READ_BYTE_LENGTH")
	command := func(op string, body any, status int, out any) {
		prefix := map[string]string{"setPolicy": "PREPARE_CMD_POLICY_", "setModule": "PREPARE_CMD_MODULE_", "createGrant": "PREPARE_CMD_GRANT_"}[op]
		requireMySQLAccess(t, prefix != "", "PREPARE_COMMAND_OPERATION")
		requestID := common.GenerateUUID()
		var env struct {
			Data      json.RawMessage `json:"data"`
			RequestID string          `json:"request_id"`
		}
		detail, replyStatus := "", 0
		commandError, commandContextError = nil, nil
		captureCommand = true
		err := fence.command(db, func() error {
			reply := callMySQLAccess(service, ownerCtx, base.Owner, op, requestID, body)
			replyStatus = reply.status
			if reply.status != status {
				detail = "ERROR"
				return errMySQLBaseScope
			}
			if entity.KnowledgeAccessDecode(reply.raw, accessDefinition(op), nil) != nil {
				detail = "SCHEMA"
				return errMySQLBaseScope
			}
			if json.Unmarshal(reply.raw, &env) != nil {
				detail = "ENVELOPE"
				return errMySQLBaseScope
			}
			if env.RequestID != requestID {
				detail = "REQUEST_ID"
				return errMySQLBaseScope
			}
			if json.Unmarshal(env.Data, out) != nil {
				detail = "DATA"
				return errMySQLBaseScope
			}
			return nil
		}, func() error {
			key := map[string]string{"setPolicy": "mysql-policy-", "setModule": "mysql-module-", "createGrant": "mysql-grant-"}[op] + lease.RunID
			var row entity.KnowledgeAccessAuditRecord
			if err := db.WithContext(ownerCtx).Where("request_id = ? AND actor_user_id = ? AND tenant_id = ? AND domain_id = ? AND command_operation = ? AND command_key = ?", requestID, base.Owner, base.Tenant, base.Domain, op, key).Take(&row).Error; err != nil {
				detail = "QUERY"
				return err
			}
			resource := ""
			switch value := out.(type) {
			case *entity.KnowledgeAccessPolicy:
				resource = value.DomainID
			case *entity.KnowledgeAccessModuleAdmin:
				resource = value.ModuleID
			case *entity.KnowledgeAccessGrant:
				resource = value.GrantID
			}
			if row.Action != op {
				detail = "ACTION"
				return errMySQLBaseScope
			}
			if row.Outcome != "changed" {
				detail = "OUTCOME"
				return errMySQLBaseScope
			}
			if row.CommandState == nil {
				detail = "STATE_PRESENT"
				return errMySQLBaseScope
			}
			if *row.CommandState != "completed" {
				detail = "STATE"
				return errMySQLBaseScope
			}
			if row.CommandCommittedAt == nil {
				detail = "COMMITTED_AT_PRESENT"
				return errMySQLBaseScope
			}
			if row.CommandCommittedAt.Before(lease.NotBefore) {
				detail = "COMMITTED_NOT_BEFORE"
				return errMySQLBaseScope
			}
			if row.CommandCommittedAt.After(time.Now()) {
				detail = "COMMITTED_NOT_AFTER"
				return errMySQLBaseScope
			}
			if row.CommandScopeKind == nil {
				detail = "SCOPE_KIND_PRESENT"
				return errMySQLBaseScope
			}
			if *row.CommandScopeKind != "domain" {
				detail = "SCOPE_KIND"
				return errMySQLBaseScope
			}
			if row.CommandScopeID == nil {
				detail = "SCOPE_ID_PRESENT"
				return errMySQLBaseScope
			}
			if *row.CommandScopeID != base.Domain {
				detail = "SCOPE_ID"
				return errMySQLBaseScope
			}
			if row.CommandResourceID == nil {
				detail = "RESOURCE_PRESENT"
				return errMySQLBaseScope
			}
			if *row.CommandResourceID != resource {
				detail = "RESOURCE"
				return errMySQLBaseScope
			}
			if resource == "" {
				detail = "RESOURCE_NONEMPTY"
				return errMySQLBaseScope
			}
			if row.CommandHTTPStatus == nil {
				detail = "HTTP_STATUS_PRESENT"
				return errMySQLBaseScope
			}
			if *row.CommandHTTPStatus != status {
				detail = "HTTP_STATUS"
				return errMySQLBaseScope
			}
			if row.CommandResultJSON == nil {
				detail = "RESULT_PRESENT"
				return errMySQLBaseScope
			}
			if !bytes.Equal([]byte(*row.CommandResultJSON), env.Data) {
				detail = "RESULT_BYTES"
				return errMySQLBaseScope
			}
			if !bytes.Equal(row.CommandRequestSHA256, accessCommandDigest(base.Owner, op, body)) {
				detail = "REQUEST_DIGEST"
				return errMySQLBaseScope
			}
			return nil
		})
		captureCommand = false
		if err == nil {
			return
		}
		observed := errors.Join(err, commandError)
		hasMySQL := logMySQLPreparationDriver(t, observed)
		class := "OTHER"
		deadline := errors.Is(commandContextError, context.DeadlineExceeded) || errors.Is(observed, context.DeadlineExceeded)
		canceled := errors.Is(commandContextError, context.Canceled) || errors.Is(observed, context.Canceled)
		switch {
		case statements > mysqlPreparationMaxCallbacks && errors.Is(ctx.Err(), context.DeadlineExceeded):
			class = "CAP_PREPARE_DEADLINE"
		case statements > mysqlPreparationMaxCallbacks && errors.Is(ctx.Err(), context.Canceled):
			class = "CAP_PREPARE_CANCEL"
		case statements > mysqlPreparationMaxCallbacks && deadline:
			class = "CAP_CONTEXT_DEADLINE"
		case statements > mysqlPreparationMaxCallbacks && canceled:
			class = "CAP_CONTEXT_CANCEL"
		case statements > mysqlPreparationMaxCallbacks:
			class = "CAP"
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			class = "PREPARE_DEADLINE"
		case errors.Is(ctx.Err(), context.Canceled):
			class = "PREPARE_CANCEL"
		case deadline:
			class = "CONTEXT_DEADLINE"
		case canceled:
			class = "CONTEXT_CANCEL"
		case hasMySQL:
			class = "MYSQL"
		case errors.Is(observed, gorm.ErrRecordNotFound):
			class = "NOT_FOUND"
		default:
			var network net.Error
			if errors.As(observed, &network) && network != nil && network.Timeout() {
				class = "TIMEOUT"
			} else if fence.commandStage == "CALL" && detail == "ERROR" {
				switch replyStatus {
				case 400, 401, 403, 404, 409, 412, 413, 429, 503:
					class = "HTTP_" + strconv.Itoa(replyStatus)
				}
			}
		}
		reason := "INTERNAL"
		switch fence.commandStage {
		case "PRE", "ROLLBACK", "POST":
			if fence.generationFailure == "READ" {
				reason = fence.commandStage + "_READ_" + class
			} else if fence.generationFailure != "" {
				reason = fence.commandStage + "_" + fence.generationFailure
			}
		case "HEADROOM":
			reason = "HEADROOM"
		case "CALL":
			if fence.generationFailure == "READ" {
				reason = "CALL_FENCE_READ_" + class
			} else if fence.generationFailure != "" {
				reason = "CALL_FENCE_" + fence.generationFailure
			} else if detail == "ERROR" {
				reason = "CALL_ERROR_" + class
			} else {
				reason = "CALL_" + detail
			}
		case "RECEIPT":
			if detail == "QUERY" {
				reason = "RECEIPT_QUERY_" + class
			} else {
				reason = "RECEIPT_" + detail
			}
		}
		requireMySQLAccess(t, false, prefix+reason)
	}
	var policy entity.KnowledgeAccessPolicy
	command("setPolicy", entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: base.Domain, ExpectedVersion: 0, Enabled: true, Confirmed: true, IdempotencyKey: "mysql-policy-" + lease.RunID}, 200, &policy)
	requireMySQLAccess(t, policy.DomainID == base.Domain && policy.Enabled && policy.Version == 1, "PREPARE_POLICY_BINDING")
	var module entity.KnowledgeAccessModuleAdmin
	moduleScope := entity.KnowledgeAccessModuleScope{Kind: "fixed_published_targets", Publication: entity.KnowledgeAccessOwnerPublication{ID: record.ID, ManifestSHA256: record.ManifestSHA256}, Targets: []entity.KnowledgeAccessTarget{base.Target}}
	command("setModule", entity.KnowledgeAccessSetModuleRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: base.Domain, ExpectedVersion: 0, Name: "Synthetic MySQL barrier", Enabled: true, Scope: moduleScope, Confirmed: true, IdempotencyKey: "mysql-module-" + lease.RunID}, 200, &module)
	requireMySQLAccess(t, module.DomainID == base.Domain && module.Version == 1 && module.Enabled && module.Name == "Synthetic MySQL barrier" && module.Policy.DomainID == base.Domain && module.Policy.Version == 1 && module.Policy.Enabled && accessJSON(module.Scope) == accessJSON(moduleScope) && len(module.Grants) == 0, "PREPARE_MODULE_BINDING")
	var grant entity.KnowledgeAccessGrant
	command("createGrant", entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: base.Domain, ModuleID: module.ModuleID, ExpectedModuleVersion: module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: base.Recipient}, Mode: "read", Delivery: "web_only", ExpiresAt: base.GrantExpires.UTC().Format(time.RFC3339Nano), Confirmed: true, IdempotencyKey: "mysql-grant-" + lease.RunID}, 201, &grant)
	expires, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	requireMySQLAccess(t, err == nil && expires.Equal(base.GrantExpires) && grant.DomainID == base.Domain && grant.ModuleID == module.ModuleID && grant.ModuleVersion == 1 && grant.Subject.Kind == "user" && grant.Subject.ID == base.Recipient && grant.IssuedByUserID == base.Owner && grant.Mode == "read" && grant.Delivery == "web_only" && grant.RevokedAt == nil, "PREPARE_GRANT_BINDING")
	for _, model := range []any{&entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessGrantRecord{}} {
		var n int64
		requireMySQLAccess(t, db.WithContext(ctx).Model(model).Count(&n).Error == nil && n == 1, "PREPARE_EXACT_OUTPUT_COUNT")
	}
	indexOperationID := lexicalDigest("knowledge-content-operation/0.2.0", base.Owner, base.Domain, "index", "mysql-prepare-"+lease.RunID)
	verifyMySQLBasePolicyBaseline(t, db, ownerCtx, base, indexOperationID, job.ID)
	var commands int64
	requireMySQLAccess(t, db.WithContext(ctx).Model(&entity.KnowledgeAccessAuditRecord{}).Where("command_operation IS NOT NULL").Count(&commands).Error == nil && commands == 3 && fence.committed == 3 && len(native.rows) == 1 && fence.check(db) == nil && statements <= mysqlPreparationMaxCallbacks, "PREPARE_CALL_LEDGER")
	fixture := mysqlAccessFixture{Owner: base.Owner, Recipient: base.Recipient, Tenant: base.Tenant, Domain: base.Domain, Module: module.ModuleID, ModuleVersion: 1, Grant: grant.GrantID, GrantExpires: expires, SessionExpires: base.RecipientSessionExpires, ProducerSHA: producer, Target: base.Target, ExpectedSHA: lexicalSHA([]byte(content.Text)), ExpectedBytes: int64(len(content.Text)), Objects: base.Objects}
	fixtureRaw, err := json.Marshal(fixture)
	requireMySQLAccess(t, err == nil, "PREPARE_PUBLIC_FIXTURE")
	proof := struct {
		Version               string          `json:"version"`
		Fixture               json.RawMessage `json:"fixture"`
		FixtureSHA            string          `json:"fixture_sha256"`
		ELFSHA                string          `json:"test_elf_sha256"`
		Producer              string          `json:"producer_sha256"`
		Publication           string          `json:"publication_id"`
		ManifestSHA           string          `json:"manifest_sha256"`
		BaseReceiptSHA        string          `json:"base_receipt_sha256"`
		PolicyBaselineSHA     string          `json:"native_policy_baseline_sha256"`
		GenerationBaselineSHA string          `json:"native_generation_baseline_sha256"`
		BaseTenantEpoch       int64           `json:"base_tenant_epoch"`
		CommittedEpochDelta   int64           `json:"committed_epoch_delta"`
		SQLStatements         int             `json:"gorm_statement_callbacks"`
		Counts                map[string]int  `json:"counts"`
	}{Version: "mysql-public-fixture/1", Fixture: fixtureRaw, FixtureSHA: lexicalSHA(fixtureRaw), ELFSHA: lease.TestELFSHA, Producer: producer, Publication: record.ID, ManifestSHA: record.ManifestSHA256, BaseReceiptSHA: base.ReceiptSHA, PolicyBaselineSHA: lexicalSHA([]byte(accessJSON(base.PolicyBaseline))), GenerationBaselineSHA: lexicalSHA([]byte(accessJSON(base.GenerationBaseline))), BaseTenantEpoch: base.GenerationBaseline.TenantEpoch, CommittedEpochDelta: fence.committed, SQLStatements: statements, Counts: map[string]int{"explicit_prepare": 1, "prepare_total_source_count": 4, "create": 1, "claim": 1, "build": 1, "owner_get": 1, "owner_read": 1, "s1_mutations": 3, "job": 1, "base_local_policy_operation": 1, "base_content_policy_operation": 1, "local_operation_total": 2, "content_operation_total": 1, "index_operation_delta": 1, "publication": 1, "target": 1, "unit": 1}}
	output, err := json.Marshal(proof)
	requireMySQLAccess(t, err == nil, "PREPARE_PUBLIC_OUTPUT")
	for _, secret := range []string{wire.DSN, wire.OwnerSessionSHA, wire.RecipientSessionSHA} {
		requireMySQLAccess(t, !bytes.Contains(output, []byte(secret)), "PREPARE_NO_SECRET_OUTPUT")
	}
	requireMySQLAccess(t, time.Now().Before(lease.Expires) && base.RecipientSessionExpires.After(base.GrantExpires), "PREPARE_OUTPUT_LIFETIME")
	writeMySQLPreparationOutput(t, ctx, lease.OutputPipe, output)
	t.Log("MYSQL_PUBLIC_FIXTURE_PREPARED_NOT_BARRIER_ACCEPTANCE")
}
