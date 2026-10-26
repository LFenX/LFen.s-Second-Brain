package router

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func TestKnowledgeStructureFrozenRoutesRequireNativeAuthentication(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-structure-v1.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
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
	for path, methods := range spec.Paths {
		for method := range methods {
			count++
			ginPath := "/api/v1" + parameter.ReplaceAllString(strings.ReplaceAll(path, "{id}", "{dataset_id}"), ":$1")
			if !routes[strings.ToUpper(method)+" "+ginPath] {
				t.Error("missing frozen route", method, ginPath)
			}
			url := "/api/v1" + parameter.ReplaceAllString(path, strings.Repeat("a", 32))
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(strings.ToUpper(method), url, nil))
			var body map[string]any
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 401 || body["code"] != "not_found" || body["data"] != nil || w.Header().Get("X-Knowledge-Structure-Contract-Version") != "0.1.0" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(method, url, w.Code, w.Body.String())
			}
		}
	}
	if count != 15 {
		t.Fatal("operation count", count)
	}
}
