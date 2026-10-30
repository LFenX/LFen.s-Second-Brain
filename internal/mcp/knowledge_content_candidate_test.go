package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ragflow/internal/entity"
)

type contentCandidateFixture struct {
	Inspect entity.KnowledgeContentLexicalInspectData `json:"inspect"`
	Search  entity.KnowledgeContentLexicalSearchData  `json:"search"`
	Reads   []entity.KnowledgeContentLexicalReadData  `json:"reads"`
}

func contentCandidateActual(t *testing.T) contentCandidateFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/content_candidate_actual_shapes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture contentCandidateFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func contentCandidateTools(t *testing.T) map[string]knowledgeTool {
	t.Helper()
	var definitions []*sdk.Tool
	if err := json.Unmarshal(contentKnowledgeToolsJSON, &definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 3 {
		t.Fatalf("tool count = %d", len(definitions))
	}
	tools := make(map[string]knowledgeTool, 3)
	for _, definition := range definitions {
		input, err := resolveKnowledgeSchema(definition.InputSchema)
		if err != nil {
			t.Fatalf("%s input: %v", definition.Name, contentCandidateResolverCause(definition.InputSchema))
		}
		output, err := resolveKnowledgeSchema(definition.OutputSchema)
		if err != nil {
			t.Fatalf("%s output: %v", definition.Name, contentCandidateResolverCause(definition.OutputSchema))
		}
		tools[definition.Name] = knowledgeTool{definition: definition, input: input, output: output}
	}
	for _, name := range []string{"knowledge_inspect_local_index", "knowledge_search_local", "knowledge_read_local"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("required tool missing: %s", name)
		}
	}
	return tools
}

func contentCandidateResolverCause(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return err
	}
	_, err = schema.Resolve(nil)
	return err
}

func contentCandidateArguments(operation, domain string, body any) map[string]any {
	return map[string]any{"profile_id": contentKnowledgeProfile, "operation": operation, "path": map[string]string{"domain_id": domain}, "body": body}
}

