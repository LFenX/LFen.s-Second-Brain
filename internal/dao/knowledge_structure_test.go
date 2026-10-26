package dao

import (
	"testing"

	"ragflow/internal/entity"
)

func TestKnowledgeStructureMigrationPreservesExplicitPolicyAndHistory(t *testing.T) {
	db := setupFileTestDB(t)
	if err := migrateKnowledgeStructure(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if !KnowledgeStructureRuntimeReady() {
		t.Fatal("migration did not finish")
	}
	var count int64
	if err := db.Model(&entity.KnowledgeStructurePolicyRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("migration must not enable or invent domain policies", count, err)
	}
	policy := entity.KnowledgeStructurePolicyRecord{DomainID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OwnerID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Version: 7, Enabled: true}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	view := entity.KnowledgeStructureViewRecord{ID: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", OwnerID: policy.OwnerID, DomainID: policy.DomainID, ProjectID: "dddddddddddddddddddddddddddddddd", Published: true, Payload: "{}"}
	if err := db.Create(&view).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateKnowledgeStructure(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var after entity.KnowledgeStructurePolicyRecord
	if err := db.Where("domain_id = ?", policy.DomainID).Take(&after).Error; err != nil || after.Version != 7 || !after.Enabled {
		t.Fatal("migration reset explicit policy", after, err)
	}
	if err := db.Where("id = ? AND published = ?", view.ID, true).Take(&view).Error; err != nil {
		t.Fatal("migration lost history", err)
	}
}
