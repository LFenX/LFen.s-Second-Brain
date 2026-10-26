package dataset

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/syncer/connector"
)

func webFlowFixture(t *testing.T) (*DatasetService, *entity.KnowledgeWebRegistration, *originalFlowStore) {
	t.Helper()
	s, reg := setupWebTest(t)
	factory := storage.GetStorageFactory()
	before := factory.GetStorage()
	store := newOriginalFlowStore()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(before) })
	return s, reg, store
}
func webReadyFixture(t *testing.T, s *DatasetService, reg *entity.KnowledgeWebRegistration, key string, raw []byte) *entity.KnowledgeWebCapture {
	t.Helper()
	req := webLocalTestRequest(reg, key)
	req.DeclaredByteLength = int64(len(raw))
	req.ExpectedSHA256 = directoryString(directorySHA(raw))
	capture, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, req)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReceiveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, io.NopCloser(bytes.NewReader(raw)), int64(len(raw)), "", "", "application/octet-stream")
	if err != nil || ready.State != "ready" || ready.RawSHA256 == nil || *ready.RawSHA256 != directorySHA(raw) || ready.Revision <= capture.Revision {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	return ready
}

func TestKnowledgeWebLocalUploadRawIntegrityAndReplay(t *testing.T) {
	s, reg, store := webFlowFixture(t)
	raw := []byte("\xef\xbb\xbf<rss version=\"2.0\"><channel><item><guid>合成</guid><description>code\r\n  line</description></item></channel></rss>")
	ready := webReadyFixture(t, s, reg, "local:raw", raw)
	if ready.MediaType == nil || *ready.MediaType != "application/rss+xml" || ready.Provenance.Kind != "user_supplied" || ready.Provenance.ServerReceivedAt == nil || ready.Provenance.UpstreamHTTPStatus != nil || ready.Provenance.FetchStartedAt != nil {
		t.Fatal("local provenance claimed HTTP")
	}
	var record entity.KnowledgeWebCaptureRecord
	if err := dao.DB.Where("id = ?", ready.CaptureID).Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	stored, err := webReadObject(t.Context(), record.ObjectFolderID, *ready.RawSHA256, *ready.RawByteLength)
	if err != nil || !bytes.Equal(stored, raw) || store.wholeReads != 0 {
		t.Fatal("raw bytes or streaming path changed", err)
	}
	_, err = s.ReceiveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, ready.CaptureID, io.NopCloser(bytes.NewReader(raw)), int64(len(raw)), "", "", "application/octet-stream")
	assertOriginalCode(t, err, 409, "web_state_conflict")
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("upload published before confirmation")
	}
}

func TestKnowledgeWebUploadRejectsBoundariesAndRetainsState(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentRange, encoding, contentType, code string
		status                                                int
		length                                                int64
		expected                                              *string
	}{
		{name: "short", body: "<rss/>", contentType: "application/octet-stream", code: "web_length_mismatch", status: 400, length: -1},
		{name: "SHA", body: strings.Repeat("x", 128), contentType: "application/octet-stream", code: "web_sha256_mismatch", status: 422, length: 128, expected: directoryString(strings.Repeat("a", 64))},
		{name: "encoding", body: strings.Repeat("x", 128), contentType: "application/octet-stream", encoding: "gzip", code: "web_encoding_unsupported", status: 415, length: 128},
		{name: "range", body: strings.Repeat("x", 128), contentType: "application/octet-stream", contentRange: "bytes 0-127/128", code: "web_range_unsupported", status: 416, length: 128},
		{name: "media", body: strings.Repeat("x", 128), contentType: "text/xml", code: "web_format_unsupported", status: 415, length: 128},
		{name: "UTF8", body: string(bytes.Repeat([]byte{0xff}, 128)), contentType: "application/octet-stream", code: "web_encoding_unsupported", status: 415, length: 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, reg, _ := webFlowFixture(t)
			request := webLocalTestRequest(reg, "bad:"+tc.name)
			request.ExpectedSHA256 = tc.expected
			capture, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ReceiveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, io.NopCloser(strings.NewReader(tc.body)), tc.length, tc.contentRange, tc.encoding, tc.contentType)
			assertOriginalCode(t, err, tc.status, tc.code)
			view, err := s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, "")
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "short" || tc.name == "SHA" || tc.name == "UTF8" {
				if view.State != "rejected" || view.Failure == nil || view.RawSHA256 != nil {
					t.Fatal("bad snapshot became ready", view)
				}
			}
		})
	}
}

