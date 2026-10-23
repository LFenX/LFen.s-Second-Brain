package document

import (
	"bytes"
	"mime/multipart"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

func TestKnowledgeUploadKeepsCodeNameAndBytesWithoutParsing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	store := newFakeUploadStorage()
	factory := storage.GetStorageFactory()
	old := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(old) })
	kb := &entity.Knowledgebase{ID: "private", TenantID: "owner", CreatedBy: "owner", Name: "Synthetic", ParserID: "general", ParserConfig: entity.JSONMap{}, KnowledgeEnabled: true}
	if err := db.Create(kb).Error; err != nil {
		t.Fatal(err)
	}
	body := []byte("标题\r\nexport const value = 1;\n")
	files := []*multipart.FileHeader{makeTestFileHeader(t, "file", "demo.tsx", body), makeTestFileHeader(t, "file", "demo.jsx", body), makeTestFileHeader(t, "file", "slides.pptx", []byte("synthetic unsupported")), makeTestFileHeader(t, "file", "bad.tsx", []byte{0xff}), makeTestFileHeader(t, "file", "large.txt", bytes.Repeat([]byte{'x'}, int(entity.KnowledgeMaxSourceBytes)+1))}
	got, errs := testDocumentService(t).UploadLocalDocuments(t.Context(), kb, "owner", files, "", nil)
	if len(got) != 2 || len(errs) != 3 {
		t.Fatalf("results=%d errors=%v", len(got), errs)
	}
	for _, doc := range got {
		if doc["name"] != "demo.tsx" && doc["name"] != "demo.jsx" {
			t.Fatalf("filename changed: %v", doc["name"])
		}
		if !bytes.Equal(store.objects[store.key(kb.ID, doc["location"].(string))], body) {
			t.Fatal("source bytes changed")
		}
	}
	var tasks int64
	if err := dao.DB.Model(&entity.IngestionTask{}).Count(&tasks).Error; err != nil || tasks != 0 {
		t.Fatalf("upload scheduled ingestion count=%d err=%v", tasks, err)
	}
	kb.KnowledgeEnabled = false
	got, errs = testDocumentService(t).UploadLocalDocuments(t.Context(), kb, "owner", []*multipart.FileHeader{makeTestFileHeader(t, "file", "native-reject.tsx", body)}, "", nil)
	if len(got) != 0 || len(errs) != 1 {
		t.Fatal("unregistered native upload behavior changed")
	}
}
