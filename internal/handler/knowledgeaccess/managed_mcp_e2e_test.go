//go:build e2e && linux

package knowledgeaccess

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ragflow/internal/entity"
)

type managedMCPLiveInput struct {
	Purpose        string                       `json:"purpose"`
	ReferenceID    string                       `json:"reference_id"`
	ManifestSHA256 string                       `json:"manifest_sha256"`
	RunID          string                       `json:"run_id"`
	ExpiresAt      time.Time                    `json:"expires_at"`
	Canonical      string                       `json:"canonical"`
	DialAddress    string                       `json:"dial_address"`
	Protocol       string                       `json:"protocol"`
	Mode           string                       `json:"mode"`
	Token          string                       `json:"token"`
	DomainID       string                       `json:"domain_id"`
	ModuleID       string                       `json:"module_id"`
	ModuleVersion  int64                        `json:"module_version"`
	NativeViewID   string                       `json:"native_view_id"`
	Target         entity.KnowledgeAccessTarget `json:"target"`
	DeniedSourceID string                       `json:"denied_source_id"`
	MaterialSHA256 string                       `json:"material_sha256"`
	MaterialBytes  int64                        `json:"material_bytes"`
}
type managedMCPTrace struct {
	Method        string          `json:"method"`
	Protocol      string          `json:"protocol"`
	Authenticated bool            `json:"authenticated"`
	Status        int             `json:"status"`
	Bytes         int             `json:"bytes"`
	Request       json.RawMessage `json:"request"`
	Response      json.RawMessage `json:"response"`
}
type managedMCPClientTransport struct {
	input    *managedMCPLiveInput
	next     http.RoundTripper
	mu       sync.Mutex
	requests int
	trace    []managedMCPTrace
}

