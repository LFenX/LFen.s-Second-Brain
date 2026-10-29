package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
)

// ProjectKnowledgeParseResult only projects supplied typed results. Markdown
// normalization remains at the existing component boundary; this function has no I/O.
func ProjectKnowledgeParseResult(result ParseResult) (*entity.KnowledgeContentProjection, error) {
	p := &entity.KnowledgeContentProjection{
		KnowledgeContentRepresentation: entity.KnowledgeContentRepresentation{
			Version: entity.KnowledgeContentVersion, Binding: "unbound",
			Ref:             entity.KnowledgeContentRepresentationRef{Kind: "parser_result"},
			CoordinateSpace: "item_utf8", State: "empty",
		},
		Nodes: []entity.KnowledgeContentNode{}, Relationships: []entity.KnowledgeContentRelationship{},
	}
	finish := func(state, reason string) (*entity.KnowledgeContentProjection, error) {
		p.State = state
		if reason != "" {
			p.Reason = &reason
		}
		count := int64(len(p.Nodes))
		p.NodeCount = &count
		if err := p.Validate(); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("invalid content projection payload")
		}
		if len(raw) > 32<<20 {
			return nil, fmt.Errorf("content projection capacity exceeded")
		}
		return p, nil
	}
	if result.Err != nil {
		return finish("unavailable", "parser_failed")
	}
	if result.OutputFormat == "" {
		return finish("unavailable", "no_parser_result")
	}
	items := result.JSON
	if len(items) == 0 {
		if result.Markdown != "" {
			return finish("normalization_required", "use_existing_component_normalizer")
		}
		if result.Text != "" {
			items = []DocItem{NewTextJSONItem(result.Text)}
		}
	}
	if len(items) > 32768 {
		return nil, fmt.Errorf("content projection capacity exceeded")
	}
	supported, unavailable, available, textBytes := 0, 0, 0, 0
	for i, item := range items {
		index := int64(i)
		attrs := &entity.KnowledgeContentParserAttributes{ItemType: "opaque"}
		node := entity.KnowledgeContentNode{
			Ref:  entity.KnowledgeContentNodeRef{Kind: "parser_item", ItemIndex: &index},
			Kind: "opaque", Ordinal: &index,
			Locator:             entity.KnowledgeContentLocator{Kind: "parser_item", ItemIndex: &index},
			CoordinatePrecision: "item_only",
			Text:                parserContentUnavailable("uninterpreted_doc_item"),
			Attributes:          entity.KnowledgeContentAttributes{Parser: attrs},
		}
		var err error
		switch value := item.(type) {
		case TextItem:
			err = parserProjectText(&node, value)
		case *TextItem:
			if value == nil {
				return nil, fmt.Errorf("nil parser text item")
			}
			err = parserProjectText(&node, *value)
		case TableItem:
			err = parserProjectTable(&node, value)
		case *TableItem:
			if value == nil {
				return nil, fmt.Errorf("nil parser table item")
			}
			err = parserProjectTable(&node, *value)
		case ImageItem:
			err = parserProjectImage(&node, value)
		case *ImageItem:
			if value == nil {
				return nil, fmt.Errorf("nil parser image item")
			}
			err = parserProjectImage(&node, *value)
		case RawItem, *RawItem:
			node.Text = parserContentUnavailable("uninterpreted_raw_item")
		case nil:
			return nil, fmt.Errorf("nil parser item")
		}
		if err != nil {
			return nil, err
		}
		if node.Text.Value != nil {
			textBytes += len(*node.Text.Value)
		}
		if textBytes > 16<<20 {
			return nil, fmt.Errorf("content projection capacity exceeded")
		}
		if node.Text.Status == "available" || node.Text.Status == "empty" {
			supported++
			if node.Text.Status == "available" {
				available++
			}
		} else {
			unavailable++
		}
		p.Nodes = append(p.Nodes, node)
	}
	if unavailable > 0 {
		if supported > 0 {
			return finish("partial", "some_items_have_no_extractable_text")
		}
		return finish("unavailable", "no_extractable_items")
	}
	if supported > 0 {
		if available == 0 {
			return finish("empty", "")
		}
		return finish("ready", "")
	}
	return finish("unavailable", "no_explicit_content_items")
}

func parserContentUnavailable(reason string) entity.KnowledgeContentText {
	return entity.KnowledgeContentText{CoordinateSpace: "item_utf8", Status: "unavailable", Reason: &reason}
}

