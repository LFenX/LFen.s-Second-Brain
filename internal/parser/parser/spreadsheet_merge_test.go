package parser

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/chunk"
)

func TestEmailBodyExcludesUnselectedAttachmentParts(t *testing.T) {
	for _, kind := range []string{"text/plain", "text/html", "multipart/alternative; boundary=nested"} {
		t.Run(kind, func(t *testing.T) {
			payload := "ATTACHMENT_MUST_NOT_BE_BODY"
			if strings.HasPrefix(kind, "multipart/") {
				payload = "--nested\r\nContent-Type: text/plain\r\n\r\n" + payload + "\r\n--nested--\r\n"
			}
			raw := "From: synthetic@example.invalid\r\nContent-Type: multipart/mixed; boundary=outer\r\n\r\n--outer\r\nContent-Type: text/plain\r\n\r\nBODY_REMAINS\r\n--outer\r\nContent-Type: " + kind + "\r\nContent-Disposition: attachment; filename=excluded.txt\r\n\r\n" + payload + "\r\n--outer--\r\n"
			for _, collect := range []bool{false, true} {
				fields := []string{"body"}
				if collect {
					fields = append(fields, "attachments")
				}
				content, err := parseEMLWithError(bytes.NewReader([]byte(raw)), fields)
				if err != nil {
					t.Fatal(err)
				}
				text, _ := content["text"].(string)
				html, _ := content["text_html"].(string)
				if !strings.Contains(text, "BODY_REMAINS") || strings.Contains(text+html, "ATTACHMENT_MUST_NOT_BE_BODY") {
					t.Fatalf("attachment crossed body boundary: %q %q", text, html)
				}
				attachments, _ := content["attachments"].([]map[string]any)
				expected := 0
				if collect {
					expected = 1
				}
				if len(attachments) != expected {
					t.Fatalf("attachment selection got%d want%d", len(attachments), expected)
				}
			}
		})
	}
}

func TestBIFF8ParserPreservesOriginalMergedGeometry(t *testing.T) {
	input, err := base64.StdEncoding.DecodeString(originalMergedBIFF8)
	if err != nil {
		t.Fatal(err)
	}
	parser, err := NewXLSParser("")
	if err != nil {
		t.Fatal(err)
	}
	result := parser.ParseWithResult(t.Context(), "original-merged.xls", input)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	projection, err := ProjectKnowledgeParseResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != "ready" || len(projection.Nodes) != 2 {
		t.Fatal("two typed sheets missing", projection.State, len(projection.Nodes))
	}
	table := projection.Nodes[0].Attributes.Parser.Table
	if table == nil || len(table.Grid) != 4 || len(table.Grid[1]) != 3 || table.Grid[1][1].Text != "跨行" || table.Grid[1][1].RowSpan != 2 || !table.Grid[2][1].Covered || table.Grid[2][1].Text != "" {
		t.Fatal("original BIFF8 merged rectangle changed", table)
	}
	if table.Grid[1][2].Text != "0" || table.Grid[2][2].Text != "false" {
		t.Fatal("zero/false changed")
	}
	plan, err := chunk.PlanContentRanges(chunk.ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: *projection.Nodes[0].Text.Value, Table: table, MaxBytes: 8192})
	if err != nil || len(plan) != 1 {
		t.Fatal("merged output cannot be planned exactly", err)
	}
	if !strings.Contains(*projection.Nodes[1].Text.Value, "SHT-29") {
		t.Fatal("second source sheet missing")
	}
}

func TestSpreadsheetMergesRefuseAmbiguousGeometry(t *testing.T) {
	makeItems := func() []map[string]any {
		return []map[string]any{{"table": &entity.TableData{Grid: [][]entity.TableCell{{{Text: "origin"}, {Text: ""}}, {{Text: ""}, {Text: "body"}}}, HeaderRows: 1}, "positions": [][]float64{{1, 1, 1, 1, 2}, {1, 2, 2, 1, 2}}}}
	}
	if err := applySpreadsheetMerges(makeItems(), []mergeRange{{sr: 1, sc: 1, er: 2, ec: 1}}); err != nil {
		t.Fatal(err)
	}
	for _, spans := range [][]mergeRange{
		{{sr: 1, sc: 1, er: 1, ec: 1}, {sr: 1, sc: 1, er: 1, ec: 1}},
		{{sr: 1, sc: 1, er: 2, ec: 1}, {sr: 2, sc: 1, er: 2, ec: 2}},
		{{sr: 1, sc: 1, er: 3, ec: 1}},
		{{sr: 1, sc: 1, er: 2, ec: 4097}},
		{{sr: 2, sc: 1, er: 1, ec: 1}},
	} {
		if err := applySpreadsheetMerges(makeItems(), spans); err == nil {
			t.Fatal("invalid or incomplete merge accepted", spans)
		}
	}
}

