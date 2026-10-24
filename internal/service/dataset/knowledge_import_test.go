package dataset

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

func setupKnowledgeImportTest(t *testing.T) (*DatasetService, storage.Storage, *entity.KnowledgeSourceRegistration) {
	t.Helper()
	s, store := setupKnowledgeTest(t)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeImportPreview{}, &entity.KnowledgeImport{}); err != nil {
		t.Fatal(err)
	}
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	req := entity.CreateKnowledgeRegistrationRequest{RegistrationKey: "synthetic", Name: "Synthetic import", SourceKind: "text_file", InputMode: "inline_text", IncludePaths: []string{"web/card.tsx", "backend/answer.go", "docs/empty.md", "docs/large.txt"}, ComponentSide: "frontend", Language: "tsx"}
	reg, created, err := s.CreateKnowledgeRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatalf("registration=%+v created=%t err=%v", reg, created, err)
	}
	return s, store, reg
}

func previewKnowledgeTest(t *testing.T, s *DatasetService, reg *entity.KnowledgeSourceRegistration, text string) *entity.KnowledgeImportPreviewView {
	t.Helper()
	p, err := s.PreviewKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.PreviewKnowledgeImportRequest{RelativePath: "web/card.tsx", Text: text, ComponentSide: "frontend", Language: "tsx"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func commitKnowledgeTest(t *testing.T, s *DatasetService, reg *entity.KnowledgeSourceRegistration, p *entity.KnowledgeImportPreviewView, key string) *entity.KnowledgeImportReceipt {
	t.Helper()
	r, created, err := s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: key})
	if err != nil || !created || r.Status != "succeeded" {
		t.Fatalf("receipt=%+v created=%t err=%v", r, created, err)
	}
	return r
}

func TestKnowledgeImportRoundTripAndIdentity(t *testing.T) {
	s, _, reg := setupKnowledgeImportTest(t)
	text := "// 合成\r\nexport const label = '你好 🧠';\r\n"
	p := previewKnowledgeTest(t, s, reg, text)
	var documents int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", reg.DomainID).Count(&documents)
	if documents != 0 {
		t.Fatal("preview published a document")
	}
	first := commitKnowledgeTest(t, s, reg, p, "first")
	if len(first.Items) != 1 || first.Items[0].SaveStatus != "saved" || first.Items[0].VectorStatus != "disabled" || first.Items[0].CatalogueStatus != "ready" || first.ModelCalls != 0 {
		t.Fatalf("receipt=%+v", first)
	}
	content, err := s.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, reg.DomainID, *first.Items[0].RootNodeID, nil, nil)
	if err != nil || content.Text != text {
		t.Fatalf("content=%+v err=%v", content, err)
	}
	read, err := s.GetKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, first.ID)
	if err != nil || read.ID != first.ID {
		t.Fatalf("read=%+v %v", read, err)
	}
	unchanged := commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, text), "same")
	if unchanged.Items[0].SaveStatus != "unchanged" || *unchanged.Items[0].SourceRevisionID != *first.Items[0].SourceRevisionID {
		t.Fatal("unchanged created a revision")
	}
	updated := commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, text+"// 修订二号\n"), "update")
	if *updated.Items[0].SourceID != *first.Items[0].SourceID || *updated.Items[0].SourceRevisionID == *first.Items[0].SourceRevisionID {
		t.Fatal("update changed source identity or lost revision")
	}
	old, err := s.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, reg.DomainID, *first.Items[0].RootNodeID, nil, nil)
	if err != nil || old.Text != text {
		t.Fatalf("old=%+v err=%v", old, err)
	}
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", reg.DomainID).Count(&documents)
	if documents != 1 {
		t.Fatalf("documents=%d", documents)
	}
	var revisions, commits int64
	dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&revisions)
	dao.DB.Model(&entity.FileCommit{}).Count(&commits)
	if revisions != 2 || commits != 2 {
		t.Fatalf("revisions=%d commits=%d", revisions, commits)
	}
	var kb entity.Knowledgebase
	dao.DB.Where("id = ?", reg.DomainID).Take(&kb)
	if kb.DocNum != 1 || kb.ChunkNum != 0 || kb.TokenNum != 0 {
		t.Fatalf("counters=%+v", kb)
	}
}

