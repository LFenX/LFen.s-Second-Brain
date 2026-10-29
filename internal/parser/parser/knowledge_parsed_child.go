package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"path"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

const KnowledgeParsedLocalProfile = "knowledge-parsed-local/1"

// KnowledgeParsedLocalFormat shares the producer registry, excluding the three
// existing R2 profiles and families that need model or OCR processing.
func KnowledgeParsedLocalFormat(format string) bool {
	if format == "pdf" || format == "docx" || format == "pptx" {
		return false
	}
	for _, capability := range RegisteredParserCapabilities() {
		if string(capability.FileType) == format {
			return capability.BaseResult == "structured"
		}
	}
	return false
}

func knowledgeParsedFilename(format, filename string) bool {
	return utf8.ValidString(filename) && len(filename) > 0 && len(filename) <= 255 && path.Base(filename) == filename && !strings.ContainsAny(filename, "\\\x00\r\n") && string(utility.GetFileType(filename)) == format
}

type KnowledgeParsedChildResult struct {
	NormalizationStatus string                            `json:"normalization_status"`
	Projection          entity.KnowledgeContentProjection `json:"projection"`
}

// ParseKnowledgeParsedChild is called only after the owning service has checked
// this executable's actual producer. Original bytes come from the owned fd3.
func ParseKnowledgeParsedChild(ctx context.Context, control DocumentChildControl, input []byte) (json.RawMessage, error) {
	if control.ProfileID != KnowledgeParsedLocalProfile || !KnowledgeParsedLocalFormat(control.Format) || !knowledgeParsedFilename(control.Format, control.Filename) || int64(len(input)) != control.InputBytes || control.InputBytes > entity.KnowledgeMaxSourceBytes {
		return nil, documentChildError("unsupported_document", nil)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(input) > 0 && (control.Format == "epub" || control.Format == "xlsx" || bytes.HasPrefix(input, []byte{'P', 'K', 3, 4})) {
		if err := knowledgeParsedArchiveBudget(input); err != nil {
			return nil, err
		}
	}
	producer, err := GetParser(utility.FileType(control.Format))
	if err != nil {
		return nil, documentChildError("source_structure_unavailable", nil)
	}
	if markdown, ok := producer.(*MarkdownParser); ok {
		markdown.FetchRemoteImages = false
	}
	if email, ok := producer.(*EmailParser); ok {
		email.ConfigureFromSetup(map[string]any{"output_format": "text", "fields": []string{"from", "to", "cc", "bcc", "date", "subject", "body"}})
	}
	normalized := NormalizeParseResult(ctx, control.Filename, producer.ParseWithResult(ctx, control.Filename, input))
	if normalized.Diagnostic != nil || normalized.Result.Err != nil || normalized.Status != "json" && normalized.Status != "text" && normalized.Status != "markdown" {
		return nil, documentChildError("source_structure_unavailable", nil)
	}
	projection, err := ProjectKnowledgeParseResult(normalized.Result)
	if err != nil {
		return nil, documentChildError("invalid_source_structure", nil)
	}
	pack := KnowledgeParsedChildResult{NormalizationStatus: normalized.Status, Projection: *projection}
	raw, err := json.Marshal(pack)
	if err != nil || len(raw) > documentChildOutputLimit-4096 {
		return nil, documentChildError("budget_exceeded", nil)
	}
	return raw, nil
}

func DecodeKnowledgeParsedChild(raw []byte) (*entity.KnowledgeContentProjection, error) {
	var pack KnowledgeParsedChildResult
	if len(raw) > documentChildOutputLimit || entity.KnowledgeParsedDecode(raw, &pack) != nil || pack.NormalizationStatus != "json" && pack.NormalizationStatus != "text" && pack.NormalizationStatus != "markdown" || pack.Projection.Binding != "unbound" {
		return nil, documentChildError("invalid_source_structure", nil)
	}
	if pack.Projection.Validate() != nil {
		return nil, documentChildError("invalid_source_structure", nil)
	}
	return &pack.Projection, nil
}

func knowledgeParsedArchiveBudget(input []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(input), int64(len(input)))
	if err != nil || len(reader.File) > 4096 {
		return documentChildError("invalid_source_structure", nil)
	}
	var expanded uint64
	for _, entry := range reader.File {
		if entry.UncompressedSize64 > 8<<20 || expanded+entry.UncompressedSize64 > 32<<20 {
			return documentChildError("budget_exceeded", nil)
		}
		expanded += entry.UncompressedSize64
	}
	return nil
}
