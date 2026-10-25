package dataset

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"

	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// This small-fixture fake is not evidence for real storage memory or atomicity.
type originalFlowStore struct {
	storage.Storage
	mu          sync.Mutex
	objects     map[string][]byte
	digests     map[string]string
	wholeReads  int
	streamReads int
	failPut     bool
	beforeOpen  func()
}

func newOriginalFlowStore() *originalFlowStore {
	return &originalFlowStore{Storage: storage.NewMemoryStorage(), objects: map[string][]byte{}, digests: map[string]string{}}
}

func (s *originalFlowStore) Get(ctx context.Context, bucket, key string, tenant ...string) ([]byte, error) {
	s.wholeReads++
	return s.Storage.Get(ctx, bucket, key, tenant...)
}

func (s *originalFlowStore) PutImmutableStream(ctx context.Context, bucket, key string, body io.Reader, size int64, media, digest string) (minio.UploadInfo, error) {
	if s.failPut {
		return minio.UploadInfo{}, errors.New("synthetic object failure")
	}
	if size > 1<<20 {
		return minio.UploadInfo{}, errors.New("unit fake only permits small fixtures")
	}
	data, err := io.ReadAll(io.LimitReader(body, size+1))
	if err != nil || int64(len(data)) != size || originalPreparationSHA(data) != digest {
		return minio.UploadInfo{}, errors.New("fake input integrity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name := bucket + "/" + key
	if prior, exists := s.objects[name]; exists {
		if !bytes.Equal(prior, data) || s.digests[name] != digest {
			return minio.UploadInfo{}, storage.ErrStreamObjectIntegrity
		}
	} else {
		s.objects[name] = data
		s.digests[name] = digest
	}
	return minio.UploadInfo{Size: size}, nil
}

func (s *originalFlowStore) OpenStream(ctx context.Context, bucket, key string, size int64, digest string, offset, length int64) (io.ReadCloser, error) {
	s.streamReads++
	if s.beforeOpen != nil {
		before := s.beforeOpen
		s.beforeOpen = nil
		before()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name := bucket + "/" + key
	data, exists := s.objects[name]
	if !exists || int64(len(data)) != size || s.digests[name] != digest {
		return nil, storage.ErrStreamObjectIntegrity
	}
	if offset < 0 || length < 0 || offset > size || length > size-offset {
		return nil, errors.New("fake range")
	}
	return io.NopCloser(bytes.NewReader(data[offset : offset+length])), nil
}

func originalFlowFixture(t *testing.T) (*DatasetService, *entity.KnowledgeOriginalRegistration, *originalFlowStore) {
	t.Helper()
	s, reg := setupOriginalTest(t)
	store := newOriginalFlowStore()
	storage.GetStorageFactory().SetStorage(store)
	return s, reg, store
}

func prepareOriginalFlow(t *testing.T, s *DatasetService, reg *entity.KnowledgeOriginalRegistration, req entity.KnowledgeOriginalUploadRequest, body []byte) *entity.KnowledgeOriginalUpload {
	t.Helper()
	req.DeclaredByteLength = int64(len(body))
	v, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.ReceiveKnowledgeOriginal(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, v.UploadID, io.NopCloser(bytes.NewReader(body)), int64(len(body)), "", "", "application/octet-stream")
	if err != nil || v.State != "ready" || v.SHA256 == nil || *v.SHA256 != originalPreparationSHA(body) || v.Revision < 3 {
		t.Fatalf("ready=%+v err=%v", v, err)
	}
	return v
}

func confirmOriginalFlow(t *testing.T, s *DatasetService, reg *entity.KnowledgeOriginalRegistration, v *entity.KnowledgeOriginalUpload, key string) *entity.KnowledgeOriginalReceipt {
	t.Helper()
	receipt, created, err := s.ConfirmKnowledgeOriginalImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeOriginalConfirmRequest{UploadID: v.UploadID, ManifestSHA256: *v.ManifestSHA256, IdempotencyKey: key, Confirmed: true})
	if err != nil || !created || receipt.Status != "succeeded" {
		t.Fatalf("confirm=%+v new=%t err=%v", receipt, created, err)
	}
	return receipt
}

func TestKnowledgeOriginalFlowVersionsAndFixedReads(t *testing.T) {
	s, reg, store := originalFlowFixture(t)
	firstBytes, secondBytes := []byte("%PDF-1.7\nsynthetic first\n%%EOF"), []byte("%PDF-1.7\nsynthetic second\n%%EOF")
	firstReady := prepareOriginalFlow(t, s, reg, originalTestRequest("v1:create"), firstBytes)
	first := confirmOriginalFlow(t, s, reg, firstReady, "v1:confirm")
	if first.Source.RootNodeID != nil || first.Source.TextAvailable || first.Source.ParseStatus != "not_parsed" || first.ModelCalls != 0 {
		t.Fatal("original claimed parsed text")
	}
	secondRequest := originalTestRequest("v2:create")
	secondRequest.ExpectedRevisionID = &first.Source.SourceRevisionID
	secondReady := prepareOriginalFlow(t, s, reg, secondRequest, secondBytes)
	if secondReady.Action == nil || *secondReady.Action != "updated" {
		t.Fatal("missing updated action")
	}
	second := confirmOriginalFlow(t, s, reg, secondReady, "v2:confirm")
	if second.Source.SourceID != first.Source.SourceID || second.Source.SourceRevisionID == first.Source.SourceRevisionID {
		t.Fatal("source or revision identity changed incorrectly")
	}
	old, err := s.GetKnowledgeOriginalRevision(t.Context(), knowledgeTestOwner, reg.DomainID, first.Source.SourceRevisionID)
	if err != nil || old.IsCurrent {
		t.Fatal("fixed old revision became current", err)
	}
	for _, test := range []struct {
		name, rangeValue, ifRange string
		status                    int
		want                      []byte
	}{
		{"full", "", "", 200, firstBytes}, {"range", "bytes=2-8", "", 206, firstBytes[2:9]},
		{"suffix", "bytes=-7", "", 206, firstBytes[len(firstBytes)-7:]}, {"clamp", "bytes=2-999", "", 206, firstBytes[2:]},
		{"if_range_fallback", "invalid-range", `"other"`, 200, firstBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := s.OpenKnowledgeOriginalContent(t.Context(), knowledgeTestOwner, reg.DomainID, first.Source.SourceRevisionID, test.rangeValue, test.ifRange, false)
			if err != nil || content.Status != test.status {
				t.Fatalf("read=%+v err=%v", content, err)
			}
			data, err := io.ReadAll(content.Body)
			closeErr := content.Body.Close()
			if err != nil || closeErr != nil || !bytes.Equal(data, test.want) {
				t.Fatal("fixed original bytes changed", err, closeErr)
			}
		})
	}
	bad, err := s.OpenKnowledgeOriginalContent(t.Context(), knowledgeTestOwner, reg.DomainID, first.Source.SourceRevisionID, "bytes=999-", "", false)
	assertOriginalCode(t, err, 416, "range_not_satisfiable")
	if bad == nil || bad.Source.ByteLength != int64(len(firstBytes)) {
		t.Fatal("authorized range lacks total")
	}
	head, err := s.OpenKnowledgeOriginalContent(t.Context(), knowledgeTestOwner, reg.DomainID, first.Source.SourceRevisionID, "bad", "bad", true)
	if err != nil || head.Body != nil || head.Status != 200 || head.Length != int64(len(firstBytes)) {
		t.Fatal("HEAD did not ignore Range", err)
	}
	readCount := store.streamReads
	bad, err = s.OpenKnowledgeOriginalContent(t.Context(), knowledgeTestOther, reg.DomainID, first.Source.SourceRevisionID, "bytes=999-", "", false)
	assertOriginalCode(t, err, 404, "knowledge_not_found")
	if bad != nil || store.streamReads != readCount {
		t.Fatal("foreign read reached object storage")
	}
	var source entity.KnowledgeSourceRevision
	if err := dao.DB.Where("id = ?", first.Source.SourceRevisionID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := file.NewFileCommitService().GetCommitFileContent(t.Context(), source.FolderID, source.CommitID, source.FileID); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("native byte route accepted original: %v", err)
	}
	if store.wholeReads != 0 {
		t.Fatal("original used whole-object Storage.Get")
	}
	thirdRequest := originalTestRequest("v3:create")
	thirdRequest.ExpectedRevisionID = &second.Source.SourceRevisionID
	thirdReady := prepareOriginalFlow(t, s, reg, thirdRequest, firstBytes)
	third := confirmOriginalFlow(t, s, reg, thirdReady, "v3:confirm")
	if third.Source.SourceRevisionID != first.Source.SourceRevisionID || third.SaveStatus != "updated" {
		t.Fatal("historical identical input was not reused")
	}
	fourthRequest := originalTestRequest("v4:create")
	fourthRequest.ExpectedRevisionID = &first.Source.SourceRevisionID
	fourthReady := prepareOriginalFlow(t, s, reg, fourthRequest, firstBytes)
	fourth := confirmOriginalFlow(t, s, reg, fourthReady, "v4:confirm")
	if fourth.SaveStatus != "unchanged" || fourth.Source.SourceRevisionID != first.Source.SourceRevisionID {
		t.Fatal("same input created revision")
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("revision count=%d err=%v", count, err)
	}
	page, err := s.ListKnowledgeOriginalRevisions(t.Context(), knowledgeTestOwner, reg.DomainID, nil, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].SourceRevisionID != first.Source.SourceRevisionID {
		t.Fatal("original current-head view mismatch", err)
	}
	replay, created, err := s.ConfirmKnowledgeOriginalImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeOriginalConfirmRequest{UploadID: firstReady.UploadID, ManifestSHA256: *firstReady.ManifestSHA256, IdempotencyKey: "v1:confirm", Confirmed: true})
	if err != nil || created || replay.ImportTaskID != first.ImportTaskID || !replay.Source.IsCurrent {
		t.Fatal("immutable receipt changed with current head", err)
	}
}

