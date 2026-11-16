package router

import (
	"github.com/gin-gonic/gin"
)

func (r *Router) setupKnowledgeAccessRoutes(engine *gin.Engine) {
	for _, route := range []struct{ path, operation string }{
		{"/admin/departments/set", "setDepartment"},
		{"/departments/list", "listDepartments"},
		{"/admin/domains/policy/set", "setPolicy"},
		{"/admin/modules/set", "setModule"},
		{"/admin/modules/get", "getModuleAdmin"},
		{"/modules/list", "listModules"},
		{"/admin/grants/create", "createGrant"},
		{"/admin/grants/revoke", "revokeGrant"},
		{"/inspect", "inspect"},
		{"/search", "search"},
		{"/read", "read"},
	} {
		engine.Any("/api/v1/knowledge/access"+route.path, r.authHandler.KnowledgeAccessAuth(route.operation), r.datasetsHandler.KnowledgeAccess(route.operation))
	}
}
