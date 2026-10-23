package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestKnowledgeNativeChatAndIngestionStopBeforeProviderOrQueue(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := context.Background()
	if err := db.Create(&entity.Knowledgebase{ID: "private", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	// The services deliberately have no provider/engine: a missed guard would
	// enter the real pipeline instead of returning the policy denial.
	chat := &entity.Chat{ID: "synthetic", KBIDs: entity.JSONSlice{"private"}}
	_, err := (&ChatPipelineService{}).AsyncChat(ctx, "owner", chat, []map[string]interface{}{{"role": "user", "content": "synthetic"}}, false, nil)
	if !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
		t.Fatalf("chat did not stop at policy: %v", err)
	}
	if _, err := NewIngestionTaskService().CreateForDocuments(ctx, "private", "owner", []string{"synthetic-doc"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("ingestion did not stop at policy: %v", err)
	}
	if _, err := NewIngestionTaskService().CreateAndEnqueue(ctx, &entity.IngestionTask{DatasetID: "private", DocumentID: "synthetic-doc"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("queue replay did not stop at policy: %v", err)
	}
	var count int64
	if err := db.Model(&entity.IngestionTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("policy denial created task count=%d err=%v", count, err)
	}
	valid := "1"
	if err := db.Create(&entity.Knowledgebase{ID: "old-copy-target", TenantID: "owner", CreatedBy: "owner", Permission: "team", Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	fid := "synthetic-source"
	if err := db.Create(&entity.File{ID: fid, TenantID: "owner", CreatedBy: "owner", Name: "synthetic.txt", Type: "txt"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"private", "old-copy-target"} {
		doc := entity.Document{ID: id, KbID: id, Status: &valid, ParserConfig: entity.JSONMap{}}
		if err := db.Create(&doc).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&entity.File2Document{ID: id, FileID: &fid, DocumentID: &doc.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	chat.KBIDs = entity.JSONSlice{"old-copy-target"}
	_, err = (&ChatPipelineService{}).AsyncChat(ctx, "owner", chat, []map[string]interface{}{{"role": "user", "content": "synthetic"}}, false, nil)
	if !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
		t.Fatalf("old copy chat reached provider: %v", err)
	}
	if _, err := NewIngestionTaskService().CreateForDocuments(ctx, "old-copy-target", "owner", []string{"old-copy-target"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("old copy reached task queue: %v", err)
	}
	if _, err := NewIngestionTaskService().CreateAndEnqueue(ctx, &entity.IngestionTask{DatasetID: "old-copy-target", DocumentID: "old-copy-target"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("old copy replay reached queue: %v", err)
	}
	if err := db.Model(&entity.IngestionTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("old copy created task count=%d err=%v", count, err)
	}
}

func TestKnowledgeCreatedTaskRecoveryRechecksPolicyAndContinuesOrdinaryTasks(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	for _, id := range []string{"private", "copy", "ordinary"} {
		insertTestKB(t, id, "owner", 1, 0, 0)
		insertTestDoc(t, "doc-"+id, id, 0, 0)
		insertTestIngestionTaskWithStatus(t, "task-"+id, "owner", "doc-"+id, id, common.CREATED)
	}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "private").Update("knowledge_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	fid := "source"
	if err := db.Create(&entity.File{ID: fid, TenantID: "owner", CreatedBy: "owner", Type: "txt", Name: "synthetic.txt"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"private", "copy"} {
		docID := "doc-" + id
		if err := db.Create(&entity.File2Document{ID: id, FileID: &fid, DocumentID: &docID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	if err := svc.ScheduleCreatedTasks(t.Context()); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("recovery policy: %v", err)
	}
	if len(publisher.messages) != 1 || publisher.messages[0].TaskID != "task-ordinary" {
		t.Fatalf("published protected task: %+v", publisher.messages)
	}
	for _, id := range []string{"private", "copy"} {
		var task entity.IngestionTask
		if err := db.Where("id = ?", "task-"+id).Take(&task).Error; err != nil {
			t.Fatal(err)
		}
		if task.Status != common.CREATED || task.PipelineLogID != nil {
			t.Fatalf("denied task mutated: %+v", task)
		}
	}
}
