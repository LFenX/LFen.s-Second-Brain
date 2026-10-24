package handler

import (
	"github.com/gin-gonic/gin"
	"ragflow/internal/entity"
)

func knowledgeImportUser(c *gin.Context) (*entity.User, bool) {
	c.Set("knowledge_source_contract", true)
	c.Header("X-Knowledge-Source-Contract-Version", entity.KnowledgeSourceContractVersion)
	u, ok := knowledgeUser(c)
	if !ok {
		return nil, false
	}
	if c.GetHeader("X-Knowledge-Source-Contract-Version") != entity.KnowledgeSourceContractVersion {
		knowledgeResponse(c, 409, nil, "knowledge_source_contract_version", "Explicit supported source contract version is required")
		return nil, false
	}
	return u, true
}

func (h *DatasetsHandler) KnowledgeImportContract(c *gin.Context) {
	if _, ok := knowledgeImportUser(c); !ok || !knowledgeQuery(c) {
		return
	}
	knowledgeResponse(c, 200, gin.H{"version": entity.KnowledgeSourceContractVersion, "status": "ReviewCandidate", "foundation_contract_version": entity.KnowledgeContractVersion, "schema": "docs/knowledge-sources-v1.openapi.json", "source_kinds": []string{"text_file"}, "input_modes": []string{"inline_text"}, "max_preview_items": 1, "max_include_paths": 16, "source_bytes": entity.KnowledgeMaxSourceBytes, "request_bytes": entity.KnowledgeImportRequestBytes, "preview_ttl_seconds": 600, "task_execution": "synchronous_single_item", "processing": gin.H{"mode": "catalogue_only", "structure": "deferred", "full_text": "disabled", "vector": "disabled", "external_models": false, "automatic_ingestion": false, "allowed_model_uses": []string{}, "model_calls": 0}}, "", "success")
}

func (h *DatasetsHandler) CreateKnowledgeRegistration(c *gin.Context) {
	u, ok := knowledgeImportUser(c)
	if !ok || !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.CreateKnowledgeRegistrationRequest](c, "registration_key", "name", "source_kind", "input_mode", "include_paths", "group_id", "component_side", "language")
	if !ok {
		return
	}
	result, created, err := h.datasetsService.CreateKnowledgeRegistration(c.Request.Context(), u.ID, c.Param("dataset_id"), *req)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	status := 200
	if created {
		status = 201
	}
	knowledgeResponse(c, status, result, "", "success")
}

func (h *DatasetsHandler) GetKnowledgeRegistration(c *gin.Context) {
	u, ok := knowledgeImportUser(c)
	if !ok || !knowledgeQuery(c) {
		return
	}
	result, err := h.datasetsService.GetKnowledgeRegistration(c.Request.Context(), u.ID, c.Param("dataset_id"), c.Param("registration_id"))
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) PreviewKnowledgeImport(c *gin.Context) {
	u, ok := knowledgeImportUser(c)
	if !ok || !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecodeLimit[entity.PreviewKnowledgeImportRequest](c, entity.KnowledgeImportRequestBytes, "relative_path", "text", "group_id", "component_side", "language")
	if !ok {
		return
	}
	result, err := h.datasetsService.PreviewKnowledgeImport(c.Request.Context(), u.ID, c.Param("dataset_id"), c.Param("registration_id"), *req)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 201, result, "", "success")
}

func (h *DatasetsHandler) GetKnowledgeImportPreview(c *gin.Context) {
	u, ok := knowledgeImportUser(c)
	if !ok || !knowledgeQuery(c) {
		return
	}
	result, err := h.datasetsService.GetKnowledgeImportPreview(c.Request.Context(), u.ID, c.Param("dataset_id"), c.Param("registration_id"), c.Param("preview_id"))
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) CommitKnowledgeImport(c *gin.Context) {
	u, ok := knowledgeImportUser(c)
	if !ok || !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.CommitKnowledgeImportRequest](c, "preview_id", "manifest_sha256", "idempotency_key")
	if !ok {
		return
	}
	result, created, err := h.datasetsService.CommitKnowledgeImport(c.Request.Context(), u.ID, c.Param("dataset_id"), c.Param("registration_id"), *req)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	status := 200
	if created {
		status = 201
	}
	knowledgeResponse(c, status, result, "", "success")
}

func (h *DatasetsHandler) GetKnowledgeImport(c *gin.Context) {
	u, ok := knowledgeImportUser(c)
	if !ok || !knowledgeQuery(c) {
		return
	}
	result, err := h.datasetsService.GetKnowledgeImport(c.Request.Context(), u.ID, c.Param("dataset_id"), c.Param("registration_id"), c.Param("import_task_id"))
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}
