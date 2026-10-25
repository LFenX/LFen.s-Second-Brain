package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"ragflow/internal/entity"
	"strings"
	"testing"
)

func TestKnowledgeDirectoryContractHeadersAndAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, foundation, source string
		user                     bool
		status                   int
	}{{"frozen", "1.0.0", "0.3.0", true, 200}, {"missing-foundation", "", "0.3.0", true, 409}, {"old-source", "1.0.0", "0.2.0", true, 409}, {"missing-source", "1.0.0", "", true, 409}, {"anonymous", "1.0.0", "0.3.0", false, 401}} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			if tc.user {
				r.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: strings.Repeat("a", 32)}); c.Next() })
			}
			h := &DatasetsHandler{}
			r.GET("/cap", h.KnowledgeDirectory("getDirectoryImportCandidate"))
			req := httptest.NewRequest("GET", "/cap", nil)
			req.Header.Set("X-Knowledge-Contract-Version", tc.foundation)
			req.Header.Set("X-Knowledge-Source-Contract-Version", tc.source)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if tc.status == 200 {
				var body struct {
					Data map[string]any `json:"data"`
				}
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Data["status"] != "Frozen" || body.Data["implemented"] != true || body.Data["version"] != "0.3.0" {
					t.Fatal("frozen capability mismatch")
				}
			}
		})
	}
}
func TestKnowledgeDirectoryStrictNestedJSON(t *testing.T) {
	valid := `{"observation_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","observation_sha256":"` + strings.Repeat("b", 64) + `","relative_path":"web/a.ts","classification":{"group_id":null,"component_side":"frontend","language":"ts"}}`
	cases := []struct {
		name, body string
		status     int
	}{{"valid", valid, 200}, {"missing-classification", strings.Replace(valid, `,"classification":{"group_id":null,"component_side":"frontend","language":"ts"}`, "", 1), 400}, {"duplicate-nested", strings.Replace(valid, `"language":"ts"`, `"language":"ts","language":"go"`, 1), 400}, {"unknown-nested", strings.Replace(valid, `"language":"ts"`, `"language":"ts","allow_models":true`, 1), 400}, {"missing-nullable-key", strings.Replace(valid, `"group_id":null,`, "", 1), 400}, {"case-alias", strings.Replace(valid, `"relative_path"`, `"Relative_Path"`, 1), 400}, {"trailing", valid + `{}`, 400}, {"null-array", `null`, 400}, {"invalid-utf8", valid + string([]byte{255}), 400}, {"too-large", strings.Repeat(" ", 32769), 413}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.POST("/body", func(c *gin.Context) {
				c.Set("knowledge_request_id", strings.Repeat("a", 32))
				if _, ok := decodeDirectory[entity.KnowledgeDirectoryPreviewRequest](c); ok {
					c.Status(200)
				}
			})
			req := httptest.NewRequest("POST", "/body", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestKnowledgeDirectoryNativeAuthAndPanicEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name           string
		native, status int
		panic          bool
	}{{"native-503", 503, 503, false}, {"native-forbidden", 403, 404, false}, {"panic", 0, 500, true}} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(KnowledgeDirectoryAuth(func(c *gin.Context) {
				if tc.panic {
					panic("synthetic private panic")
				}
				c.JSON(tc.native, gin.H{"code": 100, "message": "synthetic native internal detail", "data": "No"})
				c.Abort()
			}))
			r.GET("/x", func(c *gin.Context) { t.Fatal("auth failure ran business handler") })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
			if w.Code != tc.status || strings.Contains(w.Body.String(), "synthetic") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["source_contract_version"] != "0.3.0" || body["data"] != nil {
				t.Fatal("non-contract auth failure")
			}
		})
	}
}
