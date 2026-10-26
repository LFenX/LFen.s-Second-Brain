package dataset

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func webPreviewFixture(t *testing.T, s *DatasetService, reg *entity.KnowledgeWebRegistration, capture *entity.KnowledgeWebCapture, key string) *entity.KnowledgeWebPreview {
	t.Helper()
	obs := webObservationFixture(t, s, reg, capture)
	entries, err := s.ListKnowledgeWebEntries(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, "")
	if err != nil {
		t.Fatal(err)
	}
	selection := []entity.KnowledgeWebSelection{}
	cursor := ""
	for {
		if cursor != "" {
			entries, err = s.ListKnowledgeWebEntries(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, cursor)
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, e := range entries.Items {
			target, err := s.GetKnowledgeWebTarget(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, e.EntryKey)
			if err != nil {
				t.Fatal(err)
			}
			selection = append(selection, entity.KnowledgeWebSelection{EntryKey: e.EntryKey, Classification: reg.DefaultClassification, ExpectedRevisionID: target.CurrentRevisionID})
		}
		if entries.NextCursor == nil {
			break
		}
		cursor = *entries.NextCursor
	}
	view, created, err := s.CreateKnowledgeWebPreview(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeWebCreatePreview{CaptureID: capture.CaptureID, ObservationID: obs.ObservationID, ObservationSHA256: *obs.ObservationSHA256, IdempotencyKey: key, Selection: selection})
	if err != nil || !created || view.State != "ready" {
		t.Fatalf("preview=%+v new=%t err=%v", view, created, err)
	}
	return view
}
func webConfirmFixture(t *testing.T, s *DatasetService, reg *entity.KnowledgeWebRegistration, preview *entity.KnowledgeWebPreview, key string) (*entity.KnowledgeWebReceipt, entity.KnowledgeWebConfirm) {
	t.Helper()
	items, err := s.ListKnowledgeWebPreviewItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, preview.PreviewID, "")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, item := range items.Items {
		keys = append(keys, item.EntryKey)
	}
	if items.NextCursor != nil {
		next, err := s.ListKnowledgeWebPreviewItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, preview.PreviewID, *items.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range next.Items {
			keys = append(keys, item.EntryKey)
		}
	}
	request := entity.KnowledgeWebConfirm{PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: key, Confirmed: true, AcknowledgedEntryKeys: keys}
	receipt, created, err := s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil || !created || receipt.Status != "completed" || receipt.ModelCalls != 0 || receipt.IndexStatus != "not_built" {
		t.Fatalf("receipt=%+v new=%t err=%v", receipt, created, err)
	}
	return receipt, request
}
func webFeedFixtureBody(items string) []byte {
	return []byte(`<rss version="2.0"><channel>` + items + `</channel></rss>`)
}

func TestKnowledgeWebAtomicThirtyTwoAndReceiptRecovery(t *testing.T) {
	s, reg, store := webFlowFixture(t)
	var body strings.Builder
	for i := 0; i < 32; i++ {
		fmt.Fprintf(&body, "<item><guid>id-%02d</guid><title>合成%d</title><description>代码%d</description></item>", i, i, i)
	}
	ready := webReadyFixture(t, s, reg, "batch:capture", webFeedFixtureBody(body.String()))
	preview := webPreviewFixture(t, s, reg, ready, "batch:preview")
	receipt, request := webConfirmFixture(t, s, reg, preview, "batch:confirm")
	if receipt.SelectedCount != 32 || receipt.SavedCount != 32 || receipt.UnchangedCount != 0 {
		t.Fatal("batch totals wrong")
	}
	for _, model := range []any{&entity.KnowledgeSourceRevision{}, &entity.KnowledgeWebRevisionRecord{}, &entity.KnowledgeWebBinding{}, &entity.Document{}, &entity.FileCommitItem{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 32 {
			t.Fatalf("%T count=%d err=%v", model, count, err)
		}
	}
	var commits int64
	if err := dao.DB.Model(&entity.FileCommit{}).Count(&commits).Error; err != nil || commits != 1 {
		t.Fatal("32 copies of native tree instead of one", commits, err)
	}
	var nodes int64
	if err := dao.DB.Model(&entity.KnowledgeNode{}).Count(&nodes).Error; err != nil || nodes != 0 {
		t.Fatal("web import invented C1 nodes")
	}
	beforeReads := store.streamReads
	replayed, created, err := s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil || created || replayed.ImportTaskID != receipt.ImportTaskID || store.streamReads != beforeReads {
		t.Fatal("receipt replay re-applied publication", err)
	}
	byKey, err := s.GetKnowledgeWebReceipt(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, "", request.IdempotencyKey)
	if err != nil || byKey.ImportTaskID != receipt.ImportTaskID {
		t.Fatal("receipt original key failed", err)
	}
	request.IdempotencyKey = "other:confirm"
	_, _, err = s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	assertOriginalCode(t, err, 409, "web_preview_consumed")
	if _, err := s.CancelKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID); err != nil {
		t.Fatal(err)
	}
	committed, err := s.GetKnowledgeWebPreview(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, preview.PreviewID, "")
	if err != nil || committed.State != "committed" || committed.ImportTaskID == nil || *committed.ImportTaskID != receipt.ImportTaskID {
		t.Fatal("committed preview lost after Capture cancellation", err)
	}
}

