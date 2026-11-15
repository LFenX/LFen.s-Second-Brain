//go:build integration && linux

package dataset

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ragflow/internal/entity"
)

type mysqlPreparationGenerationBaseline struct {
	TenantStatePresent bool   `json:"tenant_state_present"`
	TenantEpoch        int64  `json:"tenant_epoch"`
	SourceStateSHA256  string `json:"source_state_sha256"`
}

type mysqlPreparationGeneration struct {
	mysqlPreparationGenerationBaseline
	foundation, local, content int64
	valid                      bool
}

// These are eight bounded primary-row reads, not seed writes. The epoch is
// acquired first; inside an owning transaction its lock also fences the later
// native/source/policy reads against writers that obey the same owning order.
func readMySQLPreparationGeneration(db *gorm.DB, ctx context.Context, base mysqlPreparationBase) (mysqlPreparationGeneration, error) {
	// A write callback passes a DB carrying its write Model/Table/WHERE. Reset
	// that statement before choosing each read model, while retaining the same
	// owning transaction ConnPool and callback budget. Otherwise a tenant-state
	// read could accidentally inherit the job/module table being written.
	query := func(model any) *gorm.DB {
		return db.WithContext(ctx).Session(&gorm.Session{NewDB: true}).Model(model)
	}
	var result mysqlPreparationGeneration
	var state entity.KnowledgeAccessTenantStateRecord
	err := query(&state).Clauses(clause.Locking{Strength: "SHARE"}).Where("tenant_id = ?", base.Tenant).Take(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.TenantStatePresent, result.TenantEpoch = true, state.Epoch
	var kb entity.Knowledgebase
	var local entity.KnowledgeLexicalPolicyRecord
	var content entity.KnowledgeContentPolicyRecord
	var source entity.KnowledgeSourceRevision
	var lifecycle entity.KnowledgeSourceLifecycle
	var root entity.KnowledgeNode
	var doc entity.Document
	queries := []*gorm.DB{
		query(&kb).Where("id = ? AND tenant_id = ? AND created_by = ?", base.Domain, base.Tenant, base.Owner).Take(&kb),
		query(&local).Where("domain_id = ? AND owner_id = ?", base.Domain, base.Owner).Take(&local),
		query(&content).Where("domain_id = ? AND owner_id = ?", base.Domain, base.Owner).Take(&content),
		query(&source).Where("id = ? AND domain_id = ? AND document_id = ?", base.Target.Source.SourceRevisionID, base.Domain, base.Target.Source.SourceID).Take(&source),
		query(&lifecycle).Where("source_id = ? AND domain_id = ?", base.Target.Source.SourceID, base.Domain).Take(&lifecycle),
		query(&root).Where("id = ? AND domain_id = ? AND source_revision_id = ?", base.Root, base.Domain, base.Target.Source.SourceRevisionID).Take(&root),
		query(&doc).Where("id = ? AND kb_id = ? AND created_by = ?", base.Target.Source.SourceID, base.Domain, base.Owner).Take(&doc),
	}
	for _, query := range queries {
		if query.Error != nil {
			return result, query.Error
		}
	}
	result.foundation, result.local, result.content = kb.KnowledgePolicyVersion, local.Version, content.Version
	result.valid = mysqlBaseValidStatus(kb.Status) && kb.Permission == "me" && kb.KnowledgeEnabled && kb.KnowledgeFolderID != nil && *kb.KnowledgeFolderID == source.FolderID && local.Enabled && !content.ParseEnabled && content.LexicalEnabled &&
		source.IsCurrent && source.SHA256 == base.Target.Source.SourceSHA256 && source.RepresentationKind == "text" && source.GroupID == nil && source.ComponentSide == "non_code" && source.ByteLength > 0 && source.ByteLength <= 8192 &&
		lifecycle.State == "active" && lifecycle.Version > 0 && root.Kind == "file" && root.ParentID == nil && root.StartByte == 0 && root.EndByte == source.ByteLength && mysqlBaseValidStatus(doc.Status) && doc.Size == source.ByteLength
	result.SourceStateSHA256 = lexicalSHA([]byte(accessJSON([]any{source.ID, source.DomainID, source.DocumentID, source.FileID, source.CommitID, source.FolderID, source.GroupID, source.RelativePath, source.ComponentSide, source.Language, source.SHA256, source.InputHash, source.ByteLength, source.LineCount, source.IsCurrent, source.RepresentationKind, source.OriginalFilename, source.OriginalMediaType, source.OriginalRegistrationID, lifecycle.SourceID, lifecycle.DomainID, lifecycle.State, lifecycle.Version, root.ID, root.ParentID, root.Kind, root.StartByte, root.EndByte, doc.ID, doc.KbID, doc.CreatedBy, doc.Status, doc.Size, doc.Location, kb.KnowledgeFolderID})))
	return result, nil
}

func mysqlPreparationGenerationMismatch(base mysqlPreparationBase, current mysqlPreparationGeneration, committed int64) string {
	baseline, policy := base.GenerationBaseline, base.PolicyBaseline
	if !baseline.TenantStatePresent {
		return "BASELINE_STATE"
	}
	if baseline.TenantEpoch < 0 || baseline.TenantEpoch > entity.KnowledgeAccessMaxGeneration {
		return "BASELINE_EPOCH"
	}
	if !entity.ValidKnowledgeLexicalHash(baseline.SourceStateSHA256) {
		return "BASELINE_DIGEST"
	}
	if committed < 0 || committed > 3 {
		return "COMMITTED_RANGE"
	}
	if baseline.TenantEpoch > entity.KnowledgeAccessMaxGeneration-committed {
		return "EPOCH_HEADROOM"
	}
	if !current.TenantStatePresent {
		return "STATE_MISSING"
	}
	if current.TenantEpoch != baseline.TenantEpoch+committed {
		return "EPOCH"
	}
	if !current.valid {
		return "SOURCE_ENABLED"
	}
	if current.SourceStateSHA256 != baseline.SourceStateSHA256 {
		return "SOURCE_GENERATION"
	}
	if policy.FoundationVersion != 1 || policy.LocalVersion != 1 || policy.ContentVersion != 2 {
		return "POLICY_BASELINE"
	}
	if current.foundation != policy.FoundationVersion {
		return "FOUNDATION"
	}
	if current.local != policy.LocalVersion {
		return "LOCAL_POLICY"
	}
	if current.content != policy.ContentVersion {
		return "CONTENT_POLICY"
	}
	return ""
}

func validateMySQLPreparationGeneration(base mysqlPreparationBase, current mysqlPreparationGeneration, committed int64) error {
	if mysqlPreparationGenerationMismatch(base, current, committed) != "" {
		return errMySQLBaseScope
	}
	return nil
}

type mysqlPreparationFence struct {
	base                            mysqlPreparationBase
	committed                       int64
	read                            func(*gorm.DB) (mysqlPreparationGeneration, error)
	commandStage, generationFailure string
}

func (f *mysqlPreparationFence) check(db *gorm.DB) error {
	f.generationFailure = "READ"
	current, err := f.read(db)
	if err != nil {
		return err
	}
	f.generationFailure = mysqlPreparationGenerationMismatch(f.base, current, f.committed)
	if f.generationFailure != "" {
		return errMySQLBaseScope
	}
	return nil
}

// A returned HTTP status alone never advances the expected epoch. The caller
// must separately read the exact completed command receipt after Commit. A
// failed owning call must leave the prior baseline intact; no retry or repair.
func (f *mysqlPreparationFence) command(db *gorm.DB, call, committedReceipt func() error) error {
	f.commandStage = "PRE"
	if err := f.check(db); err != nil {
		return err
	}
	if f.committed >= 3 || f.base.GenerationBaseline.TenantEpoch >= entity.KnowledgeAccessMaxGeneration-f.committed {
		f.commandStage = "HEADROOM"
		return errMySQLBaseScope
	}
	f.commandStage = "CALL"
	if err := call(); err != nil {
		callGenerationFailure := f.generationFailure
		f.commandStage = "ROLLBACK"
		if rollbackErr := f.check(db); rollbackErr != nil {
			return rollbackErr
		}
		f.commandStage, f.generationFailure = "CALL", callGenerationFailure
		return err
	}
	f.commandStage = "RECEIPT"
	if err := committedReceipt(); err != nil {
		return err
	}
	f.commandStage, f.generationFailure = "POST", "READ"
	current, err := f.read(db)
	if err != nil {
		return err
	}
	f.generationFailure = mysqlPreparationGenerationMismatch(f.base, current, f.committed+1)
	if f.generationFailure != "" {
		return errMySQLBaseScope
	}
	f.committed++
	f.commandStage = "COMPLETE"
	return nil
}

// The same fence guards engine writes before SQL publication. Embedding the
// existing fake engine preserves its row and binding assertions.
type mysqlPreparationEngine struct {
	*lexicalTestEngine
	check func() error
}

func (e *mysqlPreparationEngine) PrepareKnowledgeLexical(ctx context.Context, index, domain, language string) error {
	if err := e.check(); err != nil {
		return err
	}
	return e.lexicalTestEngine.PrepareKnowledgeLexical(ctx, index, domain, language)
}

func (e *mysqlPreparationEngine) CreateKnowledgeLexicalRows(ctx context.Context, index, domain string, rows []map[string]any) error {
	if err := e.check(); err != nil {
		return err
	}
	return e.lexicalTestEngine.CreateKnowledgeLexicalRows(ctx, index, domain, rows)
}

func TestKnowledgeAccessMySQLGenerationGuards(t *testing.T) {
	t.Run("owning_write_statement_does_not_leak_into_reads", func(t *testing.T) {
		cfg := mysqlDriver.NewConfig()
		cfg.Net, cfg.Addr, cfg.DBName = "tcp", "127.0.0.1:1", "synthetic"
		connector, err := mysqlDriver.NewConnector(cfg)
		if err != nil {
			t.Fatal("dryrun_connector")
		}
		pool := sql.OpenDB(connector)
		defer pool.Close()
		db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal("dryrun_gorm")
		}
		var statements []string
		var variables [][]any
		if db.Callback().Query().After("gorm:query").Register("mysql_generation_dryrun_capture", func(tx *gorm.DB) {
			statements = append(statements, tx.Statement.SQL.String())
			variables = append(variables, append([]any(nil), tx.Statement.Vars...))
		}) != nil {
			t.Fatal("dryrun_capture")
		}
		fixture := mysqlPreparationBase{Tenant: "synthetic-tenant", Domain: "synthetic-domain", Owner: "synthetic-owner", Root: "synthetic-root", Target: entity.KnowledgeAccessTarget{Source: entity.KnowledgeAccessSource{SourceID: "synthetic-source", SourceRevisionID: "synthetic-revision"}}}
		contaminated := db.Model(&entity.KnowledgeAccessModuleRecord{}).Where("id = ?", "inherited-write-argument")
		if _, err := readMySQLPreparationGeneration(contaminated, t.Context(), fixture); err != nil {
			t.Fatal("dryrun_generation_read")
		}
		tables := []string{"knowledge_access_tenant_state", "knowledgebase", "knowledge_local_lexical_policy", "knowledge_content_policy", "knowledge_source_revision", "knowledge_source_lifecycle", "knowledge_node", "document"}
		if len(statements) != len(tables) || pool.Stats().OpenConnections != 0 {
			t.Fatal("dryrun_opened_connection_or_wrong_read_count")
		}
		for index, table := range tables {
			if !strings.Contains(statements[index], "FROM `"+table+"`") || strings.Contains(statements[index], "knowledge_access_module") || !strings.Contains(statements[index], "?") {
				t.Fatal("read_inherited_wrong_write_model_or_unbound_where")
			}
			for _, value := range variables[index] {
				if value == "inherited-write-argument" {
					t.Fatal("read_inherited_write_where")
				}
			}
		}
	})
	base := mysqlPreparationBase{PolicyBaseline: mysqlPreparationPolicyBaseline{FoundationVersion: 1, LocalVersion: 1, ContentVersion: 2}, GenerationBaseline: mysqlPreparationGenerationBaseline{TenantStatePresent: true, TenantEpoch: 7, SourceStateSHA256: strings.Repeat("a", 64)}}
	good := mysqlPreparationGeneration{mysqlPreparationGenerationBaseline: base.GenerationBaseline, foundation: 1, local: 1, content: 2, valid: true}
	for _, tc := range []struct {
		name   string
		change func(*mysqlPreparationGeneration)
	}{
		{"foundation_drift", func(g *mysqlPreparationGeneration) { g.foundation = 2 }},
		{"epoch_drift", func(g *mysqlPreparationGeneration) { g.TenantEpoch++ }},
		{"missing_epoch_row", func(g *mysqlPreparationGeneration) { g.TenantStatePresent = false }},
		{"source_generation_drift", func(g *mysqlPreparationGeneration) { g.SourceStateSHA256 = strings.Repeat("b", 64) }},
		{"local_policy_drift", func(g *mysqlPreparationGeneration) { g.local++ }},
		{"content_policy_drift", func(g *mysqlPreparationGeneration) { g.content++ }},
		{"native_or_source_disabled", func(g *mysqlPreparationGeneration) { g.valid = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, calls := good, 0
			tc.change(&current)
			fence := mysqlPreparationFence{base: base, read: func(*gorm.DB) (mysqlPreparationGeneration, error) { return current, nil }}
			if fence.command(nil, func() error { calls++; return nil }, func() error { calls++; return nil }) == nil || calls != 0 || fence.committed != 0 {
				t.Fatal("generation_denial_started_business")
			}
			engine := &mysqlPreparationEngine{lexicalTestEngine: &lexicalTestEngine{rows: map[string]map[string]any{}}, check: func() error { return fence.check(nil) }}
			if engine.CreateKnowledgeLexicalRows(t.Context(), "synthetic", "synthetic", []map[string]any{{"id": "synthetic"}}) == nil || len(engine.rows) != 0 {
				t.Fatal("generation_denial_published_native_rows")
			}
		})
	}
	t.Run("actual_committed_deltas", func(t *testing.T) {
		current, receipts := good, 0
		fence := mysqlPreparationFence{base: base, read: func(*gorm.DB) (mysqlPreparationGeneration, error) { return current, nil }}
		for step := 0; step < 3; step++ {
			if fence.command(nil, func() error { current.TenantEpoch++; return nil }, func() error { receipts++; return nil }) != nil || fence.committed != int64(step+1) || receipts != step+1 {
				t.Fatal("legitimate_committed_delta_rejected")
			}
		}
	})
	t.Run("rollback_does_not_advance", func(t *testing.T) {
		fence := mysqlPreparationFence{base: base, read: func(*gorm.DB) (mysqlPreparationGeneration, error) { return good, nil }}
		failure, receipts := errors.New("synthetic_owning_rollback"), 0
		if !errors.Is(fence.command(nil, func() error { return failure }, func() error { receipts++; return nil }), failure) || fence.committed != 0 || receipts != 0 {
			t.Fatal("rollback_advanced_epoch_expectation")
		}
	})
	t.Run("rollback_with_leaked_epoch_refused", func(t *testing.T) {
		current, receipts := good, 0
		fence := mysqlPreparationFence{base: base, read: func(*gorm.DB) (mysqlPreparationGeneration, error) { return current, nil }}
		failure := errors.New("synthetic_owning_failure")
		if err := fence.command(nil, func() error { current.TenantEpoch++; return failure }, func() error { receipts++; return nil }); !errors.Is(err, errMySQLBaseScope) || fence.committed != 0 || receipts != 0 {
			t.Fatal("failed_mutation_epoch_was_not_rolled_back")
		}
	})
	t.Run("uncommitted_or_wrong_delta_refused", func(t *testing.T) {
		for _, delta := range []int64{0, 2} {
			current := good
			fence := mysqlPreparationFence{base: base, read: func(*gorm.DB) (mysqlPreparationGeneration, error) { return current, nil }}
			if fence.command(nil, func() error { current.TenantEpoch += delta; return nil }, func() error { return nil }) == nil || fence.committed != 0 {
				t.Fatal("wrong_committed_epoch_delta_accepted")
			}
		}
		current := good
		fence := mysqlPreparationFence{base: base, read: func(*gorm.DB) (mysqlPreparationGeneration, error) { return current, nil }}
		if fence.command(nil, func() error { current.TenantEpoch++; return nil }, func() error { return errors.New("receipt_not_durable") }) == nil || fence.committed != 0 {
			t.Fatal("missing_durable_receipt_advanced_expectation")
		}
	})
}
