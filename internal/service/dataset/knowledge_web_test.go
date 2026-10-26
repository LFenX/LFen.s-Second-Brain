package dataset

import (
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupWebTest(t *testing.T) (*DatasetService, *entity.KnowledgeWebRegistration) {
	t.Helper()
	s, _, _ := setupKnowledgeImportTest(t)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeWebRegistrationRecord{}, &entity.KnowledgeWebCaptureRecord{}, &entity.KnowledgeWebSlot{}, &entity.KnowledgeWebObservationRecord{}, &entity.KnowledgeWebEntryRecord{}, &entity.KnowledgeWebPreviewRecord{}, &entity.KnowledgeWebReceiptRecord{}, &entity.KnowledgeWebRevisionRecord{}, &entity.KnowledgeWebBinding{}, &entity.KnowledgeDirectoryState{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2} {
		if err := dao.DB.Create(&entity.KnowledgeWebSlot{ID: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := entity.KnowledgeWebCreateRegistration{RegistrationKey: "web:synthetic", Name: "合成Feed", SourceKind: "web_feed", OriginURL: "https://feed.example.test/feed", Producer: "rsshub", DefaultClassification: entity.KnowledgeDirectoryClassification{ComponentSide: "frontend", Language: "typescript"}, NetworkScope: entity.KnowledgeWebNetworkScope{Enabled: true, AllowedOrigins: []string{"https://feed.example.test"}}}
	reg, created, err := s.CreateKnowledgeWebRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatalf("reg=%+v new=%t err=%v", reg, created, err)
	}
	return s, reg
}

func webLocalTestRequest(reg *entity.KnowledgeWebRegistration, key string) entity.KnowledgeWebCreateLocalCapture {
	return entity.KnowledgeWebCreateLocalCapture{RegistrationConfigSHA256: reg.ConfigSHA256, IdempotencyKey: key, AcquisitionMode: "local_snapshot", OriginalFilename: "synthetic.xml", DeclaredByteLength: 128, DeclaredMediaType: "application/rss+xml"}
}

func TestKnowledgeWebRegistrationIsNativeImmutableAndSeparated(t *testing.T) {
	s, reg := setupWebTest(t)
	byKey, err := s.GetKnowledgeWebRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, "", reg.RegistrationKey)
	if err != nil || byKey.RegistrationID != reg.RegistrationID || byKey.ConfigSHA256 != reg.ConfigSHA256 {
		t.Fatal("registration recovery changed", err)
	}
	var row entity.KnowledgeWebRegistrationRecord
	if err := dao.DB.Where("id = ?", reg.RegistrationID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var folder entity.File
	if err := dao.DB.Where("id = ? AND type = ?", row.FolderID, "folder").Take(&folder).Error; err != nil || folder.KnowledgePrivateOwnerID == nil || *folder.KnowledgePrivateOwnerID != knowledgeTestOwner {
		t.Fatal("native folder not protected", err)
	}
	req := entity.KnowledgeWebCreateRegistration{RegistrationKey: reg.RegistrationKey, Name: reg.Name, SourceKind: reg.SourceKind, OriginURL: "https://FEED.example.test:443/feed", Producer: reg.Producer, DefaultClassification: reg.DefaultClassification, NetworkScope: reg.NetworkScope}
	replay, created, err := s.CreateKnowledgeWebRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, req)
	if err != nil || created || replay.RegistrationID != reg.RegistrationID {
		t.Fatal("normalized config replay changed", err)
	}
	req.Producer = "direct"
	_, _, err = s.CreateKnowledgeWebRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, req)
	assertOriginalCode(t, err, 409, "registration_conflict")
	if _, err := s.GetKnowledgeRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID); err == nil {
		t.Fatal("old0.2 accepted web scope")
	}
	if _, err := s.GetKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ""); err == nil {
		t.Fatal("old0.5 accepted web scope")
	}
	if _, err := s.GetKnowledgeWebRegistration(t.Context(), "other", reg.DomainID, reg.RegistrationID, ""); err == nil {
		t.Fatal("cross owner read succeeded")
	}
}

