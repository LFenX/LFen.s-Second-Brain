package dataset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupOriginalTest(t *testing.T) (*DatasetService, *entity.KnowledgeOriginalRegistration) {
	t.Helper()
	t.Setenv("LFEN_KNOWLEDGE_ORIGINAL_STAGING_ROOT", filepath.Join(t.TempDir(), "staging"))
	s, _, _ := setupKnowledgeImportTest(t)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeOriginalUploadRecord{}, &entity.KnowledgeOriginalReceiveSlot{}, &entity.KnowledgeOriginalImportRecord{}, &entity.KnowledgeDirectoryState{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2} {
		if err := dao.DB.Create(&entity.KnowledgeOriginalReceiveSlot{ID: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := entity.KnowledgeOriginalRegistrationRequest{RegistrationKey: "original:synthetic", Name: "Synthetic originals", SourceKind: "original_document", InputMode: "original_upload", IncludePaths: []string{"docs/sample.pdf", "web/large.tsx", "docs/sample.docx", "docs/sample.pptx"}, DefaultClassification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code", Language: ""}}
	reg, created, err := s.CreateKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatalf("original registration: created=%t err=%v", created, err)
	}
	return s, reg
}

func originalTestRequest(key string) entity.KnowledgeOriginalUploadRequest {
	return entity.KnowledgeOriginalUploadRequest{OriginalFilename: "original.pdf", RelativePath: "docs/sample.pdf",
		Classification:     entity.KnowledgeDirectoryClassification{ComponentSide: "non_code", Language: ""},
		DeclaredByteLength: 1024, DeclaredMediaType: "application/pdf", IdempotencyKey: key}
}

func assertOriginalCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var public *KnowledgeError
	if !errors.As(err, &public) || public.Status != status || public.Code != code {
		t.Fatalf("error=%v want=%d/%s", err, status, code)
	}
}

func TestKnowledgeOriginalRegistrationAndUploadRecovery(t *testing.T) {
	s, reg := setupOriginalTest(t)
	if strings.Join(reg.IncludePaths, ",") != "docs/sample.docx,docs/sample.pdf,docs/sample.pptx,web/large.tsx" {
		t.Fatal("exact paths not canonical")
	}
	byKey, err := s.GetKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, "", reg.RegistrationKey)
	if err != nil || byKey.RegistrationID != reg.RegistrationID || byKey.ConfigSHA256 != reg.ConfigSHA256 {
		t.Fatal("registration recovery mismatch", err)
	}
	if _, err := s.GetKnowledgeRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID); err == nil {
		t.Fatal("0.2 accepted original registration")
	}
	req := originalTestRequest("create:001")
	view, created, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil || !created || view.State != "awaiting_bytes" || view.Revision != 1 || view.SourceID != nil || view.SHA256 != nil || view.ManifestSHA256 != nil {
		t.Fatalf("upload=%+v new=%t err=%v", view, created, err)
	}
	replay, created, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil || created || replay.UploadID != view.UploadID || replay.RequestSHA256 != view.RequestSHA256 {
		t.Fatal("original creation replay changed", err)
	}
	req.OriginalFilename = "other.pdf"
	_, _, err = s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	assertOriginalCode(t, err, 409, "idempotency_conflict")
	recovered, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, "", view.IdempotencyKey)
	if err != nil || recovered.UploadID != view.UploadID {
		t.Fatal("original key query changed", err)
	}
	var documents, sources int64
	if err := dao.DB.Model(&entity.Document{}).Where("kb_id = ?", reg.DomainID).Count(&documents).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&sources).Error; err != nil || documents != 0 || sources != 0 {
		t.Fatal("metadata registration published source", err)
	}
}

