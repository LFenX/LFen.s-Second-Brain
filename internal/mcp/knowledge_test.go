package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"ragflow/internal/entity"
)

type knowledgeFixture struct {
	Input   json.RawMessage `json:"input"`
	Success struct {
		Data json.RawMessage `json:"data"`
	} `json:"success"`
}

type knowledgeNativeProof struct{}
type knowledgeTestError struct{ code string }

func (e knowledgeTestError) Error() string { return "private detail that must not escape" }

func knowledgeFixtures(t *testing.T) map[string]knowledgeFixture {
	t.Helper()
	raw, err := os.ReadFile("knowledge_testdata.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]knowledgeFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func knowledgeTestOperations(t *testing.T) (KnowledgeOperations, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	fixtures := knowledgeFixtures(t)
	var inspect entity.KnowledgeLexicalReadinessData
	var search entity.KnowledgeLexicalSearchData
	var read entity.KnowledgeLexicalReadData
	if err := json.Unmarshal(fixtures["knowledge_inspect_local_index"].Success.Data, &inspect); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixtures["knowledge_search_local"].Success.Data, &search); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixtures["knowledge_read_local"].Success.Data, &read); err != nil {
		t.Fatal(err)
	}
	calls, revoked := &atomic.Int32{}, &atomic.Bool{}
	authorized := func(ctx context.Context, user, domain string) error {
		calls.Add(1)
		if ctx.Value(knowledgeNativeProof{}) != user {
			return errors.New("native proof lost")
		}
		if user != "synthetic-owner" || domain != inspect.Policy.DomainID {
			return knowledgeTestError{code: "not_found"}
		}
		return nil
	}
	ops := KnowledgeOperations{
		Authenticate: func(ctx context.Context, authorization string) (context.Context, string, error) {
			if revoked.Load() || (authorization != "Bearer synthetic-owner" && authorization != "Bearer synthetic-other") {
				return ctx, "", errors.New("denied")
			}
			user := strings.TrimPrefix(authorization, "Bearer ")
			return context.WithValue(ctx, knowledgeNativeProof{}, user), user, nil
		},
		Inspect: func(ctx context.Context, user, domain string, req entity.KnowledgeLexicalInspectRequest) (*entity.KnowledgeLexicalReadinessData, error) {
			if err := authorized(ctx, user, domain); err != nil {
				return nil, err
			}
			return &inspect, nil
		},
		Search: func(ctx context.Context, user, domain string, req entity.KnowledgeLexicalSearchRequest) (*entity.KnowledgeLexicalSearchData, error) {
			if err := authorized(ctx, user, domain); err != nil {
				return nil, err
			}
			return &search, nil
		},
		Read: func(ctx context.Context, user, domain string, req entity.KnowledgeLexicalReadRequest) (*entity.KnowledgeLexicalReadData, error) {
			if err := authorized(ctx, user, domain); err != nil {
				return nil, err
			}
			return &read, nil
		},
		Failure: func(err error) KnowledgeFailure {
			var public knowledgeTestError
			if errors.As(err, &public) {
				return KnowledgeFailure{Code: public.code, OutcomeKnown: true}
			}
			return KnowledgeFailure{Code: "unavailable", RetrySameKey: true}
		},
	}
	return ops, calls, revoked
}

func knowledgeTestServer(t *testing.T, operations KnowledgeOperations) (*Handler, *httptest.Server) {
	t.Helper()
	h, err := NewKnowledgeHandler(operations)
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(h)
	t.Cleanup(func() { h.Close(); hs.Close() })
	return h, hs
}

func knowledgeResultDTO(t *testing.T, result *sdk.CallToolResult) map[string]any {
	t.Helper()
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("missing single content block: %v", result)
	}
	text, ok := result.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatal("non-text result")
	}
	var dto map[string]any
	if err := json.Unmarshal([]byte(text.Text), &dto); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var structured any
	if err := json.Unmarshal(raw, &structured); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(structured, dto) {
		t.Fatal("structured/text result differs")
	}
	if !entity.ValidKnowledgeLexicalID(fmt.Sprint(dto["request_id"])) {
		t.Fatal("invalid response request id")
	}
	return dto
}

