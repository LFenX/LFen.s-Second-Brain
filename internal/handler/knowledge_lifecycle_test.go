package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/entity"
)

func TestKnowledgeLifecycleStrictCommandsAndUnicode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(KnowledgeLifecycleAuth(originalTestAuth))
	engine.POST("/test", func(c *gin.Context) {
		c.Writer = c.MustGet("knowledge_lifecycle_writer").(gin.ResponseWriter)
		c.Set("knowledge_lifecycle_released", true)
		r, ok := decodeLifecycle[entity.KnowledgeLifecycleReclassify](c)
		if ok {
			lifecycleResponse(c, 201, r, nil)
		}
	})
	valid := `{"idempotency_key":"test","expected_source_revision_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expected_lifecycle_version":1,"expected_policy_version":1,"confirmed":true,"classification":{"group_id":null,"component_side":"frontend","language":"tsx"}}`
	cases := []struct {
		name, body string
		status     int
	}{
		{"valid", valid, 201}, {"unknown", strings.Replace(valid, `"confirmed":true`, `"confirmed":true,"extra":1`, 1), 400},
		{"duplicate", strings.Replace(valid, `"confirmed":true`, `"confirmed":true,"confirmed":true`, 1), 400},
		{"missing", strings.Replace(valid, `"group_id":null,`, "", 1), 400}, {"null", strings.Replace(valid, `"confirmed":true`, `"confirmed":null`, 1), 400},
		{"trailing", valid + "{}", 400}, {"orphan_high", strings.Replace(valid, `"tsx"`, `"\ud800"`, 1), 400}, {"orphan_low", strings.Replace(valid, `"tsx"`, `"\udc00"`, 1), 400},
		{"valid_pair", strings.Replace(valid, `"tsx"`, `"\ud83e\udde0"`, 1), 201}, {"literal_escape", strings.Replace(valid, `"tsx"`, `"\\ud800"`, 1), 201},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/test", strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache header")
			}
		})
	}
}

func TestKnowledgeLifecycleQueryAndErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/query", func(c *gin.Context) {
		_, ok := lifecycleQuery(c, []string{"language"})
		if ok {
			c.Status(204)
		}
	})
	for _, test := range []struct {
		query  string
		status int
	}{{"?language=", 204}, {"?language=tsx", 204}, {"?language=x&language=y", 400}, {"?other=1", 400}, {"?language=%ff", 400}} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest("GET", "/query"+test.query, nil))
		if w.Code != test.status {
			t.Fatal(test, w.Code)
		}
	}
	for _, test := range []struct {
		status int
		code   string
		known  bool
	}{{409, "revision_changed", true}, {409, "lifecycle_integrity", false}, {404, "knowledge_not_found", false}, {503, "lifecycle_result_unknown", false}, {413, "lifecycle_budget_exceeded", true}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("knowledge_request_id", strings.Repeat("a", 32))
		lifecycleHandlerError(c, test.status, test.code)
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != test.status || body["code"] != test.code || body["outcome_known"] != test.known || body["source_contract_version"] != "0.7.0" {
			t.Fatal(test, w.Code, w.Body.String())
		}
	}
}
