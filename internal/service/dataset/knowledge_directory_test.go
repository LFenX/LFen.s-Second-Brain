package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"strings"
	"testing"
	"time"
)

type directoryFixture struct {
	service      *DatasetService
	root, config string
	reg          *entity.KnowledgeDirectoryRegistration
	cfg          directoryRootConfig
}

func setupDirectoryTest(t *testing.T) *directoryFixture {
	t.Helper()
	if !utility.KnowledgeDirectoryPlatformVerified() {
		t.Skip("platform is intentionally unverified")
	}
	s, _, _ := setupKnowledgeImportTest(t)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeDirectoryRegistrationBinding{}, &entity.KnowledgeDirectoryRootPin{}, &entity.KnowledgeDirectoryState{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "web"), 0700); err != nil {
		t.Fatal(err)
	}
	f := &directoryFixture{service: s, root: root, config: filepath.Join(t.TempDir(), "roots.json")}
	f.cfg = directoryRootConfig{RootID: strings.Repeat("d", 32), DomainID: knowledgeTestDomain, UserID: knowledgeTestOwner, DisplayName: "Synthetic root", AbsolutePath: root, RootVersion: 1}
	f.writeConfig(t, []directoryRootConfig{f.cfg})
	t.Setenv("LFEN_KNOWLEDGE_ROOTS_FILE", f.config)
	f.write(t, "web/card.tsx", "\ufeff// 合成\r\nexport const label = '你好 🧠';\r\n")
	req := entity.KnowledgeDirectoryRegistrationRequest{RegistrationKey: "directory", Name: "Synthetic directory", SourceKind: "controlled_directory", InputMode: "server_read", RootID: f.cfg.RootID, ScopeDirectories: []string{"web"}, DefaultClassification: entity.KnowledgeDirectoryClassification{ComponentSide: "frontend", Language: "tsx"}}
	reg, created, err := s.CreateKnowledgeDirectoryRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatalf("registration=%+v created=%v err=%v", reg, created, err)
	}
	f.reg = reg
	return f
}
func (f *directoryFixture) write(t *testing.T, relative, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, filepath.FromSlash(relative)), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f *directoryFixture) writeConfig(t *testing.T, roots []directoryRootConfig) {
	t.Helper()
	if err := os.WriteFile(f.config, []byte(directoryJSON(map[string]any{"roots": roots})), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f *directoryFixture) observe(t *testing.T, paths ...string) *entity.KnowledgeDirectoryObservation {
	t.Helper()
	o, err := f.service.CreateKnowledgeDirectoryObservation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryObservationRequest{SelectedPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func (f *directoryFixture) preview(t *testing.T, o *entity.KnowledgeDirectoryObservation, c entity.KnowledgeDirectoryClassification) *entity.KnowledgeDirectoryPreview {
	t.Helper()
	p, err := f.service.CreateKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryPreviewRequest{ObservationID: o.ObservationID, ObservationSHA256: o.ObservationSHA256, RelativePath: "web/card.tsx", Classification: c})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *directoryFixture) commit(t *testing.T, p *entity.KnowledgeDirectoryPreview, key string) *entity.KnowledgeDirectoryReceipt {
	t.Helper()
	r, created, err := f.service.CommitKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryCommitRequest{PreviewID: p.PreviewID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: key})
	if err != nil || !created {
		t.Fatalf("receipt=%+v created=%v err=%v", r, created, err)
	}
	return r
}
func assertDirectoryError(t *testing.T, err error, status int, reason string) {
	t.Helper()
	var p *KnowledgeDirectoryError
	if !errors.As(err, &p) || p.Status != status || (reason != "" && (p.Reason == nil || *p.Reason != reason)) {
		t.Fatalf("expected %d/%s got %v", status, reason, err)
	}
}

func TestKnowledgeDirectoryRoundTripAndFixedRevision(t *testing.T) {
	f := setupDirectoryTest(t)
	c := f.reg.DefaultClassification
	o := f.observe(t, "web/card.tsx")
	if !o.Complete || !o.Items[0].Eligible {
		t.Fatal("observation not verified")
	}
	p := f.preview(t, o, c)
	window, err := f.service.ReadKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, p.PreviewID, 0, 65536)
	if err != nil || !strings.Contains(window.Text, "🧠") {
		t.Fatalf("window=%+v err=%v", window, err)
	}
	first := f.commit(t, p, "first")
	i := first.Items[0]
	if first.Status != "succeeded" || first.ModelCalls != 0 || len(*i.RootNodeID) != 64 || i.SaveStatus != "created" || i.VectorStatus != "disabled" {
		t.Fatalf("%+v", first)
	}
	if _, err = f.service.ReadKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, p.PreviewID, 0, 1); err == nil {
		t.Fatal("consumed content readable")
	} else {
		assertDirectoryError(t, err, 409, "preview_consumed")
	}
	old, err := f.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *i.RootNodeID, nil, nil)
	if err != nil || old.Text != window.Text {
		t.Fatal("C1 original byte mismatch", err)
	}
	same := f.commit(t, f.preview(t, f.observe(t, "web/card.tsx"), c), "same")
	if same.Items[0].SaveStatus != "unchanged" || *same.Items[0].SourceRevisionID != *i.SourceRevisionID {
		t.Fatal("unchanged republished")
	}
	f.write(t, "web/card.tsx", "// 合成新版本\nexport const second = 2;\n")
	next := f.commit(t, f.preview(t, f.observe(t, "web/card.tsx"), c), "next")
	if *next.Items[0].SourceID != *i.SourceID || *next.Items[0].SourceRevisionID == *i.SourceRevisionID {
		t.Fatal("native identity mismatch")
	}
	c.ComponentSide = "shared"
	changed := f.commit(t, f.preview(t, f.observe(t, "web/card.tsx"), c), "classification")
	if changed.Items[0].SaveStatus != "updated" {
		t.Fatal("classification-only not updated")
	}
	old2, err := f.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *i.RootNodeID, nil, nil)
	if err != nil || old2.Text != old.Text {
		t.Fatal("fixed revision changed", err)
	}
}
func TestKnowledgeDirectoryObservationEligibilityAndMissing(t *testing.T) {
	f := setupDirectoryTest(t)
	f.write(t, "web/sample.pdf", "%PDF synthetic\x00")
	f.write(t, "web/invalid.ts", "\xff\xfe")
	f.write(t, "web/zero.ts", "a\x00b")
	o := f.observe(t, "web/sample.pdf", "web/card.tsx", "web/invalid.ts", "web/zero.ts")
	if !o.Complete || len(o.Items) != 4 {
		t.Fatal("complete and eligible conflated")
	}
	eligible := 0
	for _, i := range o.Items {
		if i.Eligible {
			eligible++
		} else if i.SHA256 != nil || i.SourceID != nil {
			t.Fatal("ineligible hash/source exposed")
		}
	}
	if eligible != 1 {
		t.Fatal("ineligible accepted")
	}
	p := f.preview(t, o, f.reg.DefaultClassification)
	f.commit(t, p, "existing")
	if err := os.Remove(filepath.Join(f.root, "web/card.tsx")); err != nil {
		t.Fatal(err)
	}
	missing := f.observe(t, "web/card.tsx")
	if missing.Complete || missing.Items[0].ChangeKind != "missing_proposal" || missing.Items[0].SourceID == nil {
		t.Fatalf("missing=%+v", missing)
	}
	_, err := f.service.CreateKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryPreviewRequest{ObservationID: missing.ObservationID, ObservationSHA256: missing.ObservationSHA256, RelativePath: "web/card.tsx", Classification: f.reg.DefaultClassification})
	assertDirectoryError(t, err, 409, "observation_incomplete")
	var documents int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", knowledgeTestDomain).Count(&documents)
	if documents != 1 {
		t.Fatal("missing deleted source")
	}
}
func TestKnowledgeDirectoryRecoveryPermissionAndOffline(t *testing.T) {
	f := setupDirectoryTest(t)
	p := f.preview(t, f.observe(t, "web/card.tsx"), f.reg.DefaultClassification)
	r := f.commit(t, p, "original")
	req := entity.KnowledgeDirectoryCommitRequest{PreviewID: p.PreviewID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: "original"}
	dao.DB.Model(&entity.KnowledgeImportPreview{}).Where("id = ?", p.PreviewID).Update("expires_at", time.Now().Add(-48*time.Hour))
	if err := os.Rename(f.root, f.root+"-offline"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(f.root+"-offline", f.root) })
	replay, created, err := f.service.CommitKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, req)
	if err != nil || created || replay.ImportTaskID != r.ImportTaskID {
		t.Fatal("original replay depends on current source", err)
	}
	byKey, err := f.service.GetKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, "", "original")
	if err != nil || byKey.ImportTaskID != r.ImportTaskID {
		t.Fatal("durable by-key failed", err)
	}
	_, err = f.service.GetKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, "", "absent")
	assertDirectoryError(t, err, 404, "")
	f.writeConfig(t, []directoryRootConfig{})
	_, err = f.service.GetKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, "", "original")
	assertDirectoryError(t, err, 404, "")
	old, err := f.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *r.Items[0].RootNodeID, nil, nil)
	if err != nil || old.Text == "" {
		t.Fatal("root revocation changed C1 domain permission", err)
	}
}
func TestKnowledgeDirectoryStaleBytesHeadVersionAndIdempotency(t *testing.T) {
	f := setupDirectoryTest(t)
	o := f.observe(t, "web/card.tsx")
	p := f.preview(t, o, f.reg.DefaultClassification)
	other := f.preview(t, o, f.reg.DefaultClassification)
	f.commit(t, p, "first")
	_, _, err := f.service.CommitKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryCommitRequest{PreviewID: other.PreviewID, ManifestSHA256: other.ManifestSHA256, IdempotencyKey: "racer"})
	assertDirectoryError(t, err, 409, "revision_changed")
	_, _, err = f.service.CommitKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryCommitRequest{PreviewID: other.PreviewID, ManifestSHA256: other.ManifestSHA256, IdempotencyKey: "first"})
	assertDirectoryError(t, err, 409, "idempotency_conflict")
	now := f.observe(t, "web/card.tsx")
	pending := f.preview(t, now, f.reg.DefaultClassification)
	f.write(t, "web/card.tsx", "// changed synthetic bytes\n")
	_, _, err = f.service.CommitKnowledgeDirectoryImport(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryCommitRequest{PreviewID: pending.PreviewID, ManifestSHA256: pending.ManifestSHA256, IdempotencyKey: "changed"})
	assertDirectoryError(t, err, 409, "source_changed")
	f.cfg.RootVersion++
	f.writeConfig(t, []directoryRootConfig{f.cfg})
	_, err = f.service.GetKnowledgeDirectoryObservation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, now.ObservationID)
	assertDirectoryError(t, err, 409, "root_changed")
}
func TestKnowledgeDirectoryFailedReceiptAndManualIsolation(t *testing.T) {
	f := setupDirectoryTest(t)
	p := f.preview(t, f.observe(t, "web/card.tsx"), f.reg.DefaultClassification)
	dao.DB.Model(&entity.KnowledgeImportPreview{}).Where("id = ?", p.PreviewID).Update("content", "synthetic staged fault")
	r := f.commit(t, p, "failed")
	if r.Status != "failed" || r.Items[0].SourceID != nil || r.Items[0].SourceRevisionID != nil || r.Items[0].RootNodeID != nil {
		t.Fatalf("failed receipt points at source: %+v", r)
	}
	var count int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", knowledgeTestDomain).Count(&count)
	if count != 0 {
		t.Fatal("failed publication moved head")
	}
	_, err := f.service.GetKnowledgeRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID)
	assertKnowledgeStatus(t, err, 404)
}
func TestKnowledgeDirectoryFixedListingAndLimits(t *testing.T) {
	f := setupDirectoryTest(t)
	for i := 0; i < 54; i++ {
		f.write(t, fmt.Sprintf("web/a%03d.ts", i), "// synthetic\n")
	}
	f.write(t, "web/.env.local", "synthetic excluded marker")
	listing, err := f.service.CreateKnowledgeDirectoryListing(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryListingRequest{RelativeDirectory: "web", NamePrefix: ""})
	if err != nil || len(listing.Items) != 50 || listing.NextCursor == nil {
		t.Fatalf("listing=%+v err=%v", listing, err)
	}
	f.write(t, "web/aaa-new.ts", "// not in frozen listing\n")
	next, err := f.service.GetKnowledgeDirectoryListing(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, listing.ListingID, *listing.NextCursor)
	if err != nil || next.TotalEntries != 55 || len(next.Items) != 5 || next.NextCursor != nil {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	for _, i := range append(listing.Items, next.Items...) {
		if strings.Contains(i.RelativePath, ".env") || strings.Contains(i.RelativePath, "aaa-new") {
			t.Fatal("listing changed or excluded name exposed")
		}
	}
	paths := make([]string, 33)
	for i := range paths {
		paths[i] = fmt.Sprintf("web/a%03d.ts", i)
	}
	_, err = f.service.CreateKnowledgeDirectoryObservation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryObservationRequest{SelectedPaths: paths})
	assertDirectoryError(t, err, 400, "")
	for i := 0; i < 4; i++ {
		f.observe(t, "web/card.tsx")
	}
	_, err = f.service.CreateKnowledgeDirectoryObservation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryObservationRequest{SelectedPaths: []string{"web/card.tsx"}})
	assertDirectoryError(t, err, 429, "")
}
func TestKnowledgeDirectoryFrozenDigestVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/knowledge-directory-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Cases []struct {
			ID   string          `json:"id"`
			Data json.RawMessage `json:"data"`
		} `json:"schema_cases"`
	}
	if err = json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	for _, c := range examples.Cases {
		switch c.ID {
		case "valid-Observation":
			var view entity.KnowledgeDirectoryObservation
			if err = json.Unmarshal(c.Data, &view); err != nil {
				t.Fatal(err)
			}
			if directoryObservationDigest(&view) != view.ObservationSHA256 {
				t.Fatal("observation vector mismatch")
			}
		case "valid-Preview":
			var view entity.KnowledgeDirectoryPreview
			if err = json.Unmarshal(c.Data, &view); err != nil {
				t.Fatal(err)
			}
			if directoryPreviewDigest(&view) != view.ManifestSHA256 {
				t.Fatal("preview vector mismatch")
			}
		}
	}
}

