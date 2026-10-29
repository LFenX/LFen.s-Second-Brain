package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
	"ragflow/internal/service/dataset"
	documentservice "ragflow/internal/service/document"
)

type contentRepresentationRequest struct {
	Version string                        `json:"contract_version"`
	Target  entity.KnowledgeContentTarget `json:"target"`
}

type contentNodeRequest struct {
	Version string                         `json:"contract_version"`
	Address entity.KnowledgeContentAddress `json:"address"`
}

type contentNodesRequest struct {
	Version string                           `json:"contract_version"`
	Query   entity.KnowledgeContentNodeQuery `json:"query"`
}

type contentParsedRepresentationsRequest struct {
	Version string                                    `json:"contract_version"`
	Query   entity.KnowledgeParsedRepresentationQuery `json:"query"`
}

type contentReadRequest struct {
	Version string                             `json:"contract_version"`
	Read    entity.KnowledgeContentReadRequest `json:"read"`
}

type contentPolicyRequest struct {
	Version string                               `json:"contract_version"`
	Request entity.KnowledgeContentPolicyRequest `json:"request"`
}

type contentParsedUploadRequest struct {
	Version string                              `json:"contract_version"`
	Request entity.KnowledgeParsedUploadRequest `json:"request"`
}

type contentParsedSnapshotRequest struct {
	Version string                                `json:"contract_version"`
	Request entity.KnowledgeParsedSnapshotRequest `json:"request"`
}

type contentParsedSourceRequest struct {
	Version  string `json:"contract_version"`
	SourceID string `json:"source_id"`
}

type contentParsedTaskRequest struct {
	Version string                            `json:"contract_version"`
	Request entity.KnowledgeParsedTaskRequest `json:"request"`
}

type contentParsedTaskGetRequest struct {
	Version string `json:"contract_version"`
	TaskID  string `json:"task_id"`
}

type contentParsedTaskCancelRequest struct {
	Version string                              `json:"contract_version"`
	TaskID  string                              `json:"task_id"`
	Request entity.KnowledgeParsedCancelRequest `json:"request"`
}

func contentHandlerResponse(c *gin.Context, status int, data any, failure error) {
	body := gin.H{"operation": c.GetString("content_operation"), "contract_version": entity.KnowledgeContentVersion, "request_id": c.GetString("content_request_id"), "data": data}
	if failure != nil {
		status = http.StatusServiceUnavailable
		code := "unavailable"
		var public *dataset.KnowledgeError
		var operation *dataset.DocumentOperationError
		if errors.As(failure, &public) && slices.Contains([]string{"invalid_request", "unauthenticated", "not_found", "contract_mismatch", "content_integrity", "capacity_exceeded", "unsupported", "unavailable"}, public.Code) {
			status, code = public.Status, public.Code
		} else if (strings.HasPrefix(c.GetString("content_operation"), "parsed-") || strings.HasPrefix(c.GetString("content_operation"), "content-policy")) && errors.As(failure, &operation) && slices.Contains([]string{"invalid_request", "not_found", "unavailable", "content_integrity", "foundation_policy_changed", "content_policy_changed", "content_policy_disabled", "source_changed", "source_archived", "lifecycle_changed", "producer_changed", "capacity_exceeded", "idempotency_conflict", "task_changed", "policy_changed", "unsupported_source", "original_integrity", "original_unavailable"}, operation.Code) && slices.Contains([]int{400, 404, 409, 413, 415, 429, 503}, operation.Status) {
			status, code = operation.Status, operation.Code
			body["outcome_known"], body["rejection_id"] = operation.OutcomeKnown, operation.RejectionID
		}
		body["data"], body["code"], body["message"] = nil, code, "Knowledge content request could not be completed"
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 1<<20 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"operation": c.GetString("content_operation"), "contract_version": entity.KnowledgeContentVersion, "request_id": c.GetString("content_request_id"), "data": nil, "code": "unavailable", "message": "Knowledge content response unavailable"})
		return
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

func contentHandlerError(c *gin.Context, status int, code string) {
	contentHandlerResponse(c, status, nil, &dataset.KnowledgeError{Status: status, Code: code})
}

