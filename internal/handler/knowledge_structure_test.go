package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/entity"
	"ragflow/internal/service/dataset"
)

func TestKnowledgeStructureStrictNestedRequestsAndExactIntegers(t *testing.T) {
	valid := `{"project_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","manifest_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","expected_active_view_id":null,"expected_policy_version":1,"expected_structure_policy_version":2,"members":[{"source_id":"cccccccccccccccccccccccccccccccc","source_revision_id":"dddddddddddddddddddddddddddddddd","lifecycle_version":1,"parser":"tsx"}],"aliases":[],"confirmed":true,"idempotency_key":"synthetic"}`
	cases := []struct {
		name, raw string
		valid     bool
	}{
		{"valid", valid, true},
		{"exact_exponent", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":1e0`, 1), true},
		{"exact_decimal", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":1.0`, 1), true},
		{"unknown_member", strings.Replace(valid, `"parser":"tsx"`, `"parser":"tsx","extra":true`, 1), false},
		{"duplicate_member", strings.Replace(valid, `"parser":"tsx"`, `"parser":"tsx","parser":"go"`, 1), false},
		{"missing_member_field", strings.Replace(valid, `,"parser":"tsx"`, "", 1), false},
		{"null_array", strings.Replace(valid, `"aliases":[]`, `"aliases":null`, 1), false},
		{"case_folded", strings.Replace(valid, `"project_id"`, `"PROJECT_ID"`, 1), false},
		{"unpaired_surrogate", strings.Replace(valid, `"synthetic"`, `"\ud800"`, 1), false},
		{"unsafe_integer", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":9007199254740992`, 1), false},
		{"rounded_fraction", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":9007199254740991.1`, 1), false},
		{"fraction", strings.Replace(valid, `"expected_policy_version":1`, `"expected_policy_version":1.5`, 1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(tc.raw))
			ctx.Request.Header.Set("Content-Type", "application/json")
			value, ok := decodeStructure[entity.KnowledgeStructureTaskRequest](ctx)
			if ok != tc.valid {
				t.Fatalf("ok=%v body=%s", ok, w.Body.String())
			}
			if ok && value.ExpectedPolicyVersion != 1 {
				t.Fatal("exact integer changed", value.ExpectedPolicyVersion)
			}
			if !ok {
				var envelope map[string]any
				if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope["code"] != "invalid_request" || envelope["rejection_id"] != nil {
					t.Fatal(envelope)
				}
			}
		})
	}
}

func TestKnowledgeStructureErrorPreservesHTTPStatus(t *testing.T) {
	for _, status := range []int{400, 404, 409, 412, 429, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			structureResponse(ctx, 200, nil, &dataset.KnowledgeError{Status: status, Code: "invalid_request", Message: "synthetic"})
			if w.Code != status {
				t.Fatalf("HTTP=%d expected=%d body=%s", w.Code, status, w.Body.String())
			}
		})
	}
}

func TestKnowledgeStructureNativeAuthDoesNotLeakOriginalBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", KnowledgeStructureAuth(func(c *gin.Context) { c.AbortWithStatusJSON(401, gin.H{"synthetic_secret": "must-not-escape"}) }), func(c *gin.Context) { t.Fatal("unauthorized handler ran") })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	var envelope map[string]any
	if w.Code != 401 || strings.Contains(w.Body.String(), "must-not-escape") || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope["structure_contract_version"] != "0.1.0" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
}
