package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeContentLexicalRoutes(engine *gin.Engine) {
	base := "/api/v1/datasets/:dataset_id/knowledge/content/lexical"
	for _, route := range []struct{ path, operation string }{
		{"/inspect", "inspectIndex"},
		{"/indexes", "createIndex"},
		{"/jobs/get", "getIndexJob"},
		{"/search", "search"},
		{"/evidence/read", "read"},
	} {
		engine.POST(base+route.path, handler.KnowledgeContentLexicalAuth(r.authHandler.AuthMiddleware(), route.operation), r.datasetsHandler.KnowledgeContentLexical(route.operation))
	}
}