// Replays assigned actual owning Go data through the candidate Go schema and
// SDK result encoder. It does not call an owning service, native session or MCP.
func TestContentKnowledgeCandidateActualShapeEnvelope(t *testing.T) {
	fixture, tools := contentCandidateActual(t), contentCandidateTools(t)
	domain := fixture.Inspect.Policy.DomainID
	type item struct {
		name, operation string
		body, data      any
	}
	items := []item{
		{"knowledge_inspect_local_index", "inspectIndex", entity.KnowledgeContentLexicalInspectRequest{Version: entity.KnowledgeContentVersion, Selection: fixture.Search.Binding.Selection}, &fixture.Inspect},
		{"knowledge_search_local", "search", entity.KnowledgeContentLexicalSearchRequest{Version: entity.KnowledgeContentVersion, Index: fixture.Search.Binding.Index, Selection: fixture.Search.Binding.Selection, Query: fixture.Search.Query, Limit: fixture.Search.RequestedLimit}, &fixture.Search},
	}
	for i := range fixture.Reads {
		read := &fixture.Reads[i]
		items = append(items, item{"knowledge_read_local", "read", entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: read.Evidence}, read})
	}
	var exported []map[string]any
	for i, item := range items {
		tool := tools[item.name]
		itemDomain := domain
		if request, ok := item.body.(entity.KnowledgeContentLexicalReadRequest); ok {
			itemDomain = request.Evidence.Address.Target.Source.DomainID
		}
		arguments := contentCandidateArguments(item.operation, itemDomain, item.body)
		raw, err := json.Marshal(arguments)
		if err != nil {
			t.Fatal(err)
		}
		normalized, value, err := normalizeContentToolInput(raw)
		if err != nil {
			t.Fatalf("actual request %d rejected by Go closed schema: %v", i, err)
		}
		if err := tool.input.Validate(&value); err != nil {
			t.Fatalf("actual request %d rejected by Go closed schema: %v", i, err)
		}
		if item.operation == "read" {
			var typed contentToolArguments[entity.KnowledgeContentLexicalReadRequest]
			if err := decodeContentTypedInput(normalized, &typed); err != nil {
				t.Fatalf("actual request %d rejected by the same frozen owning codecs: %v", i, err)
			}
		}
		result := contentKnowledgeResult(tool, item.operation, item.data, nil)
		if result.IsError {
			t.Fatalf("actual data %d became an error", i)
		}
		exported = append(exported, map[string]any{"tool_name": item.name, "arguments": arguments, "result": result})
	}
	if len(items) != 9 {
		t.Fatalf("assigned actual-data envelope count = %d", len(items))
	}
	if output := os.Getenv("LFEN_MCP_CANDIDATE_PUBLIC_OUTPUT"); output != "" {
		raw, err := json.MarshalIndent(map[string]any{"status": "Passed_GoCandidateUnitEncoder_ActualDataReplayOnly", "network_requests": 0, "actual_owning_calls": 0, "actual_MCP_calls": 0, "envelopes": exported}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// A domain whose local lexical policy was never enabled reports version 0.
// Only the policy view admits it; captured job and binding versions stay >= 1.
func TestContentKnowledgeCandidateDisabledDefaultPolicy(t *testing.T) {
	fixture, tools := contentCandidateActual(t), contentCandidateTools(t)
	tool := tools["knowledge_inspect_local_index"]
	for _, test := range []struct {
		name    string
		version int64
		valid   bool
	}{
		{"never enabled", 0, true},
		{"negative", -1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			inspect := fixture.Inspect
			inspect.Policy.LocalVersion = test.version
			inspect.Policy.LocalEnabled = false
			inspect.Policy.Enabled = false
			if result := contentKnowledgeResult(tool, "inspectIndex", &inspect, nil); result.IsError == test.valid {
				t.Fatalf("version %d: error=%v, expected valid=%v", test.version, result.IsError, test.valid)
			}
		})
	}
	search := fixture.Search
	search.Binding.LocalVersion = 0
	if result := contentKnowledgeResult(tools["knowledge_search_local"], "search", &search, nil); !result.IsError {
		t.Fatal("binding with local policy version 0 accepted")
	}
}

func TestContentKnowledgeCandidateSDKRegistration(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "candidate-unit-registry", Version: entity.KnowledgeContentVersion}, nil)
	for _, tool := range contentCandidateTools(t) {
		server.AddTool(tool.definition, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return nil, errors.New("unit registry is never dispatched")
		})
	}
}

func TestContentKnowledgeCandidateStrictNumberCodec(t *testing.T) {
	cases := []struct {
		name, raw string
		valid     bool
	}{
		{"integer", `{"n":1}`, true},
		{"integral decimal", `{"n":1.0}`, true},
		{"integral exponent", `{"n":10e-1}`, true},
		{"zero exponent bound", `{"n":0e128}`, true},
		{"quoted numeric literal", `{"n":"1e900"}`, true},
		{"underflow fraction", `{"n":1e-400}`, false},
		{"rounded fraction", `{"n":1.00000000000000000001}`, false},
		{"zero exponent over bound", `{"n":0e129}`, false},
		{"zero negative exponent over bound", `{"n":0e-129}`, false},
		{"zero numeric token over bound", `{"n":0.` + strings.Repeat("0", 129) + `}`, false},
		{"decoded duplicate key", `{"n":1,"\u006e":2}`, false},
		{"unpaired surrogate", `{"n":"\ud800"}`, false},
		{"trailing JSON", `{"n":1} false`, false},
		{"invalid UTF8", string([]byte{0xff}), false},
		{"input capacity", strings.Repeat(" ", (1<<20)+1), false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := normalizeContentToolInput([]byte(test.raw))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, expected=%v", err == nil, test.valid)
			}
		})
	}
}