func TestKnowledgeImportRegistrationReplayAndClassification(t *testing.T) {
	s, _, reg := setupKnowledgeImportTest(t)
	req := entity.CreateKnowledgeRegistrationRequest{RegistrationKey: reg.RegistrationKey, Name: reg.Name, SourceKind: reg.SourceKind, InputMode: reg.InputMode, IncludePaths: reg.IncludePaths, GroupID: reg.GroupID, ComponentSide: reg.ComponentSide, Language: reg.Language}
	again, created, err := s.CreateKnowledgeRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, req)
	if err != nil || created || again.ID != reg.ID {
		t.Fatalf("reg=%+v %t %v", again, created, err)
	}
	req.Name = "different"
	_, _, err = s.CreateKnowledgeRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, req)
	assertKnowledgeStatus(t, err, 409)
	for i, tc := range []struct{ path, side, language, text string }{{"backend/answer.go", "backend", "go", "package synthetic\n"}, {"docs/empty.md", "non_code", "markdown", ""}, {"docs/large.txt", "non_code", "text", strings.Repeat("a", 65535) + "中\n"}} {
		p, err := s.PreviewKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.PreviewKnowledgeImportRequest{RelativePath: tc.path, ComponentSide: tc.side, Language: tc.language, Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		r := commitKnowledgeTest(t, s, reg, p, tc.side+strings.Repeat("x", i))
		if r.Items[0].ComponentSide != tc.side || r.Items[0].Language != tc.language {
			t.Fatalf("classification=%+v", r.Items[0])
		}
	}
}

func TestKnowledgeImportCommitReplayExpiryAndCAS(t *testing.T) {
	s, _, reg := setupKnowledgeImportTest(t)
	p := previewKnowledgeTest(t, s, reg, "first\n")
	first := commitKnowledgeTest(t, s, reg, p, "same-key")
	dao.DB.Model(&entity.KnowledgeImportPreview{}).Where("id = ?", p.ID).Update("expires_at", time.Now().Add(-time.Hour))
	req := entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "same-key"}
	again, created, err := s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, req)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("replay=%+v %t %v", again, created, err)
	}
	req.IdempotencyKey = "different-key"
	_, _, err = s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, req)
	assertKnowledgeStatus(t, err, 409)
	stale := previewKnowledgeTest(t, s, reg, "stale\n")
	fresh := previewKnowledgeTest(t, s, reg, "new\n")
	commitKnowledgeTest(t, s, reg, fresh, "new-revision")
	req = entity.CommitKnowledgeImportRequest{PreviewID: stale.ID, ManifestSHA256: stale.ManifestSHA256, IdempotencyKey: "stale"}
	_, _, err = s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, req)
	assertKnowledgeStatus(t, err, 409)
	req.IdempotencyKey = "same-key"
	_, _, err = s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, req)
	assertKnowledgeStatus(t, err, 409)
	for _, change := range []string{"expires_at", "knowledge_policy_version", "manifest_sha256"} {
		t.Run(change, func(t *testing.T) {
			p := previewKnowledgeTest(t, s, reg, "blocked\n")
			req := entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: change}
			if change == "knowledge_policy_version" {
				dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", reg.DomainID).Update(change, p.PolicyVersion+1)
			} else {
				value := any(time.Now().Add(-time.Minute))
				if change == "manifest_sha256" {
					value = strings.Repeat("a", 64)
				}
				dao.DB.Model(&entity.KnowledgeImportPreview{}).Where("id = ?", p.ID).Update(change, value)
			}
			_, _, err := s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, req)
			assertKnowledgeStatus(t, err, 409)
		})
	}
}

type knowledgeImportFailingStorage struct{ storage.Storage }

func (s knowledgeImportFailingStorage) Put(ctx context.Context, bucket, key string, b []byte, metadata ...string) error {
	if strings.HasPrefix(key, ".objects/") {
		return errors.New("synthetic snapshot storage failure")
	}
	return s.Storage.Put(ctx, bucket, key, b, metadata...)
}

