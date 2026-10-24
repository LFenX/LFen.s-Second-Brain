package handler

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"ragflow/internal/entity"
	"strings"
	"testing"
)

func TestKnowledgeImportInternalFailureHidesDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/import", func(c *gin.Context) {
		c.Set("knowledge_source_contract", true)
		knowledgeFailure(c, errors.New("synthetic SQL and storage internal details"))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/import", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "synthetic SQL") {
		t.Fatalf("internal failure leaked or was hidden as success: status=%d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["code"] != "knowledge_internal" || body["data"] != nil || body["source_contract_version"] != "0.2.0" {
		t.Fatalf("unexpected internal failure envelope: %v", err)
	}
}

func TestKnowledgeImportHandlerVersionAndSchema(t *testing.T) {
	for _, tc := range []struct {
		version string
		user    bool
		status  int
	}{{"", true, 409}, {"0.1.0-draft", true, 409}, {entity.KnowledgeSourceContractVersion, true, 200}, {entity.KnowledgeSourceContractVersion, false, 401}} {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		if tc.user {
			r.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: "synthetic-owner"}); c.Next() })
		}
		h := &DatasetsHandler{}
		r.GET("/contract", h.KnowledgeImportContract)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/contract", nil)
		req.Header.Set("X-Knowledge-Source-Contract-Version", tc.version)
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("version=%s status=%d body=%s", tc.version, w.Code, w.Body)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["source_contract_version"] != entity.KnowledgeSourceContractVersion || body["contract_version"] != "1.0.0" {
			t.Fatal("contract drift")
		}
	}
}

func TestKnowledgeImportHandlerStrictBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user", &entity.User{ID: "synthetic-owner"}); c.Next() })
	r.POST("/preview", func(c *gin.Context) {
		if _, ok := knowledgeImportUser(c); !ok {
			return
		}
		if value, ok := knowledgeDecodeLimit[entity.PreviewKnowledgeImportRequest](c, entity.KnowledgeImportRequestBytes, "relative_path", "text", "group_id", "component_side", "language"); ok {
			knowledgeResponse(c, 200, len(value.Text), "", "success")
		}
	})
	valid := `{"relative_path":"web/card.tsx","text":"合成\r\n","group_id":null,"component_side":"frontend","language":"tsx"}`
	for _, tc := range []struct {
		body   string
		status int
	}{{valid, 200}, {strings.Replace(valid, `"group_id":null,`, "", 1), 400}, {strings.Replace(valid, `"language":"tsx"`, `"language":"tsx","external_models":true`, 1), 400}, {strings.Replace(valid, `"text":"合成\r\n"`, `"text":null`, 1), 400}, {valid + "{}", 400}, {strings.Replace(valid, `"language":"tsx"`, `"language":"tsx","language":"go"`, 1), 400}, {strings.Replace(valid, "合成", string([]byte{255}), 1), 400}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/preview", strings.NewReader(tc.body))
		req.Header.Set("X-Knowledge-Source-Contract-Version", entity.KnowledgeSourceContractVersion)
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("status=%d expected=%d body=%s", w.Code, tc.status, w.Body)
		}
	}
}

func TestKnowledgeImportBodyLimitPrecedesUTF8Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/small", func(c *gin.Context) {
		c.Set("knowledge_source_contract", true)
		knowledgeDecodeLimit[entity.PreviewKnowledgeImportRequest](c, 64, "text")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/small", strings.NewReader(`{"text":"`+strings.Repeat("中", 50)+`"}`)))
	if w.Code != 413 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}
