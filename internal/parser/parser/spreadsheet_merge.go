package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/richardlehane/mscfb"
)

// biff8SheetMergeRanges reads only the existing workbook's merge metadata;
// cell values remain owned by go-excel-reader. MS-XLS 2.4.168 and 2.5.208 define
// the MergeCells/Ref8 records, including their count and non-overlap rules.
func biff8SheetMergeRanges(input []byte) ([][]mergeRange, error) {
	compound, err := mscfb.New(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("open BIFF merge metadata: %w", err)
	}
	var workbook []byte
	for {
		entry, err := compound.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read BIFF compound metadata: %w", err)
		}
		if entry.Name != "Workbook" && entry.Name != "Book" {
			continue
		}
		if workbook != nil {
			return nil, fmt.Errorf("duplicate BIFF workbook stream")
		}
		workbook, err = io.ReadAll(io.LimitReader(entry, (32<<20)+1))
		if err != nil || len(workbook) > 32<<20 {
			return nil, fmt.Errorf("BIFF merge metadata budget exceeded")
		}
	}
	if len(workbook) < 8 {
		return nil, fmt.Errorf("BIFF workbook merge stream unavailable")
	}
	record := func(offset int) (uint16, []byte, int, error) {
		if offset < 0 || offset > len(workbook)-4 {
			return 0, nil, offset, io.ErrUnexpectedEOF
		}
		kind := binary.LittleEndian.Uint16(workbook[offset:])
		size := int(binary.LittleEndian.Uint16(workbook[offset+2:]))
		if size > 8224 || size > len(workbook)-offset-4 {
			return 0, nil, offset, fmt.Errorf("invalid BIFF record size")
		}
		return kind, workbook[offset+4 : offset+4+size], offset + 4 + size, nil
	}
	var offsets []int
	for pos := 0; ; {
		kind, payload, next, err := record(pos)
		if err != nil {
			return nil, err
		}
		pos = next
		if kind == 0x000a {
			break
		}
		if kind == 0x0085 {
			if len(payload) < 8 {
				return nil, fmt.Errorf("truncated BIFF sheet metadata")
			}
			if payload[5] == 0 {
				offset := uint64(binary.LittleEndian.Uint32(payload))
				if offset > uint64(len(workbook)-4) {
					return nil, fmt.Errorf("invalid BIFF sheet offset")
				}
				offsets = append(offsets, int(offset))
				if len(offsets) > 4096 {
					return nil, fmt.Errorf("BIFF sheet metadata budget exceeded")
				}
			}
		}
	}
	out := make([][]mergeRange, len(offsets))
	total := 0
	for index, offset := range offsets {
		depth := 0
		for pos := offset; ; {
			kind, payload, next, err := record(pos)
			if err != nil {
				return nil, err
			}
			pos = next
			if depth == 0 && kind != 0x0809 {
				return nil, fmt.Errorf("BIFF worksheet BOF missing")
			}
			if kind == 0x0809 {
				depth++
				continue
			}
			if kind == 0x000a {
				depth--
				if depth == 0 {
					break
				}
				continue
			}
			if kind != 0x00e5 || depth != 1 {
				continue
			}
			if len(payload) < 2 {
				return nil, fmt.Errorf("truncated BIFF MergeCells count")
			}
			count := int(binary.LittleEndian.Uint16(payload))
			if count > 1026 || len(payload) != 2+8*count {
				return nil, fmt.Errorf("invalid BIFF MergeCells count")
			}
			total += count
			if total > 32768 {
				return nil, fmt.Errorf("BIFF merged-cell count budget exceeded")
			}
			for n := 0; n < count; n++ {
				p := payload[2+8*n:]
				span := mergeRange{sr: int(binary.LittleEndian.Uint16(p)) + 1, er: int(binary.LittleEndian.Uint16(p[2:])) + 1, sc: int(binary.LittleEndian.Uint16(p[4:])) + 1, ec: int(binary.LittleEndian.Uint16(p[6:])) + 1}
				if span.sr > span.er || span.sc > span.ec || span.ec > 256 {
					return nil, fmt.Errorf("invalid BIFF merged-cell rectangle")
				}
				out[index] = append(out[index], span)
			}
		}
	}
	return out, nil
}
