//go:build cgo

package parser

import (
	pdfoxide "github.com/yfedoseev/pdf_oxide/go"
	"ragflow/internal/deepdoc/parser/pdf/pdfsync"
	"ragflow/internal/entity"
)

const documentStructurePDFReady = true

func (b *documentStructureBuilder) buildPDF(data []byte) error {
	var doc *pdfoxide.PdfDocument
	var err error
	pdfsync.With(func() { doc, err = pdfoxide.OpenFromBytes(data) })
	if err != nil {
		return documentFailure("native_process_failed")
	}
	defer pdfsync.With(func() { _ = doc.Close() })
	var count int
	pdfsync.With(func() { count, err = doc.PageCount() })
	if err != nil {
		return documentFailure("native_process_failed")
	}
	if count < 1 {
		return documentFailure("invalid_source_structure")
	}
	if count > 128 {
		return documentFailure("budget_exceeded")
	}
	b.result.PageCount = documentInt(int64(count))
	for index := 0; index < count; index++ {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		var text string
		pdfsync.With(func() { text, err = doc.ExtractText(index) })
		if err != nil {
			return documentFailure("native_process_failed")
		}
		locator := entity.KnowledgeDocumentLocator{Scheme: "pdf_page_v1", PageNumber: int64(index + 1)}
		position := entity.KnowledgeDocumentOriginalPosition{Kind: "pdf_page", PageNumber: int64(index + 1)}
		page, err := b.add(b.root, "page", locator, position, entity.KnowledgeDocumentPageAttributes{PageNumber: int64(index + 1), PageContentClassification: "unknown"})
		if err != nil {
			return err
		}
		if text == "" {
			page.payload.TextStatus = "unsupported_or_requires_ocr"
			page.payload.TextReason = documentString("no_extractable_text")
			b.diagnostic("no_extractable_text", &page.payload.Locator)
			continue
		}
		block, err := b.add(page, "text_block", locator, position, entity.KnowledgeDocumentEmptyAttributes{})
		if err != nil {
			return err
		}
		block.own = true
		block.ownText = text
	}
	return nil
}
