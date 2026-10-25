package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func KnowledgeBatchAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return knowledgeSourceAuth(auth, "0.4.0", "batch", batchResponse)
}
func batchResponse(c *gin.Context, status int, data any, err error) {
	body := gin.H{"code": 0, "message": "success", "data": data, "contract_version": "1.0.0", "source_contract_version": "0.4.0", "request_id": c.GetString("knowledge_request_id")}
	if err != nil {
		var public *dataset.KnowledgeDirectoryError
		if !errors.As(err, &public) {
			reason := "result_unknown"
			public = &dataset.KnowledgeDirectoryError{Status: 503, Code: "knowledge_batch_unavailable", Reason: &reason}
		}
		status = public.Status
		body["code"] = public.Code
		body["reason"] = public.Reason
		body["message"] = "Batch operation could not be completed"
		delete(body, "data")
	}
	raw, e := json.Marshal(body)
	limit := 262144
	if c.GetBool("batch_content") {
		limit = 1048576
	}
	if e != nil || len(raw) > limit {
		delete(body, "data")
		body["code"] = "knowledge_batch_too_large"
		body["reason"] = nil
		body["message"] = "Response exceeds batch limit"
		status = 413
		raw, _ = json.Marshal(body)
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}
func batchHandlerError(c *gin.Context, status int, code string) {
	batchResponse(c, status, nil, &dataset.KnowledgeDirectoryError{Status: status, Code: "knowledge_batch_" + code})
}
func decodeBatch[T any](c *gin.Context) (*T, bool) {
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		batchHandlerError(c, 400, "invalid")
		return nil, false
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			batchHandlerError(c, 413, "too_large")
		} else {
			batchHandlerError(c, 400, "invalid")
		}
		return nil, false
	}
	if !utf8.Valid(raw) || validateDirectoryObject(raw, reflect.TypeFor[T]()) != nil {
		batchHandlerError(c, 400, "invalid")
		return nil, false
	}
	var req T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF {
		batchHandlerError(c, 400, "invalid")
		return nil, false
	}
	return &req, true
}

