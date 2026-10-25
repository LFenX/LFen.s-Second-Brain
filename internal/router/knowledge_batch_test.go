package router

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"ragflow/internal/handler"
	"regexp"
	"strings"
	"testing"
)

func TestKnowledgeBatchFrozenRoutesAndNoStore(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-batch-import-v1.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
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
			url := "/api/v1" + parameter.ReplaceAllString(route, strings.Repeat("a", 32))
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(strings.ToUpper(method), url, nil))
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("route %s %s auth/cache %d/%s", method, url, w.Code, w.Header().Get("Cache-Control"))
			}
		}
	}
	if count != 17 {
		t.Fatal("operation count drift")
	}
}
