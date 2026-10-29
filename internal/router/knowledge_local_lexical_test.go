package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"ragflow/internal/handler"
	"strings"
	"testing"
)

func TestKnowledgeLocalLexicalAuthenticationEnvelope(t *testing.T) {
	r := gin.New()
	r.GET("/policy", handler.KnowledgeLocalLexicalAuth(func(c *gin.Context) { c.AbortWithStatus(401) }, "getLocalPolicy"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/policy", nil))
	if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"operation":"getLocalPolicy"`) || !strings.Contains(w.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("envelope: %d %s", w.Code, w.Body.String())
	}
	if dir := os.Getenv("LFEN_KCLS_EVIDENCE_DIR"); strings.HasPrefix(dir, "/tmp/lfen-kcls-be-20261028-01") {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		raw := w.Body.Bytes()
		digest := sha256.Sum256(raw)
		meta, _ := json.Marshal(map[string]any{"kind": "HandlerInProcessHTTP", "transport": "httptest gin router", "operation": "getLocalPolicy", "status": w.Code, "sha256": hex.EncodeToString(digest[:]), "byte_length": len(raw), "real_session_success": false})
		if err := os.WriteFile(dir+"/authentication-failure.http.json", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/authentication-failure.http.meta.json", meta, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
