package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

const knowledgeTestOwner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const knowledgeTestDomain = "11111111111111111111111111111111"
const knowledgeTestOther = "99999999999999999999999999999999"

func setupKnowledgeTest(t *testing.T) (*DatasetService, storage.Storage) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.KnowledgeGroup{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeNode{}, &entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatal(err)
	}
	old := storage.GetStorageFactory().GetStorage()
	store := storage.NewMemoryStorage()
	storage.GetStorageFactory().SetStorage(store)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(old) })
	valid := string(entity.StatusValid)
	for _, id := range []string{knowledgeTestDomain, knowledgeTestOther} {
		if err := db.Create(&entity.Knowledgebase{ID: id, TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "Synthetic knowledge", Permission: "me", Status: &valid}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ensureDatasetTestMembership(t, knowledgeTestOwner, knowledgeTestOwner, "owner")
	return NewDatasetService(), store
}

func registerKnowledgeTest(t *testing.T, s *DatasetService, id string) {
	t.Helper()
	if _, err := s.RegisterKnowledgeDomain(context.Background(), knowledgeTestOwner, id, entity.RegisterKnowledgeDomainRequest{Category: "project"}); err != nil {
		t.Fatal(err)
	}
}

func uploadKnowledgeTest(t *testing.T, store storage.Storage, docID, name string, content []byte) {
	t.Helper()
	valid := string(entity.StatusValid)
	location := "synthetic/" + docID
	folderID := "44444444444444444444444444444444"
	fileID := "f" + docID[1:]
	folder := entity.File{ID: folderID, ParentID: knowledgeTestOwner, TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "synthetic", Type: "folder"}
	if err := dao.DB.Where("id = ?", folderID).FirstOrCreate(&folder).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.Document{ID: docID, KbID: knowledgeTestDomain, CreatedBy: knowledgeTestOwner, Name: &name, Location: &location, Size: int64(len(content)), Status: &valid, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.File{ID: fileID, ParentID: folderID, TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: name, Location: &location, Type: "txt", Size: int64(len(content))}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.File2Document{ID: common.GenerateUUID(), FileID: &fileID, DocumentID: &docID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), knowledgeTestDomain, location, content); err != nil {
		t.Fatal(err)
	}
}

func assertKnowledgeStatus(t *testing.T, err error, status int) {
	t.Helper()
	var e *KnowledgeError
	if !errors.As(err, &e) || e.Status != status {
		t.Fatalf("error = %v, want status %d", err, status)
	}
}

func TestKnowledgeDomainRegistrationAndIsolation(t *testing.T) {
	s, _ := setupKnowledgeTest(t)
	ctx := context.Background()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	domain, err := s.RegisterKnowledgeDomain(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.RegisterKnowledgeDomainRequest{Category: "project"})
	if err != nil || domain.Policy.ExternalModels || domain.Policy.AutomaticIngestion || domain.Policy.Access != "owner_only" || len(domain.Policy.AllowedModelUses) != 0 {
		t.Fatalf("domain=%+v err=%v", domain, err)
	}
	_, err = s.RegisterKnowledgeDomain(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.RegisterKnowledgeDomainRequest{Category: "family"})
	assertKnowledgeStatus(t, err, 409)
	_, err = s.GetKnowledgeDomain(ctx, "intruder", knowledgeTestDomain)
	assertKnowledgeStatus(t, err, 404)
	_, err = s.GetKnowledgeDomain(ctx, knowledgeTestOwner, "not-a-domain")
	assertKnowledgeStatus(t, err, 404)
	page, err := s.ListKnowledgeDomains(ctx, knowledgeTestOwner, "", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != knowledgeTestDomain {
		t.Fatalf("list=%+v err=%v", page, err)
	}
	other, err := s.ListKnowledgeDomains(ctx, "intruder", "", 50)
	if err != nil || len(other.Items) != 0 {
		t.Fatalf("other=%+v err=%v", other, err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestOther).Update("permission", "team").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.RegisterKnowledgeDomain(ctx, knowledgeTestOwner, knowledgeTestOther, entity.RegisterKnowledgeDomainRequest{})
	assertKnowledgeStatus(t, err, 409)
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestOther).Updates(map[string]interface{}{"permission": "me", "chunk_num": 1}).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.RegisterKnowledgeDomain(ctx, knowledgeTestOwner, knowledgeTestOther, entity.RegisterKnowledgeDomainRequest{})
	assertKnowledgeStatus(t, err, 409)
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestOther).Update("chunk_num", 0).Error; err != nil {
		t.Fatal(err)
	}
	link := entity.Connector2Kb{ID: "synthetic-connector-link", ConnectorID: "synthetic-connector", KbID: knowledgeTestOther}
	if err := dao.DB.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.RegisterKnowledgeDomain(ctx, knowledgeTestOwner, knowledgeTestOther, entity.RegisterKnowledgeDomainRequest{})
	assertKnowledgeStatus(t, err, 409)
	if err := dao.DB.Delete(&link).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.IngestionTask{ID: "synthetic-history", DatasetID: knowledgeTestOther, DocumentID: "deleted-synthetic-doc", UserID: knowledgeTestOwner, Status: "COMPLETED"}).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.RegisterKnowledgeDomain(ctx, knowledgeTestOwner, knowledgeTestOther, entity.RegisterKnowledgeDomainRequest{})
	assertKnowledgeStatus(t, err, 409)
	permissionValue := "team"
	if _, _, err := s.UpdateDataset(ctx, knowledgeTestDomain, knowledgeTestOwner, service.UpdateDatasetRequest{Permission: &permissionValue}); err == nil {
		t.Fatal("native update broadened a private domain")
	}
	modelName := "synthetic-model"
	for _, change := range []service.UpdateDatasetRequest{{EmbeddingModel: &modelName}, {EmbdID: &modelName}} {
		if _, _, err := s.UpdateDataset(ctx, knowledgeTestDomain, knowledgeTestOwner, change); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
			t.Fatalf("native model change did not stop before its availability probe: %v", err)
		}
	}
}

