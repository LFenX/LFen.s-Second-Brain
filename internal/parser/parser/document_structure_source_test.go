package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
)

type documentDeniedTransport struct{ calls *atomic.Int64 }

func (d documentDeniedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return nil, fmt.Errorf("HTTP denied in fixed synthetic source test")
}

func forbidDocumentAnalysis(t *testing.T) {
	t.Helper()
	var httpCalls, analyzerCalls atomic.Int64
	previousTransport, previousFactory := http.DefaultTransport, deepdoctype.NativeDocAnalyzerFactory
	http.DefaultTransport = documentDeniedTransport{calls: &httpCalls}
	deepdoctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		analyzerCalls.Add(1)
		panic("model analyzer creation denied in fixed synthetic source test")
	})
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
		deepdoctype.SetNativeDocAnalyzerFactory(previousFactory)
		t.Logf("R2_SENTINELS http_attempts=%d analyzer_creations=%d", httpCalls.Load(), analyzerCalls.Load())
		if httpCalls.Load() != 0 || analyzerCalls.Load() != 0 {
			t.Errorf("HTTP attempts=%d analyzer creations=%d, require0", httpCalls.Load(), analyzerCalls.Load())
		}
	})
}

// This focused input suite uses the immutable ignored source matrix. The
// coordinating owner supplies its copied stage3 fixture root explicitly.
func documentSourceFixture(t *testing.T, relative, expected string) []byte {
	t.Helper()
	root := os.Getenv("LFEN_R2_STAGE3_ROOT")
	if root == "" {
		t.Skip("private source-matrix fixtures: set LFEN_R2_STAGE3_ROOT for the authorized focused run")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("fixture root must be absolute")
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	if expected != "" && documentHash(data) != expected {
		t.Fatalf("immutable source digest mismatch: %s", relative)
	}
	return data
}

func buildSourceResult(t *testing.T, format string, data []byte) DocumentStructureResult {
	t.Helper()
	result, err := BuildDocumentStructure(t.Context(), format, data)
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("LFEN_R2_PROJECTION_EVIDENCE"); dir != "" {
		if !filepath.IsAbs(dir) {
			t.Fatal("private evidence directory must be absolute")
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		record := map[string]any{"source_sha256": documentHash(data), "format": format, "reading_bytes": len(result.ReadingText), "reading_text_sha256": documentHash([]byte(result.ReadingText)), "result": result, "scope": "Fixed synthetic source projection only; no application publication or final identities"}
		encoded, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(filepath.Join(dir, format+"-"+documentHash(data)[:12]+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write(append(encoded, '\n'))
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("save source projection evidence: %v %v", writeErr, closeErr)
		}
	}
	assertDocumentRanges(t, result)
	return result
}

func documentNodeText(result DocumentStructureResult, node entity.KnowledgeDocumentNodePayload, own bool) string {
	rangeValue := node.ReadingRange
	if own {
		if node.OwnReadingRange == nil {
			return ""
		}
		rangeValue = *node.OwnReadingRange
	}
	return result.ReadingText[rangeValue.StartByte:rangeValue.EndByte]
}

func documentNodes(result DocumentStructureResult, kind string) []entity.KnowledgeDocumentNodePayload {
	nodes := []entity.KnowledgeDocumentNodePayload{}
	for _, node := range result.Payloads {
		if node.Kind == kind {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func documentOwnNode(t *testing.T, result DocumentStructureResult, text string) entity.KnowledgeDocumentNodePayload {
	t.Helper()
	for _, node := range result.Payloads {
		if node.Kind != "passage" && node.OwnReadingRange != nil && documentNodeText(result, node, true) == text {
			return node
		}
	}
	t.Fatalf("source own text missing: %q", text)
	return entity.KnowledgeDocumentNodePayload{}
}

func documentChildren(result DocumentStructureResult, parent entity.KnowledgeDocumentNodePayload, kind string) []entity.KnowledgeDocumentNodePayload {
	resultNodes := []entity.KnowledgeDocumentNodePayload{}
	for _, node := range result.Payloads {
		if node.ParentRef != nil && node.ParentRef.Kind == parent.Kind && reflect.DeepEqual(node.ParentRef.Locator, parent.Locator) && (kind == "" || node.Kind == kind) {
			resultNodes = append(resultNodes, node)
		}
	}
	return resultNodes
}

func documentGrid(t *testing.T, result DocumentStructureResult) [][]string {
	t.Helper()
	tables := documentNodes(result, "table")
	if len(tables) != 1 {
		t.Fatalf("table count=%d", len(tables))
	}
	grid := [][]string{}
	for _, row := range documentChildren(result, tables[0], "row") {
		values := []string{}
		for _, cell := range documentChildren(result, row, "cell") {
			values = append(values, documentNodeText(result, cell, false))
			var attrs entity.KnowledgeDocumentCellAttributes
			if err := json.Unmarshal(cell.Attributes, &attrs); err != nil {
				t.Fatal(err)
			}
			if attrs.RowSpan != nil || attrs.ColumnSpan != nil || attrs.IsHeader != nil {
				t.Fatal("unproven cell layout must be null")
			}
		}
		grid = append(grid, values)
	}
	return grid
}

func assertDocumentRanges(t *testing.T, result DocumentStructureResult) {
	t.Helper()
	if len(result.Payloads) < 1 || result.Payloads[0].Kind != "document" || result.Payloads[0].ParentRef != nil {
		t.Fatal("document root missing")
	}
	for _, node := range result.Payloads {
		r := node.ReadingRange
		if r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > int64(len(result.ReadingText)) || !utf8.ValidString(result.ReadingText[r.StartByte:r.EndByte]) || documentHash([]byte(result.ReadingText[r.StartByte:r.EndByte])) != r.TextSHA256 {
			t.Fatal("invalid aggregate range")
		}
		if node.OwnReadingRange != nil {
			own := *node.OwnReadingRange
			if own.StartByte < r.StartByte || own.EndByte > r.EndByte || own.EndByte < own.StartByte || documentHash([]byte(result.ReadingText[own.StartByte:own.EndByte])) != own.TextSHA256 {
				t.Fatal("invalid own range")
			}
		}
		if _, err := json.Marshal(node); err != nil {
			t.Fatalf("frozen payload schema: %v", err)
		}
	}
	passages := documentNodes(result, "passage")
	sort.Slice(passages, func(i, j int) bool { return passages[i].ReadingRange.StartByte < passages[j].ReadingRange.StartByte })
	position := int64(0)
	for _, node := range passages {
		if node.ReadingRange.StartByte != position || node.ReadingRange.EndByte <= position || node.ReadingRange.EndByte-position > 8192 {
			t.Fatal("passage gap/overlap/empty/limit")
		}
		position = node.ReadingRange.EndByte
	}
	if position != int64(len(result.ReadingText)) {
		t.Fatal("passages do not cover exact reading bytes")
	}
}

func TestDocumentStructureSourceFacts(t *testing.T) {
	forbidDocumentAnalysis(t)
	base := "rich-document-candidate-draft/"
	matrix := documentSourceFixture(t, base+"source-fact-matrix-v1.json", "c7533ffb28660fdf5a743ccdac422d5be0539443c30ab8789d236807e9ff8016")
	var input struct {
		Cases []struct {
			ID string `json:"id"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(matrix, &input); err != nil {
		t.Fatal(err)
	}
	if len(input.Cases) != 28 {
		t.Fatal("fixed matrix must have28 cases")
	}
	v1docBytes := documentSourceFixture(t, base+"fixtures-v1/synthetic-outline-table-list-image.docx", "e78dcf5e61c432d100711a182fc67fcd79850cc185bbc097a688ec720790f6e5")
	v1pptBytes := documentSourceFixture(t, base+"fixtures-v1/synthetic-slides-table-image.pptx", "8b1ded9d9d0a588f99a5160dfab1ab9299b7eb1c5a9bd04f24dba3a81fa2519f")
	v2docBytes := documentSourceFixture(t, base+"source-fixtures-v2/synthetic-docx-source-semantics-v2.docx", "c07bd44fcb7475969bba31c3573d92333f032946dcd8e385d726e2bd08181b4e")
	v2pptBytes := documentSourceFixture(t, base+"source-fixtures-v2/synthetic-pptx-source-order-v2.pptx", "2bcaf2927b6394a9b6adb81e7e568eaa796a7e18166d0882ed879296b807b1a4")
	v1doc := buildSourceResult(t, "docx", v1docBytes)
	v1ppt := buildSourceResult(t, "pptx", v1pptBytes)
	v2doc := buildSourceResult(t, "docx", v2docBytes)
	v2ppt := buildSourceResult(t, "pptx", v2pptBytes)
	cases := map[string]func(*testing.T){
		"RD10": func(t *testing.T) {
			if !reflect.DeepEqual(documentGrid(t, v1ppt), [][]string{{"header", ""}, {"a", "b"}}) {
				t.Fatal("PPTX source cells not retained")
			}
		},
		"RD11": func(t *testing.T) {
			media := documentNodes(v1ppt, "media")
			if len(media) != 1 {
				t.Fatal("source PNG missing")
			}
			a, b, c := strings.Index(v1ppt.ReadingText, "Slide One Title"), strings.Index(v1ppt.ReadingText, "Alpha body"), strings.Index(v1ppt.ReadingText, "header")
			if !(a < b && b < c && int64(c) < media[0].ReadingRange.StartByte) {
				t.Fatal("source title/body/table/media order changed")
			}
		},
		"RD13": func(t *testing.T) {
			for text, expected := range map[string]int64{"第一章 概述": 1, "1.1 背景": 2} {
				node := documentOwnNode(t, v1doc, text)
				var attrs entity.KnowledgeDocumentHeadingAttributes
				_ = json.Unmarshal(node.Attributes, &attrs)
				if node.Kind != "heading" || attrs.Level == nil || *attrs.Level != expected {
					t.Fatal("source outline level lost")
				}
			}
		},
		"RD14": func(t *testing.T) {
			title := documentOwnNode(t, v1doc, "Title")
			hello := documentOwnNode(t, v1doc, "Hello")
			if title.OwnReadingRange.EndByte >= hello.OwnReadingRange.StartByte || hello.Kind != "paragraph" {
				t.Fatal("separate Title/Hello paragraphs lost")
			}
		},
		"RD15": func(t *testing.T) {
			if !reflect.DeepEqual(documentGrid(t, v1doc), [][]string{{"cell", ""}, {"a", "b"}}) {
				t.Fatal("DOCX source cell positions lost")
			}
		},
		"RD16": func(t *testing.T) {
			lists := documentNodes(v1doc, "list")
			levels := []int64{}
			for _, node := range lists {
				var attrs entity.KnowledgeDocumentListAttributes
				_ = json.Unmarshal(node.Attributes, &attrs)
				if attrs.SourceLevel != nil {
					levels = append(levels, *attrs.SourceLevel)
				}
				if attrs.Ordered == nil || *attrs.Ordered {
					t.Fatal("explicit bullet lost")
				}
			}
			if !reflect.DeepEqual(levels, []int64{0, 1}) {
				t.Fatal("source list ilvl lost")
			}
		},
		"RD17": func(t *testing.T) {
			paragraph := documentOwnNode(t, v1doc, "beforeafter")
			media := documentChildren(v1doc, paragraph, "media")
			if len(media) != 1 {
				t.Fatal("inline media is not a paragraph child")
			}
			var attrs entity.KnowledgeDocumentMediaAttributes
			_ = json.Unmarshal(media[0].Attributes, &attrs)
			if attrs.InlineAnchorByte == nil || *attrs.InlineAnchorByte != paragraph.OwnReadingRange.StartByte+6 {
				t.Fatal("source inline before/image/after anchor lost")
			}
		},
		"RD19": func(t *testing.T) {
			node := documentOwnNode(t, v1doc, "<SEP>")
			if node.Kind != "paragraph" || strings.Contains(v1doc.ReadingText, "&lt;SEP&gt;") {
				t.Fatal("literal source entity changed")
			}
		},
		"RD20": func(t *testing.T) {
			node := documentOwnNode(t, v1doc, "a—b")
			if node.OwnReadingRange.EndByte-node.OwnReadingRange.StartByte != 5 || len([]rune(documentNodeText(v1doc, node, true))) != 3 {
				t.Fatal("UTF8 byte/scalar counts changed")
			}
		},
		"R2S01": func(t *testing.T) {
			node := documentOwnNode(t, v2doc, "Inherited outline title")
			var attrs entity.KnowledgeDocumentHeadingAttributes
			_ = json.Unmarshal(node.Attributes, &attrs)
			if node.Kind != "heading" || attrs.Level == nil || *attrs.Level != 2 {
				t.Fatal("basedOn outline not resolved")
			}
			trap := documentOwnNode(t, v2doc, "Heading name is not outline evidence")
			if trap.Kind != "paragraph" {
				t.Fatal("style name guessed heading")
			}
		},
		"R2S02": func(t *testing.T) {
			if !strings.Contains(v2doc.ReadingText, "A\tB\nC\nD\n\nTail") {
				t.Fatal("explicit tabs/breaks/empty paragraph lost")
			}
			documentOwnNode(t, v2doc, "A\tB\nC\nD")
		},
		"R2S03": func(t *testing.T) {
			foundJump, foundStart := false, false
			for _, node := range documentNodes(v2doc, "list") {
				var a entity.KnowledgeDocumentListAttributes
				_ = json.Unmarshal(node.Attributes, &a)
				if a.NumberingID != nil && *a.NumberingID == "42" && a.SourceLevel != nil && *a.SourceLevel == 2 {
					foundJump = true
				}
				if a.NumberingID != nil && *a.NumberingID == "43" && a.StartNumber != nil && *a.StartNumber == 5 && a.NumberFormat != nil && *a.NumberFormat == "decimal" {
					foundStart = true
				}
				if a.SourceLevel != nil && *a.SourceLevel == 1 {
					t.Fatal("invented skipped ilvl1")
				}
			}
			if !foundJump || !foundStart {
				t.Fatal("source numbering properties lost")
			}
		},
		"R2S04": func(t *testing.T) {
			node := documentOwnNode(t, v2doc, "前后")
			media := documentChildren(v2doc, node, "media")
			if len(media) != 1 {
				t.Fatal("multibyte inline media missing")
			}
			var a entity.KnowledgeDocumentMediaAttributes
			_ = json.Unmarshal(media[0].Attributes, &a)
			if a.InlineAnchorByte == nil || *a.InlineAnchorByte != node.OwnReadingRange.StartByte+3 {
				t.Fatal("absolute UTF8 anchor mismatch")
			}
		},
		"R2S05": func(t *testing.T) {
			if strings.Contains(v2doc.ReadingText, "HIDDEN-SYNTHETIC") || strings.Contains(v2doc.ReadingText, "visible-prefix") {
				t.Fatal("hidden paragraph was partly extracted")
			}
			assertUnsupportedType(t, v2doc, "vanish")
		},
		"R2S06": func(t *testing.T) {
			for _, name := range []string{"ins", "del", "moveFrom", "moveTo"} {
				assertUnsupportedType(t, v2doc, name)
			}
			for _, text := range []string{"INSERTED", "DELETED", "MOVED-FROM", "MOVED-TO"} {
				if strings.Contains(v2doc.ReadingText, text) {
					t.Fatal("revision body leaked into reading text")
				}
			}
		},
		"R2S07": func(t *testing.T) {
			assertUnsupportedType(t, v2doc, "fldChar")
			assertUnsupportedType(t, v2doc, "fldSimple")
			if strings.Contains(v2doc.ReadingText, "FIELD-RESULT") || strings.Contains(v2doc.ReadingText, "DATE") {
				t.Fatal("field code/result extracted")
			}
		},
		"R2S08": func(t *testing.T) {
			assertUnsupportedType(t, v2doc, "AlternateContent")
			if strings.Contains(v2doc.ReadingText, "CHOICE-TEXT") || strings.Contains(v2doc.ReadingText, "FALLBACK-TEXT") {
				t.Fatal("alternate content duplicated/extracted")
			}
		},
		"R2S09": func(t *testing.T) {
			slides := documentNodes(v2ppt, "slide")
			parts := []string{}
			for _, node := range slides {
				parts = append(parts, node.Locator.Part)
			}
			if !reflect.DeepEqual(parts, []string{"ppt/slides/slide9.xml", "ppt/slides/slide1.xml", "ppt/slides/slide3.xml"}) {
				t.Fatal("sldId/internal relationship order lost")
			}
		},
		"R2S10": func(t *testing.T) {
			values := []string{"Body before title", "x\t\tz", "Title after table", "Grouped A", "Grouped B", "Explicit bullet", "External label only"}
			previous := -1
			for _, value := range values {
				index := strings.Index(v2ppt.ReadingText, value)
				if index <= previous {
					t.Fatal("source shape/group order changed")
				}
				previous = index
			}
		},
		"R2S11": func(t *testing.T) {
			slides := documentNodes(v2ppt, "slide")
			if len(slides) != 3 || slides[1].ReadingRange.StartByte != slides[1].ReadingRange.EndByte {
				t.Fatal("empty source slide missing")
			}
			var a entity.KnowledgeDocumentSlideAttributes
			_ = json.Unmarshal(slides[2].Attributes, &a)
			if !a.Hidden || a.SlideNumber != 3 || !strings.Contains(documentNodeText(v2ppt, slides[2], false), "Hidden source title") {
				t.Fatal("hidden slide/source position lost")
			}
		},
		"R2S12": func(t *testing.T) {
			if !reflect.DeepEqual(documentGrid(t, v2ppt), [][]string{{"x", "", "z"}, {"", "", "b"}}) || !strings.Contains(v2ppt.ReadingText, "x\t\tz\n\t\tb") {
				t.Fatal("empty PPTX cells/separators lost")
			}
		},
		"R2S13": func(t *testing.T) {
			bullet, number := false, false
			for _, node := range documentNodes(v2ppt, "list") {
				var a entity.KnowledgeDocumentListAttributes
				_ = json.Unmarshal(node.Attributes, &a)
				if a.NumberFormat != nil && *a.NumberFormat == "bullet" {
					bullet = true
				}
				if a.StartNumber != nil && *a.StartNumber == 3 && a.SourceLevel != nil && *a.SourceLevel == 1 {
					number = true
				}
			}
			if !bullet || !number {
				t.Fatal("explicit PPTX list attributes lost")
			}
			plain := documentOwnNode(t, v2ppt, "• text-only marker")
			if plain.ParentRef == nil || plain.ParentRef.Kind != "text_box" {
				t.Fatal("text-only bullet guessed as list")
			}
		},
		"R2S14": func(t *testing.T) {
			external := false
			for _, node := range documentNodes(v2ppt, "media") {
				var a entity.KnowledgeDocumentMediaAttributes
				_ = json.Unmarshal(node.Attributes, &a)
				if a.Origin == "external_reference" {
					external = true
					if a.ByteLength != nil || a.MediaType != nil {
						t.Fatal("unfetched external bytes/type guessed")
					}
				}
			}
			encoded, _ := json.Marshal(v2ppt)
			if !external || !strings.Contains(v2ppt.ReadingText, "External label only") || bytes.Contains(encoded, []byte("https://")) || bytes.Contains(encoded, []byte("base64")) {
				t.Fatal("external source relationship not safely represented")
			}
		},
	}
	for _, entry := range input.Cases {
		if run := cases[entry.ID]; run != nil {
			t.Run(entry.ID, run)
		}
	}
	negativeHashes := map[string][2]string{"cycle": {"d3798bdca23f90657f48fbc3fd290b9d0b517be64ec916d5616886c10094258c", "ffb09c40dffc0ec90ea94aebee508c655fb7831da59b9a69bdf424de2234b442"}, "depth": {"0fe5f68bfdc016d4216d493c32df9d69fcd25b27d26060f104aec7d462c62f14", "1f27a45e62b8fddcd3f300eb0dc730e1c3e0e8b5f4f819848ebaebecac91e12d"}}
	for _, name := range []string{"cycle", "depth"} {
		id := "R2S15"
		if name == "depth" {
			id = "R2S16"
		}
		t.Run(id, func(t *testing.T) {
			hashes := negativeHashes[name]
			main := documentSourceFixture(t, base+"source-fixtures-v2/negative-style-"+name+".document.xml", hashes[0])
			styles := documentSourceFixture(t, base+"source-fixtures-v2/negative-style-"+name+".styles.xml", hashes[1])
			data := replaceDocumentParts(t, v2docBytes, map[string][]byte{"word/document.xml": main, "word/styles.xml": styles})
			result, err := BuildDocumentStructure(t.Context(), "docx", data)
			var controlled *DocumentStructureError
			if !errors.As(err, &controlled) || controlled.FailureCode() != "invalid_source_structure" || result.ReadingText != "" || len(result.Payloads) != 0 {
				t.Fatal("invalid style graph did not fail atomically")
			}
		})
	}
}

func assertUnsupportedType(t *testing.T, result DocumentStructureResult, name string) {
	t.Helper()
	for _, node := range documentNodes(result, "unsupported_block") {
		var a entity.KnowledgeDocumentUnsupportedBlockAttributes
		_ = json.Unmarshal(node.Attributes, &a)
		if a.NativeType == name {
			if node.TextStatus != "unsupported" || node.ReadingRange.StartByte != node.ReadingRange.EndByte {
				t.Fatal("unsupported block not zero text")
			}
			return
		}
	}
	t.Fatalf("unsupported source type missing: %s", name)
}

func replaceDocumentParts(t *testing.T, data []byte, replacements map[string][]byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, file := range reader.File {
		target, err := writer.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if replacement, exists := replacements[file.Name]; exists {
			_, err = target.Write(replacement)
		} else {
			source, openErr := file.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			_, err = io.Copy(target, source)
			_ = source.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestDocumentStructurePassageCoverage(t *testing.T) {
	b := newDocumentStructureBuilder(t.Context())
	locator := entity.KnowledgeDocumentLocator{Scheme: "pdf_page_v1", PageNumber: 1}
	page, err := b.add(b.root, "page", locator, entity.KnowledgeDocumentOriginalPosition{Kind: "pdf_page", PageNumber: 1}, entity.KnowledgeDocumentPageAttributes{PageNumber: 1, PageContentClassification: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := b.add(page, "text_block", locator, page.payload.OriginalPosition, entity.KnowledgeDocumentEmptyAttributes{})
	if err != nil {
		t.Fatal(err)
	}
	leaf.own = true
	leaf.ownText = strings.Repeat("a—b\n", 4000)
	result, err := b.finish()
	if err != nil {
		t.Fatal(err)
	}
	assertDocumentRanges(t, result)
	if len(documentNodes(result, "passage")) < 2 || result.ReadingText != leaf.ownText {
		t.Fatal("passage split did not preserve source bytes")
	}
}

func TestDocumentStructureControlledFailures(t *testing.T) {
	for _, test := range []struct {
		format string
		data   []byte
		code   string
	}{{"html", []byte("x"), "unsupported_document"}, {"docx", nil, "invalid_source_structure"}, {"docx", make([]byte, (8<<20)+1), "budget_exceeded"}, {"pptx", []byte("broken"), "invalid_source_structure"}} {
		result, err := BuildDocumentStructure(t.Context(), test.format, test.data)
		var failure *DocumentStructureError
		if !errors.As(err, &failure) || failure.Code != test.code || len(result.Payloads) != 0 {
			t.Fatal("controlled atomic failure mismatch")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := BuildDocumentStructure(ctx, "docx", []byte("broken")); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not preserved")
	}
}
