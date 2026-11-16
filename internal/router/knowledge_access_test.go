package router

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeAccessRegistersFourteenNativeAndOneManagedMCPRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{}
	r.setupKnowledgeAccessRoutes(engine)
	paths := map[string]bool{}
	for _, route := range engine.Routes() {
		paths[route.Path] = true
		if !strings.HasPrefix(route.Path, "/api/v1/knowledge/access/") {
			t.Fatal("unexpected access route", route.Path)
		}
	}
	if len(paths) != 15 {
		t.Fatal("S3 route count", len(paths))
	}
	for _, path := range []string{"/api/v1/knowledge/access/admin/departments/set", "/api/v1/knowledge/access/departments/list", "/api/v1/knowledge/access/credentials/create", "/api/v1/knowledge/access/credentials/revoke", "/api/v1/knowledge/access/credentials/list", "/api/v1/knowledge/access/mcp"} {
		if !paths[path] {
			t.Fatal("missing access route", path)
		}
	}
}