const originalMergedBIFF8 = "0M8R4KGxGuEAAAAAAAAAAAAAAAAAAAAAPgADAP7/CQAGAAAAAAAAAAAAAAABAAAAAgAAAAAAAAAAEAAAAQAAAAEAAAD+////AAAAAAAAAAD////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////9/////v////7///8EAAAABQAAAAYAAAD+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7///8CAAAAAwAAAAQAAAAFAAAABgAAAAcAAAAIAAAACQAAAAoAAAALAAAADAAAAA0AAAAOAAAADwAAABAAAAARAAAAEgAAABMAAAAUAAAAFQAAABYAAAAXAAAAGAAAABkAAAD+////GwAAAP7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+/////v////7////+////UgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQABQH//////////wEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAADAAAAAAcAAAAAAAABAFMAaAAzADMAdABKADUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEgACAf////8CAAAA/////wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAFcAbwByAGsAYgBvAG8AawAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAASAAIB/////wMAAAD/////AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAwGAAAAAAAABQBTAHUAbQBtAGEAcgB5AEkAbgBmAG8AcgBtAGEAdABpAG8AbgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACgAAgH///////////////8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAaAAAAaAAAAAAAAAA3MjYyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACQgQAAAGBQBics0HCcABAAYHAADhAAIAsATBAAIAAADiAAAAXABwAAcAAFNoMzN0SlMAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABCAAIAsARhAQIAAADAAQAAPQEEAAEAAgCcAAIAEQAZAAIAAAASAAIAAAATAAIAAACvAQIAAAC8AQIAAAA9ABIAAAAAAGBywEQ4AAAAAAABAPQBQAACAAAAjQACAAAAIgACAAAADgACAAEAtwECAAAA2gACAAAAMQAaAPAAAAAAAJABAAAAAAAABQFBAHIAaQBhAGwAHgQ1ADgAGAABIgAKTkhTLwALTkhTIAAiAGgAaAAiAEJmIgBtAG0AIgAGUiIAcwBzACIA0nkgACIA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAD0/wAAAAAAAAAAAAAAAAAA4AAUAAAAAAAAAAAAAAAAAAAAAAAAAAAAYAECAAAAhQAOAD8DAAAAAAMBCFQQYmiIhQAOAP0EAAAAAAMBLHuMTmiIjAAEAAEAAQD8AAgAAAAAAAAAAAAKAAAACQgQAAAGEABics0HCcABAAYHAAANAAIAAQAMAAIAZAAPAAIAAQARAAIAAAAQAAgA/Knx0k1iUD9fAAIAAQAqAAIAAAArAAIAAACCAAIAAQCAAAgAAAAAAAAAAACDAAIAAACEAAIAAAAAAg4AAAAAAAQAAAAAAAMAAAAEAgsAAAAAABAAAQABLpUEAgsAAAABABAAAQABxH4EAgsAAAACABAAAQABPFAEAhcAAQAAABAABwABWABMAFMALQA3ADQAMgAEAg0AAQABABAAAgAB6I1MiAMCDgABAAIAEAAAAAAAAAAAAAQCFwACAAAAEAAHAAFYAEwAUwAtADcANAAzAAUCCAACAAIAEAAAAAQCFwADAAAAEAAHAAFYAEwAUwAtADcANAA0AAQCCwADAAEAEAABAAE+XAQCDQADAAIAEAACAAHdT1l1PgISALYGAAAAAEAAAAAAAAAAAAAAAOUACgABAAEAAgABAAEAugEJAAMAAQhUEGJoiGcIEwBnCAAAAAAAAAAAAAADAAEAAAAAaAgnAGgIAAAAAAAAAAAAAAMAAAAAAAABAAQAAAAAAAAAAwAAAAIABAAAAAoAAAAJCBAAAAYQAGJyzQcJwAEABgcAAA0AAgABAAwAAgBkAA8AAgABABEAAgAAABAACAD8qfHSTWJQP18AAgABACoAAgAAACsAAgAAAIIAAgABAIAACAAAAAAAAAAAAIMAAgAAAIQAAgAAAAACDgAAAAAAAgAAAAAAAQAAAAQCDwAAAAAAEAADAAEse4xOaIgEAhUAAQAAABAABgABUwBIAFQALQAyADkAPgISALYGAAAAAEAAAAAAAAAAAAAAALoBCQADAAEse4xOaIhnCBMAZwgAAAAAAAAAAAAAAwABAAAAAGgIJwBoCAAAAAAAAAAAAAADAAAAAAAAAQAEAAAAAAAAAAEAAAAAAAQAAAAKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP7/AAA3MjYyAAAAAAAAAAAAAAAAAAAAAAEAAADghZ/y+U9oEKuRCAArJ7PZMAAAADgAAAACAAAAAQAAABgAAAASAAAAIAAAAAIAAACwBAAAHwAAAAgAAABTAGgAZQBlAHQASgBTAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
