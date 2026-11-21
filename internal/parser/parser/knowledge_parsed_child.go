package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
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
	// The revision fixes the format. Embedded image evidence lives only in
	// this child and only its descriptive attributes enter the projection.
	requested := utility.FileType(control.Format)
	producer, effective, err := GetParser(requested, input)
	if err != nil || effective != requested {
		return nil, documentChildError("source_structure_unavailable", nil)
	}
	ctx = media.WithCache(ctx, make(knowledgeChildMedia))
	switch p := producer.(type) {
	case *MarkdownParser:
		p.FetchRemoteImages = false
		p.splitEmbeddedImages = true
	case *HTMLParser:
		p.localImagesOnly = true
	case *EPUBParser:
		// The fixed EPUB profile extracts chapter text.
		p.textOnly = true
	case *EmailParser:
		p.ConfigureFromSetup(map[string]any{"fields": []string{"from", "to", "cc", "bcc", "date", "subject", "body"}})
		p.includeHTMLBodyText = true
	}
	normalized := NormalizeParseResult(producer.ParseWithResult(ctx, control.Filename, input))
	if normalized.Diagnostic != nil || normalized.Result.Err != nil || normalized.Status != "json" {
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

// knowledgeChildMedia holds budget-admitted images for one serial parse.
// The process exits after projecting the items, releasing all payloads.
type knowledgeChildMedia map[string][]byte

func (m knowledgeChildMedia) Put(ctx context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	if err := ctx.Err(); err != nil {
		return entity.MediaRef{}, err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if _, exists := m[digest]; !exists {
		m[digest] = bytes.Clone(data)
	}
	return entity.MediaRef{Kind: "child-memory", Format: format, MIME: mime, SHA256: digest, Size: int64(len(data))}, nil
}

func (m knowledgeChildMedia) Get(ctx context.Context, ref entity.MediaRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := m[ref.SHA256]
	if !ok || ref.Kind != "child-memory" {
		return nil, media.ErrMediaNotFound
	}
	return media.ValidatePayload(data, ref)
}

func DecodeKnowledgeParsedChild(raw []byte) (*entity.KnowledgeContentProjection, error) {
	var pack KnowledgeParsedChildResult
	if len(raw) > documentChildOutputLimit || entity.KnowledgeParsedDecode(raw, &pack) != nil || pack.NormalizationStatus != "json" || pack.Projection.Binding != "unbound" {
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
