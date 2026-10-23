package task

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/knowledge_compile"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

func setupKnowledgeTaskPolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	if err := db.Create(&entity.Tenant{ID: "owner"}).Error; err != nil {
		t.Fatal(err)
	}
	fid := "source"
	if err := db.Create(&entity.File{ID: fid, TenantID: "owner", CreatedBy: "owner", Name: "synthetic.txt", Type: "txt"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"private", "copy", "ordinary"} {
		kb := entity.Knowledgebase{ID: id, TenantID: "owner", CreatedBy: "owner", EmbdID: "must-not-resolve-provider", KnowledgeEnabled: id == "private", ParserConfig: entity.JSONMap{}}
		if err := db.Create(&kb).Error; err != nil {
			t.Fatal(err)
		}
		doc := entity.Document{ID: "doc-" + id, KbID: id, Name: strPtr("synthetic.txt"), ParserID: "general", ParserConfig: entity.JSONMap{}}
		if err := db.Create(&doc).Error; err != nil {
			t.Fatal(err)
		}
		if id != "ordinary" {
			if err := db.Create(&entity.File2Document{ID: id, FileID: &fid, DocumentID: &doc.ID}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return db
}

func TestKnowledgeRecoveredTaskAndDirtyWikiStopBeforeModels(t *testing.T) {
	setupKnowledgeTaskPolicyDB(t)
	for _, id := range []string{"private", "copy"} {
		task := &entity.IngestionTask{ID: "task-" + id, DatasetID: id, DocumentID: "doc-" + id}
		if _, err := LoadFromIngestionTask(t.Context(), task); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
			t.Fatalf("load %s: %v", id, err)
		}
		// Invoke production wiring. No model table or provider is installed;
		// the typed error proves rejection before model resolution.
		if _, _, err := component.DefaultEmbedderResolver(t.Context(), "owner", id); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
			t.Fatalf("embedder %s: %v", id, err)
		}
		if err := compileDirtyWikiDocument(t.Context(), knowledge_compile.WikiDirtyRequest{TenantID: "owner", DatasetID: id, DocumentID: "doc-" + id}); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
			t.Fatalf("dirty Wiki %s: %v", id, err)
		}
	}
	if _, err := LoadFromIngestionTask(t.Context(), &entity.IngestionTask{ID: "ordinary-task", DatasetID: "ordinary", DocumentID: "doc-ordinary"}); err != nil {
		t.Fatalf("ordinary task changed: %v", err)
	}
}

func TestKnowledgePipelineRechecksPreviouslyLoadedContext(t *testing.T) {
	db := setupKnowledgeTaskPolicyDB(t)
	ctx, err := LoadFromIngestionTask(t.Context(), &entity.IngestionTask{ID: "ordinary-task", DatasetID: "ordinary", DocumentID: "doc-ordinary"})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewPipelineExecutor(ctx, "synthetic-flow", 32)
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	executor.WithLoadDSLFunc(func(context.Context, string) (string, string, error) {
		called++
		return "", "", errors.New("must not load parser")
	})
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "ordinary").Update("knowledge_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(t.Context()); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) || called != 0 {
		t.Fatalf("stale-context parser calls=%d err=%v", called, err)
	}
}
