package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeLocalLexicalRoutes(engine *gin.Engine) {
	base := "/api/v1/datasets/:dataset_id/knowledge"
	for _, route := range []struct{ method, path, operation string }{
		{"GET", "/local-lexical-policy", "getLocalPolicy"},
		{"POST", "/local-lexical-policy/changes", "changeLocalPolicy"},
		{"POST", "/local-index/readiness", "inspectIndex"},
		{"POST", "/local-index/jobs", "createIndex"},
		{"GET", "/local-index/jobs/:job_id", "getIndexJob"},
		{"POST", "/local-search", "search"},
		{"POST", "/local-read", "read"},
	} {
		engine.Handle(route.method, base+route.path, handler.KnowledgeLocalLexicalAuth(r.authHandler.AuthMiddleware(), route.operation), r.datasetsHandler.KnowledgeLocalLexical(route.operation))
	}
}
