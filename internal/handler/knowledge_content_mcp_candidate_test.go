package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/service/dataset"
)

// The proposed production method is exercised with the real DatasetService
// type. Missing credentials stop before any DAO/native session operation.
// Gin is invoked directly; no socket/listener or live route is registered.
func TestContentKnowledgeCandidateAPIWiringRefusals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owner := &dataset.DatasetService{}
	handler := NewDatasetsHandler(owner, nil)
	if handler.datasetsService != owner {
		t.Fatal("dataset owner instance changed")
	}
	engine := gin.New()
	engine.POST("/api/v1/knowledge/mcp", handler.KnowledgeMCP)
	engine.POST("/api/v1/knowledge/content/mcp", handler.KnowledgeContentMCP)
	if len(engine.Routes()) != 2 {
		t.Fatal("unexpected route set")
	}
	for _, test := range []struct{ name, path, profile string }{
		{"old native route unchanged", "/api/v1/knowledge/mcp", ""},
		{"explicit profile missing", "/api/v1/knowledge/content/mcp", ""},
		{"native session missing", "/api/v1/knowledge/content/mcp", "knowledge-content-lexical/0.2.0-candidate"},
		{"old profile refused", "/api/v1/knowledge/content/mcp", "local_lexical/0.1.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Knowledge-MCP-Profile", test.profile)
			request.Header.Set("X-API-Key", "unit-key-never-authorizes")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d", response.Code)
			}
			if test.path == "/api/v1/knowledge/content/mcp" && response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("new private profile lacks no-store")
			}
		})
	}
}

func TestContentKnowledgeCandidateAPIWiringUnavailableOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewDatasetsHandler(nil, nil)
	engine.POST("/api/v1/knowledge/content/mcp", handler.KnowledgeContentMCP)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/content/mcp", nil))
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing owner must return noncacheable unavailable response")
	}
}

func TestContentKnowledgeCandidateNonPOSTNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	profile, err := NewKnowledgeContentMCPHandler(&dataset.DatasetService{})
	if err != nil {
		t.Fatal(err)
	}
	defer profile.Close()
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/content/mcp", nil)
	profile.HandleMCP(ctx)
	ctx.Writer.WriteHeaderNow()
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("method refusal must be explicit and noncacheable")
	}
}

func TestContentKnowledgeCandidateOwningFailureMapping(t *testing.T) {
	public := &dataset.KnowledgeLexicalError{Status: 409, Code: "policy_changed", OutcomeKnown: true}
	mapped := contentKnowledgeMCPFailure(fmt.Errorf("wrapped public error: %w", public))
	if mapped.Code != public.Code || mapped.OutcomeKnown != public.OutcomeKnown || mapped.RetrySameKey != public.RetrySameKey {
		t.Fatal("owning public failure fields changed")
	}
	unknown := contentKnowledgeMCPFailure(errors.New("unit unknown backend error"))
	if unknown.Code != "unavailable" || unknown.OutcomeKnown || !unknown.RetrySameKey {
		t.Fatal("unknown backend error escaped public failure mapping")
	}
}
