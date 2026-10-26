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
	"slices"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func KnowledgeDocumentAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Document-Structure-Contract-Version", "0.1.0")
		id := common.GenerateUUID()
		c.Set("knowledge_request_id", id)
		c.Header("X-Request-ID", id)
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: 200}
		c.Writer = buffer
		c.Set("knowledge_document_writer", original)
		defer func() {
			if c.GetBool("knowledge_document_released") {
				return
			}
			c.Writer = original
			status, code := 503, "unavailable"
			if buffer.status == 401 {
				status, code = 401, "unauthorized"
			} else if buffer.status == 403 || buffer.status == 404 {
				status, code = 404, "not_found"
			}
			documentHandlerError(c, status, code)
		}()
		auth(c)
	}
}

func documentResponse(c *gin.Context, status int, data any, err error) {
	body := gin.H{"code": 0, "message": "success", "contract_version": "1.0.0", "document_structure_contract_version": "0.1.0", "request_id": c.GetString("knowledge_request_id"), "data": data}
	if err != nil {
		status = 503
		code, known := "unavailable", false
		var rejected *string
		var own *dataset.DocumentOperationError
		if errors.As(err, &own) {
			status, code, known, rejected = own.Status, own.Code, own.OutcomeKnown, own.RejectionID
		} else {
			var old *dataset.KnowledgeError
			if errors.As(err, &old) && old.Status == 404 {
				status, code = 404, "not_found"
			}
		}
		allowed := map[int][]string{400: {"invalid_request"}, 401: {"unauthorized"}, 404: {"not_found"}, 412: {"contract_mismatch"}, 409: {"foundation_policy_changed", "document_policy_disabled", "document_policy_changed", "source_revision_changed", "lifecycle_changed", "source_archived", "head_changed", "producer_changed", "unsupported_source", "task_revision_changed", "idempotency_conflict", "original_integrity", "document_integrity"}, 429: {"capacity_exceeded"}, 503: {"unavailable"}}
		if !slices.Contains(allowed[status], code) {
			status, code, known, rejected = 503, "unavailable", false, nil
		}
		if !known || rejected == nil {
			known, rejected = false, nil
		}
		if status == 429 && !known {
			status, code = 503, "unavailable"
		}
		body["code"], body["message"], body["data"], body["outcome_known"], body["rejection_id"] = code, "Document operation could not be completed", nil, known, rejected
		c.Header("X-Knowledge-Error-Code", code)
	}
	budget := 262144
	if c.GetBool("knowledge_document_content") {
		budget = 524288
	}
	raw, marshalErr := json.Marshal(body)
	if marshalErr != nil || len(raw) > budget {
		status = 503
		raw, _ = json.Marshal(gin.H{"code": "unavailable", "message": "Document response unavailable", "contract_version": "1.0.0", "document_structure_contract_version": "0.1.0", "request_id": c.GetString("knowledge_request_id"), "data": nil, "outcome_known": false, "rejection_id": nil})
		c.Header("X-Knowledge-Error-Code", "unavailable")
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

func documentHandlerError(c *gin.Context, status int, code string) {
	documentResponse(c, status, nil, &dataset.DocumentOperationError{Status: status, Code: code})
}

func decodeDocument[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		documentHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if err != nil || !lifecycleUnicodeValid(raw) || validateStructureObject(raw, reflect.TypeFor[T]()) != nil {
		documentHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err = structureIntegerJSON(raw)
	if err != nil {
		documentHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value T
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		documentHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	return &value, true
}

func documentQuery(c *gin.Context, allowed []string) (url.Values, bool) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		for key, v := range values {
			if !slices.Contains(allowed, key) || len(v) != 1 || v[0] == "" || !utf8.ValidString(v[0]) {
				err = errors.New("invalid query")
			}
		}
	}
	if err != nil {
		documentHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	return values, true
}

func (h *DatasetsHandler) KnowledgeDocument(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("knowledge_document_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("knowledge_document_released", true)
		if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" || c.GetHeader("X-Knowledge-Document-Structure-Contract-Version") != "0.1.0" {
			documentHandlerError(c, 412, "contract_mismatch")
			return
		}
		user, ok := c.Get("user")
		actor, valid := user.(*entity.User)
		if !ok || !valid || actor == nil {
			documentHandlerError(c, 401, "unauthorized")
			return
		}
		for _, p := range c.Params {
			size := 32
			if p.Key == "representation_id" || p.Key == "document_node_id" {
				size = 64
			}
			if len(p.Value) != size || !structureHex(p.Value) {
				documentHandlerError(c, 404, "not_found")
				return
			}
		}
		ctx, domain := c.Request.Context(), c.Param("dataset_id")
		if domain != "" {
			if err := h.datasetsService.AuthorizeKnowledgeStructure(ctx, actor.ID, domain); err != nil {
				documentResponse(c, 503, nil, err)
				return
			}
		}
		if rep := c.Param("representation_id"); rep != "" {
			if _, err := h.datasetsService.GetKnowledgeDocumentRepresentation(ctx, actor.ID, domain, rep); err != nil {
				documentResponse(c, 503, nil, err)
				return
			}
		}
		allowed := []string{}
		switch operation {
		case "DC05":
			allowed = []string{"kind", "idempotency_key"}
		case "DC10":
			allowed = []string{"parent_id", "limit", "cursor"}
		case "DC12":
			allowed = []string{"start_byte", "end_byte"}
		}
		query, ok := documentQuery(c, allowed)
		if !ok {
			return
		}
		if operation == "DC05" && (!query.Has("kind") || !query.Has("idempotency_key")) {
			documentHandlerError(c, 400, "invalid_request")
			return
		}
		if operation == "DC10" && (!query.Has("parent_id") || len(query.Get("parent_id")) != 64 || !structureHex(query.Get("parent_id"))) {
			documentHandlerError(c, 400, "invalid_request")
			return
		}
		limit := 20
		if query.Has("limit") {
			v, ok := structureQueryNumber(query.Get("limit"), 20)
			if !ok || v < 1 {
				documentHandlerError(c, 400, "invalid_request")
				return
			}
			limit = int(v)
		}
		var value any
		var err error
		created := false
		switch operation {
		case "DC01":
			value, err = dataset.KnowledgeDocumentCapabilities()
		case "DC02":
			value, err = h.datasetsService.KnowledgeDocumentPolicy(ctx, actor.ID, domain)
		case "DC03":
			req, ok := decodeDocument[entity.KnowledgeDocumentPolicyChangeRequest](c)
			if !ok {
				return
			}
			value, created, err = h.datasetsService.ChangeKnowledgeDocumentPolicy(ctx, actor.ID, domain, *req)
		case "DC04":
			req, ok := decodeDocument[entity.KnowledgeDocumentTaskRequest](c)
			if !ok {
				return
			}
			value, created, err = h.datasetsService.CreateKnowledgeDocumentTask(ctx, actor.ID, domain, *req)
		case "DC05":
			value, err = h.datasetsService.GetKnowledgeDocumentOperation(ctx, actor.ID, domain, query.Get("kind"), query.Get("idempotency_key"))
		case "DC06":
			value, err = h.datasetsService.GetKnowledgeDocumentTask(ctx, actor.ID, domain, c.Param("task_id"))
		case "DC07":
			req, ok := decodeDocument[entity.KnowledgeDocumentCancelRequest](c)
			if !ok {
				return
			}
			value, created, err = h.datasetsService.CancelKnowledgeDocumentTask(ctx, actor.ID, domain, c.Param("task_id"), *req)
		case "DC08":
			value, err = h.datasetsService.GetKnowledgeDocumentHead(ctx, actor.ID, domain, c.Param("revision_id"))
		case "DC09":
			value, err = h.datasetsService.GetKnowledgeDocumentRepresentation(ctx, actor.ID, domain, c.Param("representation_id"))
		case "DC10":
			value, err = h.datasetsService.ListKnowledgeDocumentNodes(ctx, actor.ID, domain, c.Param("representation_id"), query.Get("parent_id"), query.Get("cursor"), limit)
		case "DC11":
			value, err = h.datasetsService.GetKnowledgeDocumentNode(ctx, actor.ID, domain, c.Param("representation_id"), c.Param("document_node_id"))
		case "DC12":
			var start, end *int64
			for _, key := range []string{"start_byte", "end_byte"} {
				if query.Has(key) {
					v, ok := structureQueryNumber(query.Get(key), 4194304)
					if !ok {
						documentHandlerError(c, 400, "invalid_request")
						return
					}
					if key == "start_byte" {
						start = &v
					} else {
						end = &v
					}
				}
			}
			if end != nil && start == nil {
				documentHandlerError(c, 400, "invalid_request")
				return
			}
			c.Set("knowledge_document_content", true)
			value, err = h.datasetsService.ReadKnowledgeDocumentNode(ctx, actor.ID, domain, c.Param("representation_id"), c.Param("document_node_id"), start, end)
		default:
			documentHandlerError(c, 404, "not_found")
			return
		}
		status := 200
		if created {
			status = 201
		}
		documentResponse(c, status, value, err)
	}
}
