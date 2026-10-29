package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/entity"
)

func contentTestContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("content_operation", "representation")
	c.Set("content_request_id", strings.Repeat("f", 32))
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

func TestKnowledgeContentStrictFixedTargetDecode(t *testing.T) {
	valid := `{"contract_version":"0.2.0-candidate","target":{"source_ref":{"domain_id":"` + strings.Repeat("a", 32) + `","source_id":"` + strings.Repeat("b", 32) + `","source_revision_id":"` + strings.Repeat("c", 32) + `","source_sha256":"` + strings.Repeat("d", 64) + `"},"representation_ref":{"kind":"source_revision","id":"` + strings.Repeat("c", 32) + `","producer_sha256":null}}}`
	for _, tc := range []struct {
		name, body string
		accepted   bool
	}{
		{"valid", valid, true},
		{"duplicate", strings.Replace(valid, `"kind":"source_revision"`, `"kind":"source_revision","kind":"parser_result"`, 1), false},
		{"unknown", strings.Replace(valid, `"producer_sha256":null`, `"producer_sha256":null,"owner_id":"injected"`, 1), false},
		{"missing nullable key", strings.Replace(valid, `,"producer_sha256":null`, ``, 1), false},
		{"wrong case", strings.Replace(valid, `"source_id"`, `"Source_ID"`, 1), false},
		{"unpaired surrogate", strings.Replace(valid, `source_revision"`, `\ud800"`, 1), false},
		{"trailing value", valid + `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder := contentTestContext(tc.body)
			_, accepted := decodeContent[contentRepresentationRequest](c)
			if accepted != tc.accepted {
				t.Fatalf("accepted=%v status=%d", accepted, recorder.Code)
			}
			if !accepted && !strings.Contains(recorder.Body.String(), `"contract_version":"`+entity.KnowledgeContentVersion+`"`) {
				t.Fatal("refusal used another contract")
			}
		})
	}
}

func TestKnowledgeContentRejectsAPITokenBeforeOwningService(t *testing.T) {
	c, recorder := contentTestContext("")
	c.Set("auth_via_api_token", true)
	(&DatasetsHandler{}).KnowledgeContent("read")(c)
	if recorder.Code != 401 || !strings.Contains(recorder.Body.String(), `"code":"unauthenticated"`) {
		t.Fatalf("API token refused incorrectly: %d", recorder.Code)
	}
}

func TestKnowledgeContentAuthRefusalDoesNotLeakNativePayload(t *testing.T) {
	for _, status := range []int{401, 503} {
		c, recorder := contentTestContext("")
		KnowledgeContentAuth(func(c *gin.Context) { c.AbortWithStatusJSON(status, gin.H{"internal_only_marker": true}) }, "read")(c)
		if recorder.Code != status || strings.Contains(recorder.Body.String(), "internal_only_marker") {
			t.Fatalf("native refusal escaped: %d", recorder.Code)
		}
		var response map[string]any
		if json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response["contract_version"] != entity.KnowledgeContentVersion || response["data"] != nil {
			t.Fatal("invalid refusal envelope")
		}
	}
}

func TestKnowledgeContentErrorsAndVersionAreBounded(t *testing.T) {
	c, recorder := contentTestContext("")
	contentHandlerResponse(c, 200, "must not escape", errors.New("private storage details"))
	if recorder.Code != 503 || strings.Contains(recorder.Body.String(), "private storage") || strings.Contains(recorder.Body.String(), "must not escape") {
		t.Fatal("untrusted failure data escaped")
	}
	c, recorder = contentTestContext("")
	if contentVersion(c, "0.1.0-candidate") || recorder.Code != 412 {
		t.Fatal("another version was accepted")
	}
}
