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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func KnowledgeLifecycleAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Contract-Version", entity.KnowledgeContractVersion)
		c.Header("X-Knowledge-Source-Contract-Version", entity.KnowledgeLifecycleContractVersion)
		id := common.GenerateUUID()
		c.Set("knowledge_request_id", id)
		c.Header("X-Request-ID", id)
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: 200}
		c.Writer = buffer
		c.Set("knowledge_lifecycle_writer", original)
		defer func() {
			if c.GetBool("knowledge_lifecycle_released") {
				return
			}
			c.Writer = original
			status, code := 503, "lifecycle_result_unknown"
			if buffer.status == 401 {
				status, code = 401, "knowledge_unauthorized"
			} else if buffer.status == 403 || buffer.status == 404 {
				status, code = 404, "knowledge_not_found"
			}
			lifecycleHandlerError(c, status, code)
		}()
		auth(c)
	}
}

func lifecycleResponse(c *gin.Context, status int, data any, err error) {
	body := gin.H{"code": 0, "message": "success", "contract_version": "1.0.0", "source_contract_version": "0.7.0", "request_id": c.GetString("knowledge_request_id"), "data": data}
	if err != nil {
		status = 503
		code := "lifecycle_result_unknown"
		var public *dataset.KnowledgeError
		if errors.As(err, &public) {
			status, code = public.Status, public.Code
		}
		allowed := []string{"knowledge_invalid_request", "knowledge_unauthorized", "knowledge_not_found", "knowledge_contract_version", "knowledge_source_contract_version", "policy_changed", "revision_changed", "idempotency_conflict", "lifecycle_source_archived", "lifecycle_action_unsupported", "lifecycle_position_occupied", "lifecycle_version_exhausted", "lifecycle_integrity", "lifecycle_budget_exceeded", "lifecycle_result_unknown"}
		if !slices.Contains(allowed, code) {
			status, code = 503, "lifecycle_result_unknown"
		}
		body["code"], body["message"], body["data"] = code, "Lifecycle operation could not be completed", nil
		body["outcome_known"] = status != 401 && status != 404 && code != "lifecycle_integrity" && code != "lifecycle_result_unknown"
		c.Header("X-Knowledge-Error-Code", code)
		c.Status(status)
	}
	raw, marshalErr := json.Marshal(body)
	if marshalErr != nil || len(raw) > entity.KnowledgeLifecycleJSONBytes {
		status = 503
		raw, _ = json.Marshal(gin.H{"code": "lifecycle_result_unknown", "message": "Lifecycle response unavailable", "data": nil, "contract_version": "1.0.0", "source_contract_version": "0.7.0", "request_id": c.GetString("knowledge_request_id"), "outcome_known": false})
		c.Header("X-Knowledge-Error-Code", "lifecycle_result_unknown")
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

func lifecycleHandlerError(c *gin.Context, status int, code string) {
	lifecycleResponse(c, status, nil, &dataset.KnowledgeError{Status: status, Code: code})
}

func lifecycleUnicodeValid(raw []byte) bool {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	for i := 0; i < len(raw); {
		if raw[i] != '\\' {
			i++
			continue
		}
		if i+1 >= len(raw) {
			return false
		}
		if raw[i+1] != 'u' {
			i += 2
			continue
		}
		if i+6 > len(raw) {
			return false
		}
		x, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return false
		}
		if x >= 0xd800 && x <= 0xdbff {
			if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
				return false
			}
			y, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
			if err != nil || y < 0xdc00 || y > 0xdfff {
				return false
			}
			i += 12
		} else {
			if x >= 0xdc00 && x <= 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func validateLifecycleObject(raw []byte, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return validateLifecycleObject(raw, typ.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("null field")
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	fields := map[string]reflect.Type{}
	var add func(reflect.Type)
	add = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				add(f.Type)
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
	}
	add(typ)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("object required")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		field, found := fields[name]
		if !ok || !found || seen[name] {
			return errors.New("unknown or repeated field")
		}
		seen[name] = true
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return err
		}
		if err = validateLifecycleObject(value, field); err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return err
	}
	if len(seen) != len(fields) {
		return errors.New("required field missing")
	}
	return nil
}

func decodeLifecycle[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		lifecycleHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, entity.KnowledgeLifecycleJSONBytes))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			lifecycleHandlerError(c, 413, "lifecycle_budget_exceeded")
		} else {
			lifecycleHandlerError(c, 400, "knowledge_invalid_request")
		}
		return nil, false
	}
	if !lifecycleUnicodeValid(raw) || validateLifecycleObject(raw, reflect.TypeFor[T]()) != nil {
		lifecycleHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request T
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		lifecycleHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return &request, true
}

func lifecycleQuery(c *gin.Context, allowed []string) (url.Values, bool) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		for name, items := range values {
			if !slices.Contains(allowed, name) || len(items) != 1 || items[0] == "" && name != "language" || !utf8.ValidString(items[0]) {
				err = errors.New("invalid query")
			}
		}
	}
	if err != nil {
		lifecycleHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return values, true
}

