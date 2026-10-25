package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeBatchRoutes(engine *gin.Engine) {
	group := engine.Group("/api/v1")
	group.Use(handler.KnowledgeBatchAuth(r.authHandler.AuthMiddleware()))
	group.GET("/knowledge/batch-import-contract", r.datasetsHandler.KnowledgeBatch("getKnowledgeBatchImportContract"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-previews", r.datasetsHandler.KnowledgeBatch("prepareKnowledgeBatch"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-previews/:preview_id", r.datasetsHandler.KnowledgeBatch("getKnowledgeBatchPreview"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-previews/:preview_id/items/:item_id/content", r.datasetsHandler.KnowledgeBatch("readKnowledgeBatchPreviewItem"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-tasks", r.datasetsHandler.KnowledgeBatch("acceptKnowledgeBatchTask"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-tasks", r.datasetsHandler.KnowledgeBatch("listKnowledgeBatchTasks"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-tasks/:task_id", r.datasetsHandler.KnowledgeBatch("getKnowledgeBatchTask"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-tasks/:task_id/items", r.datasetsHandler.KnowledgeBatch("listKnowledgeBatchTaskItems"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-tasks/by-key/:idempotency_key", r.datasetsHandler.KnowledgeBatch("getKnowledgeBatchTaskByKey"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/batch-tasks/:task_id/cancellation", r.datasetsHandler.KnowledgeBatch("cancelKnowledgeBatchTask"))
	group.POST("/datasets/:dataset_id/knowledge/project-manifest-previews", r.datasetsHandler.KnowledgeBatch("prepareKnowledgeProjectManifest"))
	group.GET("/datasets/:dataset_id/knowledge/project-manifest-previews/:preview_id", r.datasetsHandler.KnowledgeBatch("getKnowledgeProjectManifestPreview"))
	group.POST("/datasets/:dataset_id/knowledge/project-manifest-publications", r.datasetsHandler.KnowledgeBatch("publishKnowledgeProjectManifest"))
	group.GET("/datasets/:dataset_id/knowledge/projects/:project_id/manifest-publications/by-key/:idempotency_key", r.datasetsHandler.KnowledgeBatch("getKnowledgeProjectManifestPublicationByKey"))
	group.GET("/datasets/:dataset_id/knowledge/projects/:project_id/manifest-head", r.datasetsHandler.KnowledgeBatch("getKnowledgeProjectManifestHead"))
	group.GET("/datasets/:dataset_id/knowledge/projects/:project_id/manifests", r.datasetsHandler.KnowledgeBatch("listKnowledgeProjectManifests"))
	group.GET("/datasets/:dataset_id/knowledge/projects/:project_id/manifests/:manifest_id", r.datasetsHandler.KnowledgeBatch("getKnowledgeProjectManifest"))
}
