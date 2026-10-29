package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"

	"github.com/gin-gonic/gin"
)

// KnowledgeWebAuth reuses native login, then releases the bounded auth buffer
// before any verified binary content. Source HTTP errors never enter this path.
func KnowledgeWebAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Source-Contract-Version", "0.6.0")
		id := common.GenerateUUID()
		c.Set("knowledge_request_id", id)
		c.Header("X-Request-ID", id)
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: 200}
		c.Set("knowledge_web_writer", original)
		c.Writer = buffer
		defer func() {
			if c.GetBool("knowledge_web_released") {
				return
			}
			c.Writer = original
			status, code := 503, "web_result_unknown"
			if buffer.status == 401 {
				status, code = 401, "knowledge_unauthorized"
			} else if buffer.status == 403 || buffer.status == 404 {
				status, code = 404, "knowledge_not_found"
			}
			webHandlerError(c, status, code)
		}()
		auth(c)
	}
}
func webBegin(c *gin.Context) bool {
	if writer, exists := c.Get("knowledge_web_writer"); exists {
		c.Writer = writer.(gin.ResponseWriter)
	}
	c.Set("knowledge_web_released", true)
	if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" {
		webHandlerError(c, 409, "knowledge_contract_version")
		return false
	}
	if c.GetHeader("X-Knowledge-Source-Contract-Version") != "0.6.0" {
		webHandlerError(c, 409, "knowledge_source_contract_version")
		return false
	}
	return true
}
func webResponse(c *gin.Context, status int, data any, err error) {
	var code any = 0
	message := "success"
	body := gin.H{"code": code, "message": message, "data": data, "contract_version": "1.0.0", "source_contract_version": "0.6.0", "request_id": c.GetString("knowledge_request_id")}
	if err != nil {
		var public *dataset.KnowledgeError
		var web *dataset.KnowledgeWebError
		status, code = 503, "web_result_unknown"
		if errors.As(err, &public) {
			status, code = public.Status, public.Code
		}
		body["code"], body["message"], body["data"], body["failure"] = code, "Web operation could not be completed", nil, nil
		if errors.As(err, &web) {
			body["failure"] = web.Failure
		}
		c.Header("X-Knowledge-Error-Code", fmt.Sprint(code))
	}
	raw, marshalErr := json.Marshal(body)
	if marshalErr != nil || len(raw) > 32768 {
		status = 503
		raw, _ = json.Marshal(gin.H{"code": "web_result_unknown", "message": "Web response unavailable", "data": nil, "failure": nil, "contract_version": "1.0.0", "source_contract_version": "0.6.0", "request_id": c.GetString("knowledge_request_id")})
		c.Header("X-Knowledge-Error-Code", "web_result_unknown")
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}
func webHandlerError(c *gin.Context, status int, code string) {
	webResponse(c, status, nil, &dataset.KnowledgeError{Status: status, Code: code})
}

func webJSONBytes(c *gin.Context) ([]byte, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		webHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			webHandlerError(c, 413, "web_too_large")
		} else {
			webHandlerError(c, 400, "knowledge_invalid_request")
		}
		return nil, false
	}
	if !utf8.Valid(raw) {
		webHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return raw, true
}
func decodeWebBytes[T any](c *gin.Context, raw []byte) (*T, bool) {
	if validateDirectoryObject(raw, reflect.TypeFor[T]()) != nil {
		webHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request T
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		webHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return &request, true
}
func decodeWeb[T any](c *gin.Context) (*T, bool) {
	raw, ok := webJSONBytes(c)
	if !ok {
		return nil, false
	}
	return decodeWebBytes[T](c, raw)
}
func webQuery(c *gin.Context, allowed []string) (url.Values, bool) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		for name, items := range values {
			found := false
			for _, key := range allowed {
				found = found || name == key
			}
			if !found || len(items) != 1 || items[0] == "" {
				err = errors.New("invalid query")
			}
		}
	}
	if err != nil {
		webHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return values, true
}

