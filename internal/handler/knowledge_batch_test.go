package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"ragflow/internal/entity"
	"strings"
	"testing"
)

func TestKnowledgeBatchFrozenHeadersAndAuth(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		user          bool
		status        int
	}{{"frozen", "0.4.0", true, 200}, {"old", "0.3.0", true, 409}, {"missing", "", true, 409}, {"anonymous", "0.4.0", false, 401}} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			if tc.user {
				r.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: strings.Repeat("a", 32)}); c.Next() })
			}
			h := &DatasetsHandler{}
			r.GET("/cap", h.KnowledgeBatch("getKnowledgeBatchImportContract"))
			req := httptest.NewRequest("GET", "/cap", nil)
			req.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
			req.Header.Set("X-Knowledge-Source-Contract-Version", tc.version)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if tc.status == 200 {
				var b struct {
					Data map[string]any `json:"data"`
				}
				if json.Unmarshal(w.Body.Bytes(), &b) != nil || b.Data["status"] != "Frozen" || b.Data["implemented"] != true || b.Data["model_calls"] != float64(0) {
					t.Fatal("capability mismatch")
				}
			}
		})
	}
}
func TestKnowledgeBatchStrictArrayObjects(t *testing.T) {
	valid := `{"observation_id":"` + strings.Repeat("a", 32) + `","observation_sha256":"` + strings.Repeat("b", 64) + `","selected_items":[{"relative_path":"web/a.ts","classification":{"group_id":null,"component_side":"frontend","language":"ts"}}]}`
	for _, tc := range []struct {
		name, raw string
		status    int
	}{{"valid", valid, 200}, {"duplicate-in-array", strings.Replace(valid, `"language":"ts"`, `"language":"ts","language":"go"`, 1), 400}, {"unknown-in-array", strings.Replace(valid, `"language":"ts"`, `"language":"ts","allow_models":true`, 1), 400}, {"missing-nullable", strings.Replace(valid, `"group_id":null,`, "", 1), 400}, {"null-array", strings.Replace(valid, `[{"relative_path":"web/a.ts","classification":{"group_id":null,"component_side":"frontend","language":"ts"}}]`, `null`, 1), 400}, {"large", strings.Repeat(" ", 32769), 413}} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.POST("/body", func(c *gin.Context) {
				c.Set("knowledge_request_id", strings.Repeat("a", 32))
				if _, ok := decodeBatch[entity.KnowledgeBatchPreviewRequest](c); ok {
					c.Status(200)
				}
			})
			req := httptest.NewRequest("POST", "/body", strings.NewReader(tc.raw))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}
func TestKnowledgeBatchNativeFailureAndPanic(t *testing.T) {
	for _, status := range []int{403, 503, 0} {
		t.Run(string(rune('a'+status%26)), func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(KnowledgeBatchAuth(func(c *gin.Context) {
				if status == 0 {
					panic("synthetic private panic")
				}
				c.JSON(status, gin.H{"message": "synthetic internal detail"})
				c.Abort()
			}))
			r.GET("/x", func(c *gin.Context) { t.Fatal("auth failure entered business") })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
			want := 503
			if status == 403 {
				want = 404
			}
			var body map[string]any
			if w.Code != want || json.Unmarshal(w.Body.Bytes(), &body) != nil {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if _, ok := body["data"]; ok {
				t.Fatal("error includes undeclared data")
			}
			if strings.Contains(w.Body.String(), "synthetic") {
				t.Fatal("private failure leaked")
			}
		})
	}
}
