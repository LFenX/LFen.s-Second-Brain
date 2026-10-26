package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func webTestNetwork(run func(context.Context, *http.Request, string) (*http.Response, error)) knowledgeWebFetchIO {
	return knowledgeWebFetchIO{now: func() time.Time { return time.Unix(1792960000, 0) }, lookup: func(context.Context, string) ([]string, error) { return []string{"8.8.8.8"}, nil }, roundTrip: run}
}
func webTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}
func webFetchCode(t *testing.T, err error, code string) {
	t.Helper()
	var public *KnowledgeWebFetchError
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("err=%v expected=%s", err, code)
	}
}
func webTestScope() entity.KnowledgeWebNetworkScope {
	return entity.KnowledgeWebNetworkScope{Enabled: true, AllowedOrigins: []string{"https://source.example.test"}}
}

func TestKnowledgeWebFetchAnonymousPinnedAndRedirects(t *testing.T) {
	calls, lookups := 0, 0
	network := webTestNetwork(func(ctx context.Context, req *http.Request, address string) (*http.Response, error) {
		calls++
		if address != "8.8.8.8" || req.Method != "GET" || req.Header.Get("Accept-Encoding") != "identity" || len(req.Header) != 1 || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" || req.Header.Get("Referer") != "" {
			t.Fatal("request not anonymous/pinned")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 15*time.Second {
			t.Fatal("overall deadline missing")
		}
		if calls < 4 {
			r := webTestResponse(302, "")
			r.Header.Set("Location", "/hop"+string(rune('0'+calls)))
			return r, nil
		}
		return webTestResponse(200, "<p>合成资料</p>"), nil
	})
	network.lookup = func(ctx context.Context, host string) ([]string, error) {
		lookups++
		if host != "source.example.test" {
			t.Fatal("unexpected DNS host")
		}
		return []string{"8.8.8.8"}, nil
	}
	result, err := fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/start", webTestScope(), network)
	if err != nil || calls != 4 || lookups != 4 || len(result.Provenance.RedirectChain) != 3 || *result.Provenance.FinalURL != "https://source.example.test/hop3" || result.Provenance.ClaimedCapturedAt != nil || result.Provenance.Kind != "server_http" {
		t.Fatalf("capture=%+v calls=%d lookups=%d err=%v", result, calls, lookups, err)
	}
}

func TestKnowledgeWebFetchBlocksEveryDNSAddress(t *testing.T) {
	t.Setenv("SSRF_ALLOW_ANY_HOST", "true")
	for _, address := range []string{"127.0.0.1", "::1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "203.0.113.1", "0.1.2.3", "240.0.0.1", "224.0.0.1", "fc00::1", "fe80::1", "2001:db8::1", "::ffff:127.0.0.1", "2002:7f00:1::1"} {
		t.Run(address, func(t *testing.T) {
			calls := 0
			network := webTestNetwork(func(context.Context, *http.Request, string) (*http.Response, error) {
				calls++
				return webTestResponse(200, "x"), nil
			})
			network.lookup = func(context.Context, string) ([]string, error) { return []string{"8.8.8.8", address}, nil }
			_, err := fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/start", webTestScope(), network)
			webFetchCode(t, err, "web_remote_blocked")
			if calls != 0 {
				t.Fatal("private or mixed DNS dialed")
			}
		})
	}
}

