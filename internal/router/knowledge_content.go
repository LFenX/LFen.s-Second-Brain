package router

import (
	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeContentRoutes(engine *gin.Engine) {
	r.setupKnowledgeAnswerRoutes(engine)
	r.setupKnowledgeContentLexicalRoutes(engine)
	base := "/api/v1/datasets/:dataset_id/knowledge/content"
	for _, route := range []struct{ method, path, operation string }{
		{"GET", "/capabilities", "capabilities"},
		{"GET", "/policy", "content-policy"},
		{"POST", "/policy", "content-policy-change"},
		{"POST", "/parsed-source/upload", "parsed-source-upload"},
		{"POST", "/parsed-source/snapshot", "parsed-source-snapshot"},
		{"POST", "/parsed-source", "parsed-source"},
		{"POST", "/parsed-task", "parsed-task-create"},
		{"POST", "/parsed-task/get", "parsed-task-get"},
		{"POST", "/parsed-task/cancel", "parsed-task-cancel"},
		{"POST", "/representation", "representation"},
		{"POST", "/parsed-representations", "parsed-representations"},
		{"POST", "/node", "node"},
		{"POST", "/nodes", "nodes"},
		{"POST", "/read", "read"},
	} {
		engine.Handle(route.method, base+route.path, handler.KnowledgeContentAuth(r.authHandler.AuthMiddleware(), route.operation), r.datasetsHandler.KnowledgeContent(route.operation))
	}
}
