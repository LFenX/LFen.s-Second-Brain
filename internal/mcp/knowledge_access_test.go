package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ragflow/internal/entity"
)

const accessMCPTestEndpoint = "http://localhost/api/v1/knowledge/access/mcp"
const accessMCPTestBearer = "Bearer Synthetic_Managed_Credential_Marker"

type accessMCPTestFailure struct{ status int }

func (e accessMCPTestFailure) Error() string { return "synthetic unavailable" }

type accessMCPTestOwner struct {
	mu                                   sync.Mutex
	mode                                 string
	initialError, checkError, finalError error
	businessCode                         string
	invalidOutput                        bool
	auths, checks, finals, calls         int
}

func (o *accessMCPTestOwner) AuthenticateKnowledgeAccessMCP(ctx context.Context, header, _ string) (context.Context, string, string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.auths++
	if o.initialError != nil {
		return nil, "", "", o.initialError
	}
	if header != accessMCPTestBearer {
		return nil, "", "", accessMCPTestFailure{401}
	}
	mode := o.mode
	if mode == "" {
		mode = "read_search"
	}
	return ctx, strings.Repeat("a", 32), mode, nil
}
func (o *accessMCPTestOwner) CheckKnowledgeAccessMCP(context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.checks++
	return o.checkError
}
func (o *accessMCPTestOwner) FinalizeKnowledgeAccessMCP(context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.finals++
	return o.finalError
}
func (o *accessMCPTestOwner) KnowledgeAccess(_ context.Context, _, operation, requestID string, raw []byte) (int, []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	definition := map[string]string{"inspect": "InspectRequest", "search": "SearchRequest", "read": "ReadRequest"}[operation]
	if entity.KnowledgeAccessDecode(raw, definition, nil) != nil {
		return 400, accessMCPError(operation, requestID, "invalid_request", true, false)
	}
	if o.businessCode != "" {
		return 503, accessMCPError(operation, requestID, o.businessCode, true, false)
	}
	if o.invalidOutput {
		return 200, []byte(`{"private":"Rejected_Source_Marker"}`)
	}
	id := strings.Repeat("a", 32)
	target := entity.KnowledgeAccessTarget{Source: entity.KnowledgeAccessSource{DomainID: id, SourceID: id, SourceRevisionID: id, SourceSHA256: strings.Repeat("a", 64)}, Representation: entity.KnowledgeAccessRepresentation{Kind: "source_revision", ID: id}}
	data := entity.KnowledgeAccessInspectData{ViewID: id, DomainID: id, ModuleID: id, ModuleVersion: 1, Mode: "read", Delivery: "web_and_mcp", ExpiresAt: "2030-01-01T00:00:00Z", Targets: []entity.KnowledgeAccessTargetCatalogEntry{{Target: target, UnitCount: 2}}, SourceCount: 1, UnitCount: 2, Readiness: "ready", Pagination: "none"}
	body, _ := json.Marshal(struct {
		Operation string `json:"operation"`
		Version   string `json:"contract_version"`
		RequestID string `json:"request_id"`
		Data      any    `json:"data"`
	}{operation, entity.KnowledgeAccessVersion, requestID, data})
	return 200, body
}
func accessMCPTestMap(err error) KnowledgeAccessFailure {
	var failure accessMCPTestFailure
	if errors.As(err, &failure) {
		if failure.status == 401 {
			return KnowledgeAccessFailure{Status: 401, Code: "unauthenticated", OutcomeKnown: true}
		}
		return KnowledgeAccessFailure{Status: failure.status, Code: "unavailable", RetrySameKey: true}
	}
	return KnowledgeAccessFailure{Status: 503, Code: "unavailable", RetrySameKey: true}
}
func accessMCPTestHandler(t *testing.T, owner *accessMCPTestOwner, endpoint string) http.Handler {
	t.Helper()
	h, err := NewKnowledgeAccessHandler(owner, accessMCPTestMap, endpoint)
	if err != nil {
		t.Fatal("handler construction failed")
	}
	return h
}
func accessMCPTestRequest(method, body string) *http.Request {
	r := httptest.NewRequest(method, accessMCPTestEndpoint, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	r.Header.Set("Authorization", accessMCPTestBearer)
	return r
}
func accessMCPTestServe(t *testing.T, h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Length") != strconv.Itoa(w.Body.Len()) || w.Body.Len() > knowledgeMaxResponseBytes {
		t.Fatal("response byte/cache boundary failed")
	}
	for _, key := range []string{"Access-Control-Allow-Origin", "Mcp-Session-Id", "Location", "ETag"} {
		if w.Header().Get(key) != "" {
			t.Fatal("forbidden response header")
		}
	}
	if strings.Contains(w.Body.String(), "Synthetic_Managed_Credential_Marker") || strings.Contains(w.Body.String(), "Rejected_Source_Marker") {
		t.Fatal("private marker escaped")
	}
	return w
}
func accessMCPTestInspectArgs() map[string]any {
	return map[string]any{"contract_version": entity.KnowledgeAccessVersion, "domain_id": strings.Repeat("a", 32), "module_id": strings.Repeat("a", 32), "mode": "read", "delivery": "web_and_mcp", "source_ids": nil}
}
func accessMCPTestCall(name string, args any) string {
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	return string(raw)
}
func accessMCPTestToolEnvelope(t *testing.T, w *httptest.ResponseRecorder, wantError bool, wantCode string) {
	t.Helper()
	var rpc struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			IsError    *bool                         `json:"isError"`
			Structured json.RawMessage               `json:"structuredContent"`
			Content    []struct{ Type, Text string } `json:"content"`
		} `json:"result"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rpc) != nil || rpc.Error != nil || rpc.Result.IsError == nil || *rpc.Result.IsError != wantError || len(rpc.Result.Content) != 1 || rpc.Result.Content[0].Type != "text" {
		t.Fatal("tool wire mapping failed")
	}
	var structured, text any
	if json.Unmarshal(rpc.Result.Structured, &structured) != nil || json.Unmarshal([]byte(rpc.Result.Content[0].Text), &text) != nil {
		t.Fatal("invalid carrier JSON")
	}
	a, _ := json.Marshal(structured)
	b, _ := json.Marshal(text)
	if !bytes.Equal(a, b) {
		t.Fatal("tool carriers differ")
	}
	var envelope struct {
		Code string          `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(a, &envelope)
	if envelope.Code != wantCode || wantError && string(envelope.Data) != "null" || entity.KnowledgeAccessDecode(a, "InspectResponse", nil) != nil {
		t.Fatal("closed operation envelope failed")
	}
}

