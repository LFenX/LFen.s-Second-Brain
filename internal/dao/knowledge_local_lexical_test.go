package dao

import (
	"context"
	"ragflow/internal/entity"
	"testing"
)

func TestKnowledgeLocalLexicalMigrationIsIdempotent(t *testing.T) {
	db := setupFileTestDB(t)
	if err := migrateKnowledgeLocalLexical(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	p := entity.KnowledgeLexicalPolicyRecord{DomainID: "11111111111111111111111111111111", OwnerID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Version: 3, Enabled: true}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateKnowledgeLocalLexical(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var got entity.KnowledgeLexicalPolicyRecord
	if err := db.Where("domain_id = ?", p.DomainID).Take(&got).Error; err != nil || got.Version != 3 || !got.Enabled {
		t.Fatalf("policy changed: %+v %v", got, err)
	}
}
