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

func TestKnowledgeWebFrozenRoutesAndAuthentication(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-web-import-v1.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
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
				t.Error("frozen route missing", method, ginPath)
			}
			url := "/api/v1" + parameter.ReplaceAllString(route, strings.Repeat("a", 32))
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(strings.ToUpper(method), url, nil))
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Knowledge-Source-Contract-Version") != "0.6.0" || w.Header().Get("X-Request-ID") == "" {
				t.Errorf("route auth boundary %s %s %d", method, url, w.Code)
			}
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["code"] != "knowledge_unauthorized" || body["source_contract_version"] != "0.6.0" {
				t.Error("native error escaped C2 envelope", method, url)
			}
		}
	}
	if count != 34 {
		t.Fatal("operation count", count)
	}
}
