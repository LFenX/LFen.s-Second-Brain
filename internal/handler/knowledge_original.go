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
	"sync/atomic"
	"time"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
	"ragflow/internal/storage"
	"ragflow/internal/utility"

	"github.com/gin-gonic/gin"
)

type originalAuthWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	status   int
	overflow bool
}

func (w *originalAuthWriter) WriteHeader(status int) { w.status = status }
func (w *originalAuthWriter) WriteHeaderNow()        {}
func (w *originalAuthWriter) Write(raw []byte) (int, error) {
	if w.body.Len()+len(raw) > 32768 {
		w.overflow = true
		return len(raw), nil
	}
	return w.body.Write(raw)
}
func (w *originalAuthWriter) WriteString(value string) (int, error) { return w.Write([]byte(value)) }
func (w *originalAuthWriter) Status() int                           { return w.status }
func (w *originalAuthWriter) Size() int                             { return w.body.Len() }
func (w *originalAuthWriter) Written() bool                         { return w.body.Len() > 0 || w.status != 200 }

// KnowledgeOriginalAuth buffers native authentication only, never original bytes.
func KnowledgeOriginalAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Source-Contract-Version", "0.5.0")
		id := common.GenerateUUID()
		c.Set("knowledge_request_id", id)
		c.Header("X-Request-ID", id)
		original := c.Writer
		buffer := &originalAuthWriter{ResponseWriter: original, status: 200}
		c.Set("knowledge_original_writer", original)
		c.Writer = buffer
		defer func() {
			if c.GetBool("knowledge_original_released") {
				return
			}
			c.Writer = original
			if buffer.status == 401 && !buffer.overflow {
				c.Status(401)
				if c.Request.Method != http.MethodHead {
					_, _ = original.Write(buffer.body.Bytes())
				} else {
					original.WriteHeaderNow()
				}
			} else if buffer.status == 403 || buffer.status == 404 {
				originalResponse(c, 404, nil, &dataset.KnowledgeError{Status: 404, Code: "knowledge_not_found"})
			} else {
				originalResponse(c, 503, nil, &dataset.KnowledgeError{Status: 503, Code: "result_unknown"})
			}
		}()
		auth(c)
	}
}

func originalResponse(c *gin.Context, status int, data any, err error) {
	var code any = 0
	message := "success"
	if err != nil {
		var public *dataset.KnowledgeError
		if errors.As(err, &public) {
			status, code = public.Status, public.Code
		} else {
			status, code = 503, "result_unknown"
		}
		message, data = "Original operation could not be completed", nil
		c.Header("X-Knowledge-Error-Code", fmt.Sprint(code))
	}
	if c.Request.Method == http.MethodHead {
		c.Status(status)
		c.Writer.WriteHeaderNow()
		return
	}
	body := gin.H{"code": code, "message": message, "data": data, "contract_version": "1.0.0", "source_contract_version": "0.5.0", "request_id": c.GetString("knowledge_request_id")}
	raw, marshalErr := json.Marshal(body)
	if marshalErr != nil || len(raw) > 32768 {
		body["code"], body["message"], body["data"] = "knowledge_internal", "Original response unavailable", nil
		status = 500
		c.Header("X-Knowledge-Error-Code", "knowledge_internal")
		raw, _ = json.Marshal(body)
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

func originalHandlerError(c *gin.Context, status int, code string) {
	originalResponse(c, status, nil, &dataset.KnowledgeError{Status: status, Code: code})
}

func originalBegin(c *gin.Context) bool {
	if writer, exists := c.Get("knowledge_original_writer"); exists {
		c.Writer = writer.(gin.ResponseWriter)
	}
	c.Set("knowledge_original_released", true)
	if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" {
		originalHandlerError(c, 409, "knowledge_contract_version")
		return false
	}
	if c.GetHeader("X-Knowledge-Source-Contract-Version") != "0.5.0" {
		originalHandlerError(c, 409, "knowledge_source_contract_version")
		return false
	}
	return true
}

func decodeOriginal[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		originalHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			originalHandlerError(c, 413, "upload_too_large")
		} else {
			originalHandlerError(c, 400, "knowledge_invalid_request")
		}
		return nil, false
	}
	if !utf8.Valid(raw) || validateDirectoryObject(raw, reflect.TypeFor[T]()) != nil {
		originalHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request T
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		originalHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return &request, true
}

