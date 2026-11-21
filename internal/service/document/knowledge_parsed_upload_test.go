package document

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"os"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/permission"
	"ragflow/internal/service/dataset"
	"ragflow/internal/storage"
)

const parsedUploadTestDomain = "11111111111111111111111111111111"

func init() {
	if parser.IsDocumentStructureChild(os.Args[1:]) {
		os.Exit(parser.DocumentStructureChildMain(context.Background(), dataset.ParseKnowledgeDocumentChild))
	}
}

func parsedUploadFixture(t *testing.T) (*DocumentService, *dataset.DatasetService, *entity.Knowledgebase, storage.Storage) {
	t.Helper()
	db := setupConcurrentServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector2Kb{}, &entity.KnowledgeGroup{}, &entity.KnowledgeNode{}, &entity.KnowledgeContentPolicyRecord{}, &entity.KnowledgeStructureOperationRecord{}, &entity.KnowledgeStructureTaskRecord{}, &entity.KnowledgeStructureLeaseRecord{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeSourceLifecycle{}, &entity.KnowledgeSourceFence{}, &entity.KnowledgeDirectoryState{}, &entity.KnowledgeRetiredPosition{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeDocumentRepresentationRecord{}, &entity.KnowledgeDocumentNodeRecord{}); err != nil {
		t.Fatal(err)
	}
	oldLogger := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = oldLogger })
	store := storage.NewMemoryStorage()
	old := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(store)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(old) })
	valid := "1"
	kb := &entity.Knowledgebase{ID: parsedUploadTestDomain, TenantID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedBy: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "Synthetic parsed native upload", Permission: "me", Status: &valid, ParserID: "general", ParserConfig: entity.JSONMap{}}
	for _, row := range []any{kb, &entity.User{ID: kb.CreatedBy, Nickname: "Synthetic", Email: "owner@synthetic.invalid", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}, &entity.Tenant{ID: kb.TenantID, Status: &valid}, &entity.UserTenant{ID: "synthetic-owner-member", UserID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TenantID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Role: "owner", Status: &valid}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := dataset.NewDatasetService()
	if _, err := service.RegisterKnowledgeDomain(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, entity.RegisterKnowledgeDomainRequest{Category: "custom"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", kb.ID).Take(kb).Error; err != nil {
		t.Fatal(err)
	}
	return testDocumentService(t), service, kb, store
}

func parsedUploadEnable(t *testing.T, s *dataset.DatasetService) entity.KnowledgeParsedUploadRequest {
	t.Helper()
	if _, _, err := s.ChangeKnowledgeContentPolicy(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", parsedUploadTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, ParseEnabled: true, Confirmed: true, IdempotencyKey: "allow-local-upload"}); err != nil {
		t.Fatal(err)
	}
	return entity.KnowledgeParsedUploadRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 2, Confirmed: true}
}

func parsedUploadXLSX(t *testing.T) []byte {
	t.Helper()
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetCellValue("Sheet1", "A1", "Synthetic native workbook"); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := book.Write(&buffer); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func parsedUploadAssertError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var own *KnowledgeParsedUploadError
	if !errors.As(err, &own) || own.Status != status || own.Code != code {
		t.Fatalf("controlled upload error: got %T/%v; want %d/%s", err, err, status, code)
	}
}