func TestKnowledgeDirectoryExpiryCleanupAndRootConfig(t *testing.T) {
	f := setupDirectoryTest(t)
	o := f.observe(t, "web/card.tsx")
	p := f.preview(t, o, f.reg.DefaultClassification)
	dao.DB.Model(&entity.KnowledgeImportPreview{}).Where("id = ?", p.PreviewID).Update("expires_at", time.Now().Add(-time.Minute))
	dao.DB.Model(&entity.KnowledgeDirectoryState{}).Where("id = ?", p.PreviewID).Update("expires_at", time.Now().Add(-time.Minute))
	_, err := f.service.ReadKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, p.PreviewID, 0, 10)
	assertDirectoryError(t, err, 410, "")
	if err = CleanupKnowledgeDirectoryStates(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var native entity.KnowledgeImportPreview
	dao.DB.Where("id = ?", p.PreviewID).Take(&native)
	if native.Content != "" {
		t.Fatal("expired source body retained")
	}
	f.cfg.ExtraExclusions = []string{"web/private"}
	f.writeConfig(t, []directoryRootConfig{f.cfg})
	_, err = f.service.GetKnowledgeDirectoryRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID)
	assertDirectoryError(t, err, 503, "root_unready")
	f.cfg.RootVersion++
	f.writeConfig(t, []directoryRootConfig{f.cfg})
	if _, err = f.service.GetKnowledgeDirectoryRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID); err != nil {
		t.Fatal(err)
	}
	f.cfg.RootVersion--
	f.writeConfig(t, []directoryRootConfig{f.cfg})
	_, err = f.service.GetKnowledgeDirectoryRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID)
	assertDirectoryError(t, err, 503, "root_unready")
}
func TestKnowledgeDirectoryBoundariesAndRootIdentity(t *testing.T) {
	f := setupDirectoryTest(t)
	f.write(t, "web/large.ts", strings.Repeat("a", 2097152))
	f.write(t, "web/oversize.ts", strings.Repeat("a", 2097153))
	o := f.observe(t, "web/large.ts", "web/oversize.ts")
	if !o.Complete || !o.Items[0].Eligible || o.Items[1].Eligible || *o.Items[1].Reason != "too_large" {
		t.Fatalf("size boundaries %+v", o)
	}
	var count int64
	dao.DB.Model(&entity.Document{}).Where("kb_id = ?", knowledgeTestDomain).Count(&count)
	if count != 0 {
		t.Fatal("observation published")
	}
	for _, path := range []string{"../outside.ts", "web/.env.local", "elsewhere/a.ts", "web/*.ts"} {
		_, err := f.service.CreateKnowledgeDirectoryObservation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeDirectoryObservationRequest{SelectedPaths: []string{path}})
		assertDirectoryError(t, err, 400, "")
	}
	p := f.preview(t, f.observe(t, "web/card.tsx"), f.reg.DefaultClassification)
	_, err := f.service.ReadKnowledgeDirectoryPreview(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, p.PreviewID, 0, 1)
	assertDirectoryError(t, err, 416, "")
	if err = os.Rename(f.root, f.root+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(f.root, 0700); err != nil {
		t.Fatal(err)
	}
	roots, err := f.service.ListKnowledgeDirectoryRoots(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || roots.Items[0].Ready {
		t.Fatal("root ID rebound to a replacement", err)
	}
	_, err = f.service.GetKnowledgeDirectoryImport(t.Context(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", knowledgeTestDomain, f.reg.RegistrationID, "", "absent")
	assertDirectoryError(t, err, 404, "")
}
