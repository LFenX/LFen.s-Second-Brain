package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"

	"github.com/gin-gonic/gin"
)

func knowledgeResponse(c *gin.Context, status int, data interface{}, code, message string) {
	var numericCode interface{} = 0
	if code != "" {
		numericCode = code
	}
	c.JSON(status, gin.H{"code": numericCode, "message": message, "data": data,
		"contract_version": entity.KnowledgeContractVersion, "request_id": c.GetString("knowledge_request_id")})
}

func knowledgeFailure(c *gin.Context, err error) {
	var public *dataset.KnowledgeError
	if errors.As(err, &public) {
		knowledgeResponse(c, public.Status, nil, public.Code, public.Message)
		return
	}
	knowledgeResponse(c, http.StatusInternalServerError, nil, "knowledge_internal", "Knowledge operation failed")
}

func knowledgeUser(c *gin.Context) (*entity.User, bool) {
	c.Set("knowledge_request_id", common.GenerateUUID())
	c.Header("X-Knowledge-Contract-Version", entity.KnowledgeContractVersion)
	c.Header("X-Request-ID", c.GetString("knowledge_request_id"))
	user, code, _ := GetUser(c)
	if code != common.CodeSuccess || user == nil {
		knowledgeResponse(c, 401, nil, "knowledge_unauthorized", "Authentication required")
		return nil, false
	}
	if version := c.GetHeader("X-Knowledge-Contract-Version"); version != "" && version != entity.KnowledgeContractVersion {
		knowledgeResponse(c, 409, nil, "knowledge_contract_version", "Unsupported knowledge contract version")
		return nil, false
	}
	return user, true
}

func knowledgeDecode[T any](c *gin.Context, required ...string) (*T, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	raw, readErr := io.ReadAll(c.Request.Body)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input T
	var decoded *T
	err := decoder.Decode(&decoded)
	if readErr != nil {
		err = readErr
	}
	if err == nil && decoded == nil {
		err = errors.New("JSON object is required")
	}
	if decoded != nil {
		input = *decoded
	}
	if err == nil {
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(raw, &fields); err == nil {
			for field, value := range fields {
				if field != "parent_id" && field != "group_id" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					err = errors.New("field is not nullable")
					break
				}
			}
			for _, field := range required {
				value, exists := fields[field]
				if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					err = errors.New("required field missing")
					break
				}
			}
		}
	}
	if err == nil {
		var extra interface{}
		if decoder.Decode(&extra) != io.EOF {
			err = errors.New("trailing JSON")
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		status, key := 400, "knowledge_invalid_request"
		if errors.As(err, &tooLarge) {
			status, key = 413, "knowledge_body_too_large"
		}
		knowledgeResponse(c, status, nil, key, "A single JSON object with supported fields is required")
		return nil, false
	}
	return &input, true
}

func knowledgeQuery(c *gin.Context, allowed ...string) bool {
	for key, values := range c.Request.URL.Query() {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found || len(values) != 1 {
			knowledgeResponse(c, 400, nil, "knowledge_invalid_request", "Unsupported or repeated query parameter")
			return false
		}
	}
	return true
}

func knowledgeLimit(c *gin.Context) (int, bool) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			knowledgeResponse(c, 400, nil, "knowledge_invalid_request", "limit must be between 1 and 200")
			return 0, false
		}
		limit = value
	}
	return limit, true
}

func (h *DatasetsHandler) KnowledgeContract(c *gin.Context) {
	if _, ok := knowledgeUser(c); !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	knowledgeResponse(c, 200, gin.H{"version": entity.KnowledgeContractVersion, "contracts": []string{"C1", "C3"},
		"schema": "docs/knowledge-foundation-v1.openapi.json", "identity": "opaque-source-and-structure-versioned",
		"capabilities": []string{"domains.register", "groups.create", "groups.list", "sources.snapshot", "sources.list", "sources.read", "nodes.create", "nodes.list", "nodes.read", "policy.check"},
		"limits":       gin.H{"source_bytes": 2 * 1024 * 1024, "content_bytes": 65536, "page_size": 200, "depth": 16},
		"model_calls":  false, "ingestion": false}, "", "success")
}