func TestKnowledgeOriginalFlowFormatsAndRejection(t *testing.T) {
	for _, kind := range []string{"docx", "pptx", "text"} {
		t.Run(kind, func(t *testing.T) {
			s, reg, _ := originalFlowFixture(t)
			req := originalTestRequest("format:" + kind)
			var body []byte
			if kind == "text" {
				req.OriginalFilename, req.RelativePath, req.DeclaredMediaType = "large.tsx", "web/large.tsx", "text/plain"
				req.Classification = entity.KnowledgeDirectoryClassification{ComponentSide: "frontend", Language: "tsx"}
				body = []byte("\xef\xbb\xbf// 合成\r\nexport const value = '你好😀';\r\n")
			} else {
				req.OriginalFilename, req.RelativePath, req.DeclaredMediaType = "sample."+kind, "docs/sample."+kind, "application/octet-stream"
				body = originalPreparationZIP(t, originalPreparationOfficeParts(kind))
			}
			ready := prepareOriginalFlow(t, s, reg, req, body)
			receipt := confirmOriginalFlow(t, s, reg, ready, "save:"+kind)
			if receipt.Source.Classification.ComponentSide != req.Classification.ComponentSide {
				t.Fatal("component label lost")
			}
			content, err := s.OpenKnowledgeOriginalContent(t.Context(), knowledgeTestOwner, reg.DomainID, receipt.Source.SourceRevisionID, "", "", false)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := io.ReadAll(content.Body)
			_ = content.Body.Close()
			if err != nil || !bytes.Equal(actual, body) {
				t.Fatal("original bytes changed", err)
			}
		})
	}
	for _, test := range []struct {
		name        string
		body        []byte
		declared    int64
		sha         *string
		status      int
		code, state string
	}{
		{"short", []byte("%PDF-1.7\n%%EOF"), 100, nil, 400, "upload_length_mismatch", "rejected"},
		{"long", []byte("%PDF-1.7\n%%EOF extra"), 8, nil, 400, "upload_length_mismatch", "rejected"},
		{"spoof", []byte("not a PDF document"), 18, nil, 415, "upload_format_unsupported", "rejected"},
		{"sha", []byte("%PDF-1.7\n%%EOF"), 14, directoryString(strings.Repeat("a", 64)), 400, "upload_sha256_mismatch", "rejected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, reg, _ := originalFlowFixture(t)
			req := originalTestRequest("reject:" + test.name)
			req.DeclaredByteLength, req.ExpectedSHA256 = test.declared, test.sha
			upload, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ReceiveKnowledgeOriginal(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, io.NopCloser(bytes.NewReader(test.body)), -1, "", "", "application/octet-stream")
			assertOriginalCode(t, err, test.status, test.code)
			stored, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, "")
			if err != nil || stored.State != test.state {
				t.Fatalf("terminal=%+v err=%v", stored, err)
			}
			var n int64
			dao.DB.Model(&entity.Document{}).Count(&n)
			if n != 0 {
				t.Fatal("rejected upload published document")
			}
		})
	}
}