func TestKnowledgeMCPStaticDiscovery(t *testing.T) {
	ops, calls, _ := knowledgeTestOperations(t)
	_, hs := knowledgeTestServer(t, ops)
	ss := clientSession(t, hs.URL, "json", "synthetic-owner")
	listed, err := ss.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || len(listed.Tools) != 3 {
		t.Fatalf("discovery queried knowledge: calls=%d tools=%d", calls.Load(), len(listed.Tools))
	}
	wanted := map[string]bool{"knowledge_inspect_local_index": true, "knowledge_search_local": true, "knowledge_read_local": true}
	var templates []*sdk.Tool
	if err := json.Unmarshal(knowledgeToolsJSON, &templates); err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if !wanted[tool.Name] {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		delete(wanted, tool.Name)
		for _, template := range templates {
			if template.Name != tool.Name {
				continue
			}
			got, _ := json.Marshal(tool)
			want, _ := json.Marshal(template)
			if string(got) != string(want) {
				t.Fatalf("discovery descriptor changed for %s", tool.Name)
			}
		}
	}
	if len(wanted) != 0 {
		t.Fatal("missing tools")
	}
	for _, name := range []string{"ragflow_retrieval", "ragflow_list_datasets", "knowledge_change_local_policy", "knowledge_create_local_index"} {
		_, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err == nil {
			t.Fatalf("unknown tool %s was callable", name)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unknown tools invoked service")
	}
}

func TestKnowledgeMCPDelegatesNativeContextAndFixedDTO(t *testing.T) {
	ops, calls, _ := knowledgeTestOperations(t)
	fixtures := knowledgeFixtures(t)
	originalInspect, originalSearch, originalRead := ops.Inspect, ops.Search, ops.Read
	var forwarded []any
	ops.Inspect = func(ctx context.Context, user, domain string, req entity.KnowledgeLexicalInspectRequest) (*entity.KnowledgeLexicalReadinessData, error) {
		forwarded = append(forwarded, req)
		return originalInspect(ctx, user, domain, req)
	}
	ops.Search = func(ctx context.Context, user, domain string, req entity.KnowledgeLexicalSearchRequest) (*entity.KnowledgeLexicalSearchData, error) {
		forwarded = append(forwarded, req)
		return originalSearch(ctx, user, domain, req)
	}
	ops.Read = func(ctx context.Context, user, domain string, req entity.KnowledgeLexicalReadRequest) (*entity.KnowledgeLexicalReadData, error) {
		forwarded = append(forwarded, req)
		return originalRead(ctx, user, domain, req)
	}
	_, hs := knowledgeTestServer(t, ops)
	ss := clientSession(t, hs.URL, "json", "synthetic-owner")
	for _, name := range []string{"knowledge_inspect_local_index", "knowledge_search_local", "knowledge_read_local"} {
		fixture := fixtures[name]
		result, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: fixture.Input})
		if err != nil || result.IsError {
			t.Fatalf("%s: result=%v err=%v", name, result, err)
		}
		dto := knowledgeResultDTO(t, result)
		var expected any
		if err := json.Unmarshal(fixture.Success.Data, &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expected, dto["data"]) {
			t.Fatalf("response data changed for %s", name)
		}
		var request struct {
			Body any `json:"body"`
		}
		if err := json.Unmarshal(fixture.Input, &request); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(forwarded[len(forwarded)-1])
		var actual any
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(request.Body, actual) {
			t.Fatalf("fixed request body changed for %s", name)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestKnowledgeMCPRejectsClaimedIdentityAndImplicitScope(t *testing.T) {
	ops, calls, _ := knowledgeTestOperations(t)
	_, hs := knowledgeTestServer(t, ops)
	ss := clientSession(t, hs.URL, "json", "synthetic-owner")
	fixture := knowledgeFixtures(t)["knowledge_search_local"]
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["owner_id"] = "self-reported" },
		func(v map[string]any) { v["body"].(map[string]any)["embedding_model"] = "not-permitted" },
		func(v map[string]any) { delete(v["body"].(map[string]any), "selection") },
		func(v map[string]any) { delete(v["body"].(map[string]any), "index_ref") },
		func(v map[string]any) { v["operation"] = "createIndex" },
	} {
		var arguments map[string]any
		if err := json.Unmarshal(fixture.Input, &arguments); err != nil {
			t.Fatal(err)
		}
		change(arguments)
		result, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: "knowledge_search_local", Arguments: arguments})
		if err != nil || !result.IsError {
			t.Fatalf("invalid request: result=%v err=%v", result, err)
		}
		if dto := knowledgeResultDTO(t, result); dto["code"] != "invalid_request" || dto["data"] != nil {
			t.Fatalf("invalid failure=%v", dto)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached owning service")
	}
}

