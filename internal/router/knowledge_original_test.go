package router

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/handler"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeOriginalFrozenRoutesAndNoStore(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-original-import-v1.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{authHandler: handler.NewAuthHandler()}
	r.Setup(engine)
	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	parameter := regexp.MustCompile(`\{([^}]+)\}`)
	count := 0
	for route, methods := range spec.Paths {
		for method := range methods {
			count++
			ginPath := "/api/v1" + parameter.ReplaceAllString(strings.ReplaceAll(route, "{domain_id}", "{dataset_id}"), ":$1")
			if !routes[strings.ToUpper(method)+" "+ginPath] {
				t.Errorf("missing %s %s", method, ginPath)
			}
			w := httptest.NewRecorder()
			url := "/api/v1" + parameter.ReplaceAllString(route, strings.Repeat("a", 32))
			engine.ServeHTTP(w, httptest.NewRequest(strings.ToUpper(method), url, nil))
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Knowledge-Source-Contract-Version") != "0.5.0" {
				t.Errorf("unauthorized route response %s %s %d", method, url, w.Code)
			}
			if method == "head" && w.Body.Len() != 0 {
				t.Error("HEAD authentication wrote a body")
			}
		}
	}
	if count != 17 {
		t.Fatalf("operation count=%d", count)
	}
}