func TestKnowledgeOriginalFlowCASAndRevocation(t *testing.T) {
	s, reg, store := originalFlowFixture(t)
	body := []byte("%PDF-1.7\nsynthetic\n%%EOF")
	a := prepareOriginalFlow(t, s, reg, originalTestRequest("a"), body)
	b := prepareOriginalFlow(t, s, reg, originalTestRequest("b"), body)
	first := confirmOriginalFlow(t, s, reg, a, "a:save")
	_, _, err := s.ConfirmKnowledgeOriginalImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeOriginalConfirmRequest{UploadID: b.UploadID, ManifestSHA256: *b.ManifestSHA256, IdempotencyKey: "b:save", Confirmed: true})
	assertOriginalCode(t, err, 409, "revision_changed")
	still, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, b.UploadID, "")
	if err != nil || still.State != "ready" {
		t.Fatal("CAS conflict consumed ready", err)
	}
	store.beforeOpen = func() {
		if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "0").Error; err != nil {
			t.Fatal(err)
		}
	}
	content, err := s.OpenKnowledgeOriginalContent(t.Context(), knowledgeTestOwner, reg.DomainID, first.Source.SourceRevisionID, "", "", false)
	assertOriginalCode(t, err, 404, "knowledge_not_found")
	if content != nil {
		t.Fatal("revoked actor received stream")
	}
}