// KnowledgeBatch binds the frozen 0.4 operations to catalogue-only services.
func (h *DatasetsHandler) KnowledgeBatch(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Set("knowledge_request_id", common.GenerateUUID())
		c.Header("X-Request-ID", c.GetString("knowledge_request_id"))
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Source-Contract-Version", "0.4.0")
		user, code, _ := GetUser(c)
		if code != common.CodeSuccess || user == nil {
			c.JSON(401, gin.H{"code": 401, "message": "Authentication required", "data": nil})
			return
		}
		if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" || c.GetHeader("X-Knowledge-Source-Contract-Version") != "0.4.0" {
			batchHandlerError(c, 409, "version_mismatch")
			return
		}
		for _, p := range c.Params {
			if p.Key != "idempotency_key" && !directoryID.MatchString(p.Value) {
				batchHandlerError(c, 404, "not_found")
				return
			}
		}
		query, e := url.ParseQuery(c.Request.URL.RawQuery)
		if e != nil {
			batchHandlerError(c, 400, "invalid")
			return
		}
		for key, values := range query {
			allow := false
			if key == "cursor" {
				switch operation {
				case "listKnowledgeBatchTasks", "listKnowledgeBatchTaskItems", "getKnowledgeProjectManifestPreview", "listKnowledgeProjectManifests", "getKnowledgeProjectManifest":
					allow = true
				}
			}
			if (key == "offset" || key == "length") && operation == "readKnowledgeBatchPreviewItem" {
				allow = true
			}
			if !allow || len(values) != 1 || values[0] == "" {
				batchHandlerError(c, 400, "invalid")
				return
			}
		}
		ctx := c.Request.Context()
		owner, domain, reg := user.ID, c.Param("dataset_id"), c.Param("registration_id")
		var result any
		var err error
		status := 200
		switch operation {
		case "getKnowledgeBatchImportContract":
			err = json.Unmarshal(entity.KnowledgeBatchCapabilitiesJSON, &result)
		case "prepareKnowledgeBatch":
			req, ok := decodeBatch[entity.KnowledgeBatchPreviewRequest](c)
			if !ok {
				return
			}
			result, err = h.datasetsService.PrepareKnowledgeBatch(ctx, owner, domain, reg, *req)
			status = 201
		case "getKnowledgeBatchPreview":
			result, err = h.datasetsService.GetKnowledgeBatchPreview(ctx, owner, domain, reg, c.Param("preview_id"))
		case "readKnowledgeBatchPreviewItem":
			offset, e1 := strconv.ParseInt(query.Get("offset"), 10, 64)
			length, e2 := strconv.ParseInt(query.Get("length"), 10, 64)
			if e1 != nil || e2 != nil {
				batchHandlerError(c, 416, "range")
				return
			}
			c.Set("batch_content", true)
			result, err = h.datasetsService.ReadKnowledgeBatchPreviewItem(ctx, owner, domain, reg, c.Param("preview_id"), c.Param("item_id"), offset, length)
		case "acceptKnowledgeBatchTask":
			req, ok := decodeBatch[entity.KnowledgeBatchAcceptRequest](c)
			if !ok {
				return
			}
			var created bool
			result, created, err = h.datasetsService.AcceptKnowledgeBatchTask(ctx, owner, domain, reg, *req)
			if created {
				status = 202
			}
		case "listKnowledgeBatchTasks":
			result, err = h.datasetsService.ListKnowledgeBatchTasks(ctx, owner, domain, reg, query.Get("cursor"))
		case "getKnowledgeBatchTask":
			result, err = h.datasetsService.GetKnowledgeBatchTask(ctx, owner, domain, reg, c.Param("task_id"), "")
		case "getKnowledgeBatchTaskByKey":
			result, err = h.datasetsService.GetKnowledgeBatchTask(ctx, owner, domain, reg, "", c.Param("idempotency_key"))
		case "listKnowledgeBatchTaskItems":
			result, err = h.datasetsService.ListKnowledgeBatchTaskItems(ctx, owner, domain, reg, c.Param("task_id"), query.Get("cursor"))
		case "cancelKnowledgeBatchTask":
			req, ok := decodeBatch[entity.KnowledgeBatchCancellationRequest](c)
			if !ok {
				return
			}
			result, err = h.datasetsService.CancelKnowledgeBatchTask(ctx, owner, domain, reg, c.Param("task_id"), *req)
		case "prepareKnowledgeProjectManifest":
			req, ok := decodeBatch[entity.KnowledgeProjectManifestPreviewRequest](c)
			if !ok {
				return
			}
			result, err = h.datasetsService.PrepareKnowledgeProjectManifest(ctx, owner, domain, *req)
			status = 201
		case "getKnowledgeProjectManifestPreview":
			result, err = h.datasetsService.GetKnowledgeProjectManifestPreview(ctx, owner, domain, c.Param("preview_id"), query.Get("cursor"))
		case "publishKnowledgeProjectManifest":
			req, ok := decodeBatch[entity.KnowledgeProjectManifestPublicationRequest](c)
			if !ok {
				return
			}
			var created bool
			result, created, err = h.datasetsService.PublishKnowledgeProjectManifest(ctx, owner, domain, *req)
			if created {
				status = 201
			}
		case "getKnowledgeProjectManifestPublicationByKey":
			result, err = h.datasetsService.GetKnowledgeProjectManifestPublicationByKey(ctx, owner, domain, c.Param("project_id"), c.Param("idempotency_key"))
		case "getKnowledgeProjectManifestHead":
			result, err = h.datasetsService.GetKnowledgeProjectManifestHead(ctx, owner, domain, c.Param("project_id"))
		case "listKnowledgeProjectManifests":
			result, err = h.datasetsService.ListKnowledgeProjectManifests(ctx, owner, domain, c.Param("project_id"), query.Get("cursor"))
		case "getKnowledgeProjectManifest":
			result, err = h.datasetsService.GetKnowledgeProjectManifest(ctx, owner, domain, c.Param("project_id"), c.Param("manifest_id"), query.Get("cursor"))
		default:
			batchHandlerError(c, 404, "not_found")
			return
		}
		batchResponse(c, status, result, err)
	}
}
