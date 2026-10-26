package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeDocumentRoutes(engine *gin.Engine) {
	group := engine.Group("/api/v1")
	group.Use(handler.KnowledgeDocumentAuth(r.authHandler.AuthMiddleware()))
	group.GET("/knowledge/document-structure-contract", r.datasetsHandler.KnowledgeDocument("DC01"))
	p := "/datasets/:dataset_id/knowledge"
	group.GET(p+"/document-structure-policy", r.datasetsHandler.KnowledgeDocument("DC02"))
	group.POST(p+"/document-structure-policy/changes", r.datasetsHandler.KnowledgeDocument("DC03"))
	group.POST(p+"/document-tasks", r.datasetsHandler.KnowledgeDocument("DC04"))
	group.GET(p+"/document-operations/by-key", r.datasetsHandler.KnowledgeDocument("DC05"))
	group.GET(p+"/document-tasks/:task_id", r.datasetsHandler.KnowledgeDocument("DC06"))
	group.POST(p+"/document-tasks/:task_id/cancel", r.datasetsHandler.KnowledgeDocument("DC07"))
	group.GET(p+"/sources/:revision_id/document-head", r.datasetsHandler.KnowledgeDocument("DC08"))
	rp := p + "/document-representations/:representation_id"
	group.GET(rp, r.datasetsHandler.KnowledgeDocument("DC09"))
	group.GET(rp+"/nodes", r.datasetsHandler.KnowledgeDocument("DC10"))
	group.GET(rp+"/nodes/:document_node_id", r.datasetsHandler.KnowledgeDocument("DC11"))
	group.GET(rp+"/nodes/:document_node_id/content", r.datasetsHandler.KnowledgeDocument("DC12"))
}