func TestKnowledgeOriginalQuotaExpiryPolicyAndCancel(t *testing.T) {
	s, reg := setupOriginalTest(t)
	create := func(key string) *entity.KnowledgeOriginalUpload {
		t.Helper()
		v, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest(key))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first, second := create("first"), create("second")
	_, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest("third"))
	assertOriginalCode(t, err, 413, "upload_quota_exceeded")
	canceled, err := s.CancelKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, first.UploadID)
	if err != nil || canceled.State != "cancelled" || canceled.Revision <= first.Revision {
		t.Fatal("cancel failed", err)
	}
	again, err := s.CancelKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, first.UploadID)
	if err != nil || again.Revision != canceled.Revision {
		t.Fatal("cancel was not idempotent", err)
	}
	third := create("third")
	if err := dao.DB.Model(&entity.KnowledgeOriginalUploadRecord{}).Where("id = ?", second.UploadID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, second.UploadID, "")
	if err != nil || expired.State != "expired" || expired.Code == nil || *expired.Code != "upload_expired" {
		t.Fatal("expired query did not retain terminal state", err)
	}
	replay, newUpload, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, originalTestRequest("second"))
	if err != nil || newUpload || replay.UploadID != second.UploadID || replay.State != "expired" {
		t.Fatal("expiry lost original request identity", err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", reg.DomainID).Update("knowledge_policy_version", 2).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.GetKnowledgeOriginalUpload(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, third.UploadID, "")
	assertOriginalCode(t, err, 409, "policy_changed")
	var stored entity.KnowledgeOriginalUploadRecord
	if err := dao.DB.Where("id = ?", third.UploadID).Take(&stored).Error; err != nil || stored.State != "rejected" {
		t.Fatalf("policy termination was rolled back: %s %v", stored.State, err)
	}
}

func TestKnowledgeOriginalIsolationAndC1Rejection(t *testing.T) {
	s, reg := setupOriginalTest(t)
	if _, _, err := s.CreateKnowledgeOriginalUpload(t.Context(), knowledgeTestOther, reg.DomainID, reg.RegistrationID, originalTestRequest("foreign")); err == nil {
		t.Fatal("foreign owner could reserve original capacity")
	}
	registerKnowledgeTest(t, s, knowledgeTestOther)
	_, err := s.GetKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, knowledgeTestOther, reg.RegistrationID, "")
	assertOriginalCode(t, err, 404, "knowledge_not_found")
	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", reg.DomainID).Take(&kb).Error; err != nil {
		t.Fatal(err)
	}
	docID, revID := strings.Repeat("c", 32), strings.Repeat("d", 32)
	name, location, valid := "original.pdf", "synthetic-unreadable", "1"
	if err := dao.DB.Create(&entity.Document{ID: docID, KbID: reg.DomainID, Name: &name, Location: &location, Status: &valid, Size: 64 << 20, ParserConfig: entity.JSONMap{}}).Error; err != nil {
		t.Fatal(err)
	}
	original := entity.KnowledgeSourceRevision{ID: revID, DomainID: reg.DomainID, DocumentID: docID, FileID: strings.Repeat("e", 32), FolderID: *kb.KnowledgeFolderID,
		RepresentationKind: "original_only", OriginalRegistrationID: reg.RegistrationID, RelativePath: "docs/sample.pdf", SHA256: strings.Repeat("a", 64), InputHash: strings.Repeat("b", 64), ByteLength: 64 << 20, IsCurrent: true}
	if err := dao.DB.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	page, err := s.ListKnowledgeSources(t.Context(), knowledgeTestOwner, reg.DomainID, "", "", 20)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("C1 listed original-only metadata", err)
	}
	_, err = s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, reg.DomainID, revID)
	assertOriginalCode(t, err, 415, "knowledge_source_unsupported")
	_, err = s.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, reg.DomainID, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID})
	assertOriginalCode(t, err, 415, "knowledge_source_unsupported")
	_, err = knowledgeSourceBytes(t.Context(), &original)
	assertOriginalCode(t, err, 415, "knowledge_source_unsupported")
}

func TestKnowledgeOriginalDigestVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/knowledge-original-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Vectors []struct {
			Request       json.RawMessage                `json:"request"`
			Preview       entity.KnowledgeOriginalUpload `json:"preview"`
			RequestDigest struct {
				SHA256 string `json:"sha256"`
			} `json:"request_digest"`
			PreviewDigest struct {
				SHA256 string `json:"sha256"`
			} `json:"preview_digest"`
		} `json:"digest_vectors"`
	}
	if err := json.Unmarshal(raw, &examples); err != nil || len(examples.Vectors) == 0 {
		t.Fatal("missing frozen vectors", err)
	}
	for _, vector := range examples.Vectors {
		var req struct {
			entity.KnowledgeOriginalUploadRequest
			DomainID       string `json:"domain_id"`
			OwnerID        string `json:"owner_id"`
			RegistrationID string `json:"registration_id"`
		}
		if err := json.Unmarshal(vector.Request, &req); err != nil {
			t.Fatal(err)
		}
		if got := originalUploadDigest(req.DomainID, req.OwnerID, req.RegistrationID, req.KnowledgeOriginalUploadRequest); got != vector.RequestDigest.SHA256 {
			t.Fatalf("request digest=%s want=%s", got, vector.RequestDigest.SHA256)
		}
		if got := originalPreviewDigest(&vector.Preview); got != vector.PreviewDigest.SHA256 {
			t.Fatalf("preview digest=%s want=%s", got, vector.PreviewDigest.SHA256)
		}
	}
}
