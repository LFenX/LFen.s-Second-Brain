//go:build linux && cgo

package parser

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/entity"
	"ragflow/internal/parser/chunk"
)

func parsedChildTestRun(t *testing.T, name, format string, input []byte) *entity.KnowledgeContentProjection {
	t.Helper()
	_, control := documentChildOwnedTestInput(t, string(input))
	control.Format, control.Filename, control.ProfileID = format, name, KnowledgeParsedLocalProfile
	var observed DocumentChildOwnership
	raw, err := RunDocumentStructureChild(t.Context(), input, control, func(owner DocumentChildOwnership) error { observed = owner; return nil })
	if err != nil {
		t.Fatalf("owned %s child failed: %v", format, err)
	}
	if _, err := os.Lstat(observed.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("child cleanup was not proven", err)
	}
	projection, err := DecodeKnowledgeParsedChild(raw)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func parsedChildText(p *entity.KnowledgeContentProjection) string {
	var text strings.Builder
	for _, node := range p.Nodes {
		if node.Text.Value != nil {
			text.WriteString(*node.Text.Value)
			text.WriteByte('\n')
		}
	}
	return text.String()
}

func TestKnowledgeParsedChildActualLocalFamilies(t *testing.T) {
	for _, fixture := range []struct{ name, format, input, contains string }{
		{"rows.csv", "csv", "A,B\n甲,乙\n", "甲\t乙"},
		{"body.txt", "txt", "synthetic CRLF\r\nsecond", "synthetic CRLF"},
		{"body.json", "json", "{\"label\":\"synthetic-json\"}", "synthetic-json"},
		{"body.md", "md", "# synthetic heading\n\nbody", "synthetic heading"},
		{"body.html", "html", "<html><p>synthetic html</p></html>", "synthetic html"},
		{"message.eml", "email", "From: synthetic@example.invalid\r\nTo: owner@example.invalid\r\nSubject: Synthetic\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nsynthetic email body\r\n", "synthetic email body"},
	} {
		t.Run(fixture.format, func(t *testing.T) {
			p := parsedChildTestRun(t, fixture.name, fixture.format, []byte(fixture.input))
			if p.State != "ready" || !strings.Contains(parsedChildText(p), fixture.contains) {
				t.Fatal(p.State, parsedChildText(p))
			}
		})
	}
	if p := parsedChildTestRun(t, "empty.json", "json", nil); p.State != "empty" {
		t.Fatal("explicit empty JSON is not empty", p.State)
	}
	_, empty := documentChildOwnedTestInput(t, "")
	empty.Format, empty.Filename, empty.ProfileID = "csv", "empty.csv", KnowledgeParsedLocalProfile
	raw, err := RunDocumentStructureChild(t.Context(), nil, empty, func(DocumentChildOwnership) error { return nil })
	if len(raw) != 0 || documentChildFailure(err) != "source_structure_unavailable" {
		t.Fatal("normalizer no_output became a published empty CSV", err)
	}
}

func TestKnowledgeParsedChildRemoteImagesAndAttachmentsDisabled(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	p := parsedChildTestRun(t, "body.md", "md", []byte("# synthetic\n\n![image]("+server.URL+"/remote.png)\n"))
	if !strings.Contains(parsedChildText(p), "synthetic") || calls.Load() != 0 {
		t.Fatal("Markdown fetched remote images")
	}
	email := "From: a@example.invalid\r\nSubject: Synthetic\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nlocal body\r\n--x\r\nContent-Type: text/markdown\r\nContent-Disposition: attachment; filename=attached.md\r\n\r\nATTACHMENT_SENTINEL ![image](" + server.URL + "/attachment.png)\r\n--x--\r\n"
	first := parsedChildText(parsedChildTestRun(t, "message.eml", "email", []byte(email)))
	second := parsedChildText(parsedChildTestRun(t, "message.eml", "email", []byte(email)))
	if first != second || !strings.Contains(first, "local body") || strings.Contains(first, "ATTACHMENT_SENTINEL") || calls.Load() != 0 {
		t.Fatal("Email ordering or attachment boundary changed")
	}
}

func TestKnowledgeParsedChildHTMLMailBody(t *testing.T) {
	input := "Subject: Synthetic mail\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<head><title>HIDDEN</title></head><body><p>First &amp; second</p><table><tr><td>Third</td><td>Fourth</td></tr></table><script>HIDDEN</script></body>"
	p := parsedChildTestRun(t, "message.eml", "email", []byte(input))
	text := parsedChildText(p)
	if p.State != "ready" || !strings.Contains(text, "First & second\nThird\nFourth") || !strings.Contains(text, "Synthetic mail") || strings.Contains(text, "HIDDEN") || strings.Contains(text, "<td>") {
		t.Fatalf("HTML mail state=%s text=%q", p.State, text)
	}
}

func TestKnowledgeParsedChildRetainsEmbeddedMedia(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	png := officeTestPNG(t, 4)
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	book := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Header")
		mustSetCell(t, f, "Sheet1", "A2", "Local body")
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
			Extension: ".png", File: png, Format: &excelize.GraphicOptions{AltText: "figure"},
		}); err != nil {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct {
		format string
		body   []byte
	}{
		{"html", []byte("<p>Local body<img alt='figure' src='" + dataURI + "'><img src='" + server.URL + "/remote'></p>")},
		{"md", []byte("Local body ![figure](" + dataURI + ") trailing words\n\n![remote](" + server.URL + "/remote)\n")},
		{"xlsx", book},
	} {
		t.Run(tc.format, func(t *testing.T) {
			p := parsedChildTestRun(t, "source."+tc.format, tc.format, tc.body)
			body := parsedChildText(p)
			if p.State != "partial" || !strings.Contains(body, "Local body") || strings.Contains(body, "figure") {
				t.Fatalf("state=%s extracted=%q", p.State, body)
			}
			if tc.format == "md" && !strings.Contains(body, "trailing words") {
				t.Fatal("lost Markdown text after inline image")
			}
			images := 0
			for _, node := range p.Nodes {
				if node.Kind == "media" {
					images++
					a := node.Attributes.Parser
					if !a.PayloadPresent || a.CKType != "image" || a.AltText == nil || *a.AltText != "figure" || node.Text.Status != "unsupported" || node.Text.Value != nil {
						t.Fatalf("unexpected image metadata: %+v", a)
					}
				}
			}
			if images != 1 || calls.Load() != 0 {
				t.Fatalf("images=%d remote requests=%d", images, calls.Load())
			}
		})
	}
}

