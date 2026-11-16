package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func TestKnowledgeAccessMCPMissingOwnerFailsWithoutPanicDiagnostic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, audience := range []string{"", "https://Synthetic_Private_Marker@foreign.invalid/", "http://localhost/api/v1/knowledge/access/mcp"} {
		t.Setenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE", audience)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "http://localhost/api/v1/knowledge/access/mcp", strings.NewReader("Synthetic_Rejected_Body"))
		c.Request.Header.Set("Authorization", "Synthetic_Private_Marker")
		var handler *DatasetsHandler
		handler.KnowledgeAccessMCP(c)
		var body entity.KnowledgeAccessError
		if w.Code != 503 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Operation != "mcpTransport" || body.Data != nil || body.OutcomeKnown || !body.RetrySameKey || strings.Contains(w.Body.String(), "Synthetic_") || w.Header().Get("WWW-Authenticate") != "" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("closed unavailable response failed")
		}
		if entity.KnowledgeAccessDecode(w.Body.Bytes(), "Error", nil) != nil {
			t.Fatal("unavailable adapter response is not closed")
		}
	}
}

func TestKnowledgeAccessMCPFailureMappingKeepsUnknownValidity(t *testing.T) {
	for _, err := range []error{errors.New("Synthetic_Private_DB_Diagnostic"), &dataset.KnowledgeAccessError{Status: 503, Code: "unavailable", RetrySameKey: true}} {
		got := knowledgeAccessMCPFailure(err)
		if got.Status != 503 || got.Code != "unavailable" || got.OutcomeKnown || !got.RetrySameKey {
			t.Fatal("unavailable DB was reported as invalid bearer")
		}
	}
	got := knowledgeAccessMCPFailure(&dataset.KnowledgeAccessError{Status: 401, Code: "unauthenticated", OutcomeKnown: true})
	if got.Status != 401 || got.Code != "unauthenticated" || !got.OutcomeKnown || got.RetrySameKey {
		t.Fatal("invalid bearer mapping changed")
	}
}