func TestContentKnowledgeCandidateClosedInputAndNullables(t *testing.T) {
	fixture, tools := contentCandidateActual(t), contentCandidateTools(t)
	body := entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: fixture.Reads[0].Evidence}
	arguments := contentCandidateArguments("read", fixture.Inspect.Policy.DomainID, body)
	raw, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	base := string(raw)
	if !strings.Contains(base, `"original_position":null`) {
		t.Fatal("expected nullable input fixture missing")
	}
	cases := []struct {
		name, raw string
	}{
		{"missing required null", strings.Replace(base, `"original_position":null,`, "", 1)},
		{"wrong profile", strings.Replace(base, contentKnowledgeProfile, "local_lexical/0.1.0", 1)},
		{"write operation", strings.Replace(base, `"operation":"read"`, `"operation":"createIndex"`, 1)},
		{"extra identity field", strings.Replace(base, `"path":{`, `"owner_id":"caller-chosen","path":{`, 1)},
		{"old operation name", strings.Replace(base, `"operation":"read"`, `"operation":"lexical-evidence-read"`, 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, value, err := normalizeContentToolInput([]byte(test.raw))
			if err == nil && tools["knowledge_read_local"].input.Validate(&value) == nil {
				t.Fatal("closed input accepted")
			}
		})
	}
}

func TestContentKnowledgeCandidateInvalidDataCannotEscape(t *testing.T) {
	tool := contentCandidateTools(t)["knowledge_read_local"]
	for _, data := range []any{map[string]any{"text": "incomplete success"}, func() {}} {
		result := contentKnowledgeResult(tool, "read", data, nil)
		if !result.IsError {
			t.Fatal("invalid data reported as success")
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var envelope map[string]any
		if json.Unmarshal(raw, &envelope) != nil || envelope["data"] != nil || envelope["code"] != "integrity_error" {
			t.Fatal("invalid result did not become a controlled null-data error")
		}
	}
}

type contentCandidateDenyOwner struct{ authCalls, owningCalls int }

func (o *contentCandidateDenyOwner) AuthenticateKnowledgeLocalSession(context.Context, string) (context.Context, string, error) {
	o.authCalls++
	return nil, "", errors.New("unit owner refuses native session")
}
func (o *contentCandidateDenyOwner) InspectKnowledgeContentLexical(context.Context, string, string, entity.KnowledgeContentLexicalInspectRequest) (*entity.KnowledgeContentLexicalInspectData, error) {
	o.owningCalls++
	return nil, errors.New("unit owner refuses operation")
}
func (o *contentCandidateDenyOwner) SearchKnowledgeContentLexical(context.Context, string, string, entity.KnowledgeContentLexicalSearchRequest) (*entity.KnowledgeContentLexicalSearchData, error) {
	o.owningCalls++
	return nil, errors.New("unit owner refuses operation")
}
func (o *contentCandidateDenyOwner) ReadKnowledgeContentLexical(context.Context, string, string, entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error) {
	o.owningCalls++
	return nil, errors.New("unit owner refuses operation")
}

// ServeHTTP is called directly with ResponseRecorder. No socket, listener,
// standard-SDK client, successful native identity or real MCP call is created.
func TestContentKnowledgeCandidateNativeRefusals(t *testing.T) {
	owner := &contentCandidateDenyOwner{}
	handler, err := NewContentKnowledgeHandler(owner, func(error) KnowledgeFailure { return KnowledgeFailure{Code: "unavailable"} })
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	for _, test := range []struct{ name, profile, authorization string }{
		{"no profile", "", ""},
		{"no native session", contentKnowledgeProfile, ""},
		{"API key alone", contentKnowledgeProfile, ""},
		{"non-native credential denied", contentKnowledgeProfile, "not-a-native-session"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Knowledge-MCP-Profile", test.profile)
			request.Header.Set("Authorization", test.authorization)
			request.Header.Set("X-API-Key", "unit-key-never-authorizes")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", response.Code)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("native refusal response must be no-store")
			}
		})
	}
	if owner.authCalls != 1 || owner.owningCalls != 0 {
		t.Fatalf("auth calls=%d, owning calls=%d", owner.authCalls, owner.owningCalls)
	}
}