// KnowledgeWeb dispatches exactly the 34 frozen C2 0.6.0 operations.
func (h *DatasetsHandler) KnowledgeWeb(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !webBegin(c) {
			return
		}
		user, ok := c.Get("user")
		actor, typed := user.(*entity.User)
		if !ok || !typed || actor == nil {
			webHandlerError(c, 401, "knowledge_unauthorized")
			return
		}
		ctx, domain, registration := c.Request.Context(), c.Param("dataset_id"), c.Param("registration_id")
		if operation != "getKnowledgeWebImportContract" {
			if _, err := h.datasetsService.GetKnowledgeDomain(ctx, actor.ID, domain); err != nil {
				webResponse(c, 0, nil, err)
				return
			}
		}
		allowed := []string{}
		switch operation {
		case "getKnowledgeWebRegistrationByKey":
			allowed = []string{"registration_key"}
		case "getKnowledgeWebTarget":
			allowed = []string{"entry_key"}
		case "getKnowledgeWebCaptureByKey", "getKnowledgeWebPreviewByKey", "getKnowledgeWebReceiptByKey":
			allowed = []string{"idempotency_key"}
		case "listKnowledgeWebEntries", "listKnowledgeWebEntryReferences", "listKnowledgeWebPreviewItems", "listKnowledgeWebReceiptItems", "listKnowledgeWebRevisionReferences":
			allowed = []string{"cursor"}
		case "listKnowledgeWebRevisions":
			allowed = []string{"cursor", "group_id", "registration_id"}
		case "readKnowledgeWebEntryWindow", "readKnowledgeWebRevisionWindow":
			allowed = []string{"start_byte", "end_byte"}
		}
		query, valid := webQuery(c, allowed)
		if !valid {
			return
		}
		for _, required := range []struct{ operation, key string }{{"getKnowledgeWebRegistrationByKey", "registration_key"}, {"getKnowledgeWebTarget", "entry_key"}, {"getKnowledgeWebCaptureByKey", "idempotency_key"}, {"getKnowledgeWebPreviewByKey", "idempotency_key"}, {"getKnowledgeWebReceiptByKey", "idempotency_key"}} {
			if operation == required.operation && query.Get(required.key) == "" {
				webHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
		}
		if strings.HasPrefix(operation, "readKnowledgeWeb") && strings.HasSuffix(operation, "Content") || operation == "downloadKnowledgeWebCapture" || operation == "downloadKnowledgeWebRevisionCapture" {
			if c.GetHeader("Range") != "" {
				webHandlerError(c, 416, "web_range_unsupported")
				return
			}
			var result *dataset.KnowledgeWebContent
			var err error
			switch operation {
			case "readKnowledgeWebEntryContent":
				result, err = h.datasetsService.ReadKnowledgeWebEntryContent(ctx, actor.ID, domain, registration, c.Param("capture_id"), c.Param("entry_key"), c.Param("representation_kind"))
			case "readKnowledgeWebRevisionContent":
				result, err = h.datasetsService.ReadKnowledgeWebRevisionContent(ctx, actor.ID, domain, c.Param("source_revision_id"), c.Param("representation_kind"))
			case "downloadKnowledgeWebCapture":
				result, err = h.datasetsService.DownloadKnowledgeWebCapture(ctx, actor.ID, domain, registration, c.Param("capture_id"))
			case "downloadKnowledgeWebRevisionCapture":
				result, err = h.datasetsService.DownloadKnowledgeWebRevisionCapture(ctx, actor.ID, domain, c.Param("source_revision_id"))
			}
			if err != nil {
				webResponse(c, 0, nil, err)
				return
			}
			serveWebContent(c, result)
			return
		}
		var data any
		var err error
		created := false
		switch operation {
		case "getKnowledgeWebImportContract":
			var capabilities entity.KnowledgeWebCapabilities
			err = json.Unmarshal(entity.KnowledgeWebCapabilitiesJSON, &capabilities)
			data = capabilities
		case "createKnowledgeWebRegistration":
			request, ok := decodeWeb[entity.KnowledgeWebCreateRegistration](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.CreateKnowledgeWebRegistration(ctx, actor.ID, domain, *request)
		case "getKnowledgeWebRegistration", "getKnowledgeWebRegistrationByKey":
			data, err = h.datasetsService.GetKnowledgeWebRegistration(ctx, actor.ID, domain, registration, query.Get("registration_key"))
		case "getKnowledgeWebTarget":
			data, err = h.datasetsService.GetKnowledgeWebTarget(ctx, actor.ID, domain, registration, query.Get("entry_key"))
		case "createKnowledgeWebCapture":
			raw, ok := webJSONBytes(c)
			if !ok {
				return
			}
			var mode struct {
				AcquisitionMode string `json:"acquisition_mode"`
			}
			if json.Unmarshal(raw, &mode) != nil {
				webHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
			if mode.AcquisitionMode == "local_snapshot" {
				request, ok := decodeWebBytes[entity.KnowledgeWebCreateLocalCapture](c, raw)
				if !ok {
					return
				}
				data, created, err = h.datasetsService.CreateKnowledgeWebCapture(ctx, actor.ID, domain, registration, *request)
			} else if mode.AcquisitionMode == "explicit_fetch" {
				request, ok := decodeWebBytes[entity.KnowledgeWebCreateFetchCapture](c, raw)
				if !ok {
					return
				}
				data, created, err = h.datasetsService.CreateKnowledgeWebCapture(ctx, actor.ID, domain, registration, *request)
			} else {
				webHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
		case "getKnowledgeWebCapture", "getKnowledgeWebCaptureByKey":
			data, err = h.datasetsService.GetKnowledgeWebCapture(ctx, actor.ID, domain, registration, c.Param("capture_id"), query.Get("idempotency_key"))
		case "cancelKnowledgeWebCapture":
			data, err = h.datasetsService.CancelKnowledgeWebCapture(ctx, actor.ID, domain, registration, c.Param("capture_id"))
		case "uploadKnowledgeWebCapture":
			controller := http.NewResponseController(c.Writer)
			deadline := time.Now().Add(30 * time.Second)
			receiveCtx, cancel := context.WithDeadline(ctx, deadline)
			defer cancel()
			if err := controller.SetReadDeadline(deadline); err != nil {
				webHandlerError(c, 503, "web_storage_unavailable")
				return
			}
			defer controller.SetReadDeadline(time.Time{})
			media, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
			data, err = h.datasetsService.ReceiveKnowledgeWebCapture(receiveCtx, actor.ID, domain, registration, c.Param("capture_id"), &originalRequestBody{ReadCloser: c.Request.Body, controller: controller}, c.Request.ContentLength, c.GetHeader("Content-Range"), c.GetHeader("Content-Encoding"), media)
			if err != nil {
				c.Request.Close = true
				c.Header("Connection", "close")
			}
		case "acquireKnowledgeWebCapture":
			request, ok := decodeWeb[entity.KnowledgeWebAcquire](c)
			if !ok {
				return
			}
			data, err = h.datasetsService.AcquireKnowledgeWebCapture(ctx, actor.ID, domain, registration, c.Param("capture_id"), *request)
		case "observeKnowledgeWebCapture":
			request, ok := decodeWeb[entity.KnowledgeWebObserve](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.ObserveKnowledgeWebCapture(ctx, actor.ID, domain, registration, c.Param("capture_id"), *request)
		case "getKnowledgeWebObservation":
			data, err = h.datasetsService.GetKnowledgeWebObservation(ctx, actor.ID, domain, registration, c.Param("capture_id"))
		case "listKnowledgeWebEntries":
			data, err = h.datasetsService.ListKnowledgeWebEntries(ctx, actor.ID, domain, registration, c.Param("capture_id"), query.Get("cursor"))
		case "getKnowledgeWebEntry":
			data, err = h.datasetsService.GetKnowledgeWebEntry(ctx, actor.ID, domain, registration, c.Param("capture_id"), c.Param("entry_key"))
		case "listKnowledgeWebEntryReferences":
			data, err = h.datasetsService.ListKnowledgeWebEntryReferences(ctx, actor.ID, domain, registration, c.Param("capture_id"), c.Param("entry_key"), query.Get("cursor"))
		case "readKnowledgeWebEntryWindow", "readKnowledgeWebRevisionWindow":
			start, parseStart := strconv.ParseInt(query.Get("start_byte"), 10, 64)
			end, parseEnd := strconv.ParseInt(query.Get("end_byte"), 10, 64)
			if parseStart != nil || parseEnd != nil || start < 0 || end < start || end > 2<<20 || end-start > 4096 {
				webHandlerError(c, 416, "web_window_invalid")
				return
			}
			if operation == "readKnowledgeWebEntryWindow" {
				data, err = h.datasetsService.ReadKnowledgeWebEntryWindow(ctx, actor.ID, domain, registration, c.Param("capture_id"), c.Param("entry_key"), c.Param("representation_kind"), start, end)
			} else {
				data, err = h.datasetsService.ReadKnowledgeWebRevisionWindow(ctx, actor.ID, domain, c.Param("source_revision_id"), c.Param("representation_kind"), start, end)
			}
		case "createKnowledgeWebPreview":
			request, ok := decodeWeb[entity.KnowledgeWebCreatePreview](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.CreateKnowledgeWebPreview(ctx, actor.ID, domain, registration, *request)
		case "getKnowledgeWebPreview", "getKnowledgeWebPreviewByKey":
			data, err = h.datasetsService.GetKnowledgeWebPreview(ctx, actor.ID, domain, registration, c.Param("preview_id"), query.Get("idempotency_key"))
		case "cancelKnowledgeWebPreview":
			data, err = h.datasetsService.CancelKnowledgeWebPreview(ctx, actor.ID, domain, registration, c.Param("preview_id"))
		case "listKnowledgeWebPreviewItems":
			data, err = h.datasetsService.ListKnowledgeWebPreviewItems(ctx, actor.ID, domain, registration, c.Param("preview_id"), query.Get("cursor"))
		case "confirmKnowledgeWebImport":
			request, ok := decodeWeb[entity.KnowledgeWebConfirm](c)
			if !ok {
				return
			}
			data, created, err = h.datasetsService.ConfirmKnowledgeWebImport(ctx, actor.ID, domain, registration, *request)
		case "getKnowledgeWebReceipt", "getKnowledgeWebReceiptByKey":
			data, err = h.datasetsService.GetKnowledgeWebReceipt(ctx, actor.ID, domain, registration, c.Param("import_task_id"), query.Get("idempotency_key"))
		case "listKnowledgeWebReceiptItems":
			data, err = h.datasetsService.ListKnowledgeWebReceiptItems(ctx, actor.ID, domain, registration, c.Param("import_task_id"), query.Get("cursor"))
		case "getKnowledgeWebRevision":
			data, err = h.datasetsService.GetKnowledgeWebRevision(ctx, actor.ID, domain, c.Param("source_revision_id"))
		case "listKnowledgeWebRevisions":
			var group *string
			if value := query.Get("group_id"); value != "" {
				group = &value
			}
			data, err = h.datasetsService.ListKnowledgeWebRevisions(ctx, actor.ID, domain, query.Get("registration_id"), group, query.Get("cursor"))
		case "listKnowledgeWebRevisionReferences":
			data, err = h.datasetsService.ListKnowledgeWebRevisionReferences(ctx, actor.ID, domain, c.Param("source_revision_id"), query.Get("cursor"))
		default:
			webHandlerError(c, 404, "knowledge_not_found")
			return
		}
		status := 200
		if created {
			status = 201
		}
		webResponse(c, status, data, err)
	}
}

func serveWebContent(c *gin.Context, content *dataset.KnowledgeWebContent) {
	if content == nil || len(content.Raw) > 2<<20 {
		webHandlerError(c, 503, "web_result_unknown")
		return
	}
	c.Header("X-Content-SHA256", content.SHA256)
	c.Header("Content-Length", strconv.Itoa(len(content.Raw)))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; media-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'; sandbox; frame-ancestors 'self'")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": content.Filename}))
	c.Data(200, "application/octet-stream", content.Raw)
}