func TestKnowledgeOriginalFlowSQLRollbackAndRetry(t *testing.T) {
	s, reg, _ := originalFlowFixture(t)
	body := []byte("%PDF-1.7\nsynthetic rollback\n%%EOF")
	ready := prepareOriginalFlow(t, s, reg, originalTestRequest("rollback:create"), body)
	sentinel := errors.New("synthetic receipt write failure")
	if err := dao.DB.Callback().Create().Before("gorm:create").Register("original_receipt_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_original_import" {
			tx.AddError(sentinel)
		}
	}); err != nil {
		t.Fatal(err)
	}
	req := entity.KnowledgeOriginalConfirmRequest{UploadID: ready.UploadID, ManifestSHA256: *ready.ManifestSHA256, IdempotencyKey: "rollback:confirm", Confirmed: true}
	_, _, err := s.ConfirmKnowledgeOriginalImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	assertOriginalCode(t, err, 503, "result_unknown")
	for _, model := range []any{&entity.Document{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeSourceBinding{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeOriginalImportRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rollback leaked %T rows=%d err=%v", model, count, err)
		}
	}
	view, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.UploadID, "")
	if err != nil || view.State != "ready" || view.ImportTaskID != nil {
		t.Fatal("failed confirmation consumed ready", err)
	}
	if err := dao.DB.Callback().Create().Remove("original_receipt_failure"); err != nil {
		t.Fatal(err)
	}
	receipt, created, err := s.ConfirmKnowledgeOriginalImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil || !created || receipt.Source.SHA256 != originalPreparationSHA(body) {
		t.Fatal("same key retry did not publish fixed original", err)
	}
}

