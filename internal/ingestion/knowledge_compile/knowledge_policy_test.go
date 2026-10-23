package knowledge_compile

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

func setupKnowledgeCompilePolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	return db
}

func TestKnowledgeConsumerRejectsProtectedBacklogBeforeDependencies(t *testing.T) {
	db := setupKnowledgeCompilePolicyDB(t)
	fid := "source"
	if err := db.Create(&entity.File{ID: fid, TenantID: "owner", CreatedBy: "owner", Name: "synthetic.txt", Type: "txt"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"private", "copy"} {
		if err := db.Create(&entity.Knowledgebase{ID: id, TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: id == "private"}).Error; err != nil {
			t.Fatal(err)
		}
		doc := entity.Document{ID: "doc-" + id, KbID: id, ParserConfig: entity.JSONMap{}}
		if err := db.Create(&doc).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&entity.File2Document{ID: id, FileID: &fid, DocumentID: &doc.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	called := 0
	consumer := NewConsumer(NewFakeScheduler(), WithDeduperFactory(func(context.Context, string) (Deduper, error) {
		called++
		return nil, errors.New("must not resolve model dependencies")
	}))
	for _, id := range []string{"private", "copy"} {
		if err := consumer.processBatch(t.Context(), "owner", id, "synthetic-token", []BacklogEntry{{DocID: "doc-" + id, EventType: string(EventTypeCompleted)}}, nil); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) || called != 0 {
			t.Fatalf("backlog %s dependencies=%d err=%v", id, called, err)
		}
	}
}