func TestKnowledgeParsedChildMergedGeometrySurvivesJSON(t *testing.T) {
	body := `<table><tr><td>A</td><td>B</td><td>C</td></tr><tr><td>D</td><td rowspan="2">MERGED</td><td>E</td></tr><tr><td>F</td><td>G</td></tr></table>`
	p := parsedChildTestRun(t, "table.html", "html", []byte(body))
	found := false
	for _, node := range p.Nodes {
		if node.Attributes.Parser == nil || node.Attributes.Parser.Table == nil {
			continue
		}
		table := node.Attributes.Parser.Table
		if len(table.Grid) < 3 || len(table.Grid[2]) < 3 || !table.Grid[2][1].Covered {
			continue
		}
		found = true
		if table.Grid[2][1].OriginRow != 1 || table.Grid[2][1].OriginCol != 1 {
			t.Fatal("covered cell lost its origin across JSON")
		}
		ranges, err := chunk.PlanContentRanges(chunk.ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: *node.Text.Value, Table: table, MaxBytes: 128})
		if err != nil || len(ranges) == 0 {
			t.Fatal("valid nonzero-origin geometry rejected after child JSON", err)
		}
		table.Grid[2][1].Covered = false
		if _, err := chunk.PlanContentRanges(chunk.ContentRangeInput{Kind: "table", CoordinateSpace: "item_utf8", Text: *node.Text.Value, Table: table, MaxBytes: 128}); err == nil {
			t.Fatal("overlapping explicit geometry accepted")
		}
	}
	if !found {
		t.Fatal("actual HTML producer did not emit the merged table")
	}
}