func TestKnowledgeAccessMCPTransportAdmission(t *testing.T) {
	valid := accessMCPTestCall("knowledge_access_inspect", accessMCPTestInspectArgs())
	for _, test := range []struct {
		name         string
		mutate       func(*http.Request)
		body         string
		status, auth int
	}{
		{"foreign host", func(r *http.Request) { r.Host = "foreign.invalid" }, valid, 403, 0},
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://foreign.invalid") }, valid, 403, 0},
		{"duplicate origin", func(r *http.Request) {
			r.Header.Add("Origin", "http://localhost")
			r.Header.Add("Origin", "http://localhost")
		}, valid, 403, 0},
		{"query", func(r *http.Request) { r.URL.RawQuery = "private=Rejected_Source_Marker" }, valid, 400, 0},
		{"unsupported content", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, valid, 415, 0},
		{"future protocol", func(r *http.Request) { r.Header.Set("Mcp-Protocol-Version", "2026-07-28") }, valid, 400, 0},
		{"duplicate protocol", func(r *http.Request) { r.Header.Add("Mcp-Protocol-Version", "2025-11-25") }, valid, 400, 0},
		{"bad accept", func(r *http.Request) { r.Header.Set("Accept", "application/json") }, valid, 400, 1},
		{"missing authorization", func(r *http.Request) { r.Header.Del("Authorization") }, valid, 401, 0},
		{"duplicate authorization", func(r *http.Request) { r.Header.Add("Authorization", accessMCPTestBearer) }, valid, 401, 0},
		{"native token", func(r *http.Request) { r.Header.Set("Authorization", "Synthetic_Native_Marker") }, valid, 401, 1},
		{"malformed JSON", nil, `{"jsonrpc":`, 400, 0},
		{"duplicate key", nil, `{"jsonrpc":"2.0","id":1,"id":2,"method":"ping"}`, 400, 0},
		{"batch", nil, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, 400, 0},
		{"oversized", nil, strings.Repeat(" ", 32769), 413, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := &accessMCPTestOwner{}
			r := accessMCPTestRequest("POST", test.body)
			if test.mutate != nil {
				test.mutate(r)
			}
			w := accessMCPTestServe(t, accessMCPTestHandler(t, o, accessMCPTestEndpoint), r)
			if w.Code != test.status || o.auths != test.auth || o.calls != 0 {
				t.Fatalf("admission status/auth/call mismatch: %d/%d/%d", w.Code, o.auths, o.calls)
			}
			if test.name == "bad accept" && o.finals != 1 {
				t.Fatal("SDK transport error skipped final auth")
			}
		})
	}
}

