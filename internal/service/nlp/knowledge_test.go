package nlp

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/permission"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Native retrieval tests now supply the persisted policy facts used by the
// production execution guard. The fake embedding driver remains local.
func setupRetrievalKnowledgePolicy(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1", CreatedBy: "owner"}).Error; err != nil {
		t.Fatal(err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old; sqlDB.Close() })
}

func TestKnowledgeRetrievalStopsBeforeEmbeddingAndRerank(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: "private", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	old := dao.DB
	dao.DB = db
	defer func() { dao.DB = old }()
	s := &RetrievalService{} // nil engine/client must never be reached.
	_, err = s.Search(context.Background(), &RetrievalSearchRequest{Question: "synthetic", KbIDs: []string{"private"}, EmbeddingModel: &models.EmbeddingModel{}})
	if !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
		t.Fatalf("embedding guard=%v", err)
	}
	_, err = s.Retrieval(context.Background(), &RetrievalRequest{Question: "synthetic", KbIDs: []string{"private"}, RerankModel: &models.RerankModel{}})
	if !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
		t.Fatalf("rerank guard=%v", err)
	}
}
