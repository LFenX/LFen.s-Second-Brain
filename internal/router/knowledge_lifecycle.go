package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeLifecycleRoutes(engine *gin.Engine) {
	group := engine.Group("/api/v1")
	group.Use(handler.KnowledgeLifecycleAuth(r.authHandler.AuthMiddleware()))
	group.GET("/knowledge/source-lifecycle-contract", r.datasetsHandler.KnowledgeLifecycle("getKnowledgeSourceLifecycleContract"))
	base := "/datasets/:dataset_id/knowledge"
	group.GET(base+"/source-lifecycles", r.datasetsHandler.KnowledgeLifecycle("listKnowledgeSourceLifecycles"))
	group.GET(base+"/source-lifecycles/:source_id", r.datasetsHandler.KnowledgeLifecycle("getKnowledgeSourceLifecycle"))
	group.GET(base+"/source-lifecycles/:source_id/move-target", r.datasetsHandler.KnowledgeLifecycle("checkKnowledgeSourceMoveTarget"))
	group.POST(base+"/source-lifecycles/:source_id/reclassifications", r.datasetsHandler.KnowledgeLifecycle("commandKnowledgeSourceReclassify"))
	group.POST(base+"/source-lifecycles/:source_id/moves", r.datasetsHandler.KnowledgeLifecycle("commandKnowledgeSourceMove"))
	group.POST(base+"/source-lifecycles/:source_id/archives", r.datasetsHandler.KnowledgeLifecycle("commandKnowledgeSourceArchive"))
	group.POST(base+"/source-lifecycles/:source_id/restores", r.datasetsHandler.KnowledgeLifecycle("commandKnowledgeSourceRestore"))
	group.GET(base+"/lifecycle-commands/by-key", r.datasetsHandler.KnowledgeLifecycle("getKnowledgeLifecycleCommandByKey"))
	group.GET(base+"/lifecycle-commands/:command_id", r.datasetsHandler.KnowledgeLifecycle("getKnowledgeLifecycleCommand"))
}