func TestKnowledgeGroupsPaginationAndParentScope(t *testing.T) {
	s, _ := setupKnowledgeTest(t)
	ctx := context.Background()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	registerKnowledgeTest(t, s, knowledgeTestOther)
	parent, err := s.CreateKnowledgeGroup(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "Sample"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateKnowledgeGroup(ctx, knowledgeTestOwner, knowledgeTestOther, entity.CreateKnowledgeGroupRequest{ParentID: &parent.ID, Kind: "module", Name: "Cross domain"})
	assertKnowledgeStatus(t, err, 404)
	for _, side := range []string{"frontend", "backend"} {
		if _, err := s.CreateKnowledgeGroup(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{ParentID: &parent.ID, Kind: "module", Name: side, ComponentSide: side}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListKnowledgeGroups(ctx, knowledgeTestOwner, knowledgeTestDomain, parent.ID, "", 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := s.ListKnowledgeGroups(ctx, knowledgeTestOwner, knowledgeTestDomain, parent.ID, first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || first.Items[0].ID == second.Items[0].ID || second.NextCursor != "" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	_, err = s.ListKnowledgeGroups(ctx, knowledgeTestOwner, knowledgeTestDomain, "", first.NextCursor, 1)
	assertKnowledgeStatus(t, err, 400)
	_, err = s.ListKnowledgeGroups(ctx, knowledgeTestOwner, knowledgeTestOther, parent.ID, "", 1)
	assertKnowledgeStatus(t, err, 404)
	_, err = s.ListKnowledgeGroups(ctx, knowledgeTestOwner, knowledgeTestDomain, "", "broken", 1)
	assertKnowledgeStatus(t, err, 400)
	_, err = s.ListKnowledgeGroups(ctx, knowledgeTestOwner, knowledgeTestDomain, "", "", 201)
	assertKnowledgeStatus(t, err, 400)
}

func TestKnowledgeSnapshotRevisionCASAndNativeRead(t *testing.T) {
	s, store := setupKnowledgeTest(t)
	ctx := context.Background()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	docID := "22222222222222222222222222222222"
	original := []byte("标题\r\nconst value = 1;\n")
	uploadKnowledgeTest(t, store, docID, "demo.tsx", original)
	req := entity.SnapshotKnowledgeSourceRequest{DocumentID: docID, RelativePath: "web/demo.tsx", ComponentSide: "frontend", Language: "tsx"}
	first, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !first.Created || first.Root.EndLine != 2 || first.Source.ComponentSide != "frontend" {
		t.Fatalf("snapshot=%+v err=%v", first, err)
	}
	repeat, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || repeat.Created || repeat.Source.ID != first.Source.ID {
		t.Fatalf("idempotency=%+v err=%v", repeat, err)
	}
	var fileRow entity.File
	if err := dao.DB.Where("id = ?", first.Source.FileID).Take(&fileRow).Error; err != nil {
		t.Fatal(err)
	}
	if fileRow.Location == nil || *fileRow.Location != "synthetic/"+docID {
		t.Fatal("snapshot changed the native current file location")
	}
	read, err := s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, first.Root.ID, nil, nil)
	if err != nil || read.Text != string(original) {
		t.Fatalf("read=%+v err=%v", read, err)
	}
	_, err = s.ReadKnowledgeNode(ctx, "intruder", knowledgeTestDomain, first.Root.ID, nil, nil)
	assertKnowledgeStatus(t, err, 404)
	updated := []byte("标题\r\nconst value = 2;\n")
	if err := store.Put(ctx, knowledgeTestDomain, "synthetic/"+docID, updated); err != nil {
		t.Fatal(err)
	}
	_, err = s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	assertKnowledgeStatus(t, err, 409)
	req.ExpectedRevisionID = first.Source.ID
	second, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !second.Created || second.Root.ID == first.Root.ID || second.Root.LogicalID != first.Root.LogicalID {
		t.Fatalf("revision=%+v err=%v", second, err)
	}
	old, err := s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, first.Root.ID, nil, nil)
	if err != nil || old.Text != string(original) {
		t.Fatalf("old revision=%+v err=%v", old, err)
	}
	current, err := s.ListKnowledgeSources(ctx, knowledgeTestOwner, knowledgeTestDomain, "", "", 50)
	if err != nil || len(current.Items) != 1 || current.Items[0].ID != second.Source.ID {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	_, _, err = s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeNodeRequest{SourceRevisionID: first.Source.ID, ParentID: first.Root.ID, Kind: "symbol", LogicalKey: "value", Name: "value", StartByte: 8, EndByte: int64(len(original))})
	assertKnowledgeStatus(t, err, 409)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", docID).Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, first.Root.ID, nil, nil)
	assertKnowledgeStatus(t, err, 404)
}

func TestKnowledgeNodeRangesIdentityAndIntegrity(t *testing.T) {
	s, store := setupKnowledgeTest(t)
	ctx := context.Background()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	registerKnowledgeTest(t, s, knowledgeTestOther)
	docID := "22222222222222222222222222222222"
	content := []byte("标题\r\nconst value = 1;\n")
	uploadKnowledgeTest(t, store, docID, "demo.tsx", content)
	snapshot, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID})
	if err != nil {
		t.Fatal(err)
	}
	req := entity.CreateKnowledgeNodeRequest{SourceRevisionID: snapshot.Source.ID, ParentID: snapshot.Root.ID, Kind: "symbol", LogicalKey: "value", Name: "value", StartByte: 8, EndByte: int64(len(content))}
	node, created, err := s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created || node.StartLine != 2 || node.EndLine != 2 {
		t.Fatalf("node=%+v created=%v err=%v", node, created, err)
	}
	repeat, created, err := s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || created || repeat.ID != node.ID {
		t.Fatalf("repeat=%+v created=%v err=%v", repeat, created, err)
	}
	req.Name = "changed"
	_, _, err = s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	assertKnowledgeStatus(t, err, 409)
	req.Name = "value"
	_, _, err = s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestOther, req)
	assertKnowledgeStatus(t, err, 404)
	req.SourceRevisionID = knowledgeTestOther
	_, _, err = s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	assertKnowledgeStatus(t, err, 404)
	req.SourceRevisionID = snapshot.Source.ID
	req.LogicalKey = "split"
	req.StartByte = 1
	_, _, err = s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	assertKnowledgeStatus(t, err, 400)
	start := int64(1)
	_, err = s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, snapshot.Root.ID, &start, nil)
	assertKnowledgeStatus(t, err, 400)
	start = 0
	_, err = s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, node.ID, &start, nil)
	assertKnowledgeStatus(t, err, 400)
	page, err := s.ListKnowledgeNodes(ctx, knowledgeTestOwner, knowledgeTestDomain, snapshot.Source.ID, snapshot.Root.ID, "", 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != node.ID {
		t.Fatalf("children=%+v err=%v", page, err)
	}
	digest := sha256.Sum256(content)
	if err := store.Put(ctx, snapshot.Source.FolderID, ".objects/"+hex.EncodeToString(digest[:]), []byte("tampered")); err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, node.ID, nil, nil)
	assertKnowledgeStatus(t, err, 409)
}

