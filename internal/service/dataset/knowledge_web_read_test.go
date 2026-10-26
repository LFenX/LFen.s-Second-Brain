package dataset

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestKnowledgeWebObservationPagesAndBoundedReferences(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	var items strings.Builder
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&items, "<item><guid>id-%03d</guid><description>合成%d</description></item>", i, i)
	}
	ready := webReadyFixture(t, s, reg, "read:pages", []byte(`<rss version="2.0"><channel>`+items.String()+`</channel></rss>`))
	obs := webObservationFixture(t, s, reg, ready)
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := s.ListKnowledgeWebEntries(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, cursor)
		if err != nil || len(page.Items) > 20 || len(directoryJSON(page)) > 31744 {
			t.Fatal("invalid bounded page", err)
		}
		pages++
		for _, entry := range page.Items {
			if seen[entry.EntryKey] {
				t.Fatal("page repeated entry")
			}
			seen[entry.EntryKey] = true
		}
		if page.NextCursor == nil {
			break
		}
		if *page.NextCursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 65 || pages != 4 {
		t.Fatal("page coverage wrong", len(seen), pages)
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeWebEntryRecord{}).Where("observation_id = ?", obs.ObservationID).Count(&count).Error; err != nil || count != 65 {
		t.Fatal("page read mutated observation")
	}
}

func TestKnowledgeWebEntryBinaryAndUTF8Window(t *testing.T) {
	s, reg, _ := webFlowFixture(t)
	text := "甲乙🧠\n  code"
	raw := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>code</id><content type="text">` + text + `</content></entry></feed>`)
	request := webLocalTestRequest(reg, "read:atom")
	request.DeclaredMediaType = "application/atom+xml"
	request.DeclaredByteLength = int64(len(raw))
	capture, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReceiveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, io.NopCloser(bytes.NewReader(raw)), int64(len(raw)), "", "", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	webObservationFixture(t, s, reg, ready)
	page, err := s.ListKnowledgeWebEntries(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(err)
	}
	key := page.Items[0].EntryKey
	content, err := s.ReadKnowledgeWebEntryContent(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, key, "decoded_content")
	if err != nil || string(content.Raw) != text || content.SHA256 != directorySHA([]byte(text)) {
		t.Fatal("decoded full read lost bytes", err)
	}
	window, err := s.ReadKnowledgeWebEntryWindow(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, key, "decoded_content", 0, 6)
	if err != nil || window.Text != "甲乙" || window.NextStartByte == nil || *window.NextStartByte != 6 {
		t.Fatal("UTF8 aligned window failed", err)
	}
	for _, rangeBytes := range [][2]int64{{1, 6}, {0, 7}, {1, 1}, {0, 4097}} {
		_, err := s.ReadKnowledgeWebEntryWindow(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, key, "decoded_content", rangeBytes[0], rangeBytes[1])
		assertOriginalCode(t, err, 416, "web_window_invalid")
	}
	_, err = s.ReadKnowledgeWebEntryWindow(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, key, "decoded_content", 3, 3)
	if err != nil {
		t.Fatal("empty aligned interval rejected", err)
	}
	reading, err := s.ReadKnowledgeWebEntryContent(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, key, "safe_reading")
	if err != nil || !bytes.Contains(reading.Raw, []byte("甲乙🧠\n  code")) {
		t.Fatal("safe reading wrong", err)
	}
	download, err := s.DownloadKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID)
	if err != nil || !bytes.Equal(download.Raw, raw) {
		t.Fatal("raw evidence changed", err)
	}
}

func TestKnowledgeWebReadReauthorizesAfterObjectIO(t *testing.T) {
	s, reg, store := webFlowFixture(t)
	ready := webReadyFixture(t, s, reg, "read:revoke", []byte(`<rss version="2.0"><channel><item><guid>x</guid><description>合成</description></item></channel></rss>`))
	webObservationFixture(t, s, reg, ready)
	page, err := s.ListKnowledgeWebEntries(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, "")
	if err != nil {
		t.Fatal(err)
	}
	store.beforeOpen = func() {
		if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ?", knowledgeTestOwner).Update("status", "0").Error; err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.ReadKnowledgeWebEntryContent(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, page.Items[0].EntryKey, "safe_reading")
	assertOriginalCode(t, err, 404, "knowledge_not_found")
	if result != nil {
		t.Fatal("object bytes escaped revocation")
	}
}
