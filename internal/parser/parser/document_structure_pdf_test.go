//go:build cgo

package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestDocumentStructurePDFSourceFacts(t *testing.T) {
	forbidDocumentAnalysis(t)
	cases := []struct{ id, name, sha string }{{"R2P01", "blank-physical-page.pdf", "ca3522dab8464a7d1b186cce98f336a43a5ae5b10593ac6fa4f29d80a17c4094"}, {"R2P02", "image-only-physical-page.pdf", "59d9c33321ae7f3f14faa4e54e4f481e5e640a1238b28226cee08a85d0f9bf35"}}
	for _, test := range cases {
		t.Run(test.id, func(t *testing.T) {
			data := documentSourceFixture(t, "document-fixtures/pdf/"+test.name, test.sha)
			result := buildSourceResult(t, "pdf", data)
			pages := documentNodes(result, "page")
			if result.PageCount == nil || *result.PageCount != 3 || len(pages) != 3 {
				t.Fatal("physical PDF page identities missing")
			}
			var a entity.KnowledgeDocumentPageAttributes
			for index, node := range pages {
				if node.Locator.PageNumber != int64(index+1) || node.OriginalPosition.PageNumber != int64(index+1) {
					t.Fatal("physical PDF page numbering changed")
				}
				if err := json.Unmarshal(node.Attributes, &a); err != nil {
					t.Fatal(err)
				}
				if a.PageContentClassification != "unknown" {
					t.Fatal("PDF page classification guessed")
				}
			}
			middle := pages[1]
			if middle.TextStatus != "unsupported_or_requires_ocr" || middle.TextReason == nil || *middle.TextReason != "no_extractable_text" || middle.ReadingRange.StartByte != middle.ReadingRange.EndByte {
				t.Fatal("empty/image-only page2 was lost or classified")
			}
			if !strings.Contains(documentNodeText(result, pages[0], false), "page one") || !strings.Contains(documentNodeText(result, pages[2], false), "page three") {
				t.Fatal("source marker not retained on physical pages")
			}
		})
	}
}
