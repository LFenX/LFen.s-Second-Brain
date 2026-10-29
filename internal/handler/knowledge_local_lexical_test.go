package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"ragflow/internal/entity"
	"strings"
	"testing"
)

func TestKnowledgeLocalLexicalStrictDecode(t *testing.T) {
	valid := `{"contract_version":"0.1.0-candidate","idempotency_key":"synthetic_1","expected_local_policy_version":0,"expected_foundation_policy_version":1,"enabled":true}`
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{valid, true},
		{strings.Replace(valid, `:0,`, `:0e0,`, 1), true},
		{strings.Replace(valid, `"enabled":true`, `"enabled":true,"enabled":false`, 1), false},
		{strings.Replace(valid, `"enabled":true`, `"Enabled":true`, 1), false},
		{strings.Replace(valid, `"enabled":true`, `"enabled":null`, 1), false},
		{strings.Replace(valid, `:0,`, `:0.5,`, 1), false},
		{strings.Replace(valid, `synthetic_1`, `\ud800`, 1), false},
		{strings.Replace(valid, `"enabled":true`, `"enabled":true,"unknown":0`, 1), false},
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("lexical_operation", "changeLocalPolicy")
		c.Set("lexical_request_id", strings.Repeat("a", 32))
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		_, ok := decodeLexical[entity.KnowledgeLexicalPolicyChange](c)
		if ok != tc.valid {
			t.Fatalf("decode=%t body=%s", ok, tc.body)
		}
	}
}
func TestKnowledgeLocalLexicalRejectsAPITokenBeforeService(t *testing.T) {
	h := &DatasetsHandler{}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Set("auth_via_api_token", true)
	c.Set("lexical_operation", "search")
	c.Set("lexical_request_id", strings.Repeat("a", 32))
	h.KnowledgeLocalLexical("search")(c)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("API fallback accepted: %d %s", rec.Code, rec.Body.String())
	}
}