func TestKnowledgeWebFetchRejectsStatusEncodingAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		response   func() *http.Response
	}{
		{"source401", "web_upstream_http_error", func() *http.Response { return webTestResponse(401, "denied") }},
		{"source403", "web_upstream_http_error", func() *http.Response { return webTestResponse(403, "denied") }},
		{"source404", "web_upstream_http_error", func() *http.Response { return webTestResponse(404, "missing") }},
		{"304", "web_upstream_http_error", func() *http.Response { return webTestResponse(304, "") }},
		{"204", "web_upstream_http_error", func() *http.Response { return webTestResponse(204, "") }},
		{"gzip", "web_compression_rejected", func() *http.Response {
			r := webTestResponse(200, "x")
			r.Header.Set("Content-Encoding", "gzip")
			return r
		}},
		{"auto decompressed", "web_compression_rejected", func() *http.Response { r := webTestResponse(200, "x"); r.Uncompressed = true; return r }},
		{"nonUTF8 declaration", "web_encoding_unsupported", func() *http.Response {
			r := webTestResponse(200, "x")
			r.Header.Set("Content-Type", "text/html; charset=gbk")
			return r
		}},
		{"large headers", "web_budget_exceeded", func() *http.Response {
			r := webTestResponse(200, "x")
			r.Header.Set("X-Large", strings.Repeat("x", 16384))
			return r
		}},
		{"over declared body", "web_too_large", func() *http.Response {
			r := webTestResponse(200, "x")
			r.ContentLength = knowledgeWebMaxBytes + 1
			return r
		}},
		{"over chunked body", "web_too_large", func() *http.Response {
			r := webTestResponse(200, strings.Repeat("x", knowledgeWebMaxBytes+1))
			r.ContentLength = -1
			return r
		}},
		{"short body", "web_length_mismatch", func() *http.Response { r := webTestResponse(200, "x"); r.ContentLength = 10; return r }},
		{"empty body", "web_length_mismatch", func() *http.Response { return webTestResponse(200, "") }},
		{"truncated body", "web_result_unknown", func() *http.Response {
			r := webTestResponse(200, "")
			r.Body = io.NopCloser(webErrorReader{})
			return r
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			network := webTestNetwork(func(context.Context, *http.Request, string) (*http.Response, error) {
				calls++
				return tc.response(), nil
			})
			result, err := fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/start", webTestScope(), network)
			webFetchCode(t, err, tc.code)
			if result != nil || calls != 1 {
				t.Fatal("failure retried or partial capture returned")
			}
		})
	}
}

type webErrorReader struct{}

func (webErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestKnowledgeWebFetchRejectsRedirectAndRebinding(t *testing.T) {
	for _, location := range []string{"http://127.0.0.1/", "https://other.example.test/a", "file:///etc/passwd", "https://source.example.test:444/a", "https://source.example.test/?token=x"} {
		t.Run(location, func(t *testing.T) {
			network := webTestNetwork(func(context.Context, *http.Request, string) (*http.Response, error) {
				r := webTestResponse(302, "")
				r.Header.Set("Location", location)
				return r, nil
			})
			_, err := fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/", webTestScope(), network)
			webFetchCode(t, err, "web_redirect_denied")
		})
	}
	calls, lookups := 0, 0
	network := webTestNetwork(func(context.Context, *http.Request, string) (*http.Response, error) {
		calls++
		r := webTestResponse(302, "")
		r.Header.Set("Location", "/again")
		return r, nil
	})
	network.lookup = func(context.Context, string) ([]string, error) {
		lookups++
		if lookups == 1 {
			return []string{"8.8.8.8"}, nil
		}
		return []string{"127.0.0.1"}, nil
	}
	_, err := fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/", webTestScope(), network)
	webFetchCode(t, err, "web_remote_blocked")
	if calls != 1 || lookups != 2 {
		t.Fatal("redirect did not recheck DNS")
	}
	network = webTestNetwork(func(context.Context, *http.Request, string) (*http.Response, error) {
		r := webTestResponse(302, "")
		r.Header.Set("Location", "/loop")
		return r, nil
	})
	_, err = fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/", webTestScope(), network)
	webFetchCode(t, err, "web_redirect_denied")
}

func TestKnowledgeWebFetchOverallDeadlineAndScope(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	network := webTestNetwork(func(ctx context.Context, _ *http.Request, _ string) (*http.Response, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	})
	_, err := fetchKnowledgeWebCapture(ctx, "https://source.example.test/", webTestScope(), network)
	webFetchCode(t, err, "web_fetch_timeout")
	if calls != 1 {
		t.Fatal("deadline retried")
	}
	for _, scope := range []entity.KnowledgeWebNetworkScope{{Enabled: false, AllowedOrigins: []string{}}, {Enabled: true, AllowedOrigins: []string{"https://other.example.test"}}, {Enabled: true, AllowedOrigins: []string{"https://source.example.test/path"}}, {Enabled: true, AllowedOrigins: []string{"https://source.example.test", "https://SOURCE.example.test:443/"}}} {
		_, err := fetchKnowledgeWebCapture(t.Context(), "https://source.example.test/", scope, network)
		webFetchCode(t, err, "web_remote_blocked")
	}
}
