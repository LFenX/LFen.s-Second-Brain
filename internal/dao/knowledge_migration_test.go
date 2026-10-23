package dao

import (
	"context"
	"net/url"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestKnowledgeMigrationIsAdditiveAndRepeatable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Exec("CREATE TABLE knowledgebase (id TEXT PRIMARY KEY, name TEXT, permission TEXT, embd_id TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id,name,permission,embd_id) VALUES (?,?,?,?)", "legacy", "Baseline", "team", "existing-model").Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateKnowledgeFoundation(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	var old struct {
		Name                   string
		Permission             string
		EmbdID                 string
		KnowledgeEnabled       bool
		KnowledgeCategory      string
		KnowledgePolicyVersion int64
	}
	if err := db.Table("knowledgebase").Where("id = ?", "legacy").Take(&old).Error; err != nil {
		t.Fatal(err)
	}
	if old.Name != "Baseline" || old.Permission != "team" || old.EmbdID != "existing-model" || old.KnowledgeEnabled || old.KnowledgePolicyVersion != 0 || old.KnowledgeCategory != "" {
		t.Fatalf("migration changed old row: %+v", old)
	}
	for _, model := range []interface{}{&entity.KnowledgeGroup{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeNode{}} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("missing %T", model)
		}
	}
}