func parserContentText(value string) entity.KnowledgeContentText {
	digest := sha256.Sum256([]byte(value))
	hash := hex.EncodeToString(digest[:])
	status := "available"
	if value == "" {
		status = "empty"
	}
	return entity.KnowledgeContentText{
		CoordinateSpace: "item_utf8", Status: status, Value: &value,
		Range: &entity.KnowledgeContentRange{StartByte: 0, EndByte: int64(len(value)), SHA256: &hash},
	}
}

func parserProjectText(node *entity.KnowledgeContentNode, item TextItem) error {
	if len(item.CKType) > 128 {
		return fmt.Errorf("content metadata capacity exceeded")
	}
	if !utf8.ValidString(item.Text) || !utf8.ValidString(item.CKType) {
		return fmt.Errorf("invalid parser text UTF-8")
	}
	node.Kind = "text_block"
	switch item.CKType {
	case "heading", "list", "code":
		node.Kind = item.CKType
	}
	node.Text = parserContentText(item.Text)
	node.Attributes.Parser.ItemType, node.Attributes.Parser.CKType = "text", item.CKType
	return nil
}

func parserContentPositions(input [][]float64) ([][]float64, error) {
	if input == nil {
		return nil, nil
	}
	if len(input) > 4096 {
		return nil, fmt.Errorf("content positions capacity exceeded")
	}
	out := make([][]float64, len(input))
	for i, row := range input {
		if len(row) > 16 {
			return nil, fmt.Errorf("content positions capacity exceeded")
		}
		out[i] = append([]float64(nil), row...)
	}
	return out, nil
}

func parserProjectTable(node *entity.KnowledgeContentNode, item TableItem) error {
	if len(item.CKType) > 128 || len(item.Sheet) > 4096 {
		return fmt.Errorf("content metadata capacity exceeded")
	}
	if !utf8.ValidString(item.Sheet) || !utf8.ValidString(item.CKType) {
		return fmt.Errorf("invalid parser table UTF-8")
	}
	node.Kind = "table"
	attrs := node.Attributes.Parser
	attrs.ItemType, attrs.CKType, attrs.Sheet = "table", item.CKType, item.Sheet
	positions, err := parserContentPositions(item.Positions)
	if err != nil {
		return err
	}
	attrs.Positions = positions
	if item.Table == nil {
		node.Text = parserContentUnavailable("missing_table_data")
		return nil
	}
	if len(item.Table.Caption) > 65536 {
		return fmt.Errorf("content metadata capacity exceeded")
	}
	if !utf8.ValidString(item.Table.Caption) {
		return fmt.Errorf("invalid parser table caption UTF-8")
	}
	table := *item.Table
	table.Grid = make([][]entity.TableCell, len(item.Table.Grid))
	if len(table.Grid) > 32768 {
		return fmt.Errorf("content table capacity exceeded")
	}
	rows := make([]string, 0, len(table.Grid))
	cells, textBytes := 0, 0
	for i, row := range item.Table.Grid {
		table.Grid[i] = append([]entity.TableCell(nil), row...)
		values := make([]string, 0, len(row))
		for _, cell := range row {
			cells++
			if !utf8.ValidString(cell.Text) {
				return fmt.Errorf("invalid parser table cell UTF-8")
			}
			if !cell.Covered {
				values = append(values, cell.Text)
				textBytes += len(cell.Text) + 1
			}
		}
		if cells > 1048576 || textBytes > 16<<20 {
			return fmt.Errorf("content table capacity exceeded")
		}
		rows = append(rows, strings.Join(values, "\t"))
	}
	attrs.Table = &table
	assembly := "row_major_uncovered_cells_tab_lf_v1"
	attrs.TextAssembly = &assembly
	node.Text = parserContentText(strings.Join(rows, "\n"))
	return nil
}

func parserProjectImage(node *entity.KnowledgeContentNode, item ImageItem) error {
	if len(item.Text) > 65536 || len(item.Sheet) > 4096 || len(item.CKType) > 128 {
		return fmt.Errorf("content metadata capacity exceeded")
	}
	if !utf8.ValidString(item.Text) || !utf8.ValidString(item.Sheet) || !utf8.ValidString(item.CKType) {
		return fmt.Errorf("invalid parser image metadata UTF-8")
	}
	node.Kind = "media"
	attrs := node.Attributes.Parser
	alt := item.Text
	attrs.ItemType, attrs.CKType, attrs.Sheet = "image", item.CKType, item.Sheet
	attrs.AltText, attrs.PayloadPresent = &alt, item.Image != ""
	positions, err := parserContentPositions(item.Positions)
	if err != nil {
		return err
	}
	attrs.Positions = positions
	reason := "media_not_ocr"
	node.Text = entity.KnowledgeContentText{CoordinateSpace: "item_utf8", Status: "unsupported", Reason: &reason}
	return nil
}