// KnowledgeContentAuth retains the existing native middleware and translates its
// refusals into this version's bounded response. It does not authenticate API keys.
func KnowledgeContentAuth(auth gin.HandlerFunc, operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Content-Version", entity.KnowledgeContentVersion)
		c.Set("content_operation", operation)
		c.Set("content_request_id", common.GenerateUUID())
		c.Header("X-Request-ID", c.GetString("content_request_id"))
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: http.StatusOK}
		c.Writer = buffer
		c.Set("content_writer", original)
		defer func() {
			if !c.GetBool("content_released") {
				c.Writer = original
				if buffer.status == http.StatusServiceUnavailable {
					contentHandlerError(c, http.StatusServiceUnavailable, "unavailable")
				} else {
					contentHandlerError(c, http.StatusUnauthorized, "unauthenticated")
				}
			}
		}()
		auth(c)
	}
}

func decodeContent[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		contentHandlerError(c, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if err != nil {
		contentHandlerError(c, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	return decodeContentBytes[T](c, raw)
}

func decodeContentBytes[T any](c *gin.Context, raw []byte) (*T, bool) {
	if len(raw) > 32768 || !lifecycleUnicodeValid(raw) || validateStructureObject(raw, reflect.TypeFor[T]()) != nil {
		contentHandlerError(c, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	raw, err := structureIntegerJSON(raw)
	if err != nil {
		contentHandlerError(c, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	var request T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		contentHandlerError(c, http.StatusBadRequest, "invalid_request")
		return nil, false
	}
	return &request, true
}

func decodeContentUpload(c *gin.Context) (*contentParsedUploadRequest, *multipart.FileHeader, func(), bool) {
	cleanup := func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}
	fail := func(status int, code string) (*contentParsedUploadRequest, *multipart.FileHeader, func(), bool) {
		cleanup()
		contentHandlerError(c, status, code)
		return nil, nil, func() {}, false
	}
	media, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "multipart/form-data" || params["boundary"] == "" {
		return fail(400, "invalid_request")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, entity.KnowledgeMaxSourceBytes+65536)
	if err := c.Request.ParseMultipartForm(entity.KnowledgeMaxSourceBytes); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return fail(413, "capacity_exceeded")
		}
		return fail(400, "invalid_request")
	}
	form := c.Request.MultipartForm
	if form == nil || len(form.Value) != 1 || len(form.Value["request"]) != 1 || len(form.File) != 1 || len(form.File["file"]) != 1 {
		return fail(400, "invalid_request")
	}
	request, ok := decodeContentBytes[contentParsedUploadRequest](c, []byte(form.Value["request"][0]))
	if !ok {
		cleanup()
		return nil, nil, func() {}, false
	}
	return request, form.File["file"][0], cleanup, true
}

func contentVersion(c *gin.Context, version string) bool {
	if version != entity.KnowledgeContentVersion {
		contentHandlerError(c, http.StatusPreconditionFailed, "contract_mismatch")
		return false
	}
	return true
}

// KnowledgeContent adapts unified reads and explicit owned local parsed tasks.
func (h *DatasetsHandler) KnowledgeContent(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("content_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("content_released", true)
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				contentHandlerError(c, http.StatusServiceUnavailable, "unavailable")
			}
		}()
		if c.GetBool("auth_via_api_token") {
			contentHandlerError(c, http.StatusUnauthorized, "unauthenticated")
			return
		}
		ctx, user, err := h.datasetsService.AuthenticateKnowledgeLocalSession(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			contentHandlerError(c, http.StatusUnauthorized, "unauthenticated")
			return
		}
		actor, _ := c.Get("user")
		u, ok := actor.(*entity.User)
		if !ok || u == nil || u.ID != user {
			contentHandlerError(c, http.StatusUnauthorized, "unauthenticated")
			return
		}
		domain := c.Param("dataset_id")
		if !entity.ValidKnowledgeLexicalID(domain) {
			contentHandlerError(c, http.StatusNotFound, "not_found")
			return
		}
		if _, err := h.datasetsService.GetKnowledgeDomain(ctx, user, domain); err != nil {
			var public *dataset.KnowledgeError
			if errors.As(err, &public) && public.Status == http.StatusNotFound {
				contentHandlerError(c, http.StatusNotFound, "not_found")
			} else {
				contentHandlerError(c, http.StatusServiceUnavailable, "unavailable")
			}
			return
		}
		if c.Request.URL.RawQuery != "" {
			contentHandlerError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		var data any
		status := http.StatusOK
		switch operation {
		case "capabilities":
			if c.Request.Body != nil {
				raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
				if err != nil || len(raw) != 0 {
					contentHandlerError(c, http.StatusBadRequest, "invalid_request")
					return
				}
			}
			profiles, profileErr := dataset.KnowledgeParsedProfiles()
			if profileErr != nil {
				contentHandlerResponse(c, http.StatusServiceUnavailable, nil, profileErr)
				return
			}
			data = gin.H{"projection_version": entity.KnowledgeContentVersion, "owner_only": true, "parser_registrations": parser.RegisteredParserCapabilities(), "fixed_representation_kinds": []string{"source_revision", "structure_view", "document_representation", "parsed_representation"}, "coordinates": []string{"source_utf8", "derived_utf8", "item_utf8"}, "parser_registration_is_extraction_proof": false, "unbound_parser_items_publishable": false, "max_content_bytes": 65536, "max_page_items": 100, "max_parsed_representations_page_items": 20, "parsed_profiles": profiles, "parsed_publication_runtime_ready": dao.KnowledgeContentRuntimeReady(), "parsed_policy_default_enabled": false, "max_parsed_original_bytes": entity.KnowledgeMaxSourceBytes}
		case "content-policy":
			if c.Request.Body != nil {
				raw, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 1))
				if readErr != nil || len(raw) != 0 {
					contentHandlerError(c, 400, "invalid_request")
					return
				}
			}
			data, err = h.datasetsService.KnowledgeContentPolicy(ctx, user, domain)
		case "content-policy-change":
			request, ok := decodeContent[contentPolicyRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, _, err = h.datasetsService.ChangeKnowledgeContentPolicy(ctx, user, domain, request.Request)
		case "parsed-source-upload":
			request, file, cleanup, ok := decodeContentUpload(c)
			if !ok {
				return
			}
			defer cleanup()
			if !contentVersion(c, request.Version) {
				return
			}
			data, err = documentservice.NewDocumentService().UploadKnowledgeParsedLocalDocument(ctx, user, domain, request.Request, file)
			if err != nil {
				var upload *documentservice.KnowledgeParsedUploadError
				if errors.As(err, &upload) {
					err = &dataset.DocumentOperationError{Status: upload.Status, Code: upload.Code}
				}
			}
		case "parsed-source-snapshot":
			request, ok := decodeContent[contentParsedSnapshotRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			source, sourceErr := h.datasetsService.SnapshotKnowledgeParsedSource(ctx, user, domain, request.Request)
			err = sourceErr
			if err == nil {
				data = gin.H{"source_ref": entity.KnowledgeContentSourceRef{DomainID: source.DomainID, SourceID: source.DocumentID, RevisionID: source.ID, SHA256: source.SHA256}, "representation_kind": "original_only", "source_kind": "parsed_file"}
			}
		case "parsed-source":
			request, ok := decodeContent[contentParsedSourceRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.GetKnowledgeParsedSource(ctx, user, domain, request.SourceID)
		case "parsed-task-create":
			request, ok := decodeContent[contentParsedTaskRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			var created bool
			data, created, err = h.datasetsService.CreateKnowledgeParsedTask(ctx, user, domain, request.Request)
			if created {
				status = http.StatusCreated
			}
		case "parsed-task-get":
			request, ok := decodeContent[contentParsedTaskGetRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.GetKnowledgeParsedTask(ctx, user, domain, request.TaskID)
		case "parsed-task-cancel":
			request, ok := decodeContent[contentParsedTaskCancelRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, _, err = h.datasetsService.CancelKnowledgeParsedTask(ctx, user, domain, request.TaskID, request.Request)
		case "parsed-representations":
			request, ok := decodeContent[contentParsedRepresentationsRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.ListKnowledgeParsedRepresentations(ctx, user, domain, request.Query)
		case "representation":
			request, ok := decodeContent[contentRepresentationRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.GetKnowledgeContentRepresentation(ctx, user, domain, request.Target)
		case "node":
			request, ok := decodeContent[contentNodeRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.GetKnowledgeContentNode(ctx, user, domain, request.Address)
		case "nodes":
			request, ok := decodeContent[contentNodesRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.ListKnowledgeContentNodes(ctx, user, domain, request.Query)
		case "read":
			request, ok := decodeContent[contentReadRequest](c)
			if !ok || !contentVersion(c, request.Version) {
				return
			}
			data, err = h.datasetsService.ReadKnowledgeContentNode(ctx, user, domain, request.Read)
		default:
			contentHandlerError(c, http.StatusNotFound, "not_found")
			return
		}
		contentHandlerResponse(c, status, data, err)
	}
}