func TestKnowledgeAccessMCPRegistryAndExplicitCarriers(t *testing.T) {
	for _, mode := range []string{"read", "read_search"} {
		t.Run(mode, func(t *testing.T) {
			o := &accessMCPTestOwner{mode: mode}
			h := accessMCPTestHandler(t, o, accessMCPTestEndpoint)
			w := accessMCPTestServe(t, h, accessMCPTestRequest("POST", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
			var rpc struct {
				Result struct {
					Tools []*sdk.Tool `json:"tools"`
				} `json:"result"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rpc) != nil {
				t.Fatal("registry unavailable")
			}
			want := 2
			if mode == "read_search" {
				want = 3
			}
			if len(rpc.Result.Tools) != want {
				t.Fatal("registry exceeded current mode")
			}
			for _, tool := range rpc.Result.Tools {
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.InputSchema == nil || tool.OutputSchema == nil {
					t.Fatal("registry lacks closed schemas/read-only declaration")
				}
			}
			w = accessMCPTestServe(t, h, accessMCPTestRequest("POST", accessMCPTestCall("knowledge_access_inspect", accessMCPTestInspectArgs())))
			accessMCPTestToolEnvelope(t, w, false, "")
			if mode == "read" {
				w = accessMCPTestServe(t, h, accessMCPTestRequest("POST", accessMCPTestCall("knowledge_access_search", map[string]any{})))
				var denied struct {
					Result json.RawMessage `json:"result"`
					Error  struct {
						Code int `json:"code"`
					} `json:"error"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &denied) != nil || denied.Error.Code != -32602 || denied.Result != nil {
					t.Fatal("read-only search did not use SDK unavailable-tool mapping")
				}
			}
		})
	}
}

func TestKnowledgeAccessMCPBusinessAndProtocolErrors(t *testing.T) {
	for _, code := range []string{"action_denied", "not_found", "view_stale", "scope_not_supported", "index_stale", "unavailable"} {
		t.Run(code, func(t *testing.T) {
			o := &accessMCPTestOwner{businessCode: code}
			w := accessMCPTestServe(t, accessMCPTestHandler(t, o, accessMCPTestEndpoint), accessMCPTestRequest("POST", accessMCPTestCall("knowledge_access_inspect", accessMCPTestInspectArgs())))
			accessMCPTestToolEnvelope(t, w, true, code)
			if o.finals != 1 {
				t.Fatal("business error bypassed final auth")
			}
		})
	}
	o := &accessMCPTestOwner{}
	h := accessMCPTestHandler(t, o, accessMCPTestEndpoint)
	args := accessMCPTestInspectArgs()
	args["unknown"] = true
	accessMCPTestToolEnvelope(t, accessMCPTestServe(t, h, accessMCPTestRequest("POST", accessMCPTestCall("knowledge_access_inspect", args))), true, "invalid_request")
	for _, body := range []string{`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":17}}`, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unknown"}}`} {
		w := accessMCPTestServe(t, h, accessMCPTestRequest("POST", body))
		var rpc struct {
			Result json.RawMessage `json:"result"`
			Error  struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rpc) != nil || rpc.Error.Code != -32602 || rpc.Result != nil {
			t.Fatal("protocol error mapping failed")
		}
	}
	w := accessMCPTestServe(t, h, accessMCPTestRequest("POST", `{"jsonrpc":"2.0","id":1,"method":"unoffered"}`))
	if w.Code != 400 {
		t.Fatal("selected legacy SDK method status changed")
	}
}

func TestKnowledgeAccessMCPFinalAuthorityDiscardsAllPlannedResults(t *testing.T) {
	for _, status := range []int{401, 503} {
		for _, kind := range []string{"success", "business failure", "unknown tool", "unsupported method", "notification"} {
			t.Run(strconv.Itoa(status)+"/"+kind, func(t *testing.T) {
				o := &accessMCPTestOwner{finalError: accessMCPTestFailure{status}}
				body := accessMCPTestCall("knowledge_access_inspect", accessMCPTestInspectArgs())
				switch kind {
				case "business failure":
					o.businessCode = "not_found"
				case "unknown tool":
					body = accessMCPTestCall("unknown", nil)
				case "unsupported method":
					body = `{"jsonrpc":"2.0","id":1,"method":"unoffered"}`
				case "notification":
					body = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
				}
				w := accessMCPTestServe(t, accessMCPTestHandler(t, o, accessMCPTestEndpoint), accessMCPTestRequest("POST", body))
				var envelope entity.KnowledgeAccessError
				if w.Code != status || o.finals != 1 || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Operation != "mcpTransport" || envelope.Data != nil || strings.Contains(w.Body.String(), "structuredContent") || strings.Contains(w.Body.String(), "view_id") {
					t.Fatal("stale planned result escaped final authority")
				}
				if (w.Header().Get("WWW-Authenticate") != "") != (status == 401) {
					t.Fatal("authentication diagnostic misstates validity")
				}
			})
		}
	}
}

func TestKnowledgeAccessMCPFullResponseBudgetAndInvalidOutput(t *testing.T) {
	for _, text := range []string{strings.Repeat("a", knowledgeMaxResponseBytes*3/5), strings.Repeat(`"`, knowledgeMaxResponseBytes/4)} {
		body, _ := json.Marshal(map[string]string{"text": text})
		wire, err := accessMCPRPCResult(json.RawMessage(`7`), body)
		if err != nil || len(body) >= knowledgeMaxResponseBytes || len(wire) <= knowledgeMaxResponseBytes {
			t.Fatal("budget fixture invalid")
		}
		buffer := &knowledgeResponseBuffer{header: make(http.Header)}
		_, _ = buffer.Write(wire)
		status, out := accessMCPOutput(buffer, &accessCallPlan{operation: "inspect", envelope: body, rpcID: json.RawMessage(`7`)}, strings.Repeat("a", 32))
		w := httptest.NewRecorder()
		accessMCPWrite(w, status, out)
		accessMCPTestToolEnvelope(t, w, true, "capacity_exceeded")
		if bytes.Contains(out, body) || len(out) > knowledgeMaxResponseBytes {
			t.Fatal("overflow body retained")
		}
	}
	o := &accessMCPTestOwner{invalidOutput: true}
	w := accessMCPTestServe(t, accessMCPTestHandler(t, o, accessMCPTestEndpoint), accessMCPTestRequest("POST", accessMCPTestCall("knowledge_access_inspect", accessMCPTestInspectArgs())))
	accessMCPTestToolEnvelope(t, w, true, "integrity_error")
	buffer := &knowledgeResponseBuffer{header: make(http.Header)}
	_, _ = buffer.Write([]byte(`not JSON`))
	if status, out := accessMCPOutput(buffer, &accessCallPlan{}, strings.Repeat("a", 32)); status != 503 || len(out) != 0 {
		t.Fatal("invalid fallback emitted partial data")
	}
}

func TestKnowledgeAccessMCPMethodsNotificationsAndNoSessionAuthority(t *testing.T) {
	for _, method := range []string{"GET", "DELETE"} {
		for _, valid := range []bool{true, false} {
			o := &accessMCPTestOwner{}
			r := accessMCPTestRequest(method, "")
			r.Header.Set("Mcp-Session-Id", "Synthetic_Ignored_Session")
			if !valid {
				r.Header.Del("Authorization")
			}
			w := accessMCPTestServe(t, accessMCPTestHandler(t, o, accessMCPTestEndpoint), r)
			if valid {
				if w.Code != 405 || w.Body.Len() != 0 || w.Header().Get("Allow") != "POST" || o.finals != 1 {
					t.Fatal("authenticated method mapping failed")
				}
			} else if w.Code != 401 {
				t.Fatal("session ID borrowed authority")
			}
		}
	}
	for _, version := range []string{"2025-06-18", "2025-11-25"} {
		o := &accessMCPTestOwner{}
		h := accessMCPTestHandler(t, o, accessMCPTestEndpoint)
		r := accessMCPTestRequest("POST", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		r.Header.Set("Mcp-Protocol-Version", version)
		w := accessMCPTestServe(t, h, r)
		if w.Code != 202 || w.Body.Len() != 0 || o.finals != 1 {
			t.Fatal("notification mapping failed")
		}
	}
}

func TestKnowledgeAccessMCPRejectsUnadvertisedMethods(t *testing.T) {
	for _, method := range []string{"discovery", "prompts/list", "prompts/get", "resources/list", "resources/templates/list", "resources/read", "resources/subscribe", "subscriptions/listen", "completion/complete", "logging/setLevel", "notifications/roots/list_changed"} {
		t.Run(method, func(t *testing.T) {
			owner := &accessMCPTestOwner{}
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 9, "method": method, "params": map[string]any{}})
			response := accessMCPTestServe(t, accessMCPTestHandler(t, owner, accessMCPTestEndpoint), accessMCPTestRequest("POST", string(body)))
			if response.Code != 400 || owner.auths != 1 || owner.finals != 1 || owner.calls != 0 || strings.Contains(response.Body.String(), "result") {
				t.Fatal("unadvertised SDK capability became available")
			}
		})
	}
}

