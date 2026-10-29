package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"ragflow/internal/entity"
	"ragflow/internal/mcp"
)

type knowledgeAPIHeaderTransport struct{ base http.RoundTripper }

func (h knowledgeAPIHeaderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer synthetic-native-session")
	return h.base.RoundTrip(request)
}

func TestKnowledgeMCPAPIProfileUsesNativeTransport(t *testing.T) {
	operations := mcp.KnowledgeOperations{
		Authenticate: func(ctx context.Context, authorization string) (context.Context, string, error) {
			if authorization != "Bearer synthetic-native-session" {
				return ctx, "", errors.New("denied")
			}
			return ctx, "synthetic-owner", nil
		},
		Inspect: func(context.Context, string, string, entity.KnowledgeLexicalInspectRequest) (*entity.KnowledgeLexicalReadinessData, error) {
			panic("discovery must not read knowledge")
		},
		Search: func(context.Context, string, string, entity.KnowledgeLexicalSearchRequest) (*entity.KnowledgeLexicalSearchData, error) {
			panic("discovery must not search")
		},
		Read: func(context.Context, string, string, entity.KnowledgeLexicalReadRequest) (*entity.KnowledgeLexicalReadData, error) {
			panic("discovery must not read content")
		},
		Failure: func(error) mcp.KnowledgeFailure { return mcp.KnowledgeFailure{Code: "unavailable", RetrySameKey: true} },
	}
	transport, err := mcp.NewKnowledgeHandler(operations)
	if err != nil {
		t.Fatal(err)
	}
	handler := &KnowledgeMCPHandler{transport: transport}
	defer handler.Close()
	engine := gin.New()
	engine.POST("/api/v1/knowledge/mcp", handler.HandleMCP)
	server := httptest.NewServer(engine)
	defer server.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "knowledge-api-test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: server.URL + "/api/v1/knowledge/mcp", HTTPClient: &http.Client{Transport: knowledgeAPIHeaderTransport{http.DefaultTransport}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil || len(listed.Tools) != 3 {
		t.Fatalf("tools=%v error=%v", listed, err)
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/knowledge/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("X-API-Key", "synthetic-native-session")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("API key header accepted: %d", response.StatusCode)
	}
}
