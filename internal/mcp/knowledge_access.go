package mcp

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// KnowledgeAccessOwner keeps credential and source authority in one service.
type KnowledgeAccessOwner interface {
	AuthenticateKnowledgeAccessMCP(context.Context, string, string) (context.Context, string, string, error)
	CheckKnowledgeAccessMCP(context.Context) error
	FinalizeKnowledgeAccessMCP(context.Context) error
	KnowledgeAccess(context.Context, string, string, string, []byte) (int, []byte)
}

type KnowledgeAccessFailure struct {
	Status                     int
	Code                       string
	OutcomeKnown, RetrySameKey bool
}

//go:embed knowledge_access_tools.json
var knowledgeAccessToolsJSON []byte

type accessCallPlan struct {
	mu        sync.Mutex
	operation string
	envelope  []byte
	rpcID     json.RawMessage
	authError error
}

func accessMCPError(operation, requestID, code string, known, retry bool) []byte {
	body, _ := json.Marshal(entity.KnowledgeAccessError{Operation: operation, ContractVersion: entity.KnowledgeAccessVersion, RequestID: requestID, Data: nil, Code: code, Message: "Knowledge access operation could not be completed", OutcomeKnown: known, RetrySameKey: retry})
	return body
}

func accessMCPTransportError(w http.ResponseWriter, requestID string, failure KnowledgeAccessFailure) {
	if failure.Status < 400 || failure.Status > 599 {
		failure = KnowledgeAccessFailure{Status: 503, Code: "unavailable", RetrySameKey: true}
	}
	if failure.Status == 401 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="knowledge-access",error="invalid_token"`)
	}
	accessMCPWrite(w, failure.Status, accessMCPError("mcpTransport", requestID, failure.Code, failure.OutcomeKnown, failure.RetrySameKey))
}

func accessMCPWrite(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("Transfer-Encoding")
	w.Header().Del("ETag")
	w.Header().Del("Mcp-Session-Id")
	w.Header().Del("Location")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// The outer request has four standard fields. Tool arguments retain their raw
// JSON so application DTO errors are handled by the owning tool, not transport.
func accessMCPOuter(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	var version, method string
	for decoder.More() {
		name, err := decoder.Token()
		key, ok := name.(string)
		if err != nil || !ok || seen[key] || key != "jsonrpc" && key != "id" && key != "method" && key != "params" {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
		switch key {
		case "jsonrpc":
			if json.Unmarshal(value, &version) != nil {
				return false
			}
		case "method":
			if entity.KnowledgeParsedDecode(value, &method) != nil {
				return false
			}
		case "id":
			var id any
			if entity.KnowledgeParsedDecode(value, &id) != nil {
				return false
			}
			switch id.(type) {
			case nil, string, float64, json.Number:
			default:
				return false
			}
		}
	}
	if _, err = decoder.Token(); err != nil {
		return false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return false
	}
	return version == "2.0" && method != ""
}

func accessMCPToolResult(body []byte) *sdk.CallToolResult {
	var value any
	_ = json.Unmarshal(body, &value)
	var envelope struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &envelope)
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(body)}}, StructuredContent: value, IsError: envelope.Code != ""}
}

func accessMCPRPCResult(id json.RawMessage, body []byte) ([]byte, error) {
	var envelope struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil, errors.New("invalid access response")
	}
	result := struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}{StructuredContent: body, IsError: envelope.Code != ""}
	result.Content = append(result.Content, struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{Type: "text", Text: string(body)})
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
}

func accessMCPOutput(buffer *knowledgeResponseBuffer, plan *accessCallPlan, requestID string) (int, []byte) {
	plan.mu.Lock()
	defer plan.mu.Unlock()
	status := buffer.status
	if status == 0 {
		status = 200
	}
	if status == 202 {
		return 202, nil
	}
	if status != 200 {
		code := "invalid_request"
		if status >= 500 {
			code = "unavailable"
		}
		return status, accessMCPError("mcpTransport", requestID, code, status < 500, status >= 500)
	}
	var rpc map[string]json.RawMessage
	if buffer.overflow && plan.operation != "" {
		fallback := accessMCPError(plan.operation, requestID, "capacity_exceeded", true, false)
		body, err := accessMCPRPCResult(plan.rpcID, fallback)
		if err == nil && len(body) <= knowledgeMaxResponseBytes {
			return 200, body
		}
		return 503, nil
	}
	if buffer.overflow || json.Unmarshal(buffer.body.Bytes(), &rpc) != nil {
		return 503, nil
	}
	if rawError, ok := rpc["error"]; ok {
		var problem struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(rawError, &problem) != nil {
			return 503, nil
		}
		rpc["error"], _ = json.Marshal(struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{problem.Code, "Invalid JSON-RPC request"})
		delete(rpc, "result")
		raw, err := json.Marshal(rpc)
		if err != nil || len(raw) > knowledgeMaxResponseBytes {
			return 503, nil
		}
		return 200, raw
	}
	if plan.operation != "" && rpc["result"] != nil {
		body, err := accessMCPRPCResult(rpc["id"], plan.envelope)
		if err == nil && len(body) <= knowledgeMaxResponseBytes {
			return 200, body
		}
		fallback := accessMCPError(plan.operation, requestID, "capacity_exceeded", true, false)
		body, err = accessMCPRPCResult(rpc["id"], fallback)
		if err != nil || len(body) > knowledgeMaxResponseBytes {
			return 503, nil
		}
		return 200, body
	}
	return 200, append([]byte(nil), buffer.body.Bytes()...)
}

// NewKnowledgeAccessHandler uses the official SDK for sessionless dispatch.
// Only the final bounded response reaches the network after fresh auth.
func NewKnowledgeAccessHandler(owner KnowledgeAccessOwner, failure func(error) KnowledgeAccessFailure, canonical string) (http.Handler, error) {
	if owner == nil || failure == nil {
		return nil, errors.New("knowledge access owner is incomplete")
	}
	endpoint, err := url.Parse(canonical)
	if err != nil || endpoint.Host == "" || endpoint.Path != "/api/v1/knowledge/access/mcp" {
		return nil, errors.New("knowledge access audience is unavailable")
	}
	var definitions []*sdk.Tool
	if json.Unmarshal(knowledgeAccessToolsJSON, &definitions) != nil || len(definitions) != 3 {
		return nil, errors.New("invalid access tool registry")
	}
	operations := map[string]string{"knowledge_access_inspect": "inspect", "knowledge_access_search": "search", "knowledge_access_read": "read"}
	responseDefinitions := map[string]string{"inspect": "InspectResponse", "search": "SearchResponse", "read": "ReadResponse"}
	seen := map[string]bool{}
	for _, tool := range definitions {
		if operations[tool.Name] == "" || seen[tool.Name] {
			return nil, errors.New("invalid access tool registry")
		}
		seen[tool.Name] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		requestID := common.GenerateUUID()
		for key := range w.Header() {
			if strings.HasPrefix(strings.ToLower(key), "access-control-") {
				w.Header().Del(key)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Knowledge-Access-Version", entity.KnowledgeAccessVersion)
		w.Header().Set("X-Request-ID", requestID)
		var authenticated context.Context
		defer func() {
			if recover() != nil {
				if authenticated != nil {
					if err := owner.FinalizeKnowledgeAccessMCP(authenticated); err != nil {
						accessMCPTransportError(w, requestID, failure(err))
						return
					}
				}
				accessMCPTransportError(w, requestID, KnowledgeAccessFailure{Status: 503, Code: "unavailable", RetrySameKey: true})
			}
		}()
		deny := func(status int, code string) {
			accessMCPTransportError(w, requestID, KnowledgeAccessFailure{Status: status, Code: code, OutcomeKnown: true})
		}
		origins := r.Header.Values("Origin")
		if r.Host != endpoint.Host || len(origins) > 1 || len(origins) == 1 && origins[0] != endpoint.Scheme+"://"+endpoint.Host {
			deny(403, "action_denied")
			return
		}
		if r.URL.Path != endpoint.Path || r.URL.RawPath != "" || r.URL.RawQuery != "" {
			deny(400, "invalid_request")
			return
		}
		versions := r.Header.Values("Mcp-Protocol-Version")
		if len(versions) > 1 || len(versions) == 1 && versions[0] != "2025-11-25" && versions[0] != "2025-06-18" {
			deny(400, "invalid_request")
			return
		}
		var raw []byte
		if r.Method == http.MethodPost {
			media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
				deny(415, "invalid_request")
				return
			}
			raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
			if err != nil {
				var limit *http.MaxBytesError
				if errors.As(err, &limit) {
					deny(413, "capacity_exceeded")
				} else {
					deny(400, "invalid_request")
				}
				return
			}
			if !accessMCPOuter(raw) {
				deny(400, "invalid_request")
				return
			}
		}
		authorizations := r.Header.Values("Authorization")
		if len(authorizations) != 1 {
			deny(401, "unauthenticated")
			return
		}
		var actor, mode string
		authenticated, actor, mode, err = owner.AuthenticateKnowledgeAccessMCP(ctx, authorizations[0], requestID)
		if err != nil {
			accessMCPTransportError(w, requestID, failure(err))
			return
		}
		if authenticated == nil || actor == "" || mode != "read" && mode != "read_search" {
			accessMCPTransportError(w, requestID, KnowledgeAccessFailure{Status: 503, Code: "unavailable", RetrySameKey: true})
			return
		}
		if r.Method != http.MethodPost {
			if err = owner.FinalizeKnowledgeAccessMCP(authenticated); err != nil {
				accessMCPTransportError(w, requestID, failure(err))
				return
			}
			w.Header().Set("Allow", "POST")
			accessMCPWrite(w, 405, nil)
			return
		}
		plan := &accessCallPlan{}
		var outer map[string]json.RawMessage
		_ = json.Unmarshal(raw, &outer)
		plan.rpcID = outer["id"]
		var method string
		_ = json.Unmarshal(outer["method"], &method)
		switch method {
		case "initialize", "ping", "tools/list", "tools/call", "notifications/initialized", "notifications/cancelled":
		default:
			if err = owner.FinalizeKnowledgeAccessMCP(authenticated); err != nil {
				accessMCPTransportError(w, requestID, failure(err))
				return
			}
			deny(400, "invalid_request")
			return
		}
		server := sdk.NewServer(&sdk.Implementation{Name: "knowledge-access-mcp-server", Version: entity.KnowledgeAccessVersion}, &sdk.ServerOptions{Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}}, SupportedProtocolVersions: []string{"2025-11-25", "2025-06-18"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		for _, tool := range definitions {
			operation := operations[tool.Name]
			if mode == "read" && operation == "search" {
				continue
			}
			server.AddTool(tool, func(callContext context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				call, stop := context.WithCancel(authenticated)
				defer stop()
				detach := context.AfterFunc(callContext, stop)
				defer detach()
				plan.mu.Lock()
				plan.operation = operation
				plan.mu.Unlock()
				if err := owner.CheckKnowledgeAccessMCP(call); err != nil {
					body := accessMCPError(operation, requestID, "unavailable", false, true)
					plan.mu.Lock()
					plan.authError = err
					plan.envelope = body
					plan.mu.Unlock()
					return accessMCPToolResult(body), nil
				}
				_, body := owner.KnowledgeAccess(call, actor, operation, requestID, request.Params.Arguments)
				if entity.KnowledgeAccessDecode(body, responseDefinitions[operation], nil) != nil {
					body = accessMCPError(operation, requestID, "integrity_error", true, false)
				}
				plan.mu.Lock()
				plan.envelope = body
				plan.mu.Unlock()
				return accessMCPToolResult(body), nil
			})
		}
		// Exact configured Host/Origin admission above protects reverse proxies
		// whose internal listener is loopback while the public host is HTTPS.
		transport := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 32768, PropagateRequestCancellation: true, DisableLocalhostProtection: true})
		buffer := &knowledgeResponseBuffer{header: make(http.Header)}
		request := r.Clone(authenticated)
		request.Body = io.NopCloser(bytes.NewReader(raw))
		transport.ServeHTTP(buffer, request)
		status, body := accessMCPOutput(buffer, plan, requestID)
		if err = owner.FinalizeKnowledgeAccessMCP(authenticated); err != nil {
			accessMCPTransportError(w, requestID, failure(err))
			return
		}
		plan.mu.Lock()
		authError := plan.authError
		plan.mu.Unlock()
		if authError != nil {
			accessMCPTransportError(w, requestID, failure(authError))
			return
		}
		accessMCPWrite(w, status, body)
	}), nil
}