type accessMCPTestRoundTrip struct {
	target string
	next   http.RoundTripper
}

func (r accessMCPTestRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != r.target {
		return nil, errors.New("SDK request left exact endpoint")
	}
	copy := req.Clone(req.Context())
	copy.Header.Set("Authorization", accessMCPTestBearer)
	return r.next.RoundTrip(copy)
}

// This exercises the real named Go SDK over an httptest transport. Real-service
// credential/source authorization is separately covered by the e2e tier.
func TestKnowledgeAccessMCPGoSDKSessionlessClient(t *testing.T) {
	for _, version := range []string{"2025-06-18", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			o := &accessMCPTestOwner{mode: "read"}
			var handler http.Handler
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
			defer server.Close()
			endpoint := server.URL + "/api/v1/knowledge/access/mcp"
			handler = accessMCPTestHandler(t, o, endpoint)
			httpClient := &http.Client{Transport: accessMCPTestRoundTrip{target: endpoint, next: http.DefaultTransport}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }, Timeout: 5 * time.Second}
			client := sdk.NewClient(&sdk.Implementation{Name: "synthetic-restricted-client", Version: "1"}, &sdk.ClientOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, &sdk.ClientSessionOptions{ProtocolVersion: version})
			if err != nil {
				t.Fatal("official SDK initialization failed")
			}
			defer session.Close()
			tools, err := session.ListTools(ctx, nil)
			if err != nil || len(tools.Tools) != 2 {
				t.Fatal("official SDK registry failed")
			}
			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "knowledge_access_inspect", Arguments: accessMCPTestInspectArgs()})
			if err != nil || result.IsError || len(result.Content) != 1 || result.StructuredContent == nil {
				t.Fatal("official SDK inspect failed")
			}
			if _, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "knowledge_access_search", Arguments: map[string]any{}}); err == nil {
				t.Fatal("official SDK read-only search was offered")
			}
		})
	}
}
