// Copyright 2026 LFen's Second Brain contributors.
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"unicode/utf8"

	"ragflow/internal/entity"
)

type documentStructureNode struct {
	payload                      entity.KnowledgeDocumentNodePayload
	children                     []*documentStructureNode
	parent                       *documentStructureNode
	ownText                      string
	own                          bool
	separator                    string
	inline                       bool
	inlineOffset                 int64
	start, end, ownStart, ownEnd int64
	depth                        int
}

type documentStructureBuilder struct {
	ctx        context.Context
	root       *documentStructureNode
	count      int
	result     DocumentStructureResult
	text       []byte
	boundaries []int
}

func newDocumentStructureBuilder(ctx context.Context) *documentStructureBuilder {
	b := &documentStructureBuilder{ctx: ctx, result: DocumentStructureResult{Payloads: []entity.KnowledgeDocumentNodePayload{}, Diagnostics: []DocumentStructureDiagnostic{}, Quality: "complete", TextCoverage: "supported_profile_blocks"}}
	b.root = &documentStructureNode{separator: "\n", payload: entity.KnowledgeDocumentNodePayload{Kind: "document", Attributes: json.RawMessage(`{}`), Locator: entity.KnowledgeDocumentLocator{Scheme: "document_v1"}, OriginalPosition: entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}, CoordinatePrecision: "document"}}
	b.count = 1
	return b
}

func documentHash(text []byte) string {
	value := sha256.Sum256(text)
	return hex.EncodeToString(value[:])
}
func documentInt(value int64) *int64      { return &value }
func documentString(value string) *string { return &value }

func (b *documentStructureBuilder) add(parent *documentStructureNode, kind string, locator entity.KnowledgeDocumentLocator, position entity.KnowledgeDocumentOriginalPosition, attrs any) (*documentStructureNode, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if b.count >= 8192 || parent.depth >= 16 || (locator.Scheme == "office_package_v1" && len(locator.Segments) > 32) {
		return nil, documentFailure("budget_exceeded")
	}
	encoded, err := json.Marshal(attrs)
	if err != nil {
		return nil, documentFailure("invalid_source_structure")
	}
	precision := "source_path"
	if locator.Scheme == "pdf_page_v1" {
		precision = "physical_page"
	}
	if locator.Scheme == "derived_passage_v1" {
		precision = "derived_range"
	}
	node := &documentStructureNode{parent: parent, depth: parent.depth + 1, separator: "\n", payload: entity.KnowledgeDocumentNodePayload{Kind: kind, Attributes: encoded, Locator: locator, OriginalPosition: position, CoordinatePrecision: precision}}
	parent.children = append(parent.children, node)
	b.count++
	return node, nil
}

func (b *documentStructureBuilder) diagnostic(code string, locator *entity.KnowledgeDocumentLocator) {
	if len(b.result.Diagnostics) >= 8 {
		return
	}
	messages := map[string]string{"unsupported_source_block": "A source block is outside this profile.", "media_not_ocr": "Media text was not requested.", "no_extractable_text": "No extractable text is available at this source address."}
	b.result.Diagnostics = append(b.result.Diagnostics, DocumentStructureDiagnostic{Code: code, Message: messages[code], NativeLocator: locator})
}

func (b *documentStructureBuilder) unsupported(parent *documentStructureNode, locator entity.KnowledgeDocumentLocator, position entity.KnowledgeDocumentOriginalPosition, nativeType string) error {
	if len(nativeType) > 64 {
		nativeType = "unknown"
	}
	node, err := b.add(parent, "unsupported_block", locator, position, entity.KnowledgeDocumentUnsupportedBlockAttributes{NativeType: nativeType, Reason: "unsupported_source_block"})
	if err != nil {
		return err
	}
	node.payload.TextStatus = "unsupported"
	node.payload.TextReason = documentString("unsupported_source_block")
	b.result.UnsupportedBlockCount++
	b.diagnostic("unsupported_source_block", &node.payload.Locator)
	return nil
}

func (b *documentStructureBuilder) appendText(text string) error {
	if !utf8.ValidString(text) {
		return documentFailure("invalid_source_structure")
	}
	if len(text) > 4<<20-len(b.text) {
		return documentFailure("budget_exceeded")
	}
	b.text = append(b.text, text...)
	return nil
}

