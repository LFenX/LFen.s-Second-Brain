package mcp

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"ragflow/internal/entity"
)

// KnowledgeOperations delegates all knowledge authorization and reads to their
// owning service. Authenticate must preserve the native signed-session context.
type KnowledgeOperations struct {
	Authenticate func(context.Context, string) (context.Context, string, error)
	Inspect      func(context.Context, string, string, entity.KnowledgeLexicalInspectRequest) (*entity.KnowledgeLexicalReadinessData, error)
	Search       func(context.Context, string, string, entity.KnowledgeLexicalSearchRequest) (*entity.KnowledgeLexicalSearchData, error)
	Read         func(context.Context, string, string, entity.KnowledgeLexicalReadRequest) (*entity.KnowledgeLexicalReadData, error)
	Failure      func(error) KnowledgeFailure
}

// KnowledgeFailure carries only the owning service's public error fields.
type KnowledgeFailure struct {
	Code         string
	OutcomeKnown bool
	RetrySameKey bool
}

//go:embed knowledge_tools.json
var knowledgeToolsJSON []byte

type knowledgeTool struct {
	definition *sdk.Tool
	input      *jsonschema.Resolved
	output     *jsonschema.Resolved
}

// NewKnowledgeHandler exposes only static, read-only knowledge tools using the
// existing HTTP transport. It accepts Authorization only, with no key fallback.
func NewKnowledgeHandler(operations KnowledgeOperations) (*Handler, error) {
	if operations.Authenticate == nil || operations.Inspect == nil || operations.Search == nil || operations.Read == nil || operations.Failure == nil {
		return nil, errors.New("knowledge MCP operations are incomplete")
	}
	tools, err := loadKnowledgeTools()
	if err != nil {
		return nil, err
	}
	return newHTTPHandler(func(ctx context.Context, headers http.Header) (context.Context, string, error) {
		authorization := headers.Get("Authorization")
		if strings.TrimSpace(authorization) == "" {
			return ctx, "", errors.New("native session required")
		}
		return operations.Authenticate(ctx, authorization)
	}, func(ctx context.Context, userID string) *sdk.Server {
		return newKnowledgeServer(ctx, userID, operations, tools)
	}, Options{StreamableHTTP: true, JSONResponse: true}), nil
}

func loadKnowledgeTools() ([]knowledgeTool, error) {
	var definitions []*sdk.Tool
	if err := json.Unmarshal(knowledgeToolsJSON, &definitions); err != nil {
		return nil, errors.New("invalid knowledge MCP tool definitions")
	}
	if len(definitions) != 3 {
		return nil, errors.New("invalid knowledge MCP tool count")
	}
	tools := make([]knowledgeTool, 0, len(definitions))
	for _, definition := range definitions {
		input, err := resolveKnowledgeSchema(definition.InputSchema)
		if err != nil {
			return nil, err
		}
		output, err := resolveKnowledgeSchema(definition.OutputSchema)
		if err != nil {
			return nil, err
		}
		tools = append(tools, knowledgeTool{definition: definition, input: input, output: output})
	}
	return tools, nil
}

func resolveKnowledgeSchema(value any) (*jsonschema.Resolved, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("invalid knowledge MCP schema")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, errors.New("invalid knowledge MCP schema")
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, errors.New("unresolved knowledge MCP schema")
	}
	return resolved, nil
}

func newKnowledgeServer(requestContext context.Context, userID string, operations KnowledgeOperations, tools []knowledgeTool) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "knowledge-mcp-server", Version: entity.KnowledgeLocalLexicalVersion}, &sdk.ServerOptions{
		Capabilities:              &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
		SupportedProtocolVersions: []string{"2025-11-25", "2025-06-18"},
	})
	for _, tool := range tools {
		server.AddTool(tool.definition, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			// The transport's authenticated context contains the native-session proof.
			// Preserve it while also honoring SDK cancellation for this request.
			callContext, cancel := context.WithTimeout(requestContext, 60*time.Second)
			defer cancel()
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			return callKnowledgeTool(callContext, userID, operations, tool, request.Params.Arguments), nil
		})
	}
	return server
}