func TestKnowledgeSourceLimitsAndPathValidation(t *testing.T) {
	s, store := setupKnowledgeTest(t)
	ctx := context.Background()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	docID := "22222222222222222222222222222222"
	uploadKnowledgeTest(t, store, docID, "large.txt", []byte(strings.Repeat("x", 65537)))
	req := entity.SnapshotKnowledgeSourceRequest{DocumentID: docID}
	snapshot, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, snapshot.Root.ID, nil, nil)
	assertKnowledgeStatus(t, err, 413)
	end := int64(65536)
	part, err := s.ReadKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, snapshot.Root.ID, nil, &end)
	if err != nil || len(part.Text) != 65536 {
		t.Fatalf("range length err=%v", err)
	}
	for _, p := range []string{"../secret.txt", "/tmp/a.txt", "C:\\a.txt", "a//b.txt", "a/./b.txt", strings.Repeat("x", 257) + ".txt"} {
		req.RelativePath = p
		_, err = s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
		assertKnowledgeStatus(t, err, 400)
	}
	req.RelativePath = "slides.pptx"
	_, err = s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	assertKnowledgeStatus(t, err, 415)
	req.RelativePath = "bad.txt"
	if err := store.Put(ctx, knowledgeTestDomain, "synthetic/"+docID, []byte{0xff}); err != nil {
		t.Fatal(err)
	}
	_, err = s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	assertKnowledgeStatus(t, err, 415)
}