func TestKnowledgeMCPAuthenticationAndRevocation(t *testing.T) {
	ops, calls, revoked := knowledgeTestOperations(t)
	_, hs := knowledgeTestServer(t, ops)
	for _, headers := range []http.Header{{}, {"X-Api-Key": []string{"synthetic-owner"}}, {"Authorization": []string{"Bearer synthetic-invalid"}}} {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, hs.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		request.Header = headers
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated status=%d", response.StatusCode)
		}
	}
	owner := clientSession(t, hs.URL, "json", "synthetic-owner")
	other := clientSession(t, hs.URL, "json", "synthetic-other")
	fixture := knowledgeFixtures(t)["knowledge_read_local"]
	result, err := other.CallTool(t.Context(), &sdk.CallToolParams{Name: "knowledge_read_local", Arguments: fixture.Input})
	if err != nil || !result.IsError {
		t.Fatalf("other subject: %v %v", result, err)
	}
	dto := knowledgeResultDTO(t, result)
	if dto["code"] != "not_found" || dto["data"] != nil {
		t.Fatalf("owning-service refusal changed: %v", dto)
	}
	revoked.Store(true)
	if _, err := owner.ListTools(t.Context(), nil); err == nil {
		t.Fatal("revoked session reused discovery")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected service calls=%d", calls.Load())
	}
}

func TestKnowledgeMCPDoesNotPublishInvalidOutput(t *testing.T) {
	ops, _, _ := knowledgeTestOperations(t)
	ops.Read = func(context.Context, string, string, entity.KnowledgeLexicalReadRequest) (*entity.KnowledgeLexicalReadData, error) {
		return &entity.KnowledgeLexicalReadData{Text: "unvalidated private output"}, nil
	}
	_, hs := knowledgeTestServer(t, ops)
	ss := clientSession(t, hs.URL, "json", "synthetic-owner")
	fixture := knowledgeFixtures(t)["knowledge_read_local"]
	result, err := ss.CallTool(t.Context(), &sdk.CallToolParams{Name: "knowledge_read_local", Arguments: fixture.Input})
	if err != nil || !result.IsError {
		t.Fatalf("invalid output: %v %v", result, err)
	}
	dto := knowledgeResultDTO(t, result)
	if dto["code"] != "integrity_error" || dto["data"] != nil {
		t.Fatalf("failure=%v", dto)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "unvalidated private output") {
		t.Fatal("invalid service output leaked")
	}
}

func TestKnowledgeMCPCancellationKeepsNativeContext(t *testing.T) {
	ops, _, _ := knowledgeTestOperations(t)
	started := make(chan context.Context, 1)
	ops.Search = func(ctx context.Context, user, _ string, _ entity.KnowledgeLexicalSearchRequest) (*entity.KnowledgeLexicalSearchData, error) {
		if ctx.Value(knowledgeNativeProof{}) != user {
			return nil, errors.New("native proof lost")
		}
		started <- ctx
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, hs := knowledgeTestServer(t, ops)
	ss := clientSession(t, hs.URL, "json", "synthetic-owner")
	fixture := knowledgeFixtures(t)["knowledge_search_local"]
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ss.CallTool(ctx, &sdk.CallToolParams{Name: "knowledge_search_local", Arguments: fixture.Input})
	}()
	var active context.Context
	select {
	case active = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("service not called")
	}
	if deadline, ok := active.Deadline(); !ok || time.Until(deadline) > 60*time.Second {
		t.Fatal("missing bounded tool context")
	}
	cancel()
	select {
	case <-active.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("service did not observe cancellation")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not finish")
	}
}
