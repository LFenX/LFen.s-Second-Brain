package dataset

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/syncer/connector"
)

func webObservationFixture(t *testing.T, s *DatasetService, reg *entity.KnowledgeWebRegistration, ready *entity.KnowledgeWebCapture) *entity.KnowledgeWebObservation {
	t.Helper()
	view, _, err := s.ObserveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, entity.KnowledgeWebObserve{NormalizerVersion: connector.KnowledgeWebProducerVersions().NormalizerVersion})
	if err != nil || view.State != "ready" || view.ObservationSHA256 == nil || !view.CaptureTraversalComplete {
		t.Fatalf("observation=%+v err=%v", view, err)
	}
	return view
}

func TestKnowledgeWebObservationUniqueFixedAndPacked(t *testing.T) {
	s, reg, store := webFlowFixture(t)
	raw := []byte(`<rss version="2.0"><channel><item><guid>甲</guid><description><![CDATA[<pre><code>const 值 = 1;
  run(值);</code></pre>]]></description></item><item><guid>乙</guid><description><![CDATA[<table><tr><td colspan="2">合成表</td></tr></table>]]></description></item></channel></rss>`)
	ready := webReadyFixture(t, s, reg, "observe:first", raw)
	observed := webObservationFixture(t, s, reg, ready)
	if *observed.InputEntryCount != 2 || *observed.EntryCount != 2 || *observed.EligibleCount != 2 {
		t.Fatal("observation counts wrong")
	}
	byCapture, err := s.GetKnowledgeWebObservation(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID)
	if err != nil || byCapture.ObservationID != observed.ObservationID || *byCapture.ObservationSHA256 != *observed.ObservationSHA256 {
		t.Fatal("unique observation recovery changed", err)
	}
	replayed, created, err := s.ObserveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, entity.KnowledgeWebObserve{NormalizerVersion: observed.ProducerVersions.NormalizerVersion})
	if err != nil || created || replayed.ObservationID != observed.ObservationID {
		t.Fatal("POST reparsed a ready observation", err)
	}
	if len(store.objects) != 2 {
		t.Fatal("observation wrote objects per item instead of one bounded pack", len(store.objects))
	}
	var entries []entity.KnowledgeWebEntryRecord
	if err := dao.DB.Where("observation_id = ?", observed.ObservationID).Find(&entries).Error; err != nil || len(entries) != 2 {
		t.Fatal("entries missing", err)
	}
	for _, row := range entries {
		var e entity.KnowledgeWebEntry
		if err := json.Unmarshal([]byte(row.Payload), &e); err != nil || e.CaptureID != ready.CaptureID || e.SourceID != nil || e.CurrentRevisionID != nil || e.ObservationID != observed.ObservationID {
			t.Fatal("observation invented published identity", err)
		}
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("observe published without confirmation")
	}
}

func TestKnowledgeWebObservationRejectsWholeConflictingFeed(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	raw := []byte(`<rss version="2.0"><channel><item><guid>same</guid><description>A</description></item><item><guid>same</guid><description>B</description></item></channel></rss>`)
	ready := webReadyFixture(t, s, reg, "observe:conflict", raw)
	_, _, err := s.ObserveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, entity.KnowledgeWebObserve{NormalizerVersion: connector.KnowledgeWebProducerVersions().NormalizerVersion})
	assertOriginalCode(t, err, 409, "web_entry_conflict")
	view, err := s.GetKnowledgeWebObservation(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID)
	if err != nil || view.State != "rejected" || view.ObservationSHA256 != nil || view.CaptureTraversalComplete {
		t.Fatal("partial failed observation escaped", err)
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeWebEntryRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial failed entries saved", err)
	}
}

func TestKnowledgeWebObservationCrashLeaseExplicitResume(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	ready := webReadyFixture(t, s, reg, "observe:crash", []byte(`<rss version="2.0"><channel><item><guid>x</guid><description>合成</description></item></channel></rss>`))
	request := entity.KnowledgeWebObserve{NormalizerVersion: connector.KnowledgeWebProducerVersions().NormalizerVersion}
	transfer, initial, created, err := s.beginWebObservation(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, request)
	if err != nil || !created || initial.State != "parsing" {
		t.Fatal("begin parse failed", err)
	}
	past := time.Now().Add(-time.Second)
	if err := dao.DB.Model(&entity.KnowledgeWebSlot{}).Where("id = ?", transfer.slot).Update("lease_until", past).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.KnowledgeWebObservationRecord{}).Where("id = ?", initial.ObservationID).Update("lease_until", past).Error; err != nil {
		t.Fatal(err)
	}
	interrupted, err := s.GetKnowledgeWebObservation(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID)
	if err != nil || interrupted.State != "interrupted" || interrupted.Revision <= initial.Revision {
		t.Fatal("lease interruption lost", err)
	}
	resumed, created, err := s.ObserveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, request)
	if err != nil || created || resumed.State != "ready" || resumed.ObservationID != initial.ObservationID || resumed.Revision <= interrupted.Revision {
		t.Fatal("explicit resume did not retain unique identity", err)
	}
	if err := s.checkWebObservationLease(t.Context(), transfer, true); err == nil {
		t.Fatal("old parser epoch resumed")
	}
}

func TestKnowledgeWebObservationEmptyWindowAndExpiry(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	ready := webReadyFixture(t, s, reg, "observe:empty", []byte(`<rss version="2.0"><channel/></rss>`))
	view := webObservationFixture(t, s, reg, ready)
	if view.EntryCount == nil || *view.EntryCount != 0 || view.ObservationSHA256 == nil {
		t.Fatal("empty window not a fixed observation")
	}
	if err := dao.DB.Model(&entity.KnowledgeWebObservationRecord{}).Where("id = ?", view.ObservationID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := s.GetKnowledgeWebObservation(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID)
	if err != nil || expired.State != "expired" || expired.ObservationID != view.ObservationID {
		t.Fatal("expiry discarded observation", err)
	}
	_, err = s.GetKnowledgeWebTarget(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal("target missing identity should be empty current head", err)
	}
}