type knowledgeArguments[T any] struct {
	Operation string `json:"operation"`
	Path      struct {
		DomainID string `json:"domain_id"`
	} `json:"path"`
	Body T `json:"body"`
}

func callKnowledgeTool(ctx context.Context, userID string, operations KnowledgeOperations, tool knowledgeTool, raw json.RawMessage) *sdk.CallToolResult {
	operation := map[string]string{
		"knowledge_inspect_local_index": "inspectIndex",
		"knowledge_search_local":        "search",
		"knowledge_read_local":          "read",
	}[tool.definition.Name]
	invalid := &KnowledgeFailure{Code: "invalid_request", OutcomeKnown: true}
	var value any
	if !utf8.Valid(raw) || json.Unmarshal(raw, &value) != nil || tool.input.Validate(&value) != nil {
		return knowledgeResult(operation, nil, invalid, tool.output)
	}
	var data any
	var err error
	switch operation {
	case "inspectIndex":
		var input knowledgeArguments[entity.KnowledgeLexicalInspectRequest]
		if json.Unmarshal(raw, &input) != nil {
			return knowledgeResult(operation, nil, invalid, tool.output)
		}
		data, err = operations.Inspect(ctx, userID, input.Path.DomainID, input.Body)
	case "search":
		var input knowledgeArguments[entity.KnowledgeLexicalSearchRequest]
		if json.Unmarshal(raw, &input) != nil {
			return knowledgeResult(operation, nil, invalid, tool.output)
		}
		data, err = operations.Search(ctx, userID, input.Path.DomainID, input.Body)
	case "read":
		var input knowledgeArguments[entity.KnowledgeLexicalReadRequest]
		if json.Unmarshal(raw, &input) != nil {
			return knowledgeResult(operation, nil, invalid, tool.output)
		}
		data, err = operations.Read(ctx, userID, input.Path.DomainID, input.Body)
	default:
		return knowledgeResult(operation, nil, invalid, tool.output)
	}
	if err != nil {
		failure := operations.Failure(err)
		return knowledgeResult(operation, nil, &failure, tool.output)
	}
	if ctx.Err() != nil {
		failure := operations.Failure(ctx.Err())
		return knowledgeResult(operation, nil, &failure, tool.output)
	}
	return knowledgeResult(operation, data, nil, tool.output)
}

func knowledgeResult(operation string, data any, failure *KnowledgeFailure, output *jsonschema.Resolved) *sdk.CallToolResult {
	var id [16]byte
	rand.Read(id[:])
	envelope := map[string]any{
		"operation": operation, "contract_version": entity.KnowledgeLocalLexicalVersion,
		"request_id": hex.EncodeToString(id[:]), "data": data,
	}
	if failure != nil {
		envelope["data"] = nil
		envelope["code"] = failure.Code
		envelope["message"] = "Knowledge operation failed"
		envelope["outcome_known"] = failure.OutcomeKnown
		envelope["retry_same_key"] = failure.RetrySameKey
	}
	raw, err := json.Marshal(envelope)
	var value any
	if err != nil || json.Unmarshal(raw, &value) != nil || output.Validate(&value) != nil {
		// Invalid service output must never escape as partial or empty success.
		envelope["data"] = nil
		envelope["code"] = "integrity_error"
		envelope["message"] = "Knowledge result validation failed"
		envelope["outcome_known"] = true
		envelope["retry_same_key"] = false
		raw, _ = json.Marshal(envelope)
		failure = &KnowledgeFailure{Code: "integrity_error", OutcomeKnown: true}
	}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: string(raw)}},
		StructuredContent: json.RawMessage(raw), IsError: failure != nil,
	}
}
