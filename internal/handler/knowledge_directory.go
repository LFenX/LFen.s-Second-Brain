package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var directoryID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type directoryBufferedWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	status   int
	written  bool
	overflow bool
}

func (w *directoryBufferedWriter) WriteHeader(status int) { w.status = status; w.written = true }
func (w *directoryBufferedWriter) WriteHeaderNow()        { w.written = true }
func (w *directoryBufferedWriter) Write(raw []byte) (int, error) {
	w.written = true
	if w.body.Len()+len(raw) > 1048576 {
		w.overflow = true
		return len(raw), nil
	}
	return w.body.Write(raw)
}
func (w *directoryBufferedWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}
func (w *directoryBufferedWriter) Status() int   { return w.status }
func (w *directoryBufferedWriter) Size() int     { return w.body.Len() }
func (w *directoryBufferedWriter) Written() bool { return w.written }

// KnowledgeDirectoryAuth reuses native authentication and keeps its non-401 errors inside C2.
func KnowledgeDirectoryAuth(auth gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Set("knowledge_request_id", common.GenerateUUID())
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Source-Contract-Version", "0.3.0")
		original := c.Writer
		buffer := &directoryBufferedWriter{ResponseWriter: original, status: 200}
		c.Writer = buffer
		defer func() {
			failure := recover()
			c.Writer = original
			if failure != nil {
				common.Warn("Knowledge directory operation panicked")
				reason := "result_unknown"
				directoryResponse(c, 500, nil, &dataset.KnowledgeDirectoryError{Status: 500, Code: "knowledge_directory_unavailable", Reason: &reason})
				c.Abort()
				return
			}
			if buffer.overflow {
				directoryHandlerError(c, 413, "too_large")
				return
			}
			var envelope struct {
				Source string `json:"source_contract_version"`
			}
			_ = json.Unmarshal(buffer.body.Bytes(), &envelope)
			if buffer.status >= 400 && buffer.status != 401 && envelope.Source != "0.3.0" {
				if buffer.status == 403 || buffer.status == 404 {
					directoryHandlerError(c, 404, "not_found")
				} else {
					reason := "result_unknown"
					directoryResponse(c, 503, nil, &dataset.KnowledgeDirectoryError{Status: 503, Code: "knowledge_directory_unavailable", Reason: &reason})
				}
				return
			}
			original.WriteHeader(buffer.status)
			_, _ = original.Write(buffer.body.Bytes())
		}()
		auth(c)
	}
}

func directoryResponse(c *gin.Context, status int, data any, err error) {
	var code any = 0
	var reason *string
	message := "success"
	if err != nil {
		var public *dataset.KnowledgeDirectoryError
		if errors.As(err, &public) {
			status = public.Status
			code = public.Code
			reason = public.Reason
		} else {
			status = 503
			code = "knowledge_directory_unavailable"
			v := "result_unknown"
			reason = &v
		}
		message = "Directory operation could not be completed"
		data = nil
	}
	body := gin.H{"code": code, "message": message, "data": data, "contract_version": "1.0.0", "source_contract_version": "0.3.0", "request_id": c.GetString("knowledge_request_id")}
	if err != nil {
		body["reason"] = reason
	}
	raw, marshalErr := json.Marshal(body)
	limit := 262144
	if c.GetBool("directory_content") {
		limit = 1048576
	}
	if marshalErr != nil || len(raw) > limit {
		body["data"] = nil
		body["code"] = "knowledge_directory_too_large"
		body["reason"] = nil
		body["message"] = "Response exceeds directory limit"
		status = 413
		raw, _ = json.Marshal(body)
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}
func directoryHandlerError(c *gin.Context, status int, code string) {
	directoryResponse(c, status, nil, &dataset.KnowledgeDirectoryError{Status: status, Code: "knowledge_directory_" + code})
}

// validateDirectoryObject checks exact, required keys recursively, including duplicate keys.
func validateDirectoryObject(raw []byte, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return validateDirectoryObject(raw, typ.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("null is not allowed")
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	fields := map[string]reflect.Type{}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = typ.Field(i).Type
		}
	}
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
		field, exists := fields[name]
		if !ok || !exists || seen[name] {
			return errors.New("unknown or repeated field")
		}
		seen[name] = true
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return err
		}
		if err = validateDirectoryObject(value, field); err != nil {
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
func decodeDirectory[T any](c *gin.Context) (*T, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		directoryHandlerError(c, 415, "unsupported")
		return nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			directoryHandlerError(c, 413, "too_large")
		} else {
			directoryHandlerError(c, 400, "invalid")
		}
		return nil, false
	}
	if !utf8.Valid(raw) || validateDirectoryObject(raw, reflect.TypeFor[T]()) != nil {
		directoryHandlerError(c, 400, "invalid")
		return nil, false
	}
	var req T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF {
		directoryHandlerError(c, 400, "invalid")
		return nil, false
	}
	return &req, true
}
func directoryQuery(c *gin.Context, allowed ...string) (url.Values, bool) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		directoryHandlerError(c, 400, "invalid")
		return nil, false
	}
	for name, items := range values {
		found := false
		for _, a := range allowed {
			if a == name {
				found = true
			}
		}
		if !found || len(items) != 1 || items[0] == "" {
			directoryHandlerError(c, 400, "invalid")
			return nil, false
		}
	}
	return values, true
}

