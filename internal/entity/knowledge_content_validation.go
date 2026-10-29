package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

func (s KnowledgeContentSourceRef) Validate(domain string) error {
	if !knowledgeDocumentHex(s.DomainID, 32) || s.DomainID != domain || !knowledgeDocumentHex(s.SourceID, 32) || !knowledgeDocumentHex(s.RevisionID, 32) || !knowledgeDocumentHex(s.SHA256, 64) {
		return fmt.Errorf("invalid content source binding")
	}
	return nil
}

func (t KnowledgeContentTarget) Validate(domain string) error {
	if err := t.Source.Validate(domain); err != nil {
		return err
	}
	r := t.Representation
	if r.ID == nil {
		return fmt.Errorf("unbound content representation")
	}
	switch r.Kind {
	case "source_revision":
		if *r.ID != t.Source.RevisionID || r.ProducerSHA256 != nil {
			return fmt.Errorf("invalid source representation binding")
		}
	case "structure_view", "document_representation", "parsed_representation":
		if !knowledgeDocumentHex(*r.ID, 64) || r.ProducerSHA256 == nil || !knowledgeDocumentHex(*r.ProducerSHA256, 64) {
			return fmt.Errorf("invalid derived representation binding")
		}
	default:
		return fmt.Errorf("unsupported bound representation")
	}
	return nil
}

func (r KnowledgeContentNodeRef) Validate(bound bool) error {
	if !bound {
		if r.Kind != "parser_item" || r.ID != nil || r.ItemIndex == nil || *r.ItemIndex < 0 {
			return fmt.Errorf("invalid unbound parser reference")
		}
		return nil
	}
	if r.Kind != "c1_node" && r.Kind != "r1_node" && r.Kind != "r2_node" && r.Kind != "parsed_node" || r.ID == nil || !knowledgeDocumentHex(*r.ID, 64) || r.ItemIndex != nil {
		return fmt.Errorf("invalid bound node reference")
	}
	return nil
}

func (a KnowledgeContentAddress) Validate(domain string) error {
	if err := a.Target.Validate(domain); err != nil {
		return err
	}
	if err := a.Node.Validate(true); err != nil {
		return err
	}
	expected := map[string]string{"source_revision": "c1_node", "structure_view": "r1_node", "document_representation": "r2_node", "parsed_representation": "parsed_node"}
	if expected[a.Target.Representation.Kind] != a.Node.Kind {
		return fmt.Errorf("node representation namespace mismatch")
	}
	return nil
}

func (r KnowledgeContentRange) Validate() error {
	if r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > 9007199254740991 || r.SHA256 != nil && !knowledgeDocumentHex(*r.SHA256, 64) {
		return fmt.Errorf("invalid content range")
	}
	return nil
}

func (t KnowledgeContentText) Validate(space string) error {
	if t.CoordinateSpace != space || space != "source_utf8" && space != "derived_utf8" && space != "item_utf8" {
		return fmt.Errorf("content coordinate space mismatch")
	}
	if t.Range != nil {
		if err := t.Range.Validate(); err != nil {
			return err
		}
	}
	if t.OwnRange != nil {
		if t.OwnRange.Validate() != nil || t.Range == nil || t.OwnRange.StartByte < t.Range.StartByte || t.OwnRange.EndByte > t.Range.EndByte {
			return fmt.Errorf("own content range outside node")
		}
	}
	switch t.Status {
	case "available":
		if t.Range == nil || t.Range.EndByte == t.Range.StartByte || t.Reason != nil {
			return fmt.Errorf("invalid available text descriptor")
		}
	case "empty":
		if t.Range == nil || t.Range.StartByte != t.Range.EndByte || t.Reason != nil {
			return fmt.Errorf("invalid empty text descriptor")
		}
	case "unsupported", "unsupported_or_requires_ocr", "unavailable":
		if t.Reason == nil || strings.TrimSpace(*t.Reason) == "" || t.Value != nil {
			return fmt.Errorf("missing unavailable text reason")
		}
	default:
		return fmt.Errorf("unknown content text state")
	}
	if t.Value != nil {
		if t.Range == nil || !utf8.ValidString(*t.Value) || int64(len(*t.Value)) != t.Range.EndByte-t.Range.StartByte {
			return fmt.Errorf("text value does not cover exact range")
		}
		if t.Range.SHA256 != nil {
			digest := sha256.Sum256([]byte(*t.Value))
			if hex.EncodeToString(digest[:]) != *t.Range.SHA256 {
				return fmt.Errorf("content fragment digest mismatch")
			}
		}
		if t.OwnRange != nil {
			a, b := t.OwnRange.StartByte-t.Range.StartByte, t.OwnRange.EndByte-t.Range.StartByte
			value := *t.Value
			if a < int64(len(value)) && !utf8.RuneStart(value[a]) || b < int64(len(value)) && !utf8.RuneStart(value[b]) {
				return fmt.Errorf("own range splits a UTF-8 rune")
			}
			if t.OwnRange.SHA256 != nil {
				digest := sha256.Sum256([]byte(value[a:b]))
				if hex.EncodeToString(digest[:]) != *t.OwnRange.SHA256 {
					return fmt.Errorf("own text digest mismatch")
				}
			}
		}
	}
	return nil
}

