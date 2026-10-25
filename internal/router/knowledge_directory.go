package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeDirectoryRoutes(engine *gin.Engine) {
	group := engine.Group("/api/v1")
	group.Use(handler.KnowledgeDirectoryAuth(r.authHandler.AuthMiddleware()))
	group.GET("/knowledge/directory-import-contract", r.datasetsHandler.KnowledgeDirectory("getDirectoryImportCandidate"))
	group.GET("/datasets/:dataset_id/knowledge/directory-roots", r.datasetsHandler.KnowledgeDirectory("listDirectoryRoots"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations", r.datasetsHandler.KnowledgeDirectory("createDirectoryRegistration"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id", r.datasetsHandler.KnowledgeDirectory("getDirectoryRegistration"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/listings", r.datasetsHandler.KnowledgeDirectory("createDirectoryListing"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/listings/:listing_id", r.datasetsHandler.KnowledgeDirectory("getDirectoryListingPage"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/observations", r.datasetsHandler.KnowledgeDirectory("createDirectoryObservation"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/observations/:observation_id", r.datasetsHandler.KnowledgeDirectory("getDirectoryObservation"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/previews", r.datasetsHandler.KnowledgeDirectory("createDirectoryPreview"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/previews/:preview_id", r.datasetsHandler.KnowledgeDirectory("getDirectoryPreview"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/previews/:preview_id/content", r.datasetsHandler.KnowledgeDirectory("readDirectoryPreviewContent"))
	group.POST("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/imports", r.datasetsHandler.KnowledgeDirectory("commitDirectoryImport"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/imports/:import_task_id", r.datasetsHandler.KnowledgeDirectory("getDirectoryImport"))
	group.GET("/datasets/:dataset_id/knowledge/directory-registrations/:registration_id/imports/by-key/:idempotency_key", r.datasetsHandler.KnowledgeDirectory("getDirectoryImportByKey"))
}
