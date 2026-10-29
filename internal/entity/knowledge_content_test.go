package entity

import (
	"strings"
	"testing"
)

func contentTestString(s string) *string { return &s }
func contentTestInt(i int64) *int64      { return &i }

func contentTestTarget(kind string) KnowledgeContentTarget {
	source := KnowledgeContentSourceRef{DomainID: strings.Repeat("1", 32), SourceID: strings.Repeat("2", 32), RevisionID: strings.Repeat("3", 32), SHA256: strings.Repeat("4", 64)}
	ref := KnowledgeContentRepresentationRef{Kind: kind, ID: &source.RevisionID}
	if kind != "source_revision" {
		ref.ID, ref.ProducerSHA256 = contentTestString(strings.Repeat("5", 64)), contentTestString(strings.Repeat("6", 64))
	}
	return KnowledgeContentTarget{Source: source, Representation: ref}
}

func TestKnowledgeContentFixedReferences(t *testing.T) {
	for _, kind := range []string{"source_revision", "structure_view", "document_representation"} {
		target := contentTestTarget(kind)
		if err := target.Validate(target.Source.DomainID); err != nil {
			t.Fatal(kind, err)
		}
		if target.Validate(strings.Repeat("9", 32)) == nil {
			t.Fatal("accepted cross-domain reference")
		}
	}
	for _, mutate := range []func(*KnowledgeContentTarget){
		func(v *KnowledgeContentTarget) { v.Representation.ID = nil },
		func(v *KnowledgeContentTarget) { v.Representation.ID = contentTestString(strings.Repeat("7", 32)) },
		func(v *KnowledgeContentTarget) {
			v.Representation.ProducerSHA256 = contentTestString(strings.Repeat("8", 64))
		},
		func(v *KnowledgeContentTarget) { v.Representation.Kind = "parser_result" },
	} {
		v := contentTestTarget("source_revision")
		mutate(&v)
		if v.Validate(v.Source.DomainID) == nil {
			t.Fatal("accepted invalid fixed source")
		}
	}
	address := KnowledgeContentAddress{Target: contentTestTarget("document_representation"), Node: KnowledgeContentNodeRef{Kind: "r1_node", ID: contentTestString(strings.Repeat("7", 64))}}
	if address.Validate(address.Target.Source.DomainID) == nil {
		t.Fatal("mixed R1/R2 node namespace")
	}
	if (KnowledgeContentNodeRef{Kind: "parser_item", ItemIndex: contentTestInt(0)}).Validate(true) == nil {
		t.Fatal("published temporary item")
	}
}

func TestKnowledgeContentTextCoordinatesAndEmptyValues(t *testing.T) {
	value := "合成\r\n"
	text := KnowledgeContentText{CoordinateSpace: "source_utf8", Range: &KnowledgeContentRange{StartByte: 5, EndByte: 5 + int64(len(value))}, Status: "available", Value: &value}
	if err := text.Validate("source_utf8"); err != nil {
		t.Fatal(err)
	}
	if text.Validate("derived_utf8") == nil {
		t.Fatal("confused original and derived coordinates")
	}
	text.Range.SHA256 = contentTestString(strings.Repeat("f", 64))
	if text.Validate("source_utf8") == nil {
		t.Fatal("accepted forged fragment digest")
	}
	empty := ""
	text = KnowledgeContentText{CoordinateSpace: "item_utf8", Range: &KnowledgeContentRange{}, Status: "empty", Value: &empty}
	if err := text.Validate("item_utf8"); err != nil {
		t.Fatal(err)
	}
	text.Status = "unavailable"
	if text.Validate("item_utf8") == nil {
		t.Fatal("unavailable accepted as empty text")
	}
	text.Reason, text.Value = contentTestString("missing_payload"), nil
	if err := text.Validate("item_utf8"); err != nil {
		t.Fatal(err)
	}
	text.OwnRange = &KnowledgeContentRange{EndByte: 1}
	if text.Validate("item_utf8") == nil {
		t.Fatal("own range exceeded node range")
	}
}

func TestKnowledgeContentParserLocatorCannotClaimOriginalPage(t *testing.T) {
	node := KnowledgeContentNode{
		Ref: KnowledgeContentNodeRef{Kind: "parser_item", ItemIndex: contentTestInt(0)}, Kind: "opaque",
		Locator: KnowledgeContentLocator{Kind: "parser_item", ItemIndex: contentTestInt(0)}, CoordinatePrecision: "item_only",
		Text:       KnowledgeContentText{CoordinateSpace: "item_utf8", Status: "unavailable", Reason: contentTestString("uninterpreted")},
		Attributes: KnowledgeContentAttributes{Parser: &KnowledgeContentParserAttributes{ItemType: "opaque"}},
	}
	if err := node.Validate(nil); err != nil {
		t.Fatal(err)
	}
	node.Locator.Document = &KnowledgeDocumentLocator{Scheme: "pdf_page_v1", PageNumber: 3}
	if node.Validate(nil) == nil {
		t.Fatal("temporary item claimed original physical page")
	}
	node.Locator.Document = nil
	node.Ref.ID = contentTestString(strings.Repeat("a", 64))
	if node.Validate(nil) == nil {
		t.Fatal("temporary item forged stable ID")
	}
}