func (n KnowledgeContentNode) Validate(target *KnowledgeContentTarget) error {
	bound := target != nil
	if err := n.Ref.Validate(bound); err != nil {
		return err
	}
	if n.ParentRef != nil {
		if n.ParentRef.Validate(bound) != nil || n.ParentRef.Kind != n.Ref.Kind || n.Ref.ID != nil && *n.ParentRef.ID == *n.Ref.ID {
			return fmt.Errorf("invalid content parent reference")
		}
		if !bound && *n.ParentRef.ItemIndex == *n.Ref.ItemIndex {
			return fmt.Errorf("self parent parser reference")
		}
	}
	if n.Kind == "" || !utf8.ValidString(n.Kind) || !utf8.ValidString(n.Name) || n.Ordinal != nil && *n.Ordinal < 0 {
		return fmt.Errorf("invalid content node metadata")
	}
	if bound {
		if err := (KnowledgeContentAddress{Target: *target, Node: n.Ref}).Validate(target.Source.DomainID); err != nil {
			return err
		}
	}
	space, locator := "item_utf8", "parser_item"
	switch n.Ref.Kind {
	case "c1_node", "r1_node":
		space, locator = "source_utf8", "source_utf8"
	case "r2_node":
		space, locator = "derived_utf8", "document"
	case "parsed_node":
		space, locator = "derived_utf8", "parsed_item"
		if n.Kind == "document" || n.Kind == "passage" {
			locator = "document"
		}
	}
	if n.Locator.Kind != locator || n.Text.Validate(space) != nil {
		return fmt.Errorf("invalid content text or locator coordinate")
	}
	switch locator {
	case "source_utf8":
		s := n.Locator.SourceUTF8
		if s == nil || n.Locator.Document != nil || n.Locator.ItemIndex != nil || n.Text.Range == nil || s.StartByte != n.Text.Range.StartByte || s.EndByte != n.Text.Range.EndByte || s.StartLine < 1 || s.EndLine < s.StartLine || n.Ordinal != nil || n.OriginalPosition != nil || n.CoordinatePrecision != "source_bytes" {
			return fmt.Errorf("invalid source byte locator")
		}
	case "document":
		if n.Locator.Document == nil || n.Locator.SourceUTF8 != nil || n.Locator.ItemIndex != nil || n.Ordinal == nil || n.OriginalPosition == nil {
			return fmt.Errorf("invalid document locator")
		}
		if _, err := json.Marshal(n.Locator.Document); err != nil {
			return fmt.Errorf("invalid natural document locator")
		}
		precision := map[string]string{"document_v1": "document", "pdf_page_v1": "physical_page", "office_package_v1": "source_path", "derived_passage_v1": "derived_range"}
		if precision[n.Locator.Document.Scheme] != n.CoordinatePrecision {
			return fmt.Errorf("natural locator precision mismatch")
		}
		if _, err := json.Marshal(n.OriginalPosition); err != nil {
			return fmt.Errorf("invalid original position")
		}
	case "parsed_item":
		if !bound || n.Ref.Kind != "parsed_node" || n.Locator.ItemIndex == nil || *n.Locator.ItemIndex < 0 || n.Locator.Document != nil || n.Locator.SourceUTF8 != nil || n.OriginalPosition != nil || n.CoordinatePrecision != "item_only" {
			return fmt.Errorf("invalid fixed parsed item locator")
		}
	case "parser_item":
		if n.Locator.ItemIndex == nil || *n.Locator.ItemIndex != *n.Ref.ItemIndex || n.Locator.Document != nil || n.Locator.SourceUTF8 != nil || n.OriginalPosition != nil || n.CoordinatePrecision != "item_only" {
			return fmt.Errorf("invalid parser item locator")
		}
	}
	attrs := n.Attributes
	count := 0
	for _, present := range []bool{attrs.Source != nil, attrs.Structure != nil, attrs.Document != nil, attrs.Parser != nil} {
		if present {
			count++
		}
	}
	if count != 1 || n.Ref.Kind == "c1_node" && attrs.Source == nil || n.Ref.Kind == "r1_node" && attrs.Structure == nil || n.Ref.Kind == "r2_node" && attrs.Document == nil || n.Ref.Kind == "parser_item" && attrs.Parser == nil {
		return fmt.Errorf("content attributes namespace mismatch")
	}
	if n.Ref.Kind == "parsed_node" && ((n.Kind == "document" || n.Kind == "passage") && attrs.Document == nil || n.Kind != "document" && n.Kind != "passage" && attrs.Parser == nil) {
		return fmt.Errorf("parsed attributes kind mismatch")
	}
	if attrs.Parser != nil {
		if !utf8.ValidString(attrs.Parser.CKType) || !utf8.ValidString(attrs.Parser.Sheet) || attrs.Parser.AltText != nil && !utf8.ValidString(*attrs.Parser.AltText) {
			return fmt.Errorf("invalid parser metadata UTF-8")
		}
		if attrs.Parser.ItemType != "text" && attrs.Parser.ItemType != "table" && attrs.Parser.ItemType != "image" && attrs.Parser.ItemType != "opaque" {
			return fmt.Errorf("unknown parser item type")
		}
		for _, position := range attrs.Parser.Positions {
			for _, value := range position {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return fmt.Errorf("invalid parser position number")
				}
			}
		}
	}
	if attrs.Document != nil && !contentDocumentAttributeKind(n.Kind, *attrs.Document) {
		return fmt.Errorf("document attribute kind mismatch")
	}
	if n.AttachmentRef != nil && (!bound || n.AttachmentRef.Validate(target.Source.DomainID) != nil) {
		return fmt.Errorf("invalid verified attachment binding")
	}
	return nil
}