func TestKnowledgeImportNewSourceFailureDoesNotPublish(t *testing.T) {
	s, store, reg := setupKnowledgeImportTest(t)
	p := previewKnowledgeTest(t, s, reg, "new synthetic bytes\n")
	storage.GetStorageFactory().SetStorage(knowledgeImportFailingStorage{store})
	r, created, err := s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "new-failure"})
	if err != nil || !created || r.Status != "failed" {
		t.Fatalf("receipt=%+v created=%t err=%v", r, created, err)
	}
	for _, model := range []any{&entity.Document{}, &entity.File2Document{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeSourceRevision{}, &entity.FileCommit{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial publication %T count=%d err=%v", model, count, err)
		}
	}
	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", reg.DomainID).Take(&kb).Error; err != nil || kb.DocNum != 0 {
		t.Fatalf("counter rollback: %+v %v", kb, err)
	}
	storage.GetStorageFactory().SetStorage(store)
	commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, "new synthetic bytes\n"), "new-retry")
}

func TestKnowledgeImportFailureReceiptAndRetry(t *testing.T) {
	s, store, reg := setupKnowledgeImportTest(t)
	first := commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, "old bytes\n"), "first")
	p := previewKnowledgeTest(t, s, reg, "updated bytes\n")
	storage.GetStorageFactory().SetStorage(knowledgeImportFailingStorage{store})
	failed, created, err := s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "failure"})
	if err != nil || !created || failed.Status != "failed" || failed.Items[0].SourceID != nil {
		t.Fatalf("failure=%+v created=%t err=%v", failed, created, err)
	}
	storage.GetStorageFactory().SetStorage(store)
	current, err := knowledgeImportCurrent(dao.DB, reg.DomainID, *first.Items[0].SourceID)
	if err != nil || current.ID != *first.Items[0].SourceRevisionID {
		t.Fatal("failed publish changed current revision")
	}
	old, err := s.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, reg.DomainID, *first.Items[0].RootNodeID, nil, nil)
	if err != nil || old.Text != "old bytes\n" {
		t.Fatal("failed import changed published source")
	}
	read, err := s.GetKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, failed.ID)
	if err != nil || read.Status != "failed" {
		t.Fatal("failed receipt not persisted")
	}
	retry := commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, "updated bytes\n"), "retry")
	if *retry.Items[0].SourceID != *first.Items[0].SourceID {
		t.Fatal("retry changed source identity")
	}
	var n int64
	dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&n)
	if n != 2 {
		t.Fatalf("partial revision leaked: %d", n)
	}
}

func TestKnowledgeImportRejectsExcludedAndUnauthorized(t *testing.T) {
	s, _, reg := setupKnowledgeImportTest(t)
	for _, p := range []string{".env.local", "../escape.ts", "/absolute.txt", "C:/secret.txt", "node_modules/x.js", "web/other.tsx", "docs/a.pem"} {
		t.Run(p, func(t *testing.T) {
			_, err := s.PreviewKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.PreviewKnowledgeImportRequest{RelativePath: p, Text: "synthetic", ComponentSide: "frontend"})
			assertKnowledgeStatus(t, err, 400)
		})
	}
	for _, tc := range []struct {
		text   string
		status int
	}{{"nul\x00", 415}, {strings.Repeat("x", 2*1024*1024+1), 413}, {"-----BEGIN PRIVATE KEY-----\nsynthetic\n", 415}, {string([]byte{255}), 415}} {
		_, err := s.PreviewKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.PreviewKnowledgeImportRequest{RelativePath: "web/card.tsx", Text: tc.text, ComponentSide: "frontend"})
		assertKnowledgeStatus(t, err, tc.status)
	}
	p := previewKnowledgeTest(t, s, reg, "private\n")
	_, err := s.GetKnowledgeRegistration(t.Context(), "intruder", reg.DomainID, reg.ID)
	assertKnowledgeStatus(t, err, 404)
	_, err = s.GetKnowledgeImportPreview(t.Context(), "intruder", reg.DomainID, reg.ID, p.ID)
	assertKnowledgeStatus(t, err, 404)
	_, _, err = s.CommitKnowledgeImport(t.Context(), "intruder", reg.DomainID, reg.ID, entity.CommitKnowledgeImportRequest{PreviewID: p.ID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "denied"})
	assertKnowledgeStatus(t, err, 404)
	_, err = s.GetKnowledgeImportPreview(t.Context(), knowledgeTestOwner, knowledgeTestOther, reg.ID, p.ID)
	assertKnowledgeStatus(t, err, 404)
}