func (r *managedMCPClientTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.requests++
	overBudget := r.requests > 40
	r.mu.Unlock()
	if request.URL.String() != r.input.Canonical || request.URL.RawQuery != "" || overBudget {
		return nil, errors.New("managed client endpoint or request budget refused")
	}
	var args []byte
	if request.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(request.Body, 32769))
		_ = request.Body.Close()
		if err != nil || len(raw) > 32768 {
			return nil, errors.New("managed request body refused")
		}
		args = raw
	}
	copy := request.Clone(request.Context())
	copy.Body = io.NopCloser(bytes.NewReader(args))
	copy.Header.Set("Authorization", "Bearer "+r.input.Token)
	response, err := r.next.RoundTrip(copy)
	if err != nil {
		return nil, errors.New("managed transport unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	_ = response.Body.Close()
	if err != nil || len(raw) > 1<<20 || bytes.Contains(raw, []byte(r.input.Token)) {
		clear(raw)
		return nil, errors.New("managed response refused")
	}
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Mcp-Session-Id") != "" || response.Header.Get("Location") != "" {
		clear(raw)
		return nil, errors.New("managed response policy refused")
	}
	if len(raw) > 0 && !json.Valid(raw) {
		clear(raw)
		return nil, errors.New("managed response JSON refused")
	}
	var outer struct {
		Result struct {
			IsError    *bool           `json:"isError"`
			Structured json.RawMessage `json:"structuredContent"`
			Content    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &outer) == nil && outer.Result.Structured != nil {
		if outer.Result.IsError == nil || len(outer.Result.Content) != 1 || outer.Result.Content[0].Type != "text" {
			clear(raw)
			return nil, errors.New("managed tool carriers refused")
		}
		var a, b any
		left, right := json.Unmarshal(outer.Result.Structured, &a), json.Unmarshal([]byte(outer.Result.Content[0].Text), &b)
		if left != nil || right != nil || !reflect.DeepEqual(a, b) {
			clear(raw)
			return nil, errors.New("managed tool carrier equality refused")
		}
	}
	r.mu.Lock()
	r.trace = append(r.trace, managedMCPTrace{Method: request.Method, Protocol: request.Header.Get("Mcp-Protocol-Version"), Authenticated: true, Status: response.StatusCode, Bytes: len(raw), Request: append(json.RawMessage(nil), args...), Response: append(json.RawMessage(nil), raw...)})
	r.mu.Unlock()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}
func managedMCPData[T any](t *testing.T, result *sdk.CallToolResult, operation, definition string) T {
	t.Helper()
	var zero T
	if result == nil || result.IsError || len(result.Content) != 1 || result.StructuredContent == nil {
		t.Fatal("managed live tool refused a permitted operation")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil || entity.KnowledgeAccessDecode(raw, definition, nil) != nil {
		t.Fatal("managed live tool response schema failed")
	}
	var envelope struct {
		Operation string `json:"operation"`
		Data      T      `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Operation != operation {
		t.Fatal("managed operation binding failed")
	}
	zero = envelope.Data
	return zero
}
func managedMCPFailure(t *testing.T, result *sdk.CallToolResult, operation, code string) {
	t.Helper()
	if result == nil || !result.IsError {
		t.Fatal("managed business error did not use isError")
	}
	raw, _ := json.Marshal(result.StructuredContent)
	var envelope entity.KnowledgeAccessError
	if entity.KnowledgeAccessDecode(raw, "Error", nil) != nil || json.Unmarshal(raw, &envelope) != nil || envelope.Operation != operation || envelope.Code != code || envelope.Data != nil {
		t.Fatal("managed business error envelope failed")
	}
}

// The private packet arrives only through stdin from the Root-owned isolated
// driver. The client has no native session, owner key, OAuth or target URL input.
func TestManagedMCPLiveScopedClient(t *testing.T) {
	stat, err := os.Stdin.Stat()
	if err != nil || stat.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("managed e2e input requires a private pipe")
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 65537))
	if err != nil || len(raw) > 65536 {
		t.Fatal("managed e2e input refused")
	}
	defer clear(raw)
	var input managedMCPLiveInput
	if entity.KnowledgeParsedDecode(raw, &input) != nil || input.Purpose != "root-synthetic-managed-mcp-e2e" || input.ReferenceID != "KCRL-20261116-44" || input.ManifestSHA256 != "df9a53d5bbbf483bc76ce72bda9e4643e937c6bd5b1a4f2e866b9a65745b8547" || len(input.RunID) != 32 || !time.Now().Before(input.ExpiresAt) || input.ExpiresAt.After(time.Now().Add(3*time.Minute)) || input.Canonical != "http://localhost:29398/api/v1/knowledge/access/mcp" || input.DialAddress != "second-brain-web-acceptance:8080" || input.Protocol != "2025-11-25" && input.Protocol != "2025-06-18" || input.Mode != "read" && input.Mode != "read_search" || len(input.Token) != 81 || !strings.HasPrefix(input.Token, "kac1_") || input.ModuleVersion != 1 || input.MaterialBytes != 9600 || input.MaterialSHA256 != "c18397c612a08773e373ffc2a56544363c1ab9430206e794a2766056cd12c6f7" {
		t.Fatal("managed e2e lease refused")
	}
	defer func() { input.Token = "" }()
	for _, id := range []string{input.DomainID, input.ModuleID, input.DeniedSourceID, input.RunID} {
		encoded, _ := json.Marshal(id)
		if entity.KnowledgeAccessDecode(encoded, "ID", nil) != nil {
			t.Fatal("managed e2e identifier refused")
		}
	}
	if input.NativeViewID != "" {
		encoded, _ := json.Marshal(input.NativeViewID)
		if entity.KnowledgeAccessDecode(encoded, "ID", nil) != nil {
			t.Fatal("managed e2e native view identifier refused")
		}
	}
	if input.Target.Source.DomainID != input.DomainID || input.Target.Source.SourceID == input.DeniedSourceID {
		t.Fatal("managed e2e source binding refused")
	}
	ctx, cancel := context.WithDeadline(t.Context(), input.ExpiresAt)
	defer cancel()
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "localhost:29398" {
			return nil, errors.New("managed dial target refused")
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, input.DialAddress)
	}
	defer base.CloseIdleConnections()
	trace := &managedMCPClientTransport{input: &input, next: base}
	clientHTTP := &http.Client{Transport: trace, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("managed redirect refused") }}
	client := sdk.NewClient(&sdk.Implementation{Name: "second-brain-synthetic-go-client", Version: "go-sdk-v1.8.0"}, &sdk.ClientOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: input.Canonical, HTTPClient: clientHTTP, DisableStandaloneSSE: true, MaxRetries: -1}, &sdk.ClientSessionOptions{ProtocolVersion: input.Protocol})
	if err != nil {
		t.Fatal("managed live official Go SDK initialization failed")
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	want := 2
	if input.Mode == "read_search" {
		want = 3
	}
	if err != nil || len(tools.Tools) != want {
		t.Fatal("managed live discovery is not mode bounded")
	}
	registry, _ := json.Marshal(tools)
	if bytes.Contains(registry, []byte(input.DomainID)) || bytes.Contains(registry, []byte(input.DeniedSourceID)) {
		t.Fatal("managed discovery exposed business identifiers")
	}
	call := func(name string, args any) *sdk.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal("managed live call failed before business result")
		}
		return result
	}
	if input.NativeViewID != "" {
		result := call("knowledge_access_read", entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: json.RawMessage(`{"kind":"unit","view_id":"` + input.NativeViewID + `","target_index":0,"unit_ordinal":0}`)})
		managedMCPFailure(t, result, "read", "not_found")
	}
	view := managedMCPData[entity.KnowledgeAccessInspectData](t, call("knowledge_access_inspect", entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: input.DomainID, ModuleID: input.ModuleID, Mode: input.Mode, Delivery: "web_and_mcp"}), "inspect", "InspectResponse")
	if view.ModuleID != input.ModuleID || view.DomainID != input.DomainID || view.ModuleVersion != input.ModuleVersion || view.Mode != input.Mode || view.Delivery != "web_and_mcp" || len(view.Targets) != 1 || view.SourceCount != 1 || view.UnitCount != 2 || !reflect.DeepEqual(view.Targets[0].Target, input.Target) {
		t.Fatal("managed live view widened or lost fixed scope")
	}
	var joined []byte
	var first entity.KnowledgeAccessReadData
	for ordinal := int64(0); ordinal < 2; ordinal++ {
		selector, _ := json.Marshal(entity.KnowledgeAccessUnitReadSelector{Kind: "unit", ViewID: view.ViewID, TargetIndex: 0, UnitOrdinal: ordinal})
		unit := managedMCPData[entity.KnowledgeAccessReadData](t, call("knowledge_access_read", entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: selector}), "read", "ReadResponse")
		if int64(len([]byte(unit.Text))) != unit.ByteLength || unit.ByteLength > 8192 || unit.Citation.Range.EndByte-unit.Citation.Range.StartByte != unit.ByteLength || !reflect.DeepEqual(unit.Citation.Target, input.Target) || unit.UnitLocator.ViewID != view.ViewID || unit.UnitLocator.UnitOrdinal != ordinal || unit.UnitLocator.TargetUnitCount != 2 {
			t.Fatal("managed live unit bounds or citation failed")
		}
		digest := sha256.Sum256([]byte(unit.Text))
		if hex.EncodeToString(digest[:]) != unit.Citation.Range.SHA256 {
			t.Fatal("managed live unit digest failed")
		}
		if ordinal == 0 {
			first = unit
			if unit.UnitLocator.NextUnitOrdinal == nil || *unit.UnitLocator.NextUnitOrdinal != 1 {
				t.Fatal("managed live first unit continuation failed")
			}
		} else if unit.UnitLocator.NextUnitOrdinal != nil {
			t.Fatal("managed live last unit continuation failed")
		}
		joined = append(joined, []byte(unit.Text)...)
	}
	digest := sha256.Sum256(joined)
	if int64(len(joined)) != input.MaterialBytes || hex.EncodeToString(digest[:]) != input.MaterialSHA256 {
		t.Fatal("managed live multi-unit reconstruction failed")
	}
	selector, _ := json.Marshal(entity.KnowledgeAccessEvidenceReadSelector{Kind: "evidence", EvidenceRef: first.EvidenceRef})
	reread := managedMCPData[entity.KnowledgeAccessReadData](t, call("knowledge_access_read", entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: selector}), "read", "ReadResponse")
	if reread.Text != first.Text || reread.EvidenceRef != first.EvidenceRef {
		t.Fatal("managed live evidence reread changed")
	}
	selector, _ = json.Marshal(entity.KnowledgeAccessUnitReadSelector{Kind: "unit", ViewID: view.ViewID, TargetIndex: 0, UnitOrdinal: 2})
	managedMCPFailure(t, call("knowledge_access_read", entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: selector}), "read", "invalid_request")
	if input.Mode == "read" {
		if _, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "knowledge_access_search", Arguments: entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "合成", Limit: 2}}); err == nil {
			t.Fatal("managed readonly credential searched")
		}
		trace.mu.Lock()
		last := trace.trace[len(trace.trace)-1]
		trace.mu.Unlock()
		var denied struct {
			Result json.RawMessage `json:"result"`
			Error  struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if last.Status != 200 || json.Unmarshal(last.Response, &denied) != nil || denied.Error.Code != -32602 || denied.Result != nil {
			t.Fatal("managed readonly search protocol mapping failed")
		}
	} else {
		search := managedMCPData[entity.KnowledgeAccessSearchData](t, call("knowledge_access_search", entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "合成", Limit: 2}), "search", "SearchResponse")
		if search.ReturnedCount != 2 || len(search.Hits) != 2 || search.TotalMatches != nil {
			t.Fatal("managed live scoped search failed")
		}
		for i, hit := range search.Hits {
			if hit.RankOrdinal != int64(i+1) || !reflect.DeepEqual(hit.Citation.Target, input.Target) {
				t.Fatal("managed live search widened or exposed ranking")
			}
		}
	}
	if err = session.Close(); err != nil {
		t.Fatal("managed live client close failed")
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	for _, row := range trace.trace {
		if bytes.Contains(row.Response, []byte(input.DeniedSourceID)) {
			t.Fatal("managed live response disclosed denied source")
		}
	}
	output, _ := json.Marshal(struct {
		State, SDK, Protocol, Mode string
		Calls                      []managedMCPTrace
	}{"Passed_ActualGoSDKScopedMCP", "github.com/modelcontextprotocol/go-sdk@v1.8.0", input.Protocol, input.Mode, trace.trace})
	t.Log(string(output))
}
