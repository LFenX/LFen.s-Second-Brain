package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

func knowledgeHandlerTestRouter(user bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if user {
		r.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: "synthetic-owner"}); c.Next() })
	}
	h := &DatasetsHandler{}
	r.GET("/contract", h.KnowledgeContract)
	r.POST("/body", func(c *gin.Context) {
		if _, ok := knowledgeUser(c); !ok {
			return
		}
		if _, ok := knowledgeDecode[entity.CreateKnowledgeNodeRequest](c, "source_revision_id", "parent_id", "kind", "logical_key", "name", "start_byte", "end_byte"); ok {
			knowledgeResponse(c, 200, true, "", "success")
		}
	})
	return r
}

func TestKnowledgeHandlerContractAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, url, version string
		user               bool
		status             int
	}{{"unauthenticated", "/contract", "", false, 401}, {"contract mismatch", "/contract", "2.0.0", true, 409}, {"unknown query", "/contract?path=/etc/passwd", "", true, 400}, {"repeated query", "/contract?limit=1&limit=2", "", true, 400}, {"capabilities", "/contract", entity.KnowledgeContractVersion, true, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			r := knowledgeHandlerTestRouter(tc.user)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req.Header.Set("X-Knowledge-Contract-Version", tc.version)
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["contract_version"] != entity.KnowledgeContractVersion || len(w.Header().Get("X-Request-ID")) != 32 {
				t.Fatalf("missing contract metadata: %s", w.Body)
			}
			if tc.status == 200 {
				data := body["data"].(map[string]interface{})
				if data["model_calls"] != false || data["ingestion"] != false {
					t.Fatalf("capability drift: %s", w.Body)
				}
			} else if body["data"] != nil {
				t.Fatal("error disclosed data")
			}
		})
	}
}

func TestKnowledgeHandlerRejectsAmbiguousBodies(t *testing.T) {
	valid := `{"source_revision_id":"33333333333333333333333333333333","parent_id":"` + strings.Repeat("a", 64) + `","kind":"passage","logical_key":"sample","name":"Sample","start_byte":0,"end_byte":1}`
	cases := []struct {
		body   string
		status int
	}{{valid, 200}, {"null", 400}, {"[]", 400}, {valid + "{}", 400}, {strings.Replace(valid, `"start_byte":0,`, "", 1), 400}, {strings.Replace(valid, `"end_byte":1`, `"end_byte":null`, 1), 400}, {strings.Replace(valid, `"end_byte":1`, `"end_byte":1,"external_models":true`, 1), 400}, {strings.Replace(valid, `"end_byte":1`, `"end_byte":1,"END_BYTE":2`, 1), 400}, {strings.Replace(valid, `"end_byte":1`, `"end_byte":1,"end_byte":2`, 1), 400}, {`{"name":"` + strings.Repeat("x", 17000) + `"}`, 413}}
	r := knowledgeHandlerTestRouter(true)
	for i, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("case %d status=%d want=%d body=%s", i, w.Code, tc.status, w.Body)
		}
	}
}