func originalQuery(c *gin.Context, allowed ...string) (url.Values, bool) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err == nil {
		for name, items := range values {
			found := false
			for _, key := range allowed {
				found = found || key == name
			}
			if !found || len(items) != 1 || items[0] == "" {
				err = errors.New("invalid query")
			}
		}
	}
	if err != nil {
		originalHandlerError(c, 400, "knowledge_invalid_request")
		return nil, false
	}
	return values, true
}

type originalRequestBody struct {
	io.ReadCloser
	controller *http.ResponseController
	reachedEOF atomic.Bool
}

func (b *originalRequestBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF {
		b.reachedEOF.Store(true)
	}
	return n, err
}

func (b *originalRequestBody) Close() error {
	// After EOF, net/http may already be reading the next request on this
	// connection. A forced deadline would cancel that connection context.
	if !b.reachedEOF.Load() {
		_ = b.controller.SetReadDeadline(time.Now())
	}
	return b.ReadCloser.Close()
}

// KnowledgeOriginal dispatches only the frozen C2 0.5.0 operations.
func (h *DatasetsHandler) KnowledgeOriginal(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !originalBegin(c) {
			return
		}
		user, ok := c.Get("user")
		actor, typed := user.(*entity.User)
		if !ok || !typed || actor == nil {
			originalHandlerError(c, 401, "knowledge_unauthorized")
			return
		}
		ctx, domain, registration := c.Request.Context(), c.Param("dataset_id"), c.Param("registration_id")
		if operation != "getKnowledgeOriginalImportContract" {
			if _, err := h.datasetsService.GetKnowledgeDomain(ctx, actor.ID, domain); err != nil {
				originalResponse(c, 0, nil, err)
				return
			}
		}
		var data any
		var err error
		status := 200
		allowed := []string{}
		switch operation {
		case "findKnowledgeOriginalRegistrationByKey":
			allowed = []string{"registration_key"}
		case "findKnowledgeOriginalUploadByKey", "findKnowledgeOriginalImportByKey":
			allowed = []string{"idempotency_key"}
		case "getKnowledgeOriginalTarget":
			allowed = []string{"relative_path"}
		case "listKnowledgeOriginalRevisions":
			allowed = []string{"group_id", "cursor"}
		}
		query, valid := originalQuery(c, allowed...)
		if !valid {
			return
		}
		created := false
		switch operation {
		case "getKnowledgeOriginalImportContract":
			var capability map[string]any
			if json.Unmarshal(entity.KnowledgeOriginalCapabilitiesJSON, &capability) != nil {
				originalHandlerError(c, 500, "knowledge_internal")
				return
			}
			_, streaming := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage)
			capability["runtime_ready"] = streaming && utility.KnowledgeDirectoryPlatformVerified()
			data = capability
		case "createKnowledgeOriginalRegistration":
			req, valid := decodeOriginal[entity.KnowledgeOriginalRegistrationRequest](c)
			if !valid {
				return
			}
			data, created, err = h.datasetsService.CreateKnowledgeOriginalRegistration(ctx, actor.ID, domain, *req)
		case "getKnowledgeOriginalRegistration", "findKnowledgeOriginalRegistrationByKey":
			if operation == "findKnowledgeOriginalRegistrationByKey" && query.Get("registration_key") == "" {
				originalHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
			data, err = h.datasetsService.GetKnowledgeOriginalRegistration(ctx, actor.ID, domain, registration, query.Get("registration_key"))
		case "getKnowledgeOriginalTarget":
			if query.Get("relative_path") == "" {
				originalHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
			data, err = h.datasetsService.GetKnowledgeOriginalTarget(ctx, actor.ID, domain, registration, query.Get("relative_path"))
		case "createKnowledgeOriginalUpload":
			req, valid := decodeOriginal[entity.KnowledgeOriginalUploadRequest](c)
			if !valid {
				return
			}
			data, created, err = h.datasetsService.CreateKnowledgeOriginalUpload(ctx, actor.ID, domain, registration, *req)
		case "getKnowledgeOriginalUpload", "findKnowledgeOriginalUploadByKey":
			if operation == "findKnowledgeOriginalUploadByKey" && query.Get("idempotency_key") == "" {
				originalHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
			data, err = h.datasetsService.GetKnowledgeOriginalUpload(ctx, actor.ID, domain, registration, c.Param("upload_id"), query.Get("idempotency_key"))
		case "putKnowledgeOriginalUploadBytes":
			controller := http.NewResponseController(c.Writer)
			deadline := time.Now().Add(120 * time.Second)
			receiveCtx, cancelReceive := context.WithDeadline(ctx, deadline)
			defer cancelReceive()
			if err := controller.SetReadDeadline(deadline); err != nil {
				originalHandlerError(c, 503, "original_storage_unavailable")
				return
			}
			defer controller.SetReadDeadline(time.Time{})
			media, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
			data, err = h.datasetsService.ReceiveKnowledgeOriginal(receiveCtx, actor.ID, domain, registration, c.Param("upload_id"), &originalRequestBody{ReadCloser: c.Request.Body, controller: controller}, c.Request.ContentLength, c.GetHeader("Content-Range"), c.GetHeader("Content-Encoding"), media)
			if err != nil {
				c.Request.Close = true
				c.Header("Connection", "close")
			}
		case "cancelKnowledgeOriginalUpload":
			data, err = h.datasetsService.CancelKnowledgeOriginalUpload(ctx, actor.ID, domain, registration, c.Param("upload_id"))
		case "confirmKnowledgeOriginalImport":
			req, valid := decodeOriginal[entity.KnowledgeOriginalConfirmRequest](c)
			if !valid {
				return
			}
			data, created, err = h.datasetsService.ConfirmKnowledgeOriginalImport(ctx, actor.ID, domain, registration, *req)
		case "getKnowledgeOriginalImport", "findKnowledgeOriginalImportByKey":
			if operation == "findKnowledgeOriginalImportByKey" && query.Get("idempotency_key") == "" {
				originalHandlerError(c, 400, "knowledge_invalid_request")
				return
			}
			data, err = h.datasetsService.GetKnowledgeOriginalImport(ctx, actor.ID, domain, registration, c.Param("import_task_id"), query.Get("idempotency_key"))
		case "listKnowledgeOriginalRevisions":
			var group *string
			if value := query.Get("group_id"); value != "" {
				group = &value
			}
			data, err = h.datasetsService.ListKnowledgeOriginalRevisions(ctx, actor.ID, domain, group, query.Get("cursor"))
		case "getKnowledgeOriginalRevision":
			data, err = h.datasetsService.GetKnowledgeOriginalRevision(ctx, actor.ID, domain, c.Param("source_revision_id"))
		case "readKnowledgeOriginalRevisionBytes", "headKnowledgeOriginalRevisionBytes":
			h.serveOriginalContent(c, actor.ID, domain)
			return
		default:
			originalHandlerError(c, 404, "knowledge_not_found")
			return
		}
		if created {
			status = 201
		}
		originalResponse(c, status, data, err)
	}
}

func (h *DatasetsHandler) serveOriginalContent(c *gin.Context, user, domain string) {
	head := c.Request.Method == http.MethodHead
	result, err := h.datasetsService.OpenKnowledgeOriginalContent(c.Request.Context(), user, domain, c.Param("source_revision_id"), c.GetHeader("Range"), c.GetHeader("If-Range"), head)
	if err != nil {
		if result != nil && result.Status == 416 {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", result.Source.ByteLength))
		}
		originalResponse(c, 0, nil, err)
		return
	}
	c.Header("Content-Type", result.Source.MediaType)
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": result.Source.OriginalFilename}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Accept-Ranges", "bytes")
	c.Header("ETag", `"sha256-`+result.Source.SHA256+`"`)
	c.Header("Content-Length", strconv.FormatInt(result.Length, 10))
	c.Header("X-Accel-Buffering", "no")
	if result.Status == 206 {
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", result.Offset, result.Offset+result.Length-1, result.Source.ByteLength))
	}
	c.Status(result.Status)
	if head {
		c.Writer.WriteHeaderNow()
		return
	}
	defer result.Body.Close()
	_, _ = io.CopyBuffer(c.Writer, result.Body, make([]byte, 64<<10))
}
