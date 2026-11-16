package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ragflow/internal/entity"
)

const contentKnowledgeProfile = "knowledge-content-lexical/0.2.0-candidate"

// ContentKnowledgeOwner matches the exact assigned DatasetService methods.
// All current native/index/policy/source/manifest/member/range/SHA checks,
// metadata assembly and budgets belong to these owning methods, not MCP.
type ContentKnowledgeOwner interface {
	AuthenticateKnowledgeLocalSession(context.Context, string) (context.Context, string, error)
	InspectKnowledgeContentLexical(context.Context, string, string, entity.KnowledgeContentLexicalInspectRequest) (*entity.KnowledgeContentLexicalInspectData, error)
	SearchKnowledgeContentLexical(context.Context, string, string, entity.KnowledgeContentLexicalSearchRequest) (*entity.KnowledgeContentLexicalSearchData, error)
	ReadKnowledgeContentLexical(context.Context, string, string, entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error)
}

//go:embed knowledge_content_tools.json
var contentKnowledgeToolsJSON []byte

type contentNativeAuthorizationKey struct{}

type contentToolArguments[T any] struct {
	ProfileID string `json:"profile_id"`
	Operation string `json:"operation"`
	Path      struct {
		DomainID string `json:"domain_id"`
	} `json:"path"`
	Body T `json:"body"`
}

// NewContentKnowledgeHandler serves three read-only tools through the owning
// DatasetService methods. Profile selection and native authentication are
// explicit; source authority and evidence assembly remain with that service.
func NewContentKnowledgeHandler(owner ContentKnowledgeOwner, failure func(error) KnowledgeFailure) (*Handler, error) {
	if owner == nil || failure == nil {
		return nil, errors.New("content knowledge owner is incomplete")
	}
	var definitions []*sdk.Tool
	if json.Unmarshal(contentKnowledgeToolsJSON, &definitions) != nil || len(definitions) != 3 {
		return nil, errors.New("invalid content knowledge registry")
	}
	operations := map[string]string{"knowledge_inspect_local_index": "inspectIndex", "knowledge_search_local": "search", "knowledge_read_local": "read"}
	seen := map[string]bool{}
	tools := make([]knowledgeTool, 0, 3)
	for _, definition := range definitions {
		if operations[definition.Name] == "" || seen[definition.Name] {
			return nil, errors.New("unexpected content tool")
		}
		seen[definition.Name] = true
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
	handler := newHTTPHandler(func(ctx context.Context, headers http.Header) (context.Context, string, error) {
		if headers.Get("X-Knowledge-MCP-Profile") != contentKnowledgeProfile {
			return nil, "", errors.New("explicit content profile required")
		}
		authorization := headers.Get("Authorization")
		if strings.TrimSpace(authorization) == "" {
			return nil, "", errors.New("native session required")
		}
		authenticated, user, err := owner.AuthenticateKnowledgeLocalSession(ctx, authorization)
		if err != nil || authenticated == nil || user == "" {
			return nil, "", errors.New("native session refused")
		}
		return context.WithValue(authenticated, contentNativeAuthorizationKey{}, authorization), user, nil
	}, func(requestContext context.Context, user string) *sdk.Server {
		server := sdk.NewServer(&sdk.Implementation{Name: "knowledge-content-mcp-server", Version: entity.KnowledgeContentVersion}, &sdk.ServerOptions{Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}}, SupportedProtocolVersions: []string{"2025-11-25", "2025-06-18"}})
		for _, tool := range tools {
			server.AddTool(tool.definition, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				call, cancel := context.WithTimeout(requestContext, 60*time.Second)
				defer cancel()
				stop := context.AfterFunc(ctx, cancel)
				defer stop()
				operation := operations[tool.definition.Name]
				authorization, _ := call.Value(contentNativeAuthorizationKey{}).(string)
				current, actualUser, err := owner.AuthenticateKnowledgeLocalSession(call, authorization)
				if err != nil || current == nil || actualUser == "" || actualUser != user {
					return contentKnowledgeResult(tool, operation, nil, &KnowledgeFailure{Code: "unauthenticated", OutcomeKnown: true}), nil
				}
				raw, value, err := normalizeContentToolInput(request.Params.Arguments)
				if err != nil || tool.input.Validate(&value) != nil {
					return contentKnowledgeResult(tool, operation, nil, &KnowledgeFailure{Code: "invalid_request", OutcomeKnown: true}), nil
				}
				var data any
				switch operation {
				case "inspectIndex":
					var args contentToolArguments[entity.KnowledgeContentLexicalInspectRequest]
					if decodeContentTypedInput(raw, &args) != nil {
						return contentKnowledgeResult(tool, operation, nil, &KnowledgeFailure{Code: "invalid_request", OutcomeKnown: true}), nil
					}
					data, err = owner.InspectKnowledgeContentLexical(current, user, args.Path.DomainID, args.Body)
				case "search":
					var args contentToolArguments[entity.KnowledgeContentLexicalSearchRequest]
					if decodeContentTypedInput(raw, &args) != nil {
						return contentKnowledgeResult(tool, operation, nil, &KnowledgeFailure{Code: "invalid_request", OutcomeKnown: true}), nil
					}
					data, err = owner.SearchKnowledgeContentLexical(current, user, args.Path.DomainID, args.Body)
				case "read":
					var args contentToolArguments[entity.KnowledgeContentLexicalReadRequest]
					if decodeContentTypedInput(raw, &args) != nil {
						return contentKnowledgeResult(tool, operation, nil, &KnowledgeFailure{Code: "invalid_request", OutcomeKnown: true}), nil
					}
					// Owner returns already proven and bounded typed enrichment.
					// No node SQL, source read, range expansion or permission logic here.
					data, err = owner.ReadKnowledgeContentLexical(current, user, args.Path.DomainID, args.Body)
				}
				if err != nil {
					public := failure(err)
					return contentKnowledgeResult(tool, operation, nil, &public), nil
				}
				if current.Err() != nil {
					return contentKnowledgeResult(tool, operation, nil, &KnowledgeFailure{Code: "unavailable", OutcomeKnown: true}), nil
				}
				return contentKnowledgeResult(tool, operation, data, nil), nil
			})
		}
		return server
	}, Options{StreamableHTTP: true, JSONResponse: true})
	handler.handler = contentKnowledgeBoundedHTTP(handler.handler)
	return handler, nil
}

