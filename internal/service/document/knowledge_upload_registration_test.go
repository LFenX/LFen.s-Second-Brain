package document

import (
	"bytes"
	"context"
	"mime/multipart"
	"ragflow/internal/permission"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
	"ragflow/internal/storage"

	"go.uber.org/zap"
)

type registrationUploadStorage struct {
	storage.Storage
	beforePut func()
}

func (s *registrationUploadStorage) Put(ctx context.Context, bucket, name string, blob []byte, tenantID ...string) error {
	if s.beforePut != nil {
		register := s.beforePut
		s.beforePut = nil
		register()
	}
	return s.Storage.Put(ctx, bucket, name, blob, tenantID...)
}

func TestKnowledgeUploadRegistrationDuringStorage(t *testing.T) {
	oldLogger := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = oldLogger })
	for _, mode := range []string{"pdf", "nul", "large", "text", "upload_first"} {
		t.Run(mode, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			if err := db.AutoMigrate(&entity.Connector2Kb{}, &entity.KnowledgeGroup{}, &entity.KnowledgeNode{}); err != nil {
				t.Fatal(err)
			}
			valid := "1"
			kb := entity.Knowledgebase{ID: "11111111111111111111111111111111", TenantID: "owner", CreatedBy: "owner", Name: "Synthetic upload", Permission: "me", Status: &valid, ParserID: "general", ParserConfig: entity.JSONMap{}}
			for _, row := range []any{&kb, &entity.User{ID: kb.CreatedBy, Nickname: "Synthetic", Email: "owner@synthetic.invalid", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}, &entity.Tenant{ID: kb.TenantID, Status: &valid}, &entity.UserTenant{ID: "owner-member", UserID: "owner", TenantID: "owner", Role: "owner", Status: &valid}} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			store := &registrationUploadStorage{Storage: storage.NewMemoryStorage()}
			old := storage.GetStorageFactory().GetStorage()
			storage.GetStorageFactory().SetStorage(store)
			t.Cleanup(func() { storage.GetStorageFactory().SetStorage(old) })
			domains := dataset.NewDatasetService()
			if mode != "upload_first" {
				store.beforePut = func() {
					if _, err := domains.RegisterKnowledgeDomain(t.Context(), "owner", kb.ID, entity.RegisterKnowledgeDomainRequest{Category: "custom"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			name, blob := "synthetic.txt", []byte("中文\r\nconst value = 1;\n")
			switch mode {
			case "pdf", "upload_first":
				name, blob = "synthetic.pdf", []byte("synthetic PDF control; no parser runs")
			case "nul":
				blob = []byte{'x', 0, 'y'}
			case "large":
				blob = bytes.Repeat([]byte{'x'}, int(entity.KnowledgeMaxSourceBytes)+1)
			}
			got, messages := testDocumentService(t).UploadLocalDocuments(permission.WithIdentity(t.Context(), "owner", kb.TenantID), &kb, []*multipart.FileHeader{makeTestFileHeader(t, "file", name, blob)}, "", nil)
			var docs, mappings, tasks int64
			for model, count := range map[any]*int64{&entity.Document{}: &docs, &entity.File2Document{}: &mappings, &entity.IngestionTask{}: &tasks} {
				if err := db.Model(model).Count(count).Error; err != nil {
					t.Fatal(err)
				}
			}
			var current entity.Knowledgebase
			if err := db.Where("id = ?", kb.ID).Take(&current).Error; err != nil {
				t.Fatal(err)
			}
			if tasks != 0 {
				t.Fatalf("unexpected parser work: %d", tasks)
			}
			if mode == "pdf" || mode == "nul" || mode == "large" {
				exists, err := store.ObjectExists(t.Context(), kb.ID, name)
				if len(got) != 0 || len(messages) != 1 || docs != 0 || mappings != 0 || current.DocNum != 0 || !current.KnowledgeEnabled || err != nil || exists {
					t.Fatalf("registration race accepted input or left residue: accepted=%d errors=%v docs=%d mappings=%d count=%d registered=%t blob=%t err=%v", len(got), messages, docs, mappings, current.DocNum, current.KnowledgeEnabled, exists, err)
				}
				return
			}
			if len(got) != 1 || len(messages) != 0 || docs != 1 || mappings != 1 || current.DocNum != 1 {
				t.Fatalf("valid upload failed: accepted=%d errors=%v docs=%d mappings=%d count=%d", len(got), messages, docs, mappings, current.DocNum)
			}
			if mode == "upload_first" {
				if _, err := domains.RegisterKnowledgeDomain(t.Context(), "owner", kb.ID, entity.RegisterKnowledgeDomainRequest{Category: "custom"}); err == nil {
					t.Fatal("registration accepted an already published ordinary PDF")
				}
				return
			}
			var source entity.File
			if err := db.Joins("JOIN file2document m ON m.file_id = file.id").Where("m.document_id = ?", got[0]["id"]).Take(&source).Error; err != nil {
				t.Fatal(err)
			}
			if current.KnowledgeFolderID == nil || source.ParentID != *current.KnowledgeFolderID {
				t.Fatal("registered upload retained the stale ordinary folder")
			}
		})
	}
}
