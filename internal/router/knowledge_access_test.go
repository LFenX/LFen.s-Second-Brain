package router

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeAccessRegistersOnlyElevenS2Operations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{}
	r.setupKnowledgeAccessRoutes(engine)
	paths := map[string]bool{}
	for _, route := range engine.Routes() {
		paths[route.Path] = true
		if strings.Contains(route.Path, "credential") || strings.Contains(route.Path, "mcp") {
			t.Fatal("later-stage route registered", route.Path)
		}
	}
	if len(paths) != 11 {
		t.Fatal("S2 route count", len(paths))
	}
	for _, path := range []string{"/api/v1/knowledge/access/admin/departments/set", "/api/v1/knowledge/access/departments/list"} {
		if !paths[path] {
			t.Fatal("missing department route", path)
		}
	}
}