func TestKnowledgeWebCaptureCreationDoesNotAcquireAndRecovers(t *testing.T) {
	s, reg := setupWebTest(t)
	req := webLocalTestRequest(reg, "cap:Case")
	view, created, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil || !created || view.State != "awaiting_bytes" || view.Revision != 1 || view.RawSHA256 != nil || view.Provenance.ServerReceivedAt != nil {
		t.Fatalf("capture=%+v new=%t err=%v", view, created, err)
	}
	replay, created, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil || created || replay.CaptureID != view.CaptureID || replay.RequestSHA256 != view.RequestSHA256 {
		t.Fatal("capture original key changed", err)
	}
	byKey, err := s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, "", req.IdempotencyKey)
	if err != nil || byKey.CaptureID != view.CaptureID {
		t.Fatal("capture lookup failed", err)
	}
	req.OriginalFilename = "different.xml"
	_, _, err = s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	assertOriginalCode(t, err, 409, "idempotency_conflict")
	fetch := entity.KnowledgeWebCreateFetchCapture{RegistrationConfigSHA256: reg.ConfigSHA256, IdempotencyKey: "fetch:001", AcquisitionMode: "explicit_fetch"}
	online, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, fetch)
	if err != nil || online.State != "awaiting_fetch" || online.AttemptStartedAt != nil || online.RawSHA256 != nil {
		t.Fatal("creation silently acquired", err)
	}
	for _, model := range []any{&entity.Document{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeWebObservationRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("creation published/parsed source", err)
		}
	}
}

func TestKnowledgeWebCaptureQuotaCancelExpiryAndPolicy(t *testing.T) {
	s, reg := setupWebTest(t)
	create := func(key string) *entity.KnowledgeWebCapture {
		t.Helper()
		view, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, webLocalTestRequest(reg, key))
		if err != nil {
			t.Fatal(err)
		}
		return view
	}
	first, second := create("one"), create("two")
	_, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, webLocalTestRequest(reg, "three"))
	assertOriginalCode(t, err, 429, "web_capture_quota_exceeded")
	cancelled, err := s.CancelKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, first.CaptureID)
	if err != nil || cancelled.State != "cancelled" || cancelled.Revision <= first.Revision {
		t.Fatal("cancel did not fence", err)
	}
	third := create("three")
	if err := dao.DB.Model(&entity.KnowledgeWebCaptureRecord{}).Where("id = ?", second.CaptureID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, second.CaptureID, "")
	if err != nil || expired.State != "expired" || expired.Failure == nil || expired.Failure.MachineCode != "web_capture_expired" {
		t.Fatal("expired identity lost", err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", reg.DomainID).Update("knowledge_policy_version", 2).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, third.CaptureID, "")
	assertOriginalCode(t, err, 409, "policy_changed")
}

func TestKnowledgeWebLongExactKeysAndLocalDeclaration(t *testing.T) {
	s, reg := setupWebTest(t)
	for _, key := range []string{strings.Repeat("A", 128), strings.Repeat("a", 128)} {
		view, created, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, webLocalTestRequest(reg, key))
		if err != nil || !created || view.IdempotencyKey != key {
			t.Fatal("128 byte exact key changed", err)
		}
	}
	for _, mutate := range []func(*entity.KnowledgeWebCreateLocalCapture){func(r *entity.KnowledgeWebCreateLocalCapture) { r.DeclaredByteLength = 0 }, func(r *entity.KnowledgeWebCreateLocalCapture) { r.ExpectedSHA256 = directoryString("invalid") }, func(r *entity.KnowledgeWebCreateLocalCapture) {
		r.DeclaredFinalURL = directoryString("https://secret.example.test/?token=x")
	}, func(r *entity.KnowledgeWebCreateLocalCapture) { r.ClaimedCapturedAt = directoryString("invalid") }} {
		request := webLocalTestRequest(reg, "invalid")
		mutate(&request)
		_, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
		assertOriginalCode(t, err, 400, "knowledge_invalid_request")
	}
}