func TestKnowledgeParsedChildNativeDOCAndSpreadsheet(t *testing.T) {
	doc := parsedChildTestRun(t, "body.doc", "doc", buildSyntheticDoc(t, "Synthetic native Word body"))
	if !strings.Contains(parsedChildText(doc), "Synthetic native Word body") {
		t.Fatal(doc.State, parsedChildText(doc))
	}
	xlsx := parsedChildTestRun(t, "rows.xlsx", "xlsx", createTestExcelBytes(t))
	if !strings.Contains(parsedChildText(xlsx), "Val1") {
		t.Fatal(xlsx.State, parsedChildText(xlsx))
	}
	xlsBytes, err := base64.StdEncoding.DecodeString(genuineBIFF8FixtureBase64)
	if err != nil {
		t.Fatal(err)
	}
	xls := parsedChildTestRun(t, "rows.xls", "xls", xlsBytes)
	if !strings.Contains(parsedChildText(xls), "Apples") {
		t.Fatal(xls.State, parsedChildText(xls))
	}
	input := buildPPTX(t, "Synthetic local presentation")
	_, control := documentChildOwnedTestInput(t, string(input))
	control.Format, control.Filename, control.ProfileID = "ppt", "slides.ppt", KnowledgeParsedLocalProfile
	raw, err := RunDocumentStructureChild(t.Context(), input, control, func(DocumentChildOwnership) error { return nil })
	if len(raw) != 0 || documentChildFailure(err) != "source_structure_unavailable" {
		t.Fatalf("OOXML container mislabeled as binary PPT: %v", err)
	}
}

func TestKnowledgeParsedChildEPUBAndArchiveBudget(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, fixture := range []struct{ name, value string }{
		{"META-INF/container.xml", `<container><rootfiles><rootfile full-path="book/content.opf"/></rootfiles></container>`},
		{"book/content.opf", `<package><manifest><item id="one" href="one.xhtml"/></manifest><spine><itemref idref="one"/></spine></package>`},
		{"book/one.xhtml", `<html><body><p>Synthetic local EPUB</p></body></html>`},
	} {
		file, err := writer.Create(fixture.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(fixture.value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	p := parsedChildTestRun(t, "book.epub", "epub", buffer.Bytes())
	if !strings.Contains(parsedChildText(p), "Synthetic local EPUB") {
		t.Fatal(p.State, parsedChildText(p))
	}
	buffer.Reset()
	writer = zip.NewWriter(&buffer)
	file, err := writer.Create("oversized.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(bytes.Repeat([]byte{'a'}, 8<<20+1)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := knowledgeParsedArchiveBudget(buffer.Bytes()); documentChildFailure(err) != "budget_exceeded" {
		t.Fatal("ZIP expansion limit not enforced", err)
	}
}

func TestKnowledgeParsedChildRejectsUnadmittedProfilesAndFilenames(t *testing.T) {
	for _, format := range []string{"pdf", "docx", "pptx", "aural", "visual", "video", "other"} {
		if KnowledgeParsedLocalFormat(format) {
			t.Fatal("unadmitted family", format)
		}
	}
	_, control := documentChildOwnedTestInput(t, "synthetic")
	control.ProfileID, control.Format, control.Filename = KnowledgeParsedLocalProfile, "csv", "rows.csv"
	if !documentChildControlValid(control, false) {
		t.Fatal("valid fixed parsed profile rejected")
	}
	for _, filename := range []string{"rows.txt", "../rows.csv", "a\\rows.csv", "rows.csv\n"} {
		control.Filename = filename
		if documentChildControlValid(control, false) {
			t.Fatal("invalid filename admitted", filename)
		}
	}
}
