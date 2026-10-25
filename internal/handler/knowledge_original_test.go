package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"

	"github.com/gin-gonic/gin"
)

func originalTestRouter(auth gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KnowledgeOriginalAuth(auth))
	return r
}

func originalTestAuth(c *gin.Context) {
	c.Set("user", &entity.User{ID: strings.Repeat("a", 32)})
	c.Next()
}

func originalTestHeaders(request *http.Request) {
	request.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
	request.Header.Set("X-Knowledge-Source-Contract-Version", "0.5.0")
}

func TestKnowledgeOriginalHandlerStrictJSON(t *testing.T) {
	r := originalTestRouter(originalTestAuth)
	r.POST("/body", func(c *gin.Context) {
		if !originalBegin(c) {
			return
		}
		if value, ok := decodeOriginal[entity.KnowledgeOriginalUploadRequest](c); ok {
			originalResponse(c, 200, value, nil)
		}
	})
	valid := `{"original_filename":"sample.pdf","relative_path":"docs/sample.pdf","classification":{"group_id":null,"component_side":"non_code","language":""},"declared_byte_length":32,"declared_media_type":"application/pdf","expected_sha256":null,"expected_revision_id":null,"idempotency_key":"test:001"}`
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"valid", valid, 200}, {"null", "null", 400}, {"trailing", valid + "{}", 400},
		{"missing", strings.Replace(valid, `"expected_sha256":null,`, "", 1), 400},
		{"duplicate", strings.Replace(valid, `"expected_sha256":null`, `"expected_sha256":null,"expected_sha256":null`, 1), 400},
		{"nested_unknown", strings.Replace(valid, `"language":""`, `"language":"","external_models":true`, 1), 400},
		{"case_alias", strings.Replace(valid, `"original_filename"`, `"Original_Filename"`, 1), 400},
		{"fraction", strings.Replace(valid, `"declared_byte_length":32`, `"declared_byte_length":3.5`, 1), 400},
		{"invalid_utf8", strings.Replace(valid, "sample.pdf", string([]byte{255}), 1), 400},
		{"too_large", `{"x":"` + strings.Repeat("a", 32768) + `"}`, 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/body", strings.NewReader(test.body))
			originalTestHeaders(req)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, test.status, w.Body)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Knowledge-Source-Contract-Version") != "0.5.0" {
				t.Fatal("contract response headers lost")
			}
		})
	}
}

func TestKnowledgeOriginalAuthStreamsAfterNativeAuth(t *testing.T) {
	r := originalTestRouter(originalTestAuth)
	content := strings.Repeat("synthetic\x00data", 300000)
	r.GET("/stream", func(c *gin.Context) {
		if !originalBegin(c) {
			return
		}
		c.Header("Content-Length", strconv.Itoa(len(content)))
		c.Status(200)
		_, _ = io.CopyBuffer(c.Writer, strings.NewReader(content), make([]byte, 64<<10))
	})
	server := httptest.NewServer(r)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/stream", nil)
	originalTestHeaders(req)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	hash := sha256.New()
	size, err := io.CopyBuffer(hash, response.Body, make([]byte, 64<<10))
	expected := sha256.Sum256([]byte(content))
	if err != nil || response.StatusCode != 200 || size != int64(len(content)) || hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(expected[:]) {
		t.Fatalf("auth buffered/truncated stream: status=%d bytes=%d err=%v", response.StatusCode, size, err)
	}
}

func TestKnowledgeOriginalHeadAndAuthErrorsHaveNoBody(t *testing.T) {
	for _, status := range []int{401, 403, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			called := false
			r := originalTestRouter(func(c *gin.Context) { c.JSON(status, gin.H{"private": "must not leak on HEAD"}); c.Abort() })
			r.HEAD("/head", func(c *gin.Context) { called = true })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("HEAD", "/head", nil))
			want := status
			if status == 403 {
				want = 404
			}
			if called || w.Code != want || w.Body.Len() != 0 {
				t.Fatalf("HEAD auth leaked/called handler: status=%d size=%d", w.Code, w.Body.Len())
			}
		})
	}
	r := originalTestRouter(originalTestAuth)
	r.HEAD("/head", func(c *gin.Context) {
		if originalBegin(c) {
			originalResponse(c, 0, nil, &dataset.KnowledgeError{Status: 409, Code: "original_integrity"})
		}
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("HEAD", "/head", nil)
	originalTestHeaders(req)
	r.ServeHTTP(w, req)
	if w.Code != 409 || w.Body.Len() != 0 || w.Header().Get("X-Knowledge-Error-Code") != "original_integrity" {
		t.Fatal("HEAD structured error mismatch")
	}
}

func TestKnowledgeOriginalHandlerVersionAndSanitizedErrors(t *testing.T) {
	r := originalTestRouter(originalTestAuth)
	h := &DatasetsHandler{}
	r.GET("/contract", h.KnowledgeOriginal("getKnowledgeOriginalImportContract"))
	for _, source := range []string{"", "0.4.0", "0.5.0"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/contract", nil)
		originalTestHeaders(req)
		req.Header.Set("X-Knowledge-Source-Contract-Version", source)
		r.ServeHTTP(w, req)
		want := 409
		if source == "0.5.0" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("version=%s status=%d", source, w.Code)
		}
		var envelope map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope["source_contract_version"] != "0.5.0" {
			t.Fatal("error envelope drift", err)
		}
	}
}
