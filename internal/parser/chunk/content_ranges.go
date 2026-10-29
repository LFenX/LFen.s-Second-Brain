package chunk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
)

const ContentRangePlanVersion = "typed_content_ranges/0.2.0"

type ContentRangeInput struct {
	Kind            string
	CoordinateSpace string
	Text            string
	Table           *entity.TableData
	MaxBytes        int64
}

type ContentRange struct {
	CoordinateSpace string `json:"coordinate_space"`
	StartByte       int64  `json:"start_byte"`
	EndByte         int64  `json:"end_byte"`
	SHA256          string `json:"sha256"`
	FirstRow        *int64 `json:"first_row"`
	EndRow          *int64 `json:"end_row"`
}

// PlanContentRanges preserves the supplied UTF-8 bytes. It does not run the
// preprocess or text-rewriting split stages and never infers original positions.
func PlanContentRanges(input ContentRangeInput) ([]ContentRange, error) {
	if input.MaxBytes < 1 || input.MaxBytes > 65536 || len(input.Text) > 4<<20 || !utf8.ValidString(input.Text) {
		return nil, fmt.Errorf("invalid bounded content range input")
	}
	if input.CoordinateSpace != "item_utf8" && input.CoordinateSpace != "derived_utf8" && input.CoordinateSpace != "source_utf8" {
		return nil, fmt.Errorf("unknown content coordinate space")
	}
	out := []ContentRange{}
	emit := func(group ContentRange) error {
		if group.EndByte-group.StartByte > input.MaxBytes {
			return fmt.Errorf("unit_oversize")
		}
		if len(out) > 0 && group.EndByte-out[len(out)-1].StartByte <= input.MaxBytes {
			out[len(out)-1].EndByte = group.EndByte
			out[len(out)-1].EndRow = group.EndRow
			return nil
		}
		if len(out) >= 32768 {
			return fmt.Errorf("range_capacity_exceeded")
		}
		out = append(out, group)
		return nil
	}
	if input.Kind == "table" {
		groups, err := tableContentGroups(input)
		if err != nil {
			return nil, err
		}
		for _, group := range groups {
			if err := emit(group); err != nil {
				return nil, err
			}
		}
	} else if input.Kind == "heading" {
		if int64(len(input.Text)) > input.MaxBytes {
			return nil, fmt.Errorf("unit_oversize")
		}
		if input.Text != "" {
			if err := emit(ContentRange{StartByte: 0, EndByte: int64(len(input.Text))}); err != nil {
				return nil, err
			}
		}
	} else {
		for start := 0; start < len(input.Text); {
			end := len(input.Text)
			if i := strings.IndexByte(input.Text[start:], '\n'); i >= 0 {
				end = start + i + 1
			}
			if int64(end-start) > input.MaxBytes {
				if input.Kind == "code" || input.Kind == "list" {
					return nil, fmt.Errorf("unit_oversize")
				}
				end = start + int(input.MaxBytes)
				for end > start && !utf8.RuneStart(input.Text[end]) {
					end--
				}
				if end > start && end < len(input.Text) && input.Text[end-1] == '\r' && input.Text[end] == '\n' {
					end--
				}
				if end == start {
					return nil, fmt.Errorf("unit_oversize")
				}
			}
			if err := emit(ContentRange{StartByte: int64(start), EndByte: int64(end)}); err != nil {
				return nil, err
			}
			start = end
		}
	}
	var cursor int64
	for i := range out {
		r := &out[i]
		if r.StartByte != cursor || r.EndByte <= r.StartByte || r.EndByte > int64(len(input.Text)) {
			return nil, fmt.Errorf("content range coverage mismatch")
		}
		digest := sha256.Sum256([]byte(input.Text[r.StartByte:r.EndByte]))
		r.SHA256, r.CoordinateSpace = hex.EncodeToString(digest[:]), input.CoordinateSpace
		cursor = r.EndByte
	}
	if cursor != int64(len(input.Text)) {
		return nil, fmt.Errorf("content range coverage mismatch")
	}
	return out, nil
}

func contentRowIndex(value int) *int64 { v := int64(value); return &v }

func tableContentGroups(input ContentRangeInput) ([]ContentRange, error) {
	table := input.Table
	if table == nil || table.HeaderRows < 0 || table.HeaderRows > len(table.Grid) || len(table.Grid) > 32768 {
		return nil, fmt.Errorf("invalid table geometry")
	}
	rows := make([]string, len(table.Grid))
	covered := map[[2]int][2]int{}
	spans := make([]int, len(table.Grid))
	cells := 0
	for r, row := range table.Grid {
		values := make([]string, 0, len(row))
		spans[r] = r + 1
		for c, cell := range row {
			cells++
			if cells > 1048576 || !utf8.ValidString(cell.Text) || cell.RowSpan < 0 || cell.ColSpan < 0 {
				return nil, fmt.Errorf("invalid table geometry")
			}
			if cell.Covered {
				_, exists := covered[[2]int{r, c}]
				if cell.Text != "" || !exists {
					return nil, fmt.Errorf("invalid covered cell")
				}
				continue
			}
			if _, exists := covered[[2]int{r, c}]; exists {
				return nil, fmt.Errorf("overlapping table cells")
			}
			values = append(values, cell.Text)
			height, width := max(1, cell.RowSpan), max(1, cell.ColSpan)
			if height > len(table.Grid)-r || width > len(row)-c {
				return nil, fmt.Errorf("table span exceeds grid")
			}
			spans[r] = max(spans[r], r+height)
			for rr := r; rr < r+height; rr++ {
				if c+width > len(table.Grid[rr]) {
					return nil, fmt.Errorf("table span exceeds grid")
				}
				for cc := c; cc < c+width; cc++ {
					if rr == r && cc == c {
						continue
					}
					key := [2]int{rr, cc}
					if _, exists := covered[key]; exists {
						return nil, fmt.Errorf("overlapping table spans")
					}
					covered[key] = [2]int{r, c}
				}
			}
		}
		rows[r] = strings.Join(values, "\t")
	}
	if strings.Join(rows, "\n") != input.Text {
		return nil, fmt.Errorf("table text assembly mismatch")
	}
	if input.Text == "" {
		return []ContentRange{}, nil
	}
	starts := make([]int64, len(rows)+1)
	for i, row := range rows {
		starts[i+1] = starts[i] + int64(len(row))
		if i+1 < len(rows) {
			starts[i+1]++
		}
	}
	groups := []ContentRange{}
	for first := 0; first < len(rows); {
		end := first + 1
		if first == 0 {
			end = max(end, table.HeaderRows)
		}
		for r := first; r < end; r++ {
			end = max(end, spans[r])
		}
		group := ContentRange{StartByte: starts[first], EndByte: starts[end], FirstRow: contentRowIndex(first), EndRow: contentRowIndex(end)}
		if group.StartByte == group.EndByte {
			if len(groups) > 0 {
				groups[len(groups)-1].EndRow = group.EndRow
			}
		} else {
			groups = append(groups, group)
		}
		first = end
	}
	return groups, nil
}
