// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"ragflow/internal/entity"
)

// DocumentStructureResult contains natural payloads only. Authorization,
// typed-LP hashes, final IDs and persistence belong to the owning service.
type DocumentStructureResult struct {
	ReadingText           string                                `json:"reading_text"`
	Payloads              []entity.KnowledgeDocumentNodePayload `json:"payloads"`
	PageCount             *int64                                `json:"page_count"`
	SlideCount            *int64                                `json:"slide_count"`
	UnsupportedBlockCount int64                                 `json:"unsupported_block_count"`
	Diagnostics           []DocumentStructureDiagnostic         `json:"diagnostics"`
	Quality               string                                `json:"quality"`
	TextCoverage          string                                `json:"text_coverage"`
}

// DocumentStructureDiagnostic has the frozen bounded diagnostic JSON shape.
type DocumentStructureDiagnostic struct {
	Code          string                           `json:"code"`
	Message       string                           `json:"message"`
	NativeLocator *entity.KnowledgeDocumentLocator `json:"native_locator"`
}

// DocumentStructureError intentionally excludes native errors and source text.
type DocumentStructureError struct{ Code string }

func (e *DocumentStructureError) Error() string {
	return "document structure production failed: " + e.Code
}
func (e *DocumentStructureError) FailureCode() string { return e.Code }

func documentFailure(code string) error { return &DocumentStructureError{Code: code} }

// DocumentStructureRuntimeReady reports only the extraction dependency. The
// owning service separately checks Linux child execution and build identity.
func DocumentStructureRuntimeReady(format string) bool {
	switch format {
	case "pdf":
		return documentStructurePDFReady
	case "docx", "pptx":
		return true
	default:
		return false
	}
}

// BuildDocumentStructure produces one bounded deterministic representation.
// It does not perform network, model, application or storage operations.
func BuildDocumentStructure(ctx context.Context, format string, data []byte) (DocumentStructureResult, error) {
	if err := ctx.Err(); err != nil {
		return DocumentStructureResult{}, err
	}
	if len(data) == 0 {
		return DocumentStructureResult{}, documentFailure("invalid_source_structure")
	}
	if len(data) > 8<<20 {
		return DocumentStructureResult{}, documentFailure("budget_exceeded")
	}
	builder := newDocumentStructureBuilder(ctx)
	var err error
	switch format {
	case "pdf":
		err = builder.buildPDF(data)
	case "docx", "pptx":
		err = builder.buildOffice(format, data)
	default:
		err = documentFailure("unsupported_document")
	}
	if err != nil {
		if ctx.Err() != nil {
			return DocumentStructureResult{}, ctx.Err()
		}
		if errors.Is(err, ErrOfficePackageBudget) {
			return DocumentStructureResult{}, documentFailure("budget_exceeded")
		}
		var controlled *DocumentStructureError
		if errors.As(err, &controlled) {
			return DocumentStructureResult{}, controlled
		}
		if errors.Is(err, ErrOfficePackageBudget) {
			return DocumentStructureResult{}, documentFailure("budget_exceeded")
		}
		return DocumentStructureResult{}, documentFailure("invalid_source_structure")
	}
	result, err := builder.finish()
	if err != nil {
		return DocumentStructureResult{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 16<<20 {
		return DocumentStructureResult{}, documentFailure("budget_exceeded")
	}
	return result, nil
}

func documentMediaType(format string) string {
	if format == "pptx" {
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	}
	return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func documentProfile(format string) string {
	switch strings.ToLower(format) {
	case "pdf":
		return "pdf-plain-pages/1"
	case "pptx":
		return "pptx-ooxml-slides/1"
	default:
		return "docx-ooxml-blocks/1"
	}
}
