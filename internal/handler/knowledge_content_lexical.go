package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"slices"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func KnowledgeContentLexicalAuth(auth gin.HandlerFunc, operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Content-Version", entity.KnowledgeContentVersion)
		c.Header("X-Knowledge-Content-Lexical-Protocol", entity.KnowledgeContentLexicalProtocol)
		c.Set("content_lexical_operation", operation)
		c.Set("content_lexical_request_id", common.GenerateUUID())
		c.Header("X-Request-ID", c.GetString("content_lexical_request_id"))
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: http.StatusOK}
		c.Writer = buffer
		c.Set("content_lexical_writer", original)
		defer func() {
			if !c.GetBool("content_lexical_released") {
				c.Writer = original
				status, code := 401, "unauthenticated"
				if buffer.status == 503 {
					status, code = 503, "unavailable"
				}
				contentLexicalHandlerError(c, status, code)
			}
		}()
		auth(c)
	}
}

func contentLexicalHandlerResponse(c *gin.Context, status int, data any, failure error) {
	body := gin.H{"operation": c.GetString("content_lexical_operation"), "contract_version": entity.KnowledgeContentVersion, "request_id": c.GetString("content_lexical_request_id"), "data": data}
	if failure != nil {
		public := &dataset.KnowledgeLexicalError{Status: 503, Code: "unavailable", RetrySameKey: true}
		var found *dataset.KnowledgeLexicalError
		if errors.As(failure, &found) && slices.Contains([]string{"invalid_request", "unauthenticated", "not_found", "contract_mismatch", "policy_disabled", "policy_changed", "profile_changed", "source_changed", "idempotency_conflict", "index_unindexed", "index_building", "index_coverage_gap", "index_stale", "index_failed", "coverage_gap", "unit_oversize", "unsupported_representation", "source_unavailable", "integrity_error", "capacity_exceeded", "empty_content", "unavailable"}, found.Code) && slices.Contains([]int{400, 401, 404, 409, 412, 413, 415, 429, 503}, found.Status) {
			public = found
		}
		status = public.Status
		body["data"], body["code"], body["message"] = nil, public.Code, "Content lexical operation could not be completed"
		body["outcome_known"], body["retry_same_key"] = public.OutcomeKnown, public.RetrySameKey
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 1<<20 {
		// The fixed fallback does not serialize the rejected value again.
		status = 503
		raw, _ = json.Marshal(gin.H{"operation": c.GetString("content_lexical_operation"), "contract_version": entity.KnowledgeContentVersion, "request_id": c.GetString("content_lexical_request_id"), "data": nil, "code": "unavailable", "message": "Content lexical operation could not be completed", "outcome_known": false, "retry_same_key": true})
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

func contentLexicalHandlerError(c *gin.Context, status int, code string) {
	contentLexicalHandlerResponse(c, status, nil, &dataset.KnowledgeLexicalError{Status: status, Code: code, OutcomeKnown: true})
}

func decodeContentLexical[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		contentLexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	var checked json.RawMessage
	if err != nil || !lifecycleUnicodeValid(raw) || entity.KnowledgeParsedDecode(raw, &checked) != nil {
		contentLexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err = structureIntegerJSON(raw)
	if err != nil || validateContentLexicalObject(raw, reflect.TypeFor[T]()) != nil {
		contentLexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request T
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		contentLexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	return &request, true
}

// The frozen locator/position codecs own their closed unions. Reflection walks
// only ordinary DTO fields after duplicate detection and exact integer handling.
func validateContentLexicalObject(raw []byte, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return validateContentLexicalObject(raw, typ.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("null field")
	}
	if typ == reflect.TypeFor[entity.KnowledgeDocumentLocator]() || typ == reflect.TypeFor[entity.KnowledgeDocumentOriginalPosition]() {
		return json.Unmarshal(raw, reflect.New(typ).Interface())
	}
	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return errors.New("array required")
		}
		for _, value := range values {
			if err := validateContentLexicalObject(value, typ.Elem()); err != nil {
				return err
			}
		}
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return errors.New("object required")
		}
		if len(fields) != typ.NumField() {
			return errors.New("unknown or missing field")
		}
		for i := range typ.NumField() {
			field := typ.Field(i)
			value, found := fields[field.Tag.Get("json")]
			if !found {
				return errors.New("required field missing")
			}
			if err := validateContentLexicalObject(value, field.Type); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *DatasetsHandler) KnowledgeContentLexical(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("content_lexical_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("content_lexical_released", true)
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				contentLexicalHandlerError(c, 503, "unavailable")
			}
		}()
		if c.GetBool("auth_via_api_token") {
			contentLexicalHandlerError(c, 401, "unauthenticated")
			return
		}
		ctx, user, err := h.datasetsService.AuthenticateKnowledgeLocalSession(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			contentLexicalHandlerResponse(c, 401, nil, err)
			return
		}
		actor, _ := c.Get("user")
		current, ok := actor.(*entity.User)
		if !ok || current == nil || current.ID != user {
			contentLexicalHandlerError(c, 401, "unauthenticated")
			return
		}
		domain := c.Param("dataset_id")
		if !entity.ValidKnowledgeLexicalID(domain) {
			contentLexicalHandlerError(c, 404, "not_found")
			return
		}
		if _, err := h.datasetsService.GetKnowledgeLocalPolicy(ctx, user, domain); err != nil {
			contentLexicalHandlerResponse(c, 503, nil, err)
			return
		}
		if c.Request.URL.RawQuery != "" {
			contentLexicalHandlerError(c, 400, "invalid_request")
			return
		}
		var data any
		status := 200
		switch operation {
		case "inspectIndex":
			request, ok := decodeContentLexical[entity.KnowledgeContentLexicalInspectRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.InspectKnowledgeContentLexical(ctx, user, domain, *request)
		case "createIndex":
			request, ok := decodeContentLexical[entity.KnowledgeContentLexicalCreateRequest](c)
			if !ok {
				return
			}
			data, _, err = h.datasetsService.CreateKnowledgeContentLexicalIndex(ctx, user, domain, *request)
			status = 202
		case "getIndexJob":
			request, ok := decodeContentLexical[entity.KnowledgeContentLexicalJobRequest](c)
			if !ok {
				return
			}
			if request.Version != entity.KnowledgeContentVersion {
				contentLexicalHandlerError(c, 412, "contract_mismatch")
				return
			}
			data, err = h.datasetsService.GetKnowledgeContentLexicalJob(ctx, user, domain, request.ID)
		case "search":
			request, ok := decodeContentLexical[entity.KnowledgeContentLexicalSearchRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.SearchKnowledgeContentLexical(ctx, user, domain, *request)
		case "read":
			request, ok := decodeContentLexical[entity.KnowledgeContentLexicalReadRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.ReadKnowledgeContentLexical(ctx, user, domain, *request)
		default:
			contentLexicalHandlerError(c, 404, "not_found")
			return
		}
		contentLexicalHandlerResponse(c, status, data, err)
	}
}