func (b *documentStructureBuilder) render(node *documentStructureNode) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	node.start = int64(len(b.text))
	node.ownStart = node.start
	if node.own {
		if err := b.appendText(node.ownText); err != nil {
			return err
		}
		node.ownEnd = int64(len(b.text))
		b.boundaries = append(b.boundaries, len(b.text))
	}
	blocks := 0
	for _, child := range node.children {
		if child.inline {
			child.start = node.ownStart + child.inlineOffset
			child.end = child.start
			var attributes entity.KnowledgeDocumentMediaAttributes
			if err := json.Unmarshal(child.payload.Attributes, &attributes); err != nil {
				return documentFailure("invalid_source_structure")
			}
			attributes.InlineAnchorByte = documentInt(child.start)
			encoded, err := json.Marshal(attributes)
			if err != nil {
				return err
			}
			child.payload.Attributes = encoded
			continue
		}
		if blocks > 0 || node.own {
			if err := b.appendText(node.separator); err != nil {
				return err
			}
		}
		if err := b.render(child); err != nil {
			return err
		}
		blocks++
	}
	node.end = int64(len(b.text))
	if node.payload.Kind == "row" {
		b.boundaries = append(b.boundaries, len(b.text))
	}
	return nil
}

func (b *documentStructureBuilder) readingRange(start, end int64) entity.KnowledgeDocumentReadingRange {
	return entity.KnowledgeDocumentReadingRange{StartByte: start, EndByte: end, TextSHA256: documentHash(b.text[start:end])}
}

func documentTruncate(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end], true
}

func (b *documentStructureBuilder) finish() (DocumentStructureResult, error) {
	if err := b.render(b.root); err != nil {
		return DocumentStructureResult{}, err
	}
	var semantic []*documentStructureNode
	var collect func(*documentStructureNode)
	collect = func(node *documentStructureNode) {
		semantic = append(semantic, node)
		for _, child := range node.children {
			collect(child)
		}
	}
	collect(b.root)
	lineEnds := []int{}
	for i, value := range b.text {
		if value == '\n' {
			lineEnds = append(lineEnds, i+1)
		}
	}
	sort.Ints(b.boundaries)
	chunks := map[*documentStructureNode]int64{}
	for start := 0; start < len(b.text); {
		if err := b.ctx.Err(); err != nil {
			return DocumentStructureResult{}, err
		}
		end := len(b.text)
		if end-start > 8192 {
			end = documentNearestCut(b.boundaries, start+2048, start+8192, start+4096)
			if end < 0 {
				end = documentNearestCut(lineEnds, start+2048, start+8192, start+4096)
			}
			if end < 0 {
				end = start + 4096
				for !utf8.RuneStart(b.text[end]) {
					end--
				}
			}
		}
		owner := b.root
		for _, node := range semantic {
			if node.start <= int64(start) && node.end >= int64(end) && node.depth > owner.depth {
				owner = node
			}
		}
		locator := entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &owner.payload.Locator, ChunkIndex: chunks[owner]}
		passage, err := b.add(owner, "passage", locator, owner.payload.OriginalPosition, entity.KnowledgeDocumentPassageAttributes{ChunkIndex: chunks[owner]})
		if err != nil {
			return DocumentStructureResult{}, err
		}
		chunks[owner]++
		passage.start = int64(start)
		passage.end = int64(end)
		passage.own = true
		passage.ownStart = passage.start
		passage.ownEnd = passage.end
		start = end
	}
	var flatten func(*documentStructureNode) error
	seen := map[string]bool{}
	flatten = func(node *documentStructureNode) error {
		payload := node.payload
		payload.ReadingRange = b.readingRange(node.start, node.end)
		if node.own {
			value := b.readingRange(node.ownStart, node.ownEnd)
			payload.OwnReadingRange = &value
		}
		if node.own && node.payload.Kind != "passage" {
			payload.Name, payload.NameTruncated = documentTruncate(node.ownText, 128)
		}
		if payload.TextStatus == "" {
			payload.TextStatus = "empty"
			if node.end > node.start {
				payload.TextStatus = "available"
			}
		}
		if node.parent != nil {
			payload.ParentRef = &entity.KnowledgeDocumentNaturalRef{Kind: node.parent.payload.Kind, Locator: node.parent.payload.Locator}
		}
		identity, err := json.Marshal(entity.KnowledgeDocumentNaturalRef{Kind: payload.Kind, Locator: payload.Locator})
		if err != nil || seen[string(identity)] {
			return documentFailure("invalid_source_structure")
		}
		seen[string(identity)] = true
		b.result.Payloads = append(b.result.Payloads, payload)
		for ordinal, child := range node.children {
			child.payload.Ordinal = int64(ordinal)
			if err := flatten(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := flatten(b.root); err != nil {
		return DocumentStructureResult{}, err
	}
	b.result.ReadingText = string(b.text)
	return b.result, nil
}

func documentNearestCut(candidates []int, low, high, target int) int {
	first := sort.SearchInts(candidates, low)
	best := -1
	for i := first; i < len(candidates) && candidates[i] <= high; i++ {
		point := candidates[i]
		if best < 0 || documentAbs(point-target) < documentAbs(best-target) {
			best = point
		}
	}
	return best
}
func documentAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
