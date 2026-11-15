package router

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeAccessRegistersOnlyNineS1Operations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{}
	r.setupKnowledgeAccessRoutes(engine)
	paths := map[string]bool{}
	for _, route := range engine.Routes() {
		paths[route.Path] = true
		if strings.Contains(route.Path, "department") || strings.Contains(route.Path, "credential") || strings.Contains(route.Path, "mcp") {
			t.Fatal("later-stage route registered", route.Path)
		}
	}
	if len(paths) != 9 {
		t.Fatal("S1 route count", len(paths))
	}
}
