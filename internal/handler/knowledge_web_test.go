package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeWebStrictJSONAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KnowledgeWebAuth(originalTestAuth))
	r.POST("/capture", func(c *gin.Context) {
		if !webBegin(c) {
			return
		}
		request, ok := decodeWeb[entity.KnowledgeWebCreateLocalCapture](c)
		if ok {
			webResponse(c, 200, request, nil)
		}
	})
	valid := `{"registration_config_sha256":"` + strings.Repeat("a", 64) + `","idempotency_key":"cap:001","acquisition_mode":"local_snapshot","original_filename":"sample.xml","declared_byte_length":128,"declared_media_type":"application/rss+xml","expected_sha256":null,"declared_final_url":null,"claimed_captured_at":null}`
	for _, tc := range []struct {
		name, body, version string
		status              int
	}{
		{"valid", valid, "0.6.0", 200}, {"old version", valid, "0.5.0", 409}, {"null", "null", "0.6.0", 400}, {"trailing", valid + "{}", "0.6.0", 400},
		{"missing", strings.Replace(valid, `"expected_sha256":null,`, "", 1), "0.6.0", 400}, {"duplicate", strings.Replace(valid, `"expected_sha256":null`, `"expected_sha256":null,"expected_sha256":null`, 1), "0.6.0", 400},
		{"unknown", strings.Replace(valid, `"claimed_captured_at":null`, `"claimed_captured_at":null,"proxy":"http://internal"`, 1), "0.6.0", 400},
		{"case alias", strings.Replace(valid, `"original_filename"`, `"Original_Filename"`, 1), "0.6.0", 400}, {"fraction", strings.Replace(valid, `"declared_byte_length":128`, `"declared_byte_length":1.5`, 1), "0.6.0", 400},
		{"invalid UTF8", strings.Replace(valid, "sample.xml", string([]byte{255}), 1), "0.6.0", 400}, {"oversize", `{"x":"` + strings.Repeat("a", 32768) + `"}`, "0.6.0", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/capture", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
			request.Header.Set("X-Knowledge-Source-Contract-Version", tc.version)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, request)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Knowledge-Source-Contract-Version") != "0.6.0" {
				t.Fatalf("status=%d expected=%d body=%s", w.Code, tc.status, w.Body)
			}
			if len(w.Body.Bytes()) > 32768 {
				t.Fatal("JSON response exceeded budget")
			}
		})
	}
}

func TestKnowledgeWebAuthBinaryAndSourceErrorSeparation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(KnowledgeWebAuth(originalTestAuth))
	raw := []byte(strings.Repeat("合成资料", 10000))
	r.GET("/binary", func(c *gin.Context) {
		if !webBegin(c) {
			return
		}
		serveWebContent(c, &dataset.KnowledgeWebContent{Raw: raw, SHA256: strings.Repeat("a", 64), Filename: "safe-reading.html"})
	})
	r.GET("/source401", func(c *gin.Context) {
		if !webBegin(c) {
			return
		}
		status := int64(401)
		webResponse(c, 0, nil, &dataset.KnowledgeWebError{Public: &dataset.KnowledgeError{Status: 502, Code: "web_upstream_http_error"}, Failure: entity.KnowledgeWebFailure{Stage: "fetch", MachineCode: "web_upstream_http_error", UpstreamHTTPStatus: &status, OutcomeKnown: true}})
	})
	for _, path := range []string{"/binary", "/source401"} {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
		request.Header.Set("X-Knowledge-Source-Contract-Version", "0.6.0")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		if path == "/binary" {
			if w.Code != 200 || w.Body.String() != string(raw) || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") || w.Header().Get("Accept-Ranges") != "" {
				t.Fatal("binary content was buffered/truncated/executable")
			}
		} else {
			var body map[string]any
			if w.Code != http.StatusBadGateway || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["code"] != "web_upstream_http_error" {
				t.Fatal("source authentication became local login error")
			}
		}
	}
}
