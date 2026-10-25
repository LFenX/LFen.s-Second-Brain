package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupBatchTest(t *testing.T) *directoryFixture {
	t.Helper()
	f := setupDirectoryTest(t)
	for _, m := range []any{&entity.KnowledgeBatchPreviewRecord{}, &entity.KnowledgeBatchItemRecord{}, &entity.KnowledgeBatchTaskRecord{}, &entity.KnowledgeBatchViewRecord{}, &entity.KnowledgeProjectManifestRecord{}, &entity.KnowledgeProjectManifestHeadRecord{}, &entity.KnowledgeBatchCommandRecord{}} {
		if e := dao.DB.AutoMigrate(m); e != nil {
			t.Fatal(e)
		}
	}
	return f
}
func prepareBatchTest(t *testing.T, f *directoryFixture, paths ...string) *entity.KnowledgeBatchPreview {
	t.Helper()
	slices.Sort(paths)
	obs := f.observe(t, paths...)
	req := entity.KnowledgeBatchPreviewRequest{ObservationID: obs.ObservationID, ObservationSHA256: obs.ObservationSHA256}
	for _, p := range paths {
		req.SelectedItems = append(req.SelectedItems, entity.KnowledgeBatchSelectionItem{RelativePath: p, Classification: f.reg.DefaultClassification})
	}
	p, e := f.service.PrepareKnowledgeBatch(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, req)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func batchRequest(p *entity.KnowledgeBatchPreview, key string) entity.KnowledgeBatchAcceptRequest {
	r := entity.KnowledgeBatchAcceptRequest{PreviewID: p.PreviewID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: key}
	for _, i := range p.Items {
		r.ConfirmedItemIDs = append(r.ConfirmedItemIDs, i.ItemID)
	}
	return r
}
func acceptBatchTest(t *testing.T, f *directoryFixture, p *entity.KnowledgeBatchPreview, key string) *entity.KnowledgeBatchTask {
	t.Helper()
	task, created, e := f.service.AcceptKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, batchRequest(p, key))
	if e != nil || !created {
		t.Fatalf("accept created=%v err=%v", created, e)
	}
	return task
}
func runBatchTest(t *testing.T, f *directoryFixture, id string) *entity.KnowledgeBatchTask {
	t.Helper()
	claim, e := claimKnowledgeBatch(t.Context(), knowledgeTestDomain, id, common.GenerateUUID())
	if e != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, e)
	}
	runKnowledgeBatch(t.Context(), f.service, claim)
	v, e := f.service.GetKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, id, "")
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestKnowledgeBatchRoundTripAndProjectVersions(t *testing.T) {
	f := setupBatchTest(t)
	project, e := f.service.CreateKnowledgeGroup(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "合成项目"})
	if e != nil {
		t.Fatal(e)
	}
	f.reg.DefaultClassification.GroupID = &project.ID
	f.write(t, "web/empty.ts", "")
	p := prepareBatchTest(t, f, "web/card.tsx", "web/empty.ts")
	w, e := f.service.ReadKnowledgeBatchPreviewItem(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, p.PreviewID, p.Items[0].ItemID, 0, 65536)
	if e != nil || !strings.Contains(w.Text, "🧠") {
		t.Fatalf("window %+v %v", w, e)
	}
	task := acceptBatchTest(t, f, p, "batch-first")
	if task.Status != "queued" || task.Counts.Pending != 2 {
		t.Fatal("accept executed instead of queued")
	}
	again, created, e := f.service.AcceptKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, batchRequest(p, "batch-first"))
	if e != nil || created || again.TaskID != task.TaskID {
		t.Fatal("same key did not replay")
	}
	if _, e = f.service.ReadKnowledgeBatchPreviewItem(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, p.PreviewID, p.Items[0].ItemID, 0, 1); e == nil {
		t.Fatal("consumed plaintext readable")
	}
	done := runBatchTest(t, f, task.TaskID)
	if done.Status != "succeeded" || done.Counts.Succeeded != 2 {
		t.Fatalf("%+v", done)
	}
	results, e := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range results.Items {
		if i.SourceID == nil || i.RootNodeID == nil || len(*i.RootNodeID) != 64 {
			t.Fatalf("bad success %+v", i)
		}
		original, e := f.service.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *i.RootNodeID, nil, nil)
		if e != nil || directorySHA([]byte(original.Text)) != i.SHA256 {
			t.Fatal("fixed byte mismatch", e)
		}
	}
	manifest, e := f.service.PrepareKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPreviewRequest{ProjectID: project.ID, TaskID: task.TaskID})
	if e != nil || manifest.TotalEntries != 2 || manifest.Added != 2 {
		t.Fatalf("manifest %+v %v", manifest, e)
	}
	pub := entity.KnowledgeProjectManifestPublicationRequest{ProjectID: project.ID, PreviewID: manifest.PreviewID, ManifestSHA256: manifest.ManifestSHA256, IdempotencyKey: "publish-first"}
	receipt, created, e := f.service.PublishKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, pub)
	if e != nil || !created {
		t.Fatal(e)
	}
	replay, created, e := f.service.PublishKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, pub)
	if e != nil || created || replay.ManifestID != receipt.ManifestID {
		t.Fatal("publish replay failed", e)
	}
	f.write(t, "web/card.tsx", "// 合成更新\nexport const second = 2;")
	p2 := prepareBatchTest(t, f, "web/card.tsx")
	task2 := runBatchTest(t, f, acceptBatchTest(t, f, p2, "batch-second").TaskID)
	next, e := f.service.PrepareKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPreviewRequest{ProjectID: project.ID, TaskID: task2.TaskID, ExpectedManifestID: &receipt.ManifestID})
	if e != nil || next.Replaced != 1 || next.Carried != 1 || next.TotalEntries != 2 {
		t.Fatalf("merge %+v %v", next, e)
	}
	_, _, e = f.service.PublishKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPublicationRequest{ProjectID: project.ID, PreviewID: next.PreviewID, ManifestSHA256: next.ManifestSHA256, IdempotencyKey: "publish-second"})
	if e != nil {
		t.Fatal(e)
	}
	old, e := f.service.GetKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, project.ID, receipt.ManifestID, "")
	if e != nil || old.Items[0].SourceRevisionID != manifest.Items[0].SourceRevisionID {
		t.Fatal("old manifest changed", e)
	}
	var bodies int64
	dao.DB.Model(&entity.KnowledgeBatchItemRecord{}).Where("content <> ?", "").Count(&bodies)
	if bodies != 0 {
		t.Fatal("terminal bodies retained")
	}
}
func TestKnowledgeBatchPartialFailureCancelAndRetry(t *testing.T) {
	f := setupBatchTest(t)
	f.write(t, "web/z.ts", "synthetic before")
	p := prepareBatchTest(t, f, "web/card.tsx", "web/z.ts")
	task := acceptBatchTest(t, f, p, "partial")
	f.write(t, "web/z.ts", "synthetic after")
	done := runBatchTest(t, f, task.TaskID)
	if done.Status != "partial_failed" || done.Counts.Succeeded != 1 || done.Counts.Failed != 1 {
		t.Fatalf("%+v", done)
	}
	results, e := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
	if e != nil {
		t.Fatal(e)
	}
	if results.Items[1].Code != "source_changed" || results.Items[1].SourceID != nil {
		t.Fatal("failed has pointer or wrong reason")
	}
	p2 := prepareBatchTest(t, f, "web/z.ts")
	req := batchRequest(p2, "retry")
	req.RetryOfTaskID = &task.TaskID
	retry, _, e := f.service.AcceptKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, req)
	if e != nil {
		t.Fatal(e)
	}
	if done = runBatchTest(t, f, retry.TaskID); done.Status != "succeeded" {
		t.Fatal("retry not succeeded")
	}
	p3 := prepareBatchTest(t, f, "web/card.tsx", "web/z.ts")
	third := acceptBatchTest(t, f, p3, "cancel")
	claim, e := claimKnowledgeBatch(t.Context(), knowledgeTestDomain, third.TaskID, common.GenerateUUID())
	if e != nil {
		t.Fatal(e)
	}
	id, e := beginKnowledgeBatchItem(t.Context(), f.service, claim)
	if e != nil {
		t.Fatal(e)
	}
	if e = executeKnowledgeBatchItem(t.Context(), f.service, claim, id); e != nil {
		t.Fatal(e)
	}
	cancelled, e := f.service.CancelKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, third.TaskID, entity.KnowledgeBatchCancellationRequest{IdempotencyKey: "cancel-command"})
	if e != nil || cancelled.Status != "cancelled" || cancelled.Counts.Succeeded != 1 || cancelled.Counts.Cancelled != 1 {
		t.Fatalf("cancel %+v %v", cancelled, e)
	}
	if _, e = beginKnowledgeBatchItem(t.Context(), f.service, claim); !errors.Is(e, errBatchLeaseLost) {
		t.Fatal("late worker kept lease", e)
	}
}
func TestKnowledgeBatchLeaseFencingAndDeadline(t *testing.T) {
	f := setupBatchTest(t)
	p := prepareBatchTest(t, f, "web/card.tsx")
	task := acceptBatchTest(t, f, p, "lease")
	old, e := claimKnowledgeBatch(t.Context(), knowledgeTestDomain, task.TaskID, common.GenerateUUID())
	if e != nil {
		t.Fatal(e)
	}
	id, e := beginKnowledgeBatchItem(t.Context(), f.service, old)
	if e != nil {
		t.Fatal(e)
	}
	past := time.Now().UTC().Add(-time.Minute)
	dao.DB.Model(&entity.KnowledgeBatchTaskRecord{}).Where("id = ?", task.TaskID).Update("lease_until", past)
	next, e := claimKnowledgeBatch(t.Context(), knowledgeTestDomain, task.TaskID, common.GenerateUUID())
	if e != nil || next == nil || next.LeaseEpoch <= old.LeaseEpoch {
		t.Fatal("not reclaimed", e)
	}
	if e = executeKnowledgeBatchItem(t.Context(), f.service, old, id); !errors.Is(e, errBatchLeaseLost) {
		t.Fatal("old epoch committed", e)
	}
	runKnowledgeBatch(t.Context(), f.service, next)
	p2 := prepareBatchTest(t, f, "web/card.tsx")
	expired := acceptBatchTest(t, f, p2, "expired")
	dao.DB.Model(&entity.KnowledgeBatchTaskRecord{}).Where("id = ?", expired.TaskID).Update("deadline_at", past)
	if _, e = claimKnowledgeBatch(t.Context(), knowledgeTestDomain, expired.TaskID, common.GenerateUUID()); e != nil {
		t.Fatal(e)
	}
	result, e := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, expired.TaskID, "")
	if e != nil || result.Items[0].Code != "execution_expired" || result.Items[0].SourceID != nil {
		t.Fatalf("expired %+v %v", result, e)
	}
}
func TestKnowledgeBatchSelectionConsentCapacityAndUnknownKey(t *testing.T) {
	f := setupBatchTest(t)
	p := prepareBatchTest(t, f, "web/card.tsx")
	bad := batchRequest(p, "bad-consent")
	bad.ConfirmedItemIDs = []string{}
	if _, _, e := f.service.AcceptKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, bad); e == nil {
		t.Fatal("empty consent accepted")
	}
	obs := f.observe(t, "web/card.tsx")
	_, e := f.service.PrepareKnowledgeBatch(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, entity.KnowledgeBatchPreviewRequest{ObservationID: obs.ObservationID, ObservationSHA256: obs.ObservationSHA256, SelectedItems: []entity.KnowledgeBatchSelectionItem{{RelativePath: "web/card.tsx", Classification: f.reg.DefaultClassification}}})
	assertDirectoryError(t, e, 429, "")
	task := acceptBatchTest(t, f, p, "same")
	changed := batchRequest(p, "same")
	changed.RetryOfTaskID = &task.TaskID
	_, _, e = f.service.AcceptKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, changed)
	assertDirectoryError(t, e, 409, "idempotency_conflict")
	_, e = f.service.GetKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, "", "no-record")
	assertDirectoryError(t, e, 404, "")
	f.writeConfig(t, nil)
	_, e = f.service.GetKnowledgeBatchTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
	assertDirectoryError(t, e, 404, "")
	_, e = claimKnowledgeBatch(t.Context(), knowledgeTestDomain, task.TaskID, common.GenerateUUID())
	if e != nil {
		t.Fatal(e)
	}
}
func TestKnowledgeBatchResultPaginationConflict(t *testing.T) {
	f := setupBatchTest(t)
	paths := []string{}
	for n := 0; n < 12; n++ {
		p := fmt.Sprintf("web/item%02d.ts", n)
		f.write(t, p, "synthetic")
		paths = append(paths, p)
	}
	p := prepareBatchTest(t, f, paths...)
	task := acceptBatchTest(t, f, p, "pages")
	first, e := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
	if e != nil || len(first.Items) != 10 || first.NextCursor == nil {
		t.Fatal("first page", e)
	}
	runBatchTest(t, f, task.TaskID)
	_, e = f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, *first.NextCursor)
	assertDirectoryError(t, e, 409, "result_changed")
	first, e = f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, "")
	if e != nil {
		t.Fatal(e)
	}
	last, e := f.service.ListKnowledgeBatchTaskItems(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.reg.RegistrationID, task.TaskID, *first.NextCursor)
	if e != nil || len(last.Items) != 2 || last.NextCursor != nil || last.Counts.Succeeded != 12 {
		t.Fatal("terminal page", e)
	}
}
func TestKnowledgeBatchDigestVectors(t *testing.T) {
	raw, e := os.ReadFile("../../../docs/knowledge-batch-import-v1.examples.json")
	if e != nil {
		t.Fatal(e)
	}
	var examples struct {
		Vectors []struct {
			Kind  string          `json:"kind"`
			Input json.RawMessage `json:"input"`
			User  string          `json:"user_id"`
			SHA   string          `json:"sha256"`
		} `json:"digest_vectors"`
	}
	if e = json.Unmarshal(raw, &examples); e != nil {
		t.Fatal(e)
	}
	for _, v := range examples.Vectors {
		switch v.Kind {
		case "batch":
			var p entity.KnowledgeBatchPreview
			if e = json.Unmarshal(v.Input, &p); e != nil {
				t.Fatal(e)
			}
			if batchPreviewDigest(&p, v.User) != v.SHA {
				t.Fatal("batch digest mismatch")
			}
		case "manifest":
			var p entity.KnowledgeProjectManifestPreviewPage
			if e = json.Unmarshal(v.Input, &p); e != nil {
				t.Fatal(e)
			}
			if projectManifestDigest(&p) != v.SHA {
				t.Fatal("manifest digest mismatch")
			}
		}
	}
}
