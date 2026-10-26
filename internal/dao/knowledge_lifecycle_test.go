package dao

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestKnowledgeLifecycleMigrationGenerationAndAmbiguousHeads(t *testing.T) {
	db := setupFileTestDB(t)
	if err := db.AutoMigrate(&entity.Document{}, &entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	domain, source := strings.Repeat("a", 32), strings.Repeat("b", 32)
	valid := "1"
	if err := db.Create(&entity.Document{ID: source, KbID: domain, Status: &valid, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatal(err)
	}
	head := entity.KnowledgeSourceRevision{ID: strings.Repeat("c", 32), DomainID: domain, DocumentID: source, InputHash: strings.Repeat("d", 64), IsCurrent: true}
	if err := db.Create(&head).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateKnowledgeLifecycle(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var state entity.KnowledgeSourceLifecycle
	if err := db.Where("source_id = ?", source).Take(&state).Error; err != nil || state.Version != 1 || state.State != "active" || !KnowledgeLifecycleRuntimeReady() {
		t.Fatal(state, err)
	}
	if err := db.Model(&state).Updates(map[string]any{"state": "archived", "version": int64(7)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLifecycle(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("source_id = ?", source).Take(&state).Error; err != nil || state.Version != 7 || state.State != "archived" {
		t.Fatal("migration reset state", err)
	}
	if err := db.Model(&head).Update("is_current", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLifecycle(t.Context(), db); err == nil || KnowledgeLifecycleRuntimeReady() {
		t.Fatal("zero-head source silently initialized")
	}
	if err := db.Model(&head).Update("is_current", true).Error; err != nil {
		t.Fatal(err)
	}
	extra := head
	extra.ID = strings.Repeat("e", 32)
	extra.InputHash = strings.Repeat("f", 64)
	if err := db.Create(&extra).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLifecycle(t.Context(), db); err == nil || KnowledgeLifecycleRuntimeReady() {
		t.Fatal("multiple heads silently chosen")
	}
	var count int64
	if err := db.Model(&entity.KnowledgeSourceRevision{}).Where("document_id = ? AND is_current = ?", source, true).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("migration edited historical rows", err)
	}
}
