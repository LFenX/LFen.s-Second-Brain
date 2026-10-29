package chunk

import (
	"runtime"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestPlanContentRangesPreservesUTF8CRLFAndWhitespace(t *testing.T) {
	text := "甲\r\n 乙\nA"
	ranges, err := PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "derived_utf8", Text: text, MaxBytes: 5})
	if err != nil || len(ranges) != 3 {
		t.Fatal(ranges, err)
	}
	var got strings.Builder
	for _, r := range ranges {
		got.WriteString(text[r.StartByte:r.EndByte])
		if r.EndByte-r.StartByte > 5 || r.SHA256 == "" {
			t.Fatal(r)
		}
	}
	if got.String() != text || ranges[0].EndByte != 5 || ranges[1].StartByte != 5 || ranges[1].EndByte != 10 {
		t.Fatal("bytes changed", ranges)
	}
	ranges, err = PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "item_utf8", Text: "甲乙丙", MaxBytes: 4})
	if err != nil || len(ranges) != 3 {
		t.Fatal(ranges, err)
	}
}

func TestPlanContentRangesDoesNotSplitCRLF(t *testing.T) {
	text := "abc\r\n"
	ranges, err := PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "derived_utf8", Text: text, MaxBytes: 4})
	if err != nil || len(ranges) != 2 || ranges[0].EndByte != 3 || ranges[1].StartByte != 3 || ranges[1].EndByte != 5 {
		t.Fatal(ranges, err)
	}
	if result, err := PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "item_utf8", Text: "\r\n", MaxBytes: 1}); err == nil || result != nil {
		t.Fatal("split CRLF to fit", result, err)
	}
}

func TestPlanContentRangesBoundsIntermediateAllocation(t *testing.T) {
	text := strings.Repeat("\n", 4<<20)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	ranges, err := PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "derived_utf8", Text: text, MaxBytes: 8192})
	runtime.ReadMemStats(&after)
	if err != nil || len(ranges) != 512 {
		t.Fatal(len(ranges), err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Fatalf("intermediate allocation %d exceeds bounded plan", allocated)
	}
	if result, err := PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "derived_utf8", Text: text, MaxBytes: 1}); err == nil || result != nil {
		t.Fatal("unbounded range count", len(result), err)
	}
}

func TestPlanContentRangesKeepsMergedTableRowsAtomic(t *testing.T) {
	table := &entity.TableData{HeaderRows: 1, Grid: [][]entity.TableCell{
		{{Text: "A"}, {Text: "B"}},
		{{Text: "甲", RowSpan: 2}, {Text: "乙"}},
		{{Covered: true, OriginRow: 1, OriginCol: 0}, {Text: "丙"}},
	}}
	text := "A\tB\n甲\t乙\n丙"
	ranges, err := PlanContentRanges(ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: text, Table: table, MaxBytes: 11})
	if err != nil || len(ranges) != 2 {
		t.Fatal(ranges, err)
	}
	if ranges[0].StartByte != 0 || ranges[0].EndByte != 4 || *ranges[0].FirstRow != 0 || *ranges[0].EndRow != 1 || ranges[1].StartByte != 4 || ranges[1].EndByte != 15 || *ranges[1].FirstRow != 1 || *ranges[1].EndRow != 3 {
		t.Fatal(ranges)
	}
	if result, err := PlanContentRanges(ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: text, Table: table, MaxBytes: 10}); err == nil || result != nil {
		t.Fatal("split merged rows", result, err)
	}
	if result, err := PlanContentRanges(ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: "forged", Table: table, MaxBytes: 16}); err == nil || result != nil {
		t.Fatal("accepted wrong table assembly", result, err)
	}
}

func TestPlanContentRangesRejectsFalseGeometryAndAtomicOversize(t *testing.T) {
	for _, kind := range []string{"heading", "code", "list"} {
		if ranges, err := PlanContentRanges(ContentRangeInput{Kind: kind, CoordinateSpace: "derived_utf8", Text: "abcdef", MaxBytes: 5}); err == nil || ranges != nil {
			t.Fatal(kind, ranges, err)
		}
	}
	bad := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "lost", Covered: true}}}}
	if ranges, err := PlanContentRanges(ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: "", Table: bad, MaxBytes: 8}); err == nil || ranges != nil {
		t.Fatal(ranges, err)
	}
	if ranges, err := PlanContentRanges(ContentRangeInput{Kind: "text_block", CoordinateSpace: "pdf_page", Text: "a", MaxBytes: 8}); err == nil || ranges != nil {
		t.Fatal(ranges, err)
	}
}
