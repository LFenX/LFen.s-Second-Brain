package parser

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
)

func TestNormalizeParseResultKeepsFailureAndActualOutputStates(t *testing.T) {
	failed := errors.New("synthetic producer failure")
	result := NormalizeParseResult(ParseResult{Err: failed})
	if result.Status != "failed" || !errors.Is(result.Result.Err, failed) {
		t.Fatal(result)
	}
	audio := NewAudioParser().ParseWithResult(t.Context(), "synthetic.wav", []byte("no transcription"))
	result = NormalizeParseResult(audio)
	if result.Status != "no_output" || len(result.Result.JSON) != 0 {
		t.Fatal(result)
	}
	projection, err := ProjectKnowledgeParseResult(result.Result)
	if err != nil || projection.State != "unavailable" {
		t.Fatal(projection, err)
	}
	result = NormalizeParseResult(ParseResult{JSON: []DocItem{NewTextJSONItem("合成\r\n正文")}})
	if result.Status != "json" || len(result.Result.JSON) != 1 || result.Result.JSON[0].(TextItem).Text != "合成\r\n正文" {
		t.Fatal(result)
	}
	result = NormalizeParseResult(ParseResult{JSON: []DocItem{TextItem{Text: ""}}})
	projection, err = ProjectKnowledgeParseResult(result.Result)
	if result.Status != "json" || err != nil || projection.State != "empty" {
		t.Fatal(result, projection, err)
	}
}

func TestNormalizeParseResultMarkdownIsTypedAndDoesNotFetch(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	text := "# 合成标题\n\n| A | B |\n| --- | --- |\n| 甲 | 乙 |\n\n![合成替代文本](" + server.URL + "/image.png)"
	markdown, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	markdown.FetchRemoteImages = false
	result := NormalizeParseResult(markdown.ParseWithResult(t.Context(), "synthetic.md", []byte(text)))
	if result.Status != "json" || result.Result.Err != nil || result.Diagnostic != nil || requests.Load() != 0 {
		t.Fatal(result.Status, result.Diagnostic, requests.Load())
	}
	tableFound := false
	for _, item := range result.Result.JSON {
		var table *entity.TableData
		switch value := item.(type) {
		case TableItem:
			table = value.Table
		case *TableItem:
			if value != nil {
				table = value.Table
			}
		}
		if table != nil {
			rows := table.CellTexts()
			if len(rows) < 2 || len(rows[1]) != 2 || rows[1][0] != "甲" || rows[1][1] != "乙" {
				t.Fatal("normalized table changed", rows)
			}
			tableFound = true
		}
	}
	if !tableFound {
		t.Fatal("Markdown table did not produce typed TableItem")
	}
}
