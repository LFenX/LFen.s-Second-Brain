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

func TestKnowledgeContentLexicalFrozenNestedEvidenceDecode(t *testing.T) {
	domain := strings.Repeat("a", 32)
	id, hash := strings.Repeat("b", 32), strings.Repeat("c", 64)
	producer := hash
	target := entity.KnowledgeContentTarget{Source: entity.KnowledgeContentSourceRef{DomainID: domain, SourceID: id, RevisionID: id, SHA256: hash}, Representation: entity.KnowledgeContentRepresentationRef{Kind: "document_representation", ID: &hash, ProducerSHA256: &producer}}
	parent := entity.KnowledgeDocumentLocator{Scheme: "pdf_page_v1", PageNumber: 1}
	value := entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: entity.KnowledgeContentLexicalEvidence{Version: entity.KnowledgeContentVersion, Index: entity.KnowledgeLexicalIndexRef{ID: hash, ManifestSHA256: hash}, RowID: hash, Address: entity.KnowledgeContentAddress{Target: target, Node: entity.KnowledgeContentNodeRef{Kind: "r2_node", ID: &hash}}, CoordinateSpace: "derived_utf8", ReadingSHA256: hash, Range: entity.KnowledgeContentRange{EndByte: 1, SHA256: &hash}, Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &parent}}, OriginalPosition: &entity.KnowledgeDocumentOriginalPosition{Kind: "pdf_page", PageNumber: 1}, CoordinatePrecision: "derived_range", Assembly: "existing_passage_full_range"}}
	raw, err := json.Marshal(value)
	if err != nil || value.Evidence.Validate(domain) != nil {
		t.Fatal("invalid frozen evidence fixture", err)
	}
	valid := string(raw)
	for _, tc := range []struct {
		name, body string
		accepted   bool
	}{
		{"valid nested codec", valid, true},
		{"exact exponent", strings.ReplaceAll(valid, `"page_number":1`, `"page_number":1e0`), true},
		{"duplicate nested", strings.Replace(valid, `"chunk_index":0`, `"chunk_index":0,"chunk_index":1`, 1), false},
		{"unknown nested", strings.Replace(valid, `"chunk_index":0`, `"chunk_index":0,"owner_id":"injected"`, 1), false},
		{"fraction", strings.Replace(valid, `"chunk_index":0`, `"chunk_index":0.5`, 1), false},
		{"missing nullable", strings.Replace(valid, `,"item_index":null`, ``, 1), false},
		{"position extra", strings.Replace(valid, `"kind":"pdf_page"`, `"kind":"pdf_page","slide_number":1`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := contentLexicalTestContext(tc.body)
			request, accepted := decodeContentLexical[entity.KnowledgeContentLexicalReadRequest](c)
			if accepted != tc.accepted {
				t.Fatalf("accepted=%v status=%d", accepted, rec.Code)
			}
			if accepted && (request.Evidence.Locator.Document.ParentLocator.PageNumber != 1 || request.Evidence.OriginalPosition.PageNumber != 1) {
				t.Fatal("frozen integer value changed")
			}
		})
	}
}

func contentLexicalTestContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("content_lexical_operation", "getIndexJob")
	c.Set("content_lexical_request_id", strings.Repeat("f", 32))
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

func TestKnowledgeContentLexicalStrictClosedDecode(t *testing.T) {
	valid := `{"contract_version":"0.2.0-candidate","job_id":"` + strings.Repeat("a", 32) + `"}`
	for _, tc := range []struct {
		name, body string
		accepted   bool
	}{
		{"valid", valid, true},
		{"duplicate", strings.Replace(valid, `"job_id":`, `"job_id":"x","job_id":`, 1), false},
		{"unknown", strings.Replace(valid, `"job_id":`, `"owner_id":"x","job_id":`, 1), false},
		{"missing", `{"contract_version":"0.2.0-candidate"}`, false},
		{"null", strings.Replace(valid, `"`+strings.Repeat("a", 32)+`"`, `null`, 1), false},
		{"case", strings.Replace(valid, `job_id`, `Job_ID`, 1), false},
		{"surrogate", strings.Replace(valid, strings.Repeat("a", 32), `\ud800`, 1), false},
		{"trailing", valid + `{}`, false},
		{"oversize", strings.Repeat(" ", 32769) + valid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, r := contentLexicalTestContext(tc.body)
			_, accepted := decodeContentLexical[entity.KnowledgeContentLexicalJobRequest](c)
			if accepted != tc.accepted {
				t.Fatalf("accepted=%v status=%d", accepted, r.Code)
			}
		})
	}
}

func TestKnowledgeContentLexicalBoundedAuthAndFailure(t *testing.T) {
	for _, status := range []int{401, 503} {
		c, r := contentLexicalTestContext("")
		KnowledgeContentLexicalAuth(func(c *gin.Context) { c.AbortWithStatusJSON(status, gin.H{"private_marker": "must not escape"}) }, "read")(c)
		if r.Code != status || strings.Contains(r.Body.String(), "private_marker") || !strings.Contains(r.Body.String(), entity.KnowledgeContentVersion) {
			t.Fatal("native refusal envelope escaped", r.Code)
		}
	}
	c, r := contentLexicalTestContext("")
	c.Set("auth_via_api_token", true)
	(&DatasetsHandler{}).KnowledgeContentLexical("read")(c)
	if r.Code != 401 {
		t.Fatal("API token reached owning service")
	}
	c, r = contentLexicalTestContext("")
	contentLexicalHandlerResponse(c, 200, "must not escape", errors.New("private details"))
	if r.Code != 503 || strings.Contains(r.Body.String(), "private details") || strings.Contains(r.Body.String(), "must not escape") {
		t.Fatal("unknown failure leaked")
	}
	c, r = contentLexicalTestContext("")
	contentLexicalHandlerResponse(c, 200, strings.Repeat("x", 1<<20), nil)
	if r.Code != 503 || r.Body.Len() > 1024 {
		t.Fatal("response cap did not produce bounded fallback")
	}
}
