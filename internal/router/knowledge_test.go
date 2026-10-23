package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/handler"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeOpenAPIRoutesAreRegisteredAndAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	data, err := os.ReadFile("../../docs/knowledge-foundation-v1.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	r := &Router{authHandler: handler.NewAuthHandler()}
	engine := gin.New()
	r.Setup(engine)
	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	param := regexp.MustCompile(`\{([^}]+)\}`)
	for path, operations := range contract.Paths {
		for method := range operations {
			ginPath := "/api/v1" + param.ReplaceAllString(path, ":$1")
			if !registered[strings.ToUpper(method)+" "+ginPath] {
				t.Errorf("contract route missing: %s %s", method, ginPath)
			}
			url := "/api/v1" + param.ReplaceAllString(path, strings.Repeat("a", 32))
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(strings.ToUpper(method), url, nil))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("route %s %s is not auth protected: %d", method, url, w.Code)
			}
		}
	}
}