func TestKnowledgeParsedUploadDefaultOffCASOwnerAndNativeLinkage(t *testing.T) {
	s, domains, kb, store := parsedUploadFixture(t)
	body := parsedUploadXLSX(t)
	request := entity.KnowledgeParsedUploadRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, Confirmed: true}
	_, err := s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, request, makeTestFileHeader(t, "file", "synthetic.xlsx", body))
	parsedUploadAssertError(t, err, 409, "content_policy_disabled")
	request = parsedUploadEnable(t, domains)
	_, err = s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", kb.TenantID), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", kb.ID, request, makeTestFileHeader(t, "file", "synthetic.xlsx", body))
	parsedUploadAssertError(t, err, 404, "not_found")
	stale := request
	stale.ExpectedContentPolicyVersion = 1
	_, err = s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, stale, makeTestFileHeader(t, "file", "synthetic.xlsx", body))
	parsedUploadAssertError(t, err, 409, "content_policy_changed")
	ordinary, _ := s.UploadLocalDocuments(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), kb, []*multipart.FileHeader{makeTestFileHeader(t, "file", "ordinary.xlsx", body)}, "", nil)
	if len(ordinary) != 0 {
		t.Fatal("ordinary upload silently widened for a registered domain")
	}
	value, err := s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, request, makeTestFileHeader(t, "file", "synthetic.xlsx", body))
	if err != nil || value == nil || value.OriginalFilename != "synthetic.xlsx" || value.ByteLength != int64(len(body)) {
		t.Fatal(value, err)
	}
	var doc entity.Document
	if err := dao.DB.Where("id = ? AND kb_id = ?", value.SourceID, kb.ID).Take(&doc).Error; err != nil {
		t.Fatal(err)
	}
	stored, err := store.Get(t.Context(), kb.ID, *doc.Location)
	if err != nil || !bytes.Equal(stored, body) {
		t.Fatal("native upload bytes changed", err)
	}
	var links int64
	if err := dao.DB.Model(&entity.File{}).Joins("JOIN file2document ON file2document.file_id = file.id").Where("file2document.document_id = ? AND file.parent_id = ? AND file.tenant_id = ?", doc.ID, *kb.KnowledgeFolderID, kb.TenantID).Count(&links).Error; err != nil || links != 1 {
		t.Fatal("missing owning native file linkage", links, err)
	}
	for _, model := range []any{&entity.IngestionTask{}, &entity.KnowledgeStructureTaskRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("upload enqueued processing work", count, err)
		}
	}
	snapshot, err := domains.SnapshotKnowledgeParsedSource(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, entity.KnowledgeParsedSnapshotRequest{DocumentID: value.SourceID, ExpectedPolicyVersion: 1, RelativePath: value.OriginalFilename, Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}})
	if err != nil || snapshot == nil || snapshot.DocumentID != value.SourceID {
		t.Fatal("genuine native upload -> snapshot failed", err)
	}
	state, err := domains.GetKnowledgeParsedSource(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, value.SourceID)
	if err != nil || state.Profile == nil {
		t.Fatal(state, err)
	}
	task, _, err := domains.CreateKnowledgeParsedTask(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, entity.KnowledgeParsedTaskRequest{Source: state.Source, ExpectedLifecycleVersion: state.LifecycleVersion, ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 2, ExpectedProducerSHA256: state.Profile.ProducerSHA256, MaxUnitBytes: 128, Confirmed: true, IdempotencyKey: "native-upload-to-owned-child"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, cancel := context.WithCancel(t.Context())
	defer cancel()
	dataset.StartKnowledgeStructureRuntime(runtime)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		current, err := domains.GetKnowledgeParsedTask(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, task.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State == "failed" {
			t.Fatalf("actual registered-domain upload failed in child: state=%s phase=%s code=%s", current.State, current.Phase, *current.FailureCode)
		}
		if current.State == "succeeded" {
			if current.Result == nil {
				t.Fatal("publication target absent")
			}
			meta, err := domains.GetKnowledgeContentRepresentation(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, *current.Result)
			if err != nil || meta.RootRef == nil {
				t.Fatal(err)
			}
			window, err := domains.ReadKnowledgeContentNode(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, entity.KnowledgeContentReadRequest{Address: entity.KnowledgeContentAddress{Target: *current.Result, Node: *meta.RootRef}})
			if err != nil || window == nil || !bytes.Contains([]byte(window.Text), []byte("Synthetic native workbook")) {
				t.Fatal("genuine upload/child publication did not read back", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, err := domains.GetKnowledgeParsedTask(t.Context(), kb.CreatedBy, kb.ID, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("native task exceeded its deadline at state %s, phase %s", current.State, current.Phase)
}

type parsedUploadRevokingStorage struct {
	storage.Storage
	beforePut func()
}

func (s *parsedUploadRevokingStorage) Put(ctx context.Context, bucket, name string, body []byte, tenant ...string) error {
	if s.beforePut != nil {
		callback := s.beforePut
		s.beforePut = nil
		callback()
	}
	return s.Storage.Put(ctx, bucket, name, body, tenant...)
}

func TestKnowledgeParsedUploadPolicyRevocationDuringStorageRollsBack(t *testing.T) {
	s, domains, kb, store := parsedUploadFixture(t)
	request := parsedUploadEnable(t, domains)
	wrapped := &parsedUploadRevokingStorage{Storage: store, beforePut: func() {
		if _, _, err := domains.ChangeKnowledgeContentPolicy(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 2, Confirmed: true, IdempotencyKey: "revoke-before-native-insert"}); err != nil {
			t.Fatal(err)
		}
	}}
	storage.GetStorageFactory().SetStorage(wrapped)
	_, err := s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, request, makeTestFileHeader(t, "file", "synthetic.xlsx", parsedUploadXLSX(t)))
	parsedUploadAssertError(t, err, 409, "content_policy_changed")
	for _, model := range []any{&entity.Document{}, &entity.File2Document{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("revoked native insert left SQL rows", count, err)
		}
	}
	if exists, err := store.ObjectExists(t.Context(), kb.ID, "synthetic.xlsx"); err != nil || exists {
		t.Fatal("revoked native insert left object", err)
	}
}

func TestKnowledgeParsedUploadBoundsActualBytesAndFamily(t *testing.T) {
	s, domains, kb, _ := parsedUploadFixture(t)
	request := parsedUploadEnable(t, domains)
	large := makeTestFileHeader(t, "file", "synthetic.txt", bytes.Repeat([]byte{'a'}, int(entity.KnowledgeMaxSourceBytes)+1))
	large.Size = 0
	_, err := s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, request, large)
	parsedUploadAssertError(t, err, 413, "capacity_exceeded")
	for _, name := range []string{"synthetic.pdf", "synthetic.docx", "synthetic.pptx", "synthetic.mp3", "synthetic.png", ".env.csv"} {
		_, err := s.UploadKnowledgeParsedLocalDocument(permission.WithIdentity(t.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.TenantID), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", kb.ID, request, makeTestFileHeader(t, "file", name, []byte("synthetic")))
		parsedUploadAssertError(t, err, 415, "unsupported_source")
	}
	var count int64
	if err := dao.DB.Model(&entity.Document{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
