package dao

import (
	"strings"
	"testing"

	gormMySQL "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/entity"
)

func TestKnowledgeAccessUniqueReconcilesCompleteColumnSets(t *testing.T) {
	db := knowledgeAccessTestDB(t)
	table := entity.KnowledgeAccessAuditRecord{}.TableName()
	name := "idx_ka_audit_command_key"
	columns := []string{"actor_user_id", "command_operation", "command_key"}
	if err := db.Migrator().DropIndex(table, name); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX synthetic_same_columns ON knowledge_access_audit (command_key,actor_user_id,command_operation)").Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureKnowledgeAccessUnique(t.Context(), db, table, name, columns); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasIndex(table, name) || !db.Migrator().HasIndex(table, "synthetic_same_columns") {
		t.Fatal("equivalent full set was renamed or duplicated")
	}
	if err := db.Migrator().DropIndex(table, "synthetic_same_columns"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX idx_ka_audit_command_key ON knowledge_access_audit (actor_user_id,command_operation)").Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureKnowledgeAccessUnique(t.Context(), db, table, name, columns); err == nil {
		t.Fatal("same-name partial column set was accepted")
	}
	indexes, err := db.Migrator().GetIndexes(table)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, index := range indexes {
		if index.Name() == name {
			found = true
			if len(index.Columns()) != 2 {
				t.Fatal("conflicting index was rewritten")
			}
		}
	}
	if !found {
		t.Fatal("conflicting index disappeared")
	}
}

func TestKnowledgeAccessMySQLTenantLockBindsUUID(t *testing.T) {
	// Dialect-only SQL generation over the existing in-memory unit DB.
	// SkipInitialize/DisableAutomaticPing/DryRun prevent MySQL connections.
	memory := knowledgeAccessTestDB(t)
	db, err := gorm.Open(gormMySQL.New(gormMySQL.Config{Conn: memory.Statement.ConnPool, SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	var queries []string
	var bindings [][]any
	if err := db.Callback().Query().After("gorm:query").Register("access:gap:bound-tenant", func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_tenant_state" {
			queries = append(queries, tx.Statement.SQL.String())
			bindings = append(bindings, append([]any(nil), tx.Statement.Vars...))
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, shared := range []bool{false, true} {
		queries, bindings = nil, nil
		if _, err := LockKnowledgeAccessTenants(db, []string{id}, shared); err != nil {
			t.Fatal(err)
		}
		if len(queries) != 1 || strings.Contains(queries[0], id) || !strings.Contains(queries[0], "tenant_id = ?") {
			t.Fatal("owning UUID lookup was not bound")
		}
		bound := false
		for _, value := range bindings[0] {
			bound = bound || value == id
		}
		if !bound {
			t.Fatal("UUID missing from statement variables")
		}
		lock := "FOR UPDATE"
		if shared {
			lock = "FOR SHARE"
		}
		if !strings.Contains(queries[0], lock) {
			t.Fatal("owning current-read lock clause missing")
		}
	}
}