// KnowledgeDirectory binds one frozen operation to the existing authenticated dataset service.
func (h *DatasetsHandler) KnowledgeDirectory(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Set("knowledge_request_id", common.GenerateUUID())
		c.Header("X-Request-ID", c.GetString("knowledge_request_id"))
		c.Header("X-Knowledge-Contract-Version", "1.0.0")
		c.Header("X-Knowledge-Source-Contract-Version", "0.3.0")
		user, code, _ := GetUser(c)
		if code != common.CodeSuccess || user == nil {
			c.JSON(401, gin.H{"code": 401, "message": "Authentication required", "data": nil})
			return
		}
		if c.GetHeader("X-Knowledge-Contract-Version") != "1.0.0" || c.GetHeader("X-Knowledge-Source-Contract-Version") != "0.3.0" {
			directoryHandlerError(c, 409, "version_mismatch")
			return
		}
		for _, p := range c.Params {
			if p.Key != "idempotency_key" && !directoryID.MatchString(p.Value) {
				directoryHandlerError(c, 404, "not_found")
				return
			}
		}
		allowed := []string{}
		if operation == "getDirectoryListingPage" {
			allowed = []string{"cursor"}
		}
		if operation == "readDirectoryPreviewContent" {
			allowed = []string{"offset", "length"}
		}
		query, ok := directoryQuery(c, allowed...)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		owner := user.ID
		domain := c.Param("dataset_id")
		reg := c.Param("registration_id")
		var result any
		var err error
		status := 200
		switch operation {
		case "getDirectoryImportCandidate":
			err = json.Unmarshal(entity.KnowledgeDirectoryCapabilitiesJSON, &result)
		case "listDirectoryRoots":
			result, err = h.datasetsService.ListKnowledgeDirectoryRoots(ctx, owner, domain)
		case "createDirectoryRegistration":
			req, ok := decodeDirectory[entity.KnowledgeDirectoryRegistrationRequest](c)
			if !ok {
				return
			}
			var created bool
			result, created, err = h.datasetsService.CreateKnowledgeDirectoryRegistration(ctx, owner, domain, *req)
			if created {
				status = 201
			}
		case "getDirectoryRegistration":
			result, err = h.datasetsService.GetKnowledgeDirectoryRegistration(ctx, owner, domain, reg)
		case "createDirectoryListing":
			req, ok := decodeDirectory[entity.KnowledgeDirectoryListingRequest](c)
			if !ok {
				return
			}
			result, err = h.datasetsService.CreateKnowledgeDirectoryListing(ctx, owner, domain, reg, *req)
			status = 201
		case "getDirectoryListingPage":
			result, err = h.datasetsService.GetKnowledgeDirectoryListing(ctx, owner, domain, reg, c.Param("listing_id"), query.Get("cursor"))
		case "createDirectoryObservation":
			req, ok := decodeDirectory[entity.KnowledgeDirectoryObservationRequest](c)
			if !ok {
				return
			}
			result, err = h.datasetsService.CreateKnowledgeDirectoryObservation(ctx, owner, domain, reg, *req)
			status = 201
		case "getDirectoryObservation":
			result, err = h.datasetsService.GetKnowledgeDirectoryObservation(ctx, owner, domain, reg, c.Param("observation_id"))
		case "createDirectoryPreview":
			req, ok := decodeDirectory[entity.KnowledgeDirectoryPreviewRequest](c)
			if !ok {
				return
			}
			result, err = h.datasetsService.CreateKnowledgeDirectoryPreview(ctx, owner, domain, reg, *req)
			status = 201
		case "getDirectoryPreview":
			result, err = h.datasetsService.GetKnowledgeDirectoryPreview(ctx, owner, domain, reg, c.Param("preview_id"))
		case "readDirectoryPreviewContent":
			offset, e1 := strconv.ParseInt(query.Get("offset"), 10, 64)
			length, e2 := strconv.ParseInt(query.Get("length"), 10, 64)
			if e1 != nil || e2 != nil {
				directoryHandlerError(c, 416, "range")
				return
			}
			c.Set("directory_content", true)
			result, err = h.datasetsService.ReadKnowledgeDirectoryPreview(ctx, owner, domain, reg, c.Param("preview_id"), offset, length)
		case "commitDirectoryImport":
			req, ok := decodeDirectory[entity.KnowledgeDirectoryCommitRequest](c)
			if !ok {
				return
			}
			var created bool
			result, created, err = h.datasetsService.CommitKnowledgeDirectoryImport(ctx, owner, domain, reg, *req)
			if created {
				status = 201
			}
		case "getDirectoryImport":
			result, err = h.datasetsService.GetKnowledgeDirectoryImport(ctx, owner, domain, reg, c.Param("import_task_id"), "")
		case "getDirectoryImportByKey":
			result, err = h.datasetsService.GetKnowledgeDirectoryImport(ctx, owner, domain, reg, "", c.Param("idempotency_key"))
		default:
			directoryHandlerError(c, 404, "not_found")
			return
		}
		directoryResponse(c, status, result, err)
	}
}
