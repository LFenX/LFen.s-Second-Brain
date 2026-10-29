// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elasticsearch

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// assertStoredField compares a field of the (JSON-decoded) captured source
// projection against the expected value using reflect.DeepEqual. The bulk body
// round-trips through JSON, so numbers arrive as float64 and arrays as
// []interface{}.
func assertStoredField(t *testing.T, doc map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got, ok := doc[key]
	if !ok {
		t.Errorf("stored doc missing field %q", key)
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("field %q = %#v, want %#v", key, got, want)
	}
}

func capturedOrdinaryChunkSource(t *testing.T, actionLine, bodyLine string) map[string]interface{} {
	t.Helper()
	var action map[string]json.RawMessage
	if err := json.Unmarshal([]byte(actionLine), &action); err != nil {
		t.Fatal(err)
	}
	if len(action) != 1 || action["update"] == nil {
		t.Fatalf("ordinary write must use atomic update: %s", actionLine)
	}
	var body struct {
		ScriptedUpsert bool `json:"scripted_upsert"`
		Script         struct {
			Source string `json:"source"`
			Params struct {
				Doc map[string]interface{} `json:"doc"`
			} `json:"params"`
		} `json:"script"`
	}
	if err := json.Unmarshal([]byte(bodyLine), &body); err != nil {
		t.Fatal(err)
	}
	if !body.ScriptedUpsert || !strings.HasPrefix(body.Script.Source, knowledgeWriteGuard) || !strings.Contains(body.Script.Source, "ctx._source.clear(); ctx._source.putAll(params.doc);") || body.Script.Params.Doc == nil {
		t.Fatalf("ordinary atomic guard/source projection missing: %s", bodyLine)
	}
	return body.Script.Params.Doc
}

// TestInsertChunks_WritesIngestionShape is the T0 unit-tier read-back baseline
// (issue #17371). It stands up an in-memory fake Elasticsearch, captures the
// bulk request that InsertChunks sends, and asserts the stored document keeps
// the index-physical field names that the ingestion pipeline currently emits
// (docnm_kwd, content_with_weight, create_timestamp_flt, question_kwd,
// important_kwd, page_num_int, position_int, kb_id overridden to datasetID).
//
// The ordinary bulk action carries the source under script.params.doc so an
// existing immutable knowledge row cannot be replaced. The source projection
// must preserve the ingestion fields. No real ES is required in this unit tier.
func TestInsertChunks_WritesIngestionShape(t *testing.T) {
	var bulkBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_bulk" {
			bulkBody, _ = io.ReadAll(r.Body)
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errors":false,"items":[]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	engine := newTestEngine(t, srv.URL)

	// Mirrors the shape produced by indexdoc.ProcessChunksForPipeline after T1
	// (kb_id is a plain string). InsertChunks overrides kb_id with datasetID,
	// so the stored value is the datasetID regardless of the input form. The
	// input kb_id ("producer-kb-1") is intentionally DISTINCT from datasetID
	// ("kb-1") so the test actually exercises the override rather than passing
	// through an already-equal value.
	chunk := map[string]interface{}{
		"doc_id":               "doc-1",
		"id":                   "chunk-1",
		"kb_id":                "producer-kb-1",
		"docnm_kwd":            "sample.md",
		"content_with_weight":  "hello world",
		"create_time":          "2026-08-04 00:00:00",
		"create_timestamp_flt": float64(123.0),
		"question_kwd":         []string{"q1", "q2"},
		"important_kwd":        []string{"k1"},
		"page_num_int":         int(1),
		"position_int":         int(2),
		"mom_id":               "parent-1",
		"available_int":        int(0),
	}

	visible := map[string]interface{}{"id": "visible", "doc_id": "doc-1", "content_with_weight": "visible source"}
	nullAvailability := map[string]interface{}{"id": "null", "doc_id": "doc-1", "available_int": nil}
	enabled := map[string]interface{}{"id": "enabled", "doc_id": "doc-1", "available_int": 1}
	if _, err := engine.InsertChunks(t.Context(), []map[string]interface{}{chunk, visible, nullAvailability, enabled}, "ragflow_chunk_readback_test", "kb-1", ""); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bulkBody)), "\n")
	if len(lines) < 2 {
		t.Fatalf("bulk body has %d lines, want >=2: %q", len(lines), string(bulkBody))
	}
	doc := capturedOrdinaryChunkSource(t, lines[0], lines[1])

	assertStoredField(t, doc, "docnm_kwd", "sample.md")
	assertStoredField(t, doc, "content_with_weight", "hello world")
	assertStoredField(t, doc, "create_timestamp_flt", float64(123.0))
	assertStoredField(t, doc, "question_kwd", []interface{}{"q1", "q2"})
	assertStoredField(t, doc, "important_kwd", []interface{}{"k1"})
	assertStoredField(t, doc, "page_num_int", float64(1))
	assertStoredField(t, doc, "position_int", float64(2))
	assertStoredField(t, doc, "mom_id", "parent-1")
	assertStoredField(t, doc, "available_int", float64(0))
	// InsertChunks overrides kb_id with datasetID on write.
	assertStoredField(t, doc, "kb_id", "kb-1")
	if len(lines) != 8 {
		t.Fatalf("bulk body has %d lines, want 8", len(lines))
	}
	for _, i := range []int{3, 5, 7} {
		stored := capturedOrdinaryChunkSource(t, lines[i-1], lines[i])
		assertStoredField(t, stored, "available_int", float64(1))
	}
	if _, ok := visible["available_int"]; ok || nullAvailability["available_int"] != nil {
		t.Fatal("InsertChunks must not modify the caller's availability fields")
	}
}