func TestKnowledgeSnapshotRollbackDoesNotPublishCatalogue(t *testing.T) {
	s, store := setupKnowledgeTest(t)
	ctx := context.Background()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	docID := "22222222222222222222222222222222"
	uploadKnowledgeTest(t, store, docID, "demo.txt", []byte("synthetic"))
	if err := dao.DB.Callback().Create().Before("gorm:create").Register("knowledge_fail_node", func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_node" {
			tx.AddError(errors.New("synthetic DB failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dao.DB.Callback().Create().Remove("knowledge_fail_node") })
	_, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID})
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	for _, model := range []interface{}{&entity.KnowledgeSourceRevision{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeNode{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rollback model=%T count=%d err=%v", model, count, err)
		}
	}
}

func TestKnowledgeReferencesRejectNoncanonicalIDs(t *testing.T) {
	s, store := setupKnowledgeTest(t)
	ctx := t.Context()
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	parent, err := s.CreateKnowledgeGroup(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "Synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(parent.ID)
	_, err = s.CreateKnowledgeGroup(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{ParentID: &upper, Kind: "module", Name: "Bad reference"})
	assertKnowledgeStatus(t, err, 400)
	docID := "abcdefabcdefabcdefabcdefabcdefab"
	uploadKnowledgeTest(t, store, docID, "test.txt", []byte("synthetic"))
	_, err = s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: strings.ToUpper(docID)})
	assertKnowledgeStatus(t, err, 400)
	_, err = s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID, GroupID: &upper})
	assertKnowledgeStatus(t, err, 400)
	_, _, err = s.CreateKnowledgeNode(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeNodeRequest{SourceRevisionID: "UPPER", ParentID: "UPPER"})
	assertKnowledgeStatus(t, err, 400)
}