func TestKnowledgeWebIncrementalUnchangedAndHistoricalRevision(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	var first, second *entity.KnowledgeWebReceiptItem
	firstCapture := ""
	for i, text := range []string{"版本一", "版本二", "版本一", "版本一"} {
		ready := webReadyFixture(t, s, reg, fmt.Sprintf("increment:c%d", i), webFeedFixtureBody(`<item><guid>stable</guid><title>相同时间</title><pubDate>same-time</pubDate><description>`+text+`</description></item>`))
		preview := webPreviewFixture(t, s, reg, ready, fmt.Sprintf("increment:p%d", i))
		receipt, _ := webConfirmFixture(t, s, reg, preview, fmt.Sprintf("increment:i%d", i))
		items, err := s.ListKnowledgeWebReceiptItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, receipt.ImportTaskID, "")
		if err != nil || len(items.Items) != 1 {
			t.Fatal(err)
		}
		item := items.Items[0]
		if i == 0 {
			first = &item
			firstCapture = ready.CaptureID
		} else if i == 1 {
			second = &item
			if second.SourceID != first.SourceID || second.SourceRevisionID == first.SourceRevisionID {
				t.Fatal("incremental identity changed incorrectly")
			}
		} else if item.SourceID != first.SourceID || item.SourceRevisionID != first.SourceRevisionID {
			t.Fatal("historical input did not reuse fixed revision")
		}
		if i == 3 && (item.Action != "unchanged" || receipt.UnchangedCount != 1 || receipt.CaptureID == firstCapture) {
			t.Fatal("new observation provenance or unchanged result wrong")
		}
		if _, err := s.CancelKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID); err != nil {
			t.Fatal(err)
		}
	}
	var sourceCount, commitCount int64
	if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&sourceCount).Error; err != nil || sourceCount != 2 {
		t.Fatal("unchanged created duplicate revisions", err)
	}
	if err := dao.DB.Model(&entity.FileCommit{}).Count(&commitCount).Error; err != nil || commitCount != 3 {
		t.Fatal("native heads not preserved", err)
	}
	var fixed entity.KnowledgeWebRevisionRecord
	if err := dao.DB.Where("id = ?", first.SourceRevisionID).Take(&fixed).Error; err != nil || fixed.CaptureID != firstCapture {
		t.Fatal("unchanged rewrote original provenance", err)
	}
	_, err := s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, reg.DomainID, first.SourceRevisionID)
	assertOriginalCode(t, err, 415, "knowledge_source_unsupported")
}

func TestKnowledgeWebSQLFailurePublishesNothing(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	ready := webReadyFixture(t, s, reg, "rollback:c", webFeedFixtureBody(`<item><guid>a</guid><description>A</description></item><item><guid>b</guid><description>B</description></item>`))
	preview := webPreviewFixture(t, s, reg, ready, "rollback:p")
	items, err := s.ListKnowledgeWebPreviewItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, preview.PreviewID, "")
	if err != nil {
		t.Fatal(err)
	}
	request := entity.KnowledgeWebConfirm{PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: "rollback:i", Confirmed: true, AcknowledgedEntryKeys: []string{items.Items[0].EntryKey, items.Items[1].EntryKey}}
	if err := dao.DB.Exec("CREATE TRIGGER web_fail_second_item BEFORE INSERT ON file_commit_item WHEN (SELECT COUNT(*) FROM file_commit_item) = 1 BEGIN SELECT RAISE(ABORT, 'synthetic second native item failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request); err == nil {
		t.Fatal("injected SQL failure succeeded")
	}
	for _, model := range []any{&entity.Document{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeWebBinding{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.KnowledgeWebReceiptRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%T survived rollback: %d %v", model, count, err)
		}
	}
	recovered, err := s.GetKnowledgeWebPreview(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, preview.PreviewID, "")
	if err != nil || recovered.State != "ready" {
		t.Fatal("rollback consumed preview", err)
	}
	if err := dao.DB.Exec("DROP TRIGGER web_fail_second_item").Error; err != nil {
		t.Fatal(err)
	}
	receipt, created, err := s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil || !created || receipt.SavedCount != 2 {
		t.Fatal("same confirmation key could not recover after rollback", err)
	}
}

func TestKnowledgeWebPreviewAcknowledgementExpiryAndLiveConflict(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	ready := webReadyFixture(t, s, reg, "concurrent:c", webFeedFixtureBody(`<item><guid>a</guid><description>合成</description></item>`))
	first := webPreviewFixture(t, s, reg, ready, "concurrent:p1")
	second := webPreviewFixture(t, s, reg, ready, "concurrent:p2")
	items, err := s.ListKnowledgeWebPreviewItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, first.PreviewID, "")
	if err != nil {
		t.Fatal(err)
	}
	bad := entity.KnowledgeWebConfirm{PreviewID: first.PreviewID, ManifestSHA256: first.ManifestSHA256, IdempotencyKey: "concurrent:bad", Confirmed: true, AcknowledgedEntryKeys: []string{strings.Repeat("a", 64)}}
	_, _, err = s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, bad)
	assertOriginalCode(t, err, 400, "knowledge_invalid_request")
	webConfirmFixture(t, s, reg, first, "concurrent:win")
	bad.PreviewID = second.PreviewID
	bad.ManifestSHA256 = second.ManifestSHA256
	bad.IdempotencyKey = "concurrent:stale"
	bad.AcknowledgedEntryKeys = []string{items.Items[0].EntryKey}
	_, _, err = s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, bad)
	assertOriginalCode(t, err, 409, "revision_changed")
	if err := dao.DB.Model(&entity.KnowledgeWebPreviewRecord{}).Where("id = ?", second.PreviewID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	_, _, err = s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, bad)
	assertOriginalCode(t, err, 410, "web_preview_expired")
}
