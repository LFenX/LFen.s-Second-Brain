package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/entity"
)

func TestKnowledgeAnswerAuthenticationClosesNativeErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/answer", KnowledgeAnswerAuth(func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"message": "PRIVATE-NATIVE-ERROR"}) }, "answer"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/answer", nil))
	if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Knowledge-Answer-Contract-Version") != entity.KnowledgeAnswerVersion || strings.Contains(w.Body.String(), "PRIVATE-NATIVE-ERROR") {
		t.Fatal("native failure leaked")
	}
	if entity.KnowledgeAnswerDecode(w.Body.Bytes(), "Error", nil) != nil {
		t.Fatal("error contract mismatch")
	}
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["data"] != nil || body["code"] != "unauthenticated" || body["request_id"] != w.Header().Get("X-Request-ID") {
		t.Fatal("error binding mismatch")
	}
}
