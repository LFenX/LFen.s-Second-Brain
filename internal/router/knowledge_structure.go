package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeStructureRoutes(engine *gin.Engine) {
	group := engine.Group("/api/v1")
	group.Use(handler.KnowledgeStructureAuth(r.authHandler.AuthMiddleware()))
	group.GET("/knowledge/structure-contract", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructureContract"))
	base := "/datasets/:dataset_id/knowledge"
	group.GET(base+"/structure-policy", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructurePolicy"))
	group.POST(base+"/structure-policy/changes", r.datasetsHandler.KnowledgeStructure("changeKnowledgeStructurePolicy"))
	group.GET(base+"/structure-commands/by-key", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructureCommandByKey"))
	group.POST(base+"/structure-tasks", r.datasetsHandler.KnowledgeStructure("createKnowledgeStructureTask"))
	group.GET(base+"/structure-tasks", r.datasetsHandler.KnowledgeStructure("listKnowledgeStructureTasks"))
	group.GET(base+"/structure-tasks/by-key", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructureTaskByKey"))
	group.GET(base+"/structure-tasks/:task_id", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructureTask"))
	group.POST(base+"/structure-tasks/:task_id/cancel", r.datasetsHandler.KnowledgeStructure("cancelKnowledgeStructureTask"))
	group.GET(base+"/projects/:project_id/structure-head", r.datasetsHandler.KnowledgeStructure("getKnowledgeProjectStructureHead"))
	group.GET(base+"/structure-views/:view_id", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructureView"))
	group.GET(base+"/structure-views/:view_id/nodes", r.datasetsHandler.KnowledgeStructure("listKnowledgeStructureNodes"))
	group.GET(base+"/structure-views/:view_id/nodes/:structure_node_id", r.datasetsHandler.KnowledgeStructure("getKnowledgeStructureNode"))
	group.GET(base+"/structure-views/:view_id/nodes/:structure_node_id/content", r.datasetsHandler.KnowledgeStructure("readKnowledgeStructureNode"))
	group.GET(base+"/structure-views/:view_id/relations", r.datasetsHandler.KnowledgeStructure("listKnowledgeStructureRelations"))
}
