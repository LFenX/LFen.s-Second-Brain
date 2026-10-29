package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
	"reflect"
	"slices"
)

func KnowledgeLocalLexicalAuth(auth gin.HandlerFunc, operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Local-Lexical-Version", entity.KnowledgeLocalLexicalVersion)
		c.Set("lexical_operation", operation)
		c.Set("lexical_request_id", common.GenerateUUID())
		c.Header("X-Request-ID", c.GetString("lexical_request_id"))
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: 200}
		c.Writer = buffer
		c.Set("lexical_writer", original)
		defer func() {
			if !c.GetBool("lexical_released") {
				c.Writer = original
				status, code := 401, "unauthenticated"
				if buffer.status == 503 {
					status, code = 503, "unavailable"
				}
				lexicalHandlerResponse(c, status, nil, &dataset.KnowledgeLexicalError{Status: status, Code: code, OutcomeKnown: true})
			}
		}()
		auth(c)
	}
}
func lexicalHandlerResponse(c *gin.Context, status int, data any, err error) {
	body := gin.H{"operation": c.GetString("lexical_operation"), "contract_version": entity.KnowledgeLocalLexicalVersion, "request_id": c.GetString("lexical_request_id"), "data": data}
	if err != nil {
		public := &dataset.KnowledgeLexicalError{Status: 503, Code: "unavailable", RetrySameKey: true}
		var e *dataset.KnowledgeLexicalError
		if errors.As(err, &e) {
			public = e
		}
		allowed := []string{"invalid_request", "unauthenticated", "not_found", "contract_mismatch", "policy_disabled", "policy_changed", "idempotency_conflict", "index_unindexed", "index_building", "index_coverage_gap", "index_stale", "index_failed", "unit_oversize", "unsupported_representation", "source_unavailable", "integrity_error", "capacity_exceeded", "unavailable"}
		if !slices.Contains(allowed, public.Code) {
			public = &dataset.KnowledgeLexicalError{Status: 503, Code: "unavailable", RetrySameKey: true}
		}
		status = public.Status
		body["data"] = nil
		body["code"] = public.Code
		body["message"] = "Local lexical operation could not be completed"
		body["outcome_known"] = public.OutcomeKnown
		body["retry_same_key"] = public.RetrySameKey
	}
	raw, marshalErr := json.Marshal(body)
	if marshalErr != nil || len(raw) > 1<<20 {
		lexicalHandlerError(c, 503, "unavailable")
		return
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}
func lexicalHandlerError(c *gin.Context, status int, code string) {
	lexicalHandlerResponse(c, status, nil, &dataset.KnowledgeLexicalError{Status: status, Code: code, OutcomeKnown: true})
}

func decodeLexical[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		lexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
	if err != nil {
		lexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	if !lifecycleUnicodeValid(raw) || validateStructureObject(raw, reflect.TypeFor[T]()) != nil {
		lexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err = structureIntegerJSON(raw)
	if err != nil {
		lexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var req T
	if decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF {
		lexicalHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	return &req, true
}

func (h *DatasetsHandler) KnowledgeLocalLexical(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("lexical_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("lexical_released", true)
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				lexicalHandlerError(c, 503, "unavailable")
			}
		}()
		if c.GetBool("auth_via_api_token") {
			lexicalHandlerError(c, 401, "unauthenticated")
			return
		}
		ctx, user, err := h.datasetsService.AuthenticateKnowledgeLocalSession(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			lexicalHandlerResponse(c, 401, nil, err)
			return
		}
		actor, _ := c.Get("user")
		u, ok := actor.(*entity.User)
		if !ok || u == nil || u.ID != user {
			lexicalHandlerError(c, 401, "unauthenticated")
			return
		}
		domain := c.Param("dataset_id")
		if !entity.ValidKnowledgeLexicalID(domain) {
			lexicalHandlerError(c, 404, "not_found")
			return
		}
		if _, err := h.datasetsService.GetKnowledgeLocalPolicy(ctx, user, domain); err != nil {
			lexicalHandlerResponse(c, 503, nil, err)
			return
		}
		if c.Request.URL.RawQuery != "" {
			lexicalHandlerError(c, 400, "invalid_request")
			return
		}
		if c.Request.Method == http.MethodGet && c.Request.Body != nil {
			raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
			if err != nil || len(raw) > 0 {
				lexicalHandlerError(c, 400, "invalid_request")
				return
			}
		}
		var data any
		status := 200
		switch operation {
		case "getLocalPolicy":
			data, err = h.datasetsService.GetKnowledgeLocalPolicy(ctx, user, domain)
		case "changeLocalPolicy":
			req, ok := decodeLexical[entity.KnowledgeLexicalPolicyChange](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.ChangeKnowledgeLocalPolicy(ctx, user, domain, *req)
		case "inspectIndex":
			req, ok := decodeLexical[entity.KnowledgeLexicalInspectRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.InspectKnowledgeLocalIndex(ctx, user, domain, *req)
		case "createIndex":
			req, ok := decodeLexical[entity.KnowledgeLexicalCreateRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.CreateKnowledgeLocalIndex(ctx, user, domain, *req)
			status = 202
		case "getIndexJob":
			data, err = h.datasetsService.GetKnowledgeLocalIndexJob(ctx, user, domain, c.Param("job_id"))
		case "search":
			req, ok := decodeLexical[entity.KnowledgeLexicalSearchRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.SearchKnowledgeLocal(ctx, user, domain, *req)
		case "read":
			req, ok := decodeLexical[entity.KnowledgeLexicalReadRequest](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.ReadKnowledgeLocal(ctx, user, domain, *req)
		default:
			lexicalHandlerError(c, 404, "not_found")
			return
		}
		if err == nil {
			var policy *entity.KnowledgeLexicalPolicy
			policy, err = h.datasetsService.GetKnowledgeLocalPolicy(ctx, user, domain)
			if err == nil && operation == "search" {
				if !policy.Enabled {
					err = &dataset.KnowledgeLexicalError{Status: 409, Code: "policy_disabled", OutcomeKnown: true}
				} else if search, ok := data.(*entity.KnowledgeLexicalSearchData); !ok || search.Binding.PolicyVersion != policy.Version || search.Binding.FoundationVersion != policy.FoundationVersion {
					err = &dataset.KnowledgeLexicalError{Status: 409, Code: "index_stale", OutcomeKnown: true}
				}
			}
		}
		lexicalHandlerResponse(c, status, data, err)
	}
}
