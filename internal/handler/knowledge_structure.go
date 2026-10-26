package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

// KnowledgeStructureAuth preserves native login while returning only the
// structure contract's bounded, non-cacheable error envelope.
func KnowledgeStructureAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Structure-Contract-Version", "0.1.0")
		requestID := common.GenerateUUID()
		c.Set("knowledge_request_id", requestID)
		c.Header("X-Request-ID", requestID)
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: 200}
		c.Writer = buffer
		c.Set("knowledge_structure_writer", original)
		defer func() {
			if c.GetBool("knowledge_structure_released") {
				return
			}
			c.Writer = original
			status, code := 503, "unavailable"
			if buffer.status == 401 {
				status, code = 401, "not_found"
			} else if buffer.status == 403 || buffer.status == 404 {
				status, code = 404, "not_found"
			}
			structureHandlerError(c, status, code)
		}()
		auth(c)
	}
}

func structureResponse(c *gin.Context, status int, data any, err error) {
	body := gin.H{"code": 0, "message": "success", "contract_version": "1.0.0", "structure_contract_version": "0.1.0", "request_id": c.GetString("knowledge_request_id"), "data": data}
	if err != nil {
		status = 503
		code, known := "unavailable", false
		var rejection *string
		var operation *dataset.StructureOperationError
		var public *dataset.KnowledgeError
		if errors.As(err, &operation) {
			status, code, known, rejection = operation.Status, operation.Code, operation.OutcomeKnown, operation.RejectionID
		} else if errors.As(err, &public) {
			status, code = public.Status, public.Code
		}
		allowed := []string{"invalid_request", "not_found", "contract_mismatch", "policy_disabled", "policy_changed", "revision_changed", "manifest_changed", "view_changed", "idempotency_conflict", "task_revision_changed", "capacity_exceeded", "source_unavailable", "lifecycle_integrity", "structure_integrity", "unavailable", "unsupported_source"}
		if !slices.Contains(allowed, code) {
			status, code, known, rejection = 503, "unavailable", false, nil
		}
		body["code"], body["message"], body["data"], body["outcome_known"], body["rejection_id"] = code, "Structure operation could not be completed", nil, known, rejection
		c.Header("X-Knowledge-Error-Code", code)
	}
	budget := 262144
	if c.GetBool("knowledge_structure_content") {
		budget = 524288
	}
	raw, marshalErr := json.Marshal(body)
	if marshalErr != nil || len(raw) > budget {
		status = 503
		raw, _ = json.Marshal(gin.H{"code": "unavailable", "message": "Structure response unavailable", "contract_version": "1.0.0", "structure_contract_version": "0.1.0", "request_id": c.GetString("knowledge_request_id"), "data": nil, "outcome_known": false, "rejection_id": nil})
		c.Header("X-Knowledge-Error-Code", "unavailable")
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

func structureHandlerError(c *gin.Context, status int, code string) {
	structureResponse(c, status, nil, &dataset.KnowledgeError{Status: status, Code: code, Message: "Structure request rejected"})
}

// Array elements need the same strict, case-sensitive required-field checks as
// their containing request; encoding/json alone accepts duplicate/unknown keys.
func validateStructureObject(raw []byte, typ reflect.Type) error {
	if err := validateLifecycleObject(raw, typ); err != nil {
		return err
	}
	if typ.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return validateStructureObject(raw, typ.Elem())
	}
	if typ.Kind() == reflect.Array || typ.Kind() == reflect.Slice {
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return errors.New("array required")
		}
		for _, value := range values {
			if err := validateStructureObject(value, typ.Elem()); err != nil {
				return err
			}
		}
	} else if typ.Kind() == reflect.Struct {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return errors.New("object required")
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if err := validateStructureObject(fields[field.Tag.Get("json")], field.Type); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeStructure[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		structureHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if err != nil {
		status := 400
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			status = 413
		}
		structureHandlerError(c, status, "invalid_request")
		return nil, false
	}
	if !lifecycleUnicodeValid(raw) || validateStructureObject(raw, reflect.TypeFor[T]()) != nil {
		structureHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	raw, err = structureIntegerJSON(raw)
	if err != nil {
		structureHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request T
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		structureHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	return &request, true
}

// JSON Schema integers include exact forms such as 1e0. Convert them without
// float64 rounding, and reject fractions/unsafe values before decoding DTOs.
func structureIntegerJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var normalize func(any) (any, error)
	normalize = func(v any) (any, error) {
		switch item := v.(type) {
		case json.Number:
			text := string(item)
			if index := strings.IndexAny(text, "eE"); index >= 0 {
				exponent, err := strconv.ParseInt(text[index+1:], 10, 64)
				if err != nil || exponent > 32768 || exponent < -32768 {
					return nil, errors.New("integer exponent exceeds request budget")
				}
			}
			rational, ok := new(big.Rat).SetString(text)
			if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
				return nil, errors.New("exact integer required")
			}
			integer := rational.Num().Int64()
			if integer > 9007199254740991 || integer < -9007199254740991 {
				return nil, errors.New("unsafe integer")
			}
			return integer, nil
		case []any:
			for i, child := range item {
				converted, err := normalize(child)
				if err != nil {
					return nil, err
				}
				item[i] = converted
			}
		case map[string]any:
			for key, child := range item {
				converted, err := normalize(child)
				if err != nil {
					return nil, err
				}
				item[key] = converted
			}
		}
		return v, nil
	}
	value, err := normalize(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func structureQuery(c *gin.Context, allowed []string) (url.Values, bool) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		for name, items := range values {
			if !slices.Contains(allowed, name) || len(items) != 1 || items[0] == "" || !utf8.ValidString(items[0]) {
				err = errors.New("invalid query")
			}
		}
	}
	if err != nil {
		structureHandlerError(c, 400, "invalid_request")
		return nil, false
	}
	return values, true
}

func structureQueryNumber(value string, maxValue int64) (int64, bool) {
	if value == "" {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	i, err := strconv.ParseInt(value, 10, 64)
	return i, err == nil && i >= 0 && i <= maxValue
}

// KnowledgeStructure binds the fifteen frozen operations to the Go service.
func (h *DatasetsHandler) KnowledgeStructure(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("knowledge_structure_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("knowledge_structure_released", true)
		if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" || c.GetHeader("X-Knowledge-Structure-Contract-Version") != "0.1.0" {
			structureHandlerError(c, 412, "contract_mismatch")
			return
		}
		user, exists := c.Get("user")
		actor, ok := user.(*entity.User)
		if !exists || !ok || actor == nil {
			structureHandlerError(c, 401, "not_found")
			return
		}
		for _, p := range c.Params {
			size := 32
			if p.Key == "view_id" || p.Key == "structure_node_id" {
				size = 64
			}
			if len(p.Value) != size || !structureHex(p.Value) {
				structureHandlerError(c, 404, "not_found")
				return
			}
		}
		ctx, domain := c.Request.Context(), c.Param("dataset_id")
		if domain != "" {
			if err := h.datasetsService.AuthorizeKnowledgeStructure(ctx, actor.ID, domain); err != nil {
				structureResponse(c, 503, nil, err)
				return
			}
		}
		// Authorize fixed objects before parsing a cursor or its filters.
		if viewID := c.Param("view_id"); viewID != "" {
			if _, err := h.datasetsService.GetKnowledgeStructureView(ctx, actor.ID, domain, viewID); err != nil {
				structureResponse(c, 503, nil, err)
				return
			}
		}
		allowed := []string{}
		switch operation {
		case "getKnowledgeStructureCommandByKey":
			allowed = []string{"kind", "idempotency_key"}
		case "getKnowledgeStructureTaskByKey":
			allowed = []string{"idempotency_key"}
		case "listKnowledgeStructureTasks":
			allowed = []string{"project_id", "limit", "cursor"}
		case "listKnowledgeStructureNodes":
			allowed = []string{"source_id", "parent_id", "limit", "cursor"}
		case "readKnowledgeStructureNode":
			allowed = []string{"start_byte", "end_byte"}
		case "listKnowledgeStructureRelations":
			allowed = []string{"node_id", "direction", "kind", "limit", "cursor"}
		}
		query, ok := structureQuery(c, allowed)
		if !ok {
			return
		}
		required := ""
		switch operation {
		case "getKnowledgeStructureCommandByKey", "getKnowledgeStructureTaskByKey":
			required = "idempotency_key"
		case "listKnowledgeStructureTasks":
			required = "project_id"
		case "listKnowledgeStructureNodes":
			required = "source_id"
		case "listKnowledgeStructureRelations":
			required = "node_id"
		}
		if required != "" && !query.Has(required) {
			structureHandlerError(c, 400, "invalid_request")
			return
		}
		limit := 20
		if query.Has("limit") {
			v, ok := structureQueryNumber(query.Get("limit"), 20)
			if !ok || v == 0 {
				structureHandlerError(c, 400, "invalid_request")
				return
			}
			limit = int(v)
		}
		var data any
		var err error
		created := false
		switch operation {
		case "getKnowledgeStructureContract":
			var capabilities entity.KnowledgeStructureCapabilities
			err = json.Unmarshal(entity.KnowledgeStructureCapabilitiesJSON, &capabilities)
			capabilities.RuntimeReady = dao.KnowledgeStructureRuntimeReady() && entity.KnowledgeStructureProducerCompatible()
			data = capabilities
		case "getKnowledgeStructurePolicy":
			data, err = h.datasetsService.KnowledgeStructurePolicy(ctx, actor.ID, domain)
		case "changeKnowledgeStructurePolicy":
			request, ok := decodeStructure[entity.KnowledgeStructurePolicyChangeRequest](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.ChangeKnowledgeStructurePolicy(ctx, actor.ID, domain, *request)
		case "getKnowledgeStructureCommandByKey":
			data, err = h.datasetsService.GetKnowledgeStructureCommandByKey(ctx, actor.ID, domain, query.Get("kind"), query.Get("idempotency_key"))
		case "createKnowledgeStructureTask":
			request, ok := decodeStructure[entity.KnowledgeStructureTaskRequest](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.CreateKnowledgeStructureTask(ctx, actor.ID, domain, *request)
		case "getKnowledgeStructureTaskByKey":
			data, err = h.datasetsService.GetKnowledgeStructureTaskByKey(ctx, actor.ID, domain, query.Get("idempotency_key"))
		case "getKnowledgeStructureTask":
			data, err = h.datasetsService.GetKnowledgeStructureTask(ctx, actor.ID, domain, c.Param("task_id"))
		case "listKnowledgeStructureTasks":
			data, err = h.datasetsService.ListKnowledgeStructureTasks(ctx, actor.ID, domain, query.Get("project_id"), query.Get("cursor"), limit)
		case "cancelKnowledgeStructureTask":
			request, ok := decodeStructure[entity.KnowledgeStructureCancelRequest](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.CancelKnowledgeStructureTask(ctx, actor.ID, domain, c.Param("task_id"), *request)
		case "getKnowledgeProjectStructureHead":
			data, err = h.datasetsService.GetKnowledgeStructureHead(ctx, actor.ID, domain, c.Param("project_id"))
		case "getKnowledgeStructureView":
			data, err = h.datasetsService.GetKnowledgeStructureView(ctx, actor.ID, domain, c.Param("view_id"))
		case "getKnowledgeStructureNode":
			data, err = h.datasetsService.GetKnowledgeStructureNode(ctx, actor.ID, domain, c.Param("view_id"), c.Param("structure_node_id"))
		case "listKnowledgeStructureNodes":
			data, err = h.datasetsService.ListKnowledgeStructureNodes(ctx, actor.ID, domain, c.Param("view_id"), query.Get("source_id"), query.Get("parent_id"), query.Get("cursor"), limit)
		case "listKnowledgeStructureRelations":
			data, err = h.datasetsService.ListKnowledgeStructureRelations(ctx, actor.ID, domain, c.Param("view_id"), query.Get("node_id"), query.Get("direction"), query.Get("kind"), query.Get("cursor"), limit)
		case "readKnowledgeStructureNode":
			var start, end *int64
			if query.Has("start_byte") {
				v, ok := structureQueryNumber(query.Get("start_byte"), 2097152)
				if !ok {
					structureHandlerError(c, 400, "invalid_request")
					return
				}
				start = &v
			}
			if query.Has("end_byte") {
				v, ok := structureQueryNumber(query.Get("end_byte"), 2097152)
				if !ok {
					structureHandlerError(c, 400, "invalid_request")
					return
				}
				end = &v
			}
			c.Set("knowledge_structure_content", true)
			data, err = h.datasetsService.ReadKnowledgeStructureNode(ctx, actor.ID, domain, c.Param("view_id"), c.Param("structure_node_id"), start, end)
		default:
			structureHandlerError(c, 404, "not_found")
			return
		}
		status := 200
		if created {
			status = 201
		}
		structureResponse(c, status, data, err)
	}
}

func structureHex(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
