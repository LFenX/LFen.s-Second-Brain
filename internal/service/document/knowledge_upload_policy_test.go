package document

import (
	"bytes"
	"go.uber.org/zap"
	"mime/multipart"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
	"ragflow/internal/storage"
	"testing"
)

func TestKnowledgeUploadRechecksConcurrentRegistration(t *testing.T) {
	oldLogger := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = oldLogger })
	for _, mode := range []string{"stale_pdf", "stale_nul", "stale_large", "fresh_pdf", "fresh_text", "stale_text", "ordinary_pdf"} {
		t.Run(mode, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			if err := db.AutoMigrate(&entity.Connector2Kb{}, &entity.KnowledgeGroup{}, &entity.KnowledgeNode{}); err != nil {
				t.Fatal(err)
			}
			store := storage.NewMemoryStorage()
			old := storage.GetStorageFactory().GetStorage()
			storage.GetStorageFactory().SetStorage(store)
			t.Cleanup(func() { storage.GetStorageFactory().SetStorage(old) })
			valid := "1"
			id := "11111111111111111111111111111111"
			kb := entity.Knowledgebase{ID: id, TenantID: "owner", CreatedBy: "owner", Name: "Concurrent source", Permission: "me", Status: &valid, ParserID: "general", ParserConfig: entity.JSONMap{}}
			for _, row := range []any{&kb, &entity.UserTenant{ID: "owner-member", UserID: "owner", TenantID: "owner", Role: "owner", Status: &valid}} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			var captured entity.Knowledgebase
			if err := db.Where("id = ?", id).Take(&captured).Error; err != nil {
				t.Fatal(err)
			}
			if mode != "ordinary_pdf" {
				if _, err := dataset.NewDatasetService().RegisterKnowledgeDomain(t.Context(), "owner", id, entity.RegisterKnowledgeDomainRequest{Category: "custom"}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "fresh_pdf" || mode == "fresh_text" {
				if err := db.Where("id = ?", id).Take(&captured).Error; err != nil {
					t.Fatal(err)
				}
			}
			name, body := "payload.pdf", []byte("synthetic non-PDF bytes; no parser is run")
			if mode == "stale_nul" {
				name, body = "payload.txt", []byte{'x', 0, 'y'}
			}
			if mode == "stale_large" {
				name, body = "payload.txt", bytes.Repeat([]byte{'x'}, int(entity.KnowledgeMaxSourceBytes)+1)
			}
			if mode == "fresh_text" || mode == "stale_text" {
				name, body = "payload.tsx", []byte("export const synthetic = 1;\r\n")
			}
			got, messages := testDocumentService(t).UploadLocalDocuments(t.Context(), &captured, "owner", []*multipart.FileHeader{makeTestFileHeader(t, "file", name, body)}, "", nil)
			var docs, tasks int64
			db.Model(&entity.Document{}).Where("kb_id = ?", id).Count(&docs)
			db.Model(&entity.IngestionTask{}).Count(&tasks)
			t.Logf("mode=%s captured_enabled=%t accepted=%d errors=%d persisted_documents=%d tasks=%d", mode, captured.KnowledgeEnabled, len(got), len(messages), docs, tasks)
			if mode == "stale_pdf" || mode == "stale_nul" || mode == "stale_large" || mode == "fresh_pdf" {
				if len(got) != 0 || docs != 0 {
					t.Error("registered domain accepted unsupported input through a previously captured native dataset")
				}
			}
			if mode == "fresh_text" || mode == "stale_text" || mode == "ordinary_pdf" {
				if len(got) != 1 || docs != 1 || tasks != 0 {
					t.Errorf("ordinary/fresh text control failed: result=%v messages=%v", got, messages)
				}
			}
		})
	}
}
