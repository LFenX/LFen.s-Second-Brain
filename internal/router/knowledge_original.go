package router

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/handler"
)

func (r *Router) setupKnowledgeOriginalRoutes(engine *gin.Engine) {
	group := engine.Group("/api/v1")
	group.Use(handler.KnowledgeOriginalAuth(r.authHandler.AuthMiddleware()))
	group.GET("/knowledge/original-import-contract", r.datasetsHandler.KnowledgeOriginal("getKnowledgeOriginalImportContract"))
	group.POST("/datasets/:dataset_id/knowledge/original-registrations", r.datasetsHandler.KnowledgeOriginal("createKnowledgeOriginalRegistration"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/by-key", r.datasetsHandler.KnowledgeOriginal("findKnowledgeOriginalRegistrationByKey"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/:registration_id", r.datasetsHandler.KnowledgeOriginal("getKnowledgeOriginalRegistration"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/target", r.datasetsHandler.KnowledgeOriginal("getKnowledgeOriginalTarget"))
	group.POST("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/uploads", r.datasetsHandler.KnowledgeOriginal("createKnowledgeOriginalUpload"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/uploads/by-key", r.datasetsHandler.KnowledgeOriginal("findKnowledgeOriginalUploadByKey"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/uploads/:upload_id", r.datasetsHandler.KnowledgeOriginal("getKnowledgeOriginalUpload"))
	group.DELETE("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/uploads/:upload_id", r.datasetsHandler.KnowledgeOriginal("cancelKnowledgeOriginalUpload"))
	group.PUT("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/uploads/:upload_id/content", r.datasetsHandler.KnowledgeOriginal("putKnowledgeOriginalUploadBytes"))
	group.POST("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/imports", r.datasetsHandler.KnowledgeOriginal("confirmKnowledgeOriginalImport"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/imports/by-key", r.datasetsHandler.KnowledgeOriginal("findKnowledgeOriginalImportByKey"))
	group.GET("/datasets/:dataset_id/knowledge/original-registrations/:registration_id/imports/:import_task_id", r.datasetsHandler.KnowledgeOriginal("getKnowledgeOriginalImport"))
	group.GET("/datasets/:dataset_id/knowledge/original-revisions", r.datasetsHandler.KnowledgeOriginal("listKnowledgeOriginalRevisions"))
	group.GET("/datasets/:dataset_id/knowledge/original-revisions/:source_revision_id", r.datasetsHandler.KnowledgeOriginal("getKnowledgeOriginalRevision"))
	group.GET("/datasets/:dataset_id/knowledge/original-revisions/:source_revision_id/content", r.datasetsHandler.KnowledgeOriginal("readKnowledgeOriginalRevisionBytes"))
	group.HEAD("/datasets/:dataset_id/knowledge/original-revisions/:source_revision_id/content", r.datasetsHandler.KnowledgeOriginal("headKnowledgeOriginalRevisionBytes"))
}
