package document

import (
	"context"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type lexicalAvailabilityProbe struct {
	engine.DocEngine
	fields       []string
	ids          [][]string
	availability []any
}

func (e *lexicalAvailabilityProbe) Search(_ context.Context, r *types.SearchRequest) (*types.SearchResult, error) {
	e.fields = r.SelectFields
	return &types.SearchResult{Total: 5, Chunks: []map[string]any{
		{"id": "ordinary-short"},
		{"id": strings.Repeat("a", 64)},
		{"id": "tree-product", "compile_kwd": "tree"},
		{"id": "marked-short", "knowledge_binding_kwd": strings.Repeat("b", 64), "knowledge_ref": "synthetic fixed reference", "available_int": 0},
		{"id": "reference-only", "knowledge_ref": "synthetic integrity sentinel", "available_int": 0},
	}}, nil
}
func (e *lexicalAvailabilityProbe) UpdateChunks(_ context.Context, condition, value map[string]any, _, _ string) error {
	e.ids = append(e.ids, condition["id"].([]string))
	e.availability = append(e.availability, value["available_int"])
	return nil
}
func TestKnowledgeLocalLexicalDocumentAvailabilityExcludesBoundRows(t *testing.T) {
	e := &lexicalAvailabilityProbe{}
	s := testDocumentService(t)
	s.docEngine = e
	for _, available := range []int{0, 1} {
		if err := s.updateDocumentChunkAvailability(t.Context(), "synthetic-tenant", "synthetic-domain", "synthetic-document", available); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"ordinary-short", strings.Repeat("a", 64), "tree-product"}
	if len(e.ids) != 2 || !reflect.DeepEqual(e.ids[0], want) || !reflect.DeepEqual(e.ids[1], want) {
		t.Fatalf("helper toggled bound rows: %#v", e.ids)
	}
	if !slices.Contains(e.fields, "knowledge_binding_kwd") || !slices.Contains(e.fields, "knowledge_ref") || e.availability[0] != 0 || e.availability[1] != 1 {
		t.Fatalf("missing marker projection or toggle: %#v %#v", e.fields, e.availability)
	}
}
