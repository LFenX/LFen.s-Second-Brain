package connector

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// KnowledgeWebFetchError separates upstream failures from local authentication.
type KnowledgeWebFetchError struct {
	Code               string
	UpstreamHTTPStatus *int64
}

func (e *KnowledgeWebFetchError) Error() string { return e.Code }

// KnowledgeWebFetchResult contains a single anonymous, complete HTTP capture.
type KnowledgeWebFetchResult struct {
	Raw               []byte
	DeclaredMediaType string
	Provenance        entity.KnowledgeWebProvenance
}

type knowledgeWebFetchIO struct {
	lookup    func(context.Context, string) ([]string, error)
	roundTrip func(context.Context, *http.Request, string) (*http.Response, error)
	now       func() time.Time
	authorize func(context.Context) error
}

func knowledgeWebOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// NormalizeKnowledgeWebScope validates and canonicalizes an immutable network
// declaration; it never resolves a host or attempts a connection.
func NormalizeKnowledgeWebScope(start string, scope entity.KnowledgeWebNetworkScope) (entity.KnowledgeWebNetworkScope, error) {
	if !scope.Enabled {
		if len(scope.AllowedOrigins) != 0 {
			return scope, webParseError("knowledge_invalid_request")
		}
		scope.AllowedOrigins = []string{}
		return scope, nil
	}
	if len(scope.AllowedOrigins) < 1 || len(scope.AllowedOrigins) > 4 {
		return scope, webParseError("knowledge_invalid_request")
	}
	seen := map[string]bool{}
	origins := []string{}
	for _, raw := range scope.AllowedOrigins {
		normalized, err := NormalizeKnowledgeWebURL("", raw, false)
		if err != nil {
			return scope, err
		}
		u, _ := url.Parse(normalized)
		if u.Path != "/" || u.RawQuery != "" || u.ForceQuery {
			return scope, webParseError("knowledge_invalid_request")
		}
		origin := knowledgeWebOrigin(normalized)
		if seen[origin] {
			return scope, webParseError("knowledge_invalid_request")
		}
		seen[origin] = true
		origins = append(origins, origin)
	}
	if !seen[knowledgeWebOrigin(start)] {
		return scope, webParseError("knowledge_invalid_request")
	}
	slices.Sort(origins)
	scope.AllowedOrigins = origins
	return scope, nil
}

func knowledgeWebProductionFetchIO() knowledgeWebFetchIO {
	return knowledgeWebFetchIO{now: time.Now, lookup: func(ctx context.Context, host string) ([]string, error) {
		return net.DefaultResolver.LookupHost(ctx, host)
	}, roundTrip: func(ctx context.Context, req *http.Request, address string) (*http.Response, error) {
		port := req.URL.Port()
		if port == "" {
			if req.URL.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: -1}
		transport := &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384, ResponseHeaderTimeout: 15 * time.Second, TLSHandshakeTimeout: 15 * time.Second, TLSClientConfig: &tls.Config{ServerName: req.URL.Hostname(), MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(address, port))
		}}
		return transport.RoundTrip(req.WithContext(ctx))
	}}
}

// FetchKnowledgeWebCapture performs one explicit anonymous attempt, using one
// overall deadline across DNS, TLS, redirects and the complete response body.
// It does not retry, parse, persist or confer acquisition authorization.
func FetchKnowledgeWebCapture(ctx context.Context, start string, scope entity.KnowledgeWebNetworkScope, authorize func(context.Context) error) (*KnowledgeWebFetchResult, error) {
	if authorize == nil {
		return nil, &KnowledgeWebFetchError{Code: "web_remote_blocked"}
	}
	network := knowledgeWebProductionFetchIO()
	network.authorize = authorize
	return fetchKnowledgeWebCapture(ctx, start, scope, network)
}