// KnowledgeLifecycle binds the ten frozen operations to the native owner-only domain.
func (h *DatasetsHandler) KnowledgeLifecycle(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer, ok := c.Get("knowledge_lifecycle_writer"); ok {
			c.Writer = writer.(gin.ResponseWriter)
		}
		c.Set("knowledge_lifecycle_released", true)
		if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" {
			lifecycleHandlerError(c, 409, "knowledge_contract_version")
			return
		}
		if c.GetHeader("X-Knowledge-Source-Contract-Version") != "0.7.0" {
			lifecycleHandlerError(c, 409, "knowledge_source_contract_version")
			return
		}
		user, exists := c.Get("user")
		actor, ok := user.(*entity.User)
		if !exists || !ok || actor == nil {
			lifecycleHandlerError(c, 401, "knowledge_unauthorized")
			return
		}
		for _, param := range c.Params {
			if !directoryID.MatchString(param.Value) {
				lifecycleHandlerError(c, 404, "knowledge_not_found")
				return
			}
		}
		allowed := []string{}
		switch operation {
		case "listKnowledgeSourceLifecycles":
			allowed = []string{"state", "group_scope", "group_id", "component_side", "language", "limit", "cursor"}
		case "checkKnowledgeSourceMoveTarget":
			allowed = []string{"relative_path", "expected_source_revision_id", "expected_lifecycle_version", "expected_policy_version"}
		case "getKnowledgeLifecycleCommandByKey":
			allowed = []string{"action", "idempotency_key"}
		}
		query, ok := lifecycleQuery(c, allowed)
		if !ok {
			return
		}
		ctx, domain, source := c.Request.Context(), c.Param("dataset_id"), c.Param("source_id")
		var data any
		var err error
		created := false
		switch operation {
		case "getKnowledgeSourceLifecycleContract":
			var capabilities map[string]any
			err = json.Unmarshal(entity.KnowledgeLifecycleCapabilitiesJSON, &capabilities)
			capabilities["runtime_ready"] = dao.KnowledgeLifecycleRuntimeReady()
			data = capabilities
		case "getKnowledgeSourceLifecycle":
			data, err = h.datasetsService.GetKnowledgeSourceLifecycle(ctx, actor.ID, domain, source)
		case "listKnowledgeSourceLifecycles":
			request := dataset.KnowledgeLifecycleListQuery{State: query.Get("state"), GroupScope: query.Get("group_scope"), GroupID: query.Get("group_id"), ComponentSide: query.Get("component_side"), Cursor: query.Get("cursor")}
			if query.Has("language") {
				value := query.Get("language")
				request.Language = &value
			}
			if query.Has("limit") {
				request.Limit, err = strconv.Atoi(query.Get("limit"))
				if err != nil || request.Limit < 1 {
					lifecycleHandlerError(c, 400, "knowledge_invalid_request")
					return
				}
			}
			data, err = h.datasetsService.ListKnowledgeSourceLifecycles(ctx, actor.ID, domain, request)
		case "checkKnowledgeSourceMoveTarget":
			version, e1 := strconv.ParseInt(query.Get("expected_lifecycle_version"), 10, 64)
			policy, e2 := strconv.ParseInt(query.Get("expected_policy_version"), 10, 64)
			if e1 != nil || e2 != nil || query.Get("relative_path") == "" {
				lifecycleHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
			data, err = h.datasetsService.CheckKnowledgeSourceMoveTarget(ctx, actor.ID, domain, source, query.Get("relative_path"), entity.KnowledgeLifecycleCommand{ExpectedSourceRevisionID: query.Get("expected_source_revision_id"), ExpectedLifecycleVersion: version, ExpectedPolicyVersion: policy})
		case "getKnowledgeLifecycleCommand", "getKnowledgeLifecycleCommandByKey":
			data, err = h.datasetsService.GetKnowledgeLifecycleCommand(ctx, actor.ID, domain, c.Param("command_id"), query.Get("action"), query.Get("idempotency_key"))
		case "commandKnowledgeSourceReclassify":
			request, ok := decodeLifecycle[entity.KnowledgeLifecycleReclassify](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.CommandKnowledgeSourceLifecycle(ctx, actor.ID, domain, source, "reclassify", *request)
		case "commandKnowledgeSourceMove":
			request, ok := decodeLifecycle[entity.KnowledgeLifecycleMove](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.CommandKnowledgeSourceLifecycle(ctx, actor.ID, domain, source, "move", *request)
		case "commandKnowledgeSourceArchive", "commandKnowledgeSourceRestore":
			request, ok := decodeLifecycle[entity.KnowledgeLifecycleCommand](c)
			if !ok {
				return
			}
			action := "archive"
			if operation == "commandKnowledgeSourceRestore" {
				action = "restore"
			}
			data, created, err = h.datasetsService.CommandKnowledgeSourceLifecycle(ctx, actor.ID, domain, source, action, *request)
		default:
			lifecycleHandlerError(c, 404, "knowledge_not_found")
			return
		}
		status := 200
		if created {
			status = 201
		}
		lifecycleResponse(c, status, data, err)
	}
}
