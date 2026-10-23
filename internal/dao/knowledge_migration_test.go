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

func TestKnowledgeMigrationKeepsMultipleHistoricalFolderIdentities(t *testing.T) {
	db := setupFileTestDB(t)
	ctx := t.Context()
	if err := db.Create(&entity.Knowledgebase{ID: "domain", Name: "Current", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ id, name string }{{"a-old", "Former"}, {"b-current", "Current"}} {
		if err := db.Create(&entity.File{ID: entry.id, ParentID: "parent", TenantID: "owner", Name: entry.name, Type: "folder", SourceType: "knowledgebase"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&entity.KnowledgeSourceRevision{ID: entry.id, DomainID: "domain", DocumentID: entry.id, InputHash: entry.id, FolderID: entry.id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateKnowledgeFoundation(ctx, db); err != nil {
			t.Fatal(err)
		}
		folder, err := NewFileDAO().EnsureKnowledgeFolder(ctx, db, "domain")
		if err != nil || folder.ID != "b-current" {
			t.Fatalf("primary=%+v err=%v", folder, err)
		}
	}
	var folders []string
	if err := db.Model(&entity.KnowledgeSourceRevision{}).Order("folder_id").Pluck("folder_id", &folders).Error; err != nil || len(folders) != 2 || folders[0] != "a-old" || folders[1] != "b-current" {
		t.Fatalf("historical identities changed: %v err=%v", folders, err)
	}
}

func TestKnowledgeNativeNameDeduplicationExcludesManagedFolders(t *testing.T) {
	db := setupFileTestDB(t)
	id := "managed"
	if err := db.Create(&entity.Knowledgebase{ID: "domain", TenantID: "owner", KnowledgeEnabled: true, KnowledgeFolderID: &id}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"managed", "ordinary"} {
		if err := db.Create(&entity.File{ID: id, ParentID: "parent", TenantID: "owner", Name: "Collision", Type: "folder", SourceType: "knowledgebase"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	folder, err := NewFileDAO().newAFileFromDataset(t.Context(), db, "owner", "Collision", "parent")
	if err != nil || folder.ID != "ordinary" {
		t.Fatalf("native adopted managed folder: %+v err=%v", folder, err)
	}
	var count int64
	if err := db.Model(&entity.File{}).Where("id = ?", "managed").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("managed folder removed: %d err=%v", count, err)
	}
}
