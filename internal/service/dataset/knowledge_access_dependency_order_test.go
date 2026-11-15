//go:build linux

package dataset

import (
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Review-only candidate. It records real production GORM reads, including SQL
// reading_text materialization that an object-storage Get counter cannot see.
// It has not been compiled or executed by this reviewer.
func TestReviewAccessDependencyMetadataPrecedesDerivedBody(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, true)
	var selected []entity.KnowledgeAccessTarget
	for _, target := range f.Scope.Targets {
		if target.Representation.Kind == "parsed_representation" {
			selected = append(selected, target)
		}
	}
	if len(selected) != 1 {
		t.Fatal("fixture must provide one selected parsed representation")
	}
	f.enable(t, selected, "read")
	events := []string{}
	const callback = "review:access:dependency-before-body"
	if err := dao.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "knowledge_document_representation" {
			return
		}
		bodySelected := len(tx.Statement.Selects) == 0
		if len(tx.Statement.Selects) > 0 {
			for _, column := range tx.Statement.Selects {
				column = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(column, "`", "")))
				if column == "*" || strings.HasSuffix(column, ".*") || column == "reading_text" || strings.HasSuffix(column, ".reading_text") {
					bodySelected = true
				}
			}
		}
		if slices.Contains(tx.Statement.Omits, "reading_text") {
			bodySelected = false
		}
		if !bodySelected {
			events = append(events, "metadata-proof-query")
			return
		}
		if bodySelected {
			events = append(events, "reading-body-query")
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dao.DB.Callback().Query().Remove(callback) })
	f.inspect(t, "read", nil)
	firstBody := slices.Index(events, "reading-body-query")
	firstProof := slices.Index(events, "metadata-proof-query")
	if firstBody >= 0 && (firstProof < 0 || firstBody < firstProof) {
		t.Fatalf("derived reading body reached SQL materialization before dependency metadata: %v", events)
	}
}
