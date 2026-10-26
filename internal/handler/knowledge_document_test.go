package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func TestKnowledgeDocumentStrictRequestBoundary(t *testing.T) {
	valid := `{"target":{"kind":"original_document","source_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source_revision_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","original_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","expected_lifecycle_version":1},"expected_policy_version":1,"expected_document_policy_version":1,"expected_active_representation_id":null,"profile_id":"pdf-plain-pages/1","expected_producer_sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","confirmed":true,"idempotency_key":"synthetic"}`
	cases := []struct {
		name, raw string
		valid     bool
	}{
		{"valid", valid, true},
		{"exact_exponent", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":1e0`, 1), true},
		{"unknown_target", strings.Replace(valid, `"kind":"original_document"`, `"kind":"original_document","project_id":"invalid"`, 1), false},
		{"duplicate_target", strings.Replace(valid, `"kind":"original_document"`, `"kind":"original_document","kind":"original_document"`, 1), false},
		{"missing_nullable", strings.Replace(valid, `,"expected_active_representation_id":null`, "", 1), false},
		{"case_folded", strings.Replace(valid, `"profile_id"`, `"PROFILE_ID"`, 1), false},
		{"surrogate", strings.Replace(valid, `"synthetic"`, `"\ud800"`, 1), false},
		{"unsafe_integer", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":9007199254740992`, 1), false},
		{"fraction", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":1.5`, 1), false},
		{"not_confirmed", strings.Replace(valid, `"confirmed":true`, `"confirmed":false`, 1), false},
		{"null_target", strings.Replace(valid, `{"kind":"original_document","source_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source_revision_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","original_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","expected_lifecycle_version":1}`, `null`, 1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("knowledge_request_id", common.GenerateUUID())
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tc.raw))
			c.Request.Header.Set("Content-Type", "application/json")
			value, ok := decodeDocument[entity.KnowledgeDocumentTaskRequest](c)
			if ok != tc.valid {
				t.Fatalf("valid=%t result=%v body=%s", tc.valid, value, w.Body.String())
			}
			if !ok {
				if w.Code != 400 || entity.KnowledgeDocumentValidateJSON("Error400", w.Body.Bytes()) != nil {
					t.Fatal(w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestKnowledgeDocumentNativeAuthAndErrorEnvelopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", KnowledgeDocumentAuth(func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"private": "synthetic-hidden"}) }), func(c *gin.Context) { t.Fatal("unauthorized handler ran") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 401 || strings.Contains(w.Body.String(), "synthetic-hidden") || entity.KnowledgeDocumentValidateJSON("Error401", w.Body.Bytes()) != nil || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Knowledge-Document-Structure-Contract-Version") != "0.1.0" {
		t.Fatal(w.Code, w.Body.String())
	}
	var envelope map[string]any
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope["request_id"] != w.Header().Get("X-Request-ID") {
		t.Fatal("request ID/header mismatch")
	}
	cases := []struct {
		status int
		code   string
	}{{400, "invalid_request"}, {401, "unauthorized"}, {404, "not_found"}, {409, "document_integrity"}, {412, "contract_mismatch"}, {429, "capacity_exceeded"}, {503, "unavailable"}}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("knowledge_request_id", common.GenerateUUID())
			failure := &dataset.DocumentOperationError{Status: tc.status, Code: tc.code}
			if tc.status == 429 {
				receipt := common.GenerateUUID()
				failure.OutcomeKnown = true
				failure.RejectionID = &receipt
			}
			documentResponse(c, 200, nil, failure)
			if w.Code != tc.status || entity.KnowledgeDocumentValidateJSON(fmt.Sprintf("Error%d", tc.status), w.Body.Bytes()) != nil {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	w = httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("knowledge_request_id", common.GenerateUUID())
	documentResponse(c, 200, nil, &dataset.DocumentOperationError{Status: 429, Code: "capacity_exceeded"})
	if w.Code != 503 || entity.KnowledgeDocumentValidateJSON("Error503", w.Body.Bytes()) != nil {
		t.Fatal("unpersisted capacity must remain unknown", w.Code, w.Body.String())
	}
}

func TestKnowledgeDocumentStrictQueryBoundary(t *testing.T) {
	for _, q := range []string{"?parent_id=x&parent_id=y", "?cursor=", "?unknown=x", "?cursor=%FF", "?cursor=%ZZ"} {
		t.Run(q, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("knowledge_request_id", common.GenerateUUID())
			c.Request = httptest.NewRequest("GET", "/"+q, nil)
			if _, ok := documentQuery(c, []string{"parent_id", "cursor"}); ok || w.Code != 400 {
				t.Fatal(q, w.Code)
			}
		})
	}
}
