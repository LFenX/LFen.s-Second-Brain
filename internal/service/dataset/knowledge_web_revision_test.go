package dataset

import (
	"bytes"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestKnowledgeWebFixedRevisionSurvivesCaptureExpiry(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	raw := webFeedFixtureBody(`<item><guid>fixed</guid><description><![CDATA[<pre><code>const 版本 = 1;
  run(版本);</code></pre><a href="https://example.test/guide#part">指南</a>]]></description></item>`)
	ready := webReadyFixture(t, s, reg, "fixed:c", raw)
	preview := webPreviewFixture(t, s, reg, ready, "fixed:p")
	receipt, request := webConfirmFixture(t, s, reg, preview, "fixed:i")
	items, err := s.ListKnowledgeWebReceiptItems(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, receipt.ImportTaskID, "")
	if err != nil {
		t.Fatal(err)
	}
	id := items.Items[0].SourceRevisionID
	if err := dao.DB.Model(&entity.KnowledgeWebCaptureRecord{}).Where("id = ?", ready.CaptureID).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.KnowledgeWebObservationRecord{}).Where("capture_id = ?", ready.CaptureID).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, ""); err != nil {
		t.Fatal(err)
	}
	view, err := s.GetKnowledgeWebRevision(t.Context(), knowledgeTestOwner, reg.DomainID, id)
	if err != nil || view.SourceRevisionID != id || view.CaptureID != ready.CaptureID || view.RootNodeID != nil || view.VectorStatus != "not_built" {
		t.Fatal("fixed revision depended on temporary TTL", err)
	}
	reading, err := s.ReadKnowledgeWebRevisionContent(t.Context(), knowledgeTestOwner, reg.DomainID, id, "safe_reading")
	if err != nil || !bytes.Contains(reading.Raw, []byte("\n  run")) {
		t.Fatal("fixed safe reading failed", err)
	}
	original, err := s.DownloadKnowledgeWebRevisionCapture(t.Context(), knowledgeTestOwner, reg.DomainID, id)
	if err != nil || !bytes.Equal(original.Raw, raw) {
		t.Fatal("fixed raw capture failed", err)
	}
	refs, err := s.ListKnowledgeWebRevisionReferences(t.Context(), knowledgeTestOwner, reg.DomainID, id, "")
	if err != nil || len(refs.Items) != 1 || refs.Items[0].Fragment == nil || *refs.Items[0].Fragment != "part" {
		t.Fatal("fixed references failed", err)
	}
	list, err := s.ListKnowledgeWebRevisions(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, nil, "")
	if err != nil || len(list.Items) != 1 || list.Items[0].SourceRevisionID != id {
		t.Fatal("current list lost saved source", err)
	}
	replayed, created, err := s.ConfirmKnowledgeWebImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil || created || replayed.ImportTaskID != receipt.ImportTaskID {
		t.Fatal("committed receipt failed after TTL", err)
	}
	if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ?", knowledgeTestOwner).Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeWebRevisionContent(t.Context(), knowledgeTestOwner, reg.DomainID, id, "safe_reading")
	assertOriginalCode(t, err, 404, "knowledge_not_found")
}