func (h *DatasetsHandler) ListKnowledgeDomains(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c, "cursor", "limit") {
		return
	}
	limit, ok := knowledgeLimit(c)
	if !ok {
		return
	}
	result, err := h.datasetsService.ListKnowledgeDomains(c.Request.Context(), user.ID, c.Query("cursor"), limit)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) RegisterKnowledgeDomain(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.RegisterKnowledgeDomainRequest](c)
	if !ok {
		return
	}
	result, err := h.datasetsService.RegisterKnowledgeDomain(c.Request.Context(), user.ID, c.Param("dataset_id"), *req)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) GetKnowledgeDomain(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	result, err := h.datasetsService.GetKnowledgeDomain(c.Request.Context(), user.ID, c.Param("dataset_id"))
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) CreateKnowledgeGroup(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.CreateKnowledgeGroupRequest](c, "kind", "name")
	if !ok {
		return
	}
	result, err := h.datasetsService.CreateKnowledgeGroup(c.Request.Context(), user.ID, c.Param("dataset_id"), *req)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 201, result, "", "success")
}

func (h *DatasetsHandler) ListKnowledgeGroups(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c, "parent_id", "cursor", "limit") {
		return
	}
	limit, ok := knowledgeLimit(c)
	if !ok {
		return
	}
	result, err := h.datasetsService.ListKnowledgeGroups(c.Request.Context(), user.ID, c.Param("dataset_id"), c.Query("parent_id"), c.Query("cursor"), limit)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) SnapshotKnowledgeSource(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.SnapshotKnowledgeSourceRequest](c, "document_id")
	if !ok {
		return
	}
	result, err := h.datasetsService.SnapshotKnowledgeSource(c.Request.Context(), user.ID, c.Param("dataset_id"), *req)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	status := 200
	if result.Created {
		status = 201
	}
	knowledgeResponse(c, status, result, "", "success")
}

func (h *DatasetsHandler) ListKnowledgeSources(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c, "group_id", "cursor", "limit") {
		return
	}
	limit, ok := knowledgeLimit(c)
	if !ok {
		return
	}
	result, err := h.datasetsService.ListKnowledgeSources(c.Request.Context(), user.ID, c.Param("dataset_id"), c.Query("group_id"), c.Query("cursor"), limit)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) GetKnowledgeSource(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	result, err := h.datasetsService.GetKnowledgeSource(c.Request.Context(), user.ID, c.Param("dataset_id"), c.Param("revision_id"))
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) CreateKnowledgeNode(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.CreateKnowledgeNodeRequest](c, "source_revision_id", "parent_id", "kind", "logical_key", "name", "start_byte", "end_byte")
	if !ok {
		return
	}
	result, created, err := h.datasetsService.CreateKnowledgeNode(c.Request.Context(), user.ID, c.Param("dataset_id"), *req)
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

func (h *DatasetsHandler) ListKnowledgeNodes(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c, "source_revision_id", "parent_id", "cursor", "limit") {
		return
	}
	if c.Query("source_revision_id") == "" {
		knowledgeResponse(c, 400, nil, "knowledge_invalid_request", "source_revision_id is required")
		return
	}
	limit, ok := knowledgeLimit(c)
	if !ok {
		return
	}
	result, err := h.datasetsService.ListKnowledgeNodes(c.Request.Context(), user.ID, c.Param("dataset_id"), c.Query("source_revision_id"), c.Query("parent_id"), c.Query("cursor"), limit)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) GetKnowledgeNode(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	result, err := h.datasetsService.GetKnowledgeNode(c.Request.Context(), user.ID, c.Param("dataset_id"), c.Param("node_id"))
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) ReadKnowledgeNode(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c, "start_byte", "end_byte") {
		return
	}
	var start, end *int64
	for _, parameter := range []struct {
		name  string
		value **int64
	}{{"start_byte", &start}, {"end_byte", &end}} {
		if raw, exists := c.GetQuery(parameter.name); exists {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				knowledgeResponse(c, 400, nil, "knowledge_invalid_request", "Invalid byte range")
				return
			}
			*parameter.value = &value
		}
	}
	result, err := h.datasetsService.ReadKnowledgeNode(c.Request.Context(), user.ID, c.Param("dataset_id"), c.Param("node_id"), start, end)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}

func (h *DatasetsHandler) CheckKnowledgePolicy(c *gin.Context) {
	user, ok := knowledgeUser(c)
	if !ok {
		return
	}
	if !knowledgeQuery(c) {
		return
	}
	req, ok := knowledgeDecode[entity.CheckKnowledgePolicyRequest](c, "purpose")
	if !ok {
		return
	}
	result, err := h.datasetsService.CheckKnowledgePolicy(c.Request.Context(), user.ID, c.Param("dataset_id"), req.Purpose)
	if err != nil {
		knowledgeFailure(c, err)
		return
	}
	knowledgeResponse(c, 200, result, "", "success")
}
