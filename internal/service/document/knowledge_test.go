package document

import (
	"bytes"
	"errors"
	"mime/multipart"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/storage"
)

func TestKnowledgeOrdinaryUploadDoesNotAdoptManagedName(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	id := "managed"
	if err := db.Create(&entity.Knowledgebase{ID: "domain", TenantID: "owner", KnowledgeEnabled: true, KnowledgeFolderID: &id}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.File{ID: id, ParentID: "parent", TenantID: "owner", CreatedBy: "owner", Name: "Collision", Type: "folder", SourceType: "knowledgebase"}).Error; err != nil {
		t.Fatal(err)
	}
	folder, err := testDocumentService(t).newAFileFromKB(t.Context(), db, "owner", "Collision", "parent")
	if err != nil || folder.ID == id {
		t.Fatalf("ordinary folder adopted private binding: %+v err=%v", folder, err)
	}
}

func TestKnowledgeOrdinaryUploadDoesNotAdoptUnclaimedFolder(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.Create(&entity.File{ID: "orphan", ParentID: "parent", TenantID: "owner", CreatedBy: "owner", Name: "Former", Type: "folder", SourceType: "knowledgebase"}).Error; err != nil {
		t.Fatal(err)
	}
	folder, err := testDocumentService(t).newAFileFromKB(t.Context(), db, "owner", "Former", "parent")
	if err != nil || folder.ID == "orphan" {
		t.Fatalf("ordinary upload adopted unclaimed private folder: %+v err=%v", folder, err)
	}
}

func TestKnowledgeUploadKeepsCodeNameAndBytesWithoutParsing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
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
	got, errs := testDocumentService(t).UploadLocalDocuments(permission.WithIdentity(t.Context(), "owner", kb.TenantID), kb, files, "", nil)
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
	kb.ID = "ordinary"
	kb.KnowledgeEnabled = false
	kb.KnowledgeFolderID = nil
	if err := db.Create(kb).Error; err != nil {
		t.Fatal(err)
	}
	got, errs = testDocumentService(t).UploadLocalDocuments(permission.WithIdentity(t.Context(), "owner", kb.TenantID), kb, []*multipart.FileHeader{makeTestFileHeader(t, "file", "native-reject.tsx", body)}, "", nil)
	if len(got) != 0 || len(errs) != 1 {
		t.Fatal("unregistered native upload behavior changed")
	}
}

func TestKnowledgeNativeFileTransferCannotRepublishProtectedSources(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: "private", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	doc := entity.Document{ID: "protected-doc", KbID: "private", ParserConfig: entity.JSONMap{}}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}
	fileID := "protected-file"
	if err := db.Create(&entity.File2Document{ID: "synthetic-link", FileID: &fileID, DocumentID: &doc.ID}).Error; err != nil {
		t.Fatal(err)
	}
	// No dependencies are configured: both the queued conversion and the direct
	// target-domain path must stop before changing any document or mapping.
	svc := &File2DocumentService{}
	if err := svc.convertFiles(permission.WithIdentity(t.Context(), "owner", "owner"), []string{fileID}, []string{"unregistered"}, "add"); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("protected source transfer=%v", err)
	}
	if err := svc.convertFiles(permission.WithIdentity(t.Context(), "owner", "owner"), []string{"ordinary"}, []string{"private"}, "add"); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("unsupported protected target import=%v", err)
	}
	var mappings int64
	if err := db.Model(&entity.File2Document{}).Count(&mappings).Error; err != nil || mappings != 1 {
		t.Fatalf("denied transfer mutated mappings: %d err=%v", mappings, err)
	}
	if err := db.Create(&entity.KnowledgeSourceRevision{ID: "historical", DomainID: "private", DocumentID: "old", FileID: "historical-file", InputHash: "hash"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := permission.CheckKnowledgeFileTransfer(t.Context(), db, []string{"historical-file"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("historical source transfer=%v", err)
	}
	if err := permission.CheckKnowledgeFileTransfer(t.Context(), db, []string{"ordinary"}); err != nil {
		t.Fatalf("unregistered file transfer changed: %v", err)
	}
	if err := db.Where("id = ?", "private").Delete(&entity.Knowledgebase{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := permission.CheckKnowledgeFileTransfer(t.Context(), db, []string{"historical-file"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("deleting the last domain removed historical protection: %v", err)
	}
}