func (p KnowledgeContentProjection) Validate() error {
	if p.Version != KnowledgeContentVersion || p.Nodes == nil || p.Relationships == nil {
		return fmt.Errorf("invalid content projection envelope")
	}
	var target *KnowledgeContentTarget
	if p.Binding == "bound" && p.Source != nil {
		target = &KnowledgeContentTarget{Source: *p.Source, Representation: p.Ref}
		if err := target.Validate(p.Source.DomainID); err != nil {
			return err
		}
	} else if p.Binding != "unbound" || p.Source != nil || p.Ref.Kind != "parser_result" || p.Ref.ID != nil || p.Ref.ProducerSHA256 != nil {
		return fmt.Errorf("invalid projection binding state")
	}
	if p.RootRef != nil && p.RootRef.Validate(target != nil) != nil {
		return fmt.Errorf("invalid projection root reference")
	}
	switch p.State {
	case "ready", "empty":
		if p.Reason != nil {
			return fmt.Errorf("unexpected projection state reason")
		}
	case "partial", "unavailable", "normalization_required":
		if p.Reason == nil || strings.TrimSpace(*p.Reason) == "" {
			return fmt.Errorf("missing projection state reason")
		}
	default:
		return fmt.Errorf("unknown projection state")
	}
	if p.NodeCount != nil && (*p.NodeCount < int64(len(p.Nodes)) || target == nil && *p.NodeCount != int64(len(p.Nodes))) {
		return fmt.Errorf("invalid projection node count")
	}
	if target == nil && (p.CoordinateSpace != "item_utf8" || len(p.Relationships) != 0) {
		return fmt.Errorf("unbound projection asserted fixed evidence")
	}
	seen := make(map[string]bool, len(p.Nodes))
	for _, node := range p.Nodes {
		if err := node.Validate(target); err != nil {
			return err
		}
		key, _ := json.Marshal(node.Ref)
		if seen[string(key)] {
			return fmt.Errorf("duplicate content node reference")
		}
		seen[string(key)] = true
	}
	return nil
}

func contentDocumentAttributeKind(kind string, a KnowledgeContentDocumentAttributes) bool {
	arms := map[string]bool{"empty": a.Empty != nil, "page": a.Page != nil, "slide": a.Slide != nil, "heading": a.Heading != nil, "table": a.Table != nil, "row": a.Row != nil, "cell": a.Cell != nil, "list": a.List != nil, "list_item": a.ListItem != nil, "media": a.Media != nil, "passage": a.Passage != nil, "unsupported_block": a.UnsupportedBlock != nil}
	count := 0
	for _, present := range arms {
		if present {
			count++
		}
	}
	if count != 1 {
		return false
	}
	switch kind {
	case "document", "section", "paragraph", "text_block", "text_box":
		return arms["empty"]
	}
	return arms[kind]
}