func TestKnowledgeOriginalFlowStorageFailureKeepsRetryableUpload(t *testing.T) {
	s, reg, store := originalFlowFixture(t)
	body := []byte("%PDF-1.7\nsynthetic storage failure\n%%EOF")
	req := originalTestRequest("storage:create")
	req.DeclaredByteLength = int64(len(body))
	upload, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil {
		t.Fatal(err)
	}
	store.failPut = true
	_, err = s.ReceiveKnowledgeOriginal(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, io.NopCloser(bytes.NewReader(body)), int64(len(body)), "", "", "application/octet-stream")
	assertOriginalCode(t, err, 503, "original_storage_unavailable")
	view, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, "")
	if err != nil || view.State != "interrupted" || view.ManifestSHA256 != nil {
		t.Fatal("storage failure exposed ready", err)
	}
	store.failPut = false
	ready, err := s.ReceiveKnowledgeOriginal(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, io.NopCloser(bytes.NewReader(body)), int64(len(body)), "", "", "application/octet-stream")
	if err != nil || ready.State != "ready" || ready.UploadID != upload.UploadID || ready.RequestSHA256 != upload.RequestSHA256 {
		t.Fatal("explicit full retry changed upload identity", err)
	}
}

func TestKnowledgeOriginalFlowReadyFailureCannotPublishOrStayBusy(t *testing.T) {
	s, reg, _ := originalFlowFixture(t)
	body := []byte("%PDF-1.7\nsynthetic ready fault\n%%EOF")
	req := originalTestRequest("ready-fault:create")
	req.DeclaredByteLength = int64(len(body))
	upload, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Callback().Update().Before("gorm:update").Register("original_ready_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_original_upload" {
			if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["state"] == "ready" {
				tx.AddError(errors.New("synthetic ready transaction failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ReceiveKnowledgeOriginal(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, io.NopCloser(bytes.NewReader(body)), int64(len(body)), "", "", "application/octet-stream")
	assertOriginalCode(t, err, 503, "original_storage_unavailable")
	if err := dao.DB.Callback().Update().Remove("original_ready_failure"); err != nil {
		t.Fatal(err)
	}
	view, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, "")
	if err != nil || view.State != "interrupted" || view.ManifestSHA256 != nil {
		t.Fatalf("ready fault was not safely recoverable: %+v %v", view, err)
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("ready fault published source", err)
	}
	ready, err := s.ReceiveKnowledgeOriginal(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, upload.UploadID, io.NopCloser(bytes.NewReader(body)), int64(len(body)), "", "", "application/octet-stream")
	if err != nil || ready.State != "ready" {
		t.Fatal("explicit retry after ready fault failed", err)
	}
}

func TestKnowledgeOriginalLeaseFencingAndCancelSlot(t *testing.T) {
	s, reg, _ := originalFlowFixture(t)
	a, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest("first"))
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.beginOriginalTransfer(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, a.UploadID, -1, "", "", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.beginOriginalTransfer(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, a.UploadID, -1, "", "", "application/octet-stream")
	assertOriginalCode(t, err, 409, "upload_busy")
	if _, err := s.CancelKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, a.UploadID); err != nil {
		t.Fatal(err)
	}
	b, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest("second"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.beginOriginalTransfer(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, b.UploadID, -1, "", "", "application/octet-stream")
	assertOriginalCode(t, err, 429, "upload_slots_busy")
	if err := dao.DB.Model(&entity.KnowledgeOriginalReceiveSlot{}).Where("id = ?", old.slot).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	active, err := s.beginOriginalTransfer(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, b.UploadID, -1, "", "", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.renewOriginalTransfer(t.Context(), old, 10); err == nil {
		t.Fatal("canceled epoch renewed")
	}
	_, err = s.finishOriginalTransfer(t.Context(), old, nil, originalError(409, "upload_interrupted"), 0)
	if err == nil {
		t.Fatal("canceled epoch published ready")
	}
	var slot entity.KnowledgeOriginalReceiveSlot
	if err := dao.DB.Where("id = ?", active.slot).Take(&slot).Error; err != nil || slot.UploadID != b.UploadID {
		t.Fatal("old epoch damaged new receive slot", err)
	}
}
