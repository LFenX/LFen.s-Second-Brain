package parser

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestKnowledgeContentParseResultTypedItems(t *testing.T) {
	table := &entity.TableData{HeaderRows: 1, Caption: "表头", Grid: [][]entity.TableCell{
		{{Text: "跨列", ColSpan: 2}, {Covered: true, OriginRow: 0, OriginCol: 0}},
		{{Text: "甲"}, {Text: ""}},
	}}
	p, err := ProjectKnowledgeParseResult(ParseResult{OutputFormat: "json", File: map[string]any{"owner_id": "FORGED_OWNER"}, JSON: []DocItem{
		&TextItem{Text: "合成\r\n正文", CKType: "heading", Extra: map[string]any{"text": "FORGED_TEXT", "source_revision_id": "FORGED_REVISION", "page_number": 99}},
		&TableItem{Table: table, Sheet: "合成页", Positions: [][]float64{{0, 1, 2, 3, 4}}},
		&ImageItem{Text: "仅替代文本", Image: "SECRET_IMAGE_PAYLOAD", Extra: map[string]any{"ocr_text": "FORGED_OCR"}},
		RawItem{M: map[string]any{"text": "FORGED_RAW", "source_ref": "FORGED_SOURCE"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "partial" || p.Source != nil || p.Ref.ID != nil || len(p.Nodes) != 4 {
		t.Fatalf("wrong draft envelope: %#v", p)
	}
	if *p.Nodes[0].Text.Value != "合成\r\n正文" || p.Nodes[0].Kind != "heading" || p.Nodes[0].Text.Range.EndByte != int64(len("合成\r\n正文")) {
		t.Fatal("text bytes or kind changed")
	}
	got := p.Nodes[1].Attributes.Parser.Table
	if got.HeaderRows != 1 || got.Grid[0][0].ColSpan != 2 || !got.Grid[0][1].Covered || got.Grid[0][1].OriginCol != 0 || got.Grid[1][1].Text != "" {
		t.Fatal("table geometry lost")
	}
	if *p.Nodes[1].Text.Value != "跨列\n甲\t" || p.Nodes[1].Text.CoordinateSpace != "item_utf8" {
		t.Fatal("table text assembly or coordinate changed")
	}
	if p.Nodes[2].Text.Status != "unsupported" || p.Nodes[2].Text.Value != nil || *p.Nodes[2].Attributes.Parser.AltText != "仅替代文本" || !p.Nodes[2].Attributes.Parser.PayloadPresent {
		t.Fatal("image alt claimed OCR")
	}
	if p.Nodes[3].Text.Status != "unavailable" || p.Nodes[3].Text.Value != nil {
		t.Fatal("uninterpreted RawItem claimed extraction")
	}
	for i, node := range p.Nodes {
		if node.Ref.ID != nil || node.Ref.ItemIndex == nil || *node.Ref.ItemIndex != int64(i) || node.Locator.Document != nil || node.AttachmentRef != nil {
			t.Fatal("forged fixed identity or locator")
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"FORGED_", "SECRET_IMAGE_PAYLOAD", "owner_id", "ocr_text"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("untrusted field escaped: %s", forbidden)
		}
	}
}

func TestKnowledgeContentParseResultOwnsCopies(t *testing.T) {
	item := TableItem{Table: &entity.TableData{Grid: [][]entity.TableCell{{{Text: "原文", RowSpan: 2}}}}, Positions: [][]float64{{1, 2, 3}}}
	p, err := ProjectKnowledgeParseResult(ParseResult{OutputFormat: "json", JSON: []DocItem{item}})
	if err != nil {
		t.Fatal(err)
	}
	item.Table.Grid[0][0].Text, item.Positions[0][0] = "改写", 99
	if p.Nodes[0].Attributes.Parser.Table.Grid[0][0].Text != "原文" || p.Nodes[0].Attributes.Parser.Positions[0][0] != 1 {
		t.Fatal("projection aliases producer memory")
	}
	p.Nodes[0].Attributes.Parser.Table.Grid[0][0].RowSpan = 4
	if item.Table.Grid[0][0].RowSpan != 2 {
		t.Fatal("projection mutated producer grid")
	}
}

func TestKnowledgeContentParseResultActualSupportStates(t *testing.T) {
	cases := []struct {
		name   string
		result ParseResult
		state  string
		count  int
	}{
		{"parser_failed", ParseResult{OutputFormat: "json", JSON: []DocItem{NewTextJSONItem("must not escape")}, Err: errors.New("private backend detail")}, "unavailable", 0},
		{"configuration_only", ParseResult{}, "unavailable", 0},
		{"empty_json", ParseResult{OutputFormat: "json"}, "unavailable", 0},
		{"pending_markdown", ParseResult{OutputFormat: "markdown", Markdown: "# 尚未归一化"}, "normalization_required", 0},
		{"text_response", ParseResult{OutputFormat: "text", Text: "逐字保留\r\n"}, "ready", 1},
		{"empty_text_item", ParseResult{OutputFormat: "json", JSON: []DocItem{NewTextJSONItem("")}}, "empty", 1},
		{"raw_only", ParseResult{OutputFormat: "json", JSON: []DocItem{RawItem{M: map[string]any{"text": "未解释"}}}}, "unavailable", 1},
		{"image_only", ParseResult{OutputFormat: "json", JSON: []DocItem{ImageItem{Text: "alt", Image: "payload"}}}, "unavailable", 1},
		{"missing_table", ParseResult{OutputFormat: "json", JSON: []DocItem{TableItem{}}}, "unavailable", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ProjectKnowledgeParseResult(tc.result)
			if err != nil || p.State != tc.state || len(p.Nodes) != tc.count || p.Binding != "unbound" {
				t.Fatalf("state=%#v err=%v", p, err)
			}
		})
	}
}

func TestKnowledgeContentAudioMetadataDoesNotProveEmptyDocument(t *testing.T) {
	result := NewAudioParser().ParseWithResult(t.Context(), "synthetic.wav", []byte("synthetic audio bytes; no transcription"))
	if result.Err != nil || result.OutputFormat != "text" || result.File == nil || len(result.JSON) != 0 || result.Text != "" || result.Markdown != "" {
		t.Fatal("audio configuration boundary changed", result.Err)
	}
	projected, err := ProjectKnowledgeParseResult(result)
	if err != nil || projected.State != "unavailable" || projected.Reason == nil || *projected.Reason != "no_explicit_content_items" || len(projected.Nodes) != 0 {
		t.Fatal(projected, err)
	}
	confirmed, err := ProjectKnowledgeParseResult(ParseResult{OutputFormat: "json", JSON: []DocItem{TextItem{Text: ""}}})
	if err != nil || confirmed.State != "empty" || len(confirmed.Nodes) != 1 || confirmed.Nodes[0].Text.Value == nil || *confirmed.Nodes[0].Text.Value != "" {
		t.Fatal(confirmed, err)
	}
}

func TestKnowledgeContentEmptyCSVAndJSONUseActualProducerItems(t *testing.T) {
	csv := NewCSVParser().ParseWithResult(t.Context(), "synthetic.csv", nil)
	if csv.Err != nil || csv.OutputFormat == "" || len(csv.JSON) != 0 {
		t.Fatal("CSV empty producer changed", csv.Err)
	}
	csvProjection, err := ProjectKnowledgeParseResult(csv)
	if err != nil || csvProjection.State != "unavailable" || csvProjection.Reason == nil {
		t.Fatal(csvProjection, err)
	}
	parsed := NewJSONParser().ParseWithResult(t.Context(), "synthetic.json", nil)
	if parsed.Err != nil || len(parsed.JSON) != 1 {
		t.Fatal("JSON explicit empty producer changed", parsed.Err)
	}
	jsonProjection, err := ProjectKnowledgeParseResult(parsed)
	if err != nil || jsonProjection.State != "empty" || len(jsonProjection.Nodes) != 1 || jsonProjection.Nodes[0].Text.Status != "empty" {
		t.Fatal(jsonProjection, err)
	}
}

func TestKnowledgeContentParseResultRejectsInvalidItems(t *testing.T) {
	var nilText *TextItem
	for _, item := range []DocItem{nil, nilText, TextItem{Text: string([]byte{0xff})}, TableItem{Table: &entity.TableData{Grid: [][]entity.TableCell{{{Text: string([]byte{0xff})}}}}}, ImageItem{Positions: [][]float64{{math.Inf(1)}}}} {
		if p, err := ProjectKnowledgeParseResult(ParseResult{OutputFormat: "json", JSON: []DocItem{item}}); err == nil || p != nil {
			t.Fatalf("accepted invalid item: %#v err=%v", p, err)
		}
	}
}