func fetchKnowledgeWebCapture(ctx context.Context, start string, scope entity.KnowledgeWebNetworkScope, network knowledgeWebFetchIO) (*KnowledgeWebFetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	start, err := NormalizeKnowledgeWebURL("", start, false)
	if err != nil {
		return nil, &KnowledgeWebFetchError{Code: "web_remote_blocked"}
	}
	scope, err = NormalizeKnowledgeWebScope(start, scope)
	if err != nil || !scope.Enabled {
		return nil, &KnowledgeWebFetchError{Code: "web_remote_blocked"}
	}
	started := network.now().UTC().Format(time.RFC3339)
	provenance := entity.KnowledgeWebProvenance{Kind: "server_http", RequestedURL: start, FetchStartedAt: &started, RedirectChain: []entity.KnowledgeWebRedirect{}}
	allowed := map[string]bool{}
	for _, o := range scope.AllowedOrigins {
		allowed[o] = true
	}
	current := start
	fail := func(code string, status *int64) (*KnowledgeWebFetchResult, error) {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = "web_fetch_timeout"
		}
		return nil, &KnowledgeWebFetchError{Code: code, UpstreamHTTPStatus: status}
	}
	for redirects := 0; redirects <= 3; redirects++ {
		if network.authorize != nil {
			if err := network.authorize(ctx); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, err
			}
			return fail("web_fetch_timeout", nil)
		}
		u, _ := url.Parse(current)
		if !allowed[knowledgeWebOrigin(current)] {
			return fail("web_redirect_denied", nil)
		}
		addresses, err := network.lookup(ctx, u.Hostname())
		if err != nil || len(addresses) == 0 {
			return fail("web_remote_blocked", nil)
		}
		for _, address := range addresses {
			if !common.IsPublicNetworkAddress(address) {
				return fail("web_remote_blocked", nil)
			}
		}
		if network.authorize != nil {
			if err := network.authorize(ctx); err != nil {
				return nil, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return fail("web_remote_blocked", nil)
		}
		req.Header.Set("Accept-Encoding", "identity")
		response, err := network.roundTrip(ctx, req, addresses[0])
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, ctx.Err()
			}
			return fail("web_result_unknown", nil)
		}
		if response == nil || response.Body == nil {
			return fail("web_result_unknown", nil)
		}
		headerBytes := 0
		for key, values := range response.Header {
			headerBytes += len(key) + 4
			for _, value := range values {
				headerBytes += len(value) + 2
			}
		}
		if headerBytes > 16384 {
			response.Body.Close()
			return fail("web_budget_exceeded", nil)
		}
		encoding := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding")))
		if response.Uncompressed || (encoding != "" && encoding != "identity") {
			response.Body.Close()
			return fail("web_compression_rejected", nil)
		}
		switch response.StatusCode {
		case 301, 302, 303, 307, 308:
			location := response.Header.Get("Location")
			response.Body.Close()
			if redirects == 3 || location == "" {
				return fail("web_redirect_denied", nil)
			}
			next, err := NormalizeKnowledgeWebURL(current, location, false)
			if err != nil || !allowed[knowledgeWebOrigin(next)] {
				return fail("web_redirect_denied", nil)
			}
			provenance.RedirectChain = append(provenance.RedirectChain, entity.KnowledgeWebRedirect{FromURL: current, ToURL: next, Status: int64(response.StatusCode)})
			current = next
			continue
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			status := int64(response.StatusCode)
			return fail("web_upstream_http_error", &status)
		}
		if response.ContentLength > knowledgeWebMaxBytes {
			response.Body.Close()
			return fail("web_too_large", nil)
		}
		contentType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil {
			response.Body.Close()
			return fail("web_format_unsupported", nil)
		}
		if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") && !strings.EqualFold(charset, "utf8") {
			response.Body.Close()
			return fail("web_encoding_unsupported", nil)
		}
		switch contentType {
		case "text/html", "application/rss+xml", "application/atom+xml", "application/xml", "text/xml":
		default:
			response.Body.Close()
			return fail("web_format_unsupported", nil)
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, knowledgeWebMaxBytes+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			return fail("web_result_unknown", nil)
		}
		if len(raw) > knowledgeWebMaxBytes {
			return fail("web_too_large", nil)
		}
		if len(raw) == 0 || (response.ContentLength >= 0 && response.ContentLength != int64(len(raw))) {
			return fail("web_length_mismatch", nil)
		}
		completed := network.now().UTC().Format(time.RFC3339)
		status := int64(200)
		identity := "identity"
		provenance.FinalURL = &current
		provenance.FetchCompletedAt = &completed
		provenance.ServerReceivedAt = &completed
		provenance.UpstreamHTTPStatus = &status
		provenance.ResponseContentType = knowledgeWebPointer(response.Header.Get("Content-Type"))
		provenance.ResponseContentEncoding = &identity
		etag, lastModified := response.Header.Get("ETag"), response.Header.Get("Last-Modified")
		if len(etag) > 256 || len(lastModified) > 128 {
			return fail("web_budget_exceeded", nil)
		}
		provenance.ResponseEtag = knowledgeWebNullable(etag)
		provenance.ResponseLastModified = knowledgeWebNullable(lastModified)
		return &KnowledgeWebFetchResult{Raw: raw, DeclaredMediaType: contentType, Provenance: provenance}, nil
	}
	return fail("web_redirect_denied", nil)
}
