package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeAnswerRoutes(engine *gin.Engine) {
	const base = "/api/v1/datasets/:dataset_id/knowledge/answer/"
	for _, route := range []struct{ path, operation string }{{"inspect", "inspect"}, {"policy/get", "getPolicy"}, {"policy/set", "setPolicy"}, {"run", "answer"}} {
		engine.POST(base+route.path, handler.KnowledgeAnswerAuth(r.authHandler.AuthMiddleware(), route.operation), r.datasetsHandler.KnowledgeAnswer(route.operation))
	}
}