func TestKnowledgeWebUploadHTTPDeadlineAndStorageFailure(t *testing.T) {
	s, reg, store := webFlowFixture(t)
	request := webLocalTestRequest(reg, "timeout")
	capture, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	deadline := time.Now().Add(100 * time.Millisecond)
	ctx := originalHTTPDeadlineContext{Context: parent, deadline: deadline}
	_, err = s.ReceiveKnowledgeWebCapture(ctx, knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, originalHTTPDeadlineBody{deadline: deadline, cancel: cancel}, -1, "", "", "application/octet-stream")
	assertOriginalCode(t, err, 504, "web_upload_timeout")
	view, err := s.GetKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, "")
	if err != nil || view.State != "interrupted" || view.Failure.MachineCode != "web_upload_timeout" {
		t.Fatal("deadline lost", err)
	}
	store.failPut = true
	raw := []byte(`<rss version="2.0"><channel/></rss>`)
	request = webLocalTestRequest(reg, "storage")
	request.DeclaredByteLength = int64(len(raw))
	capture, _, err = s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReceiveKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, io.NopCloser(bytes.NewReader(raw)), int64(len(raw)), "", "", "application/octet-stream")
	assertOriginalCode(t, err, 503, "web_storage_unavailable")
}

func TestKnowledgeWebOnlineExplicitOnceAndUpstreamError(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "upstream401"}[fail], func(t *testing.T) {
			s, reg, _ := webFlowFixture(t)
			calls := 0
			s.webFetch = func(ctx context.Context, start string, scope entity.KnowledgeWebNetworkScope, authorize func(context.Context) error) (*connector.KnowledgeWebFetchResult, error) {
				calls++
				if err := authorize(ctx); err != nil {
					return nil, err
				}
				if start != reg.OriginURL || !scope.Enabled {
					t.Fatal("wrong fixed acquisition scope")
				}
				if fail {
					status := int64(401)
					return nil, &connector.KnowledgeWebFetchError{Code: "web_upstream_http_error", UpstreamHTTPStatus: &status}
				}
				now := originalTime(time.Now())
				status := int64(200)
				identity := "identity"
				return &connector.KnowledgeWebFetchResult{Raw: []byte(`<rss version="2.0"><channel/></rss>`), DeclaredMediaType: "application/xml", Provenance: entity.KnowledgeWebProvenance{Kind: "server_http", RequestedURL: start, FinalURL: &start, ServerReceivedAt: &now, FetchStartedAt: &now, FetchCompletedAt: &now, UpstreamHTTPStatus: &status, RedirectChain: []entity.KnowledgeWebRedirect{}, ResponseContentType: directoryString("application/xml"), ResponseContentEncoding: &identity}}, nil
			}
			capture, _, err := s.CreateKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, entity.KnowledgeWebCreateFetchCapture{RegistrationConfigSHA256: reg.ConfigSHA256, IdempotencyKey: "online", AcquisitionMode: "explicit_fetch"})
			if err != nil || calls != 0 {
				t.Fatal("creation fetched")
			}
			req := entity.KnowledgeWebAcquire{ScopeSHA256: capture.ScopeSHA256, ExpectedCaptureRevision: capture.Revision, Confirmed: true}
			view, err := s.AcquireKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, req)
			if fail {
				assertOriginalCode(t, err, 502, "web_upstream_http_error")
				var public *KnowledgeWebError
				if !errors.As(err, &public) || public.Failure.UpstreamHTTPStatus == nil || *public.Failure.UpstreamHTTPStatus != 401 {
					t.Fatal("upstream authentication lost")
				}
			} else if err != nil || view.State != "ready" || view.Provenance.Kind != "server_http" {
				t.Fatal("online did not reach ready", err)
			}
			replay, err := s.AcquireKnowledgeWebCapture(t.Context(), knowledgeTestOwner, reg.DomainID, reg.RegistrationID, capture.CaptureID, req)
			if err != nil || calls != 1 || replay.AttemptStartedAt == nil {
				t.Fatal("same Capture re-fetched", err, calls)
			}
		})
	}
}