// Input numbers are integers. Finite decimal scores/positions occur in output.
// This codec is profile-local and does not alter the original tool decoder.
func normalizeContentToolInput(raw []byte) ([]byte, any, error) {
	if len(raw) > 1<<20 {
		return nil, nil, errors.New("input capacity")
	}
	var preserved json.RawMessage
	if entity.KnowledgeParsedDecode(raw, &preserved) != nil {
		return nil, nil, errors.New("invalid strict JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, nil, errors.New("invalid JSON")
	}
	var normalize func(any) (any, error)
	normalize = func(value any) (any, error) {
		switch v := value.(type) {
		case json.Number:
			token := string(v)
			if len(token) > 128 {
				return nil, errors.New("numeric token capacity")
			}
			if i := strings.IndexAny(token, "eE"); i >= 0 {
				n, err := strconv.ParseInt(token[i+1:], 10, 32)
				if err != nil || n < -128 || n > 128 {
					return nil, errors.New("numeric exponent capacity")
				}
			}
			rational, ok := new(big.Rat).SetString(token)
			if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
				return nil, errors.New("nonintegral request number")
			}
			return rational.Num().Int64(), nil
		case []any:
			for i := range v {
				x, err := normalize(v[i])
				if err != nil {
					return nil, err
				}
				v[i] = x
			}
		case map[string]any:
			for key := range v {
				x, err := normalize(v[key])
				if err != nil {
					return nil, err
				}
				v[key] = x
			}
		}
		return value, nil
	}
	normalized, err := normalize(value)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, err
	}
	if json.Unmarshal(encoded, &value) != nil {
		return nil, nil, errors.New("invalid normalized JSON")
	}
	return encoded, value, nil
}

func decodeContentTypedInput(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func contentKnowledgeResult(tool knowledgeTool, operation string, data any, failure *KnowledgeFailure) *sdk.CallToolResult {
	var id [16]byte
	rand.Read(id[:])
	requestID := hex.EncodeToString(id[:])
	body := map[string]any{"contract_version": entity.KnowledgeContentVersion, "operation": operation, "request_id": requestID, "data": data}
	if failure != nil {
		body["data"], body["code"], body["message"] = nil, failure.Code, "Content lexical operation could not be completed"
		body["outcome_known"], body["retry_same_key"] = failure.OutcomeKnown, failure.RetrySameKey
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return contentKnowledgeFailureResult(operation, requestID, "integrity_error")
	}
	if len(raw) > knowledgeMaxResponseBytes {
		return contentKnowledgeFailureResult(operation, requestID, "capacity_exceeded")
	}
	var value any
	if json.Unmarshal(raw, &value) != nil || tool.output.Validate(&value) != nil {
		return contentKnowledgeFailureResult(operation, requestID, "integrity_error")
	}
	result := contentKnowledgeEncodedResult(raw, failure != nil)
	encoded, err := json.Marshal(result)
	if err != nil {
		return contentKnowledgeFailureResult(operation, requestID, "integrity_error")
	}
	if len(encoded) > knowledgeMaxResponseBytes {
		return contentKnowledgeFailureResult(operation, requestID, "capacity_exceeded")
	}
	return result
}

func contentKnowledgeEncodedResult(raw []byte, isError bool) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(raw)}}, StructuredContent: json.RawMessage(raw), IsError: isError}
}

func contentKnowledgeFailureResult(operation, requestID, code string) *sdk.CallToolResult {
	raw, _ := json.Marshal(map[string]any{"contract_version": entity.KnowledgeContentVersion, "operation": operation, "request_id": requestID, "data": nil, "code": code, "message": "Content lexical operation could not be completed", "outcome_known": true, "retry_same_key": false})
	return contentKnowledgeEncodedResult(raw, true)
}
