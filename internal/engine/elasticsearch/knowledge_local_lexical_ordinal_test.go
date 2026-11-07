package elasticsearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	es "github.com/elastic/go-elasticsearch/v8"
)

func TestKnowledgeLocalLexicalOrdinalConflictRemainsStrict(t *testing.T) {
	id, binding := strings.Repeat("a", 64), strings.Repeat("b", 64)
	row := map[string]any{"id": id, "kb_id": "synthetic", "doc_id": "source", "knowledge_binding_kwd": binding, "knowledge_ref": `{"synthetic":true}`, "content_with_weight": "Synthetic second reading passage.\r\n", "content_ltks": "synthetic second reading passage", "available_int": 0, "chunk_order_int": 1, "create_time": "2026-01-01 00:00:00", "create_timestamp_flt": 1.0}
	cases := []struct {
		name  string
		field string
		value any
	}{
		{name: "same_ordinal_new_timestamps"},
		{name: "ordinal_changed", field: "chunk_order_int", value: 2},
		{name: "content_changed", field: "content_with_weight", value: "changed"},
		{name: "reference_changed", field: "knowledge_ref", value: `{"synthetic":false}`},
		{name: "binding_changed", field: "knowledge_binding_kwd", value: strings.Repeat("c", 64)},
		{name: "tokens_changed", field: "content_ltks", value: "changed"},
		{name: "availability_changed", field: "available_int", value: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persisted := copyFields(row)
			persisted["create_time"], persisted["create_timestamp_flt"] = "2026-01-02 00:00:00", 2.0
			if tc.field != "" {
				persisted[tc.field] = tc.value
			}
			requests := 0
			client, err := es.NewClient(es.Config{Addresses: []string{"http://synthetic.invalid"}, Transport: lexicalRoundTripper(func(r *http.Request) (*http.Response, error) {
				requests++
				var response any
				switch {
				case r.URL.Path == "/_bulk":
					decoder := json.NewDecoder(r.Body)
					var action map[string]json.RawMessage
					if err := decoder.Decode(&action); err != nil {
						t.Fatal(err)
					}
					if len(action) != 1 || action["create"] == nil {
						t.Fatal("immutable replay must remain create-only")
					}
					var sent map[string]any
					if err := decoder.Decode(&sent); err != nil || sent["chunk_order_int"] != float64(1) {
						t.Fatalf("ordinal missing from complete bulk row: %v", err)
					}
					response = map[string]any{"items": []any{map[string]any{"create": map[string]any{"_id": id, "status": http.StatusConflict}}}}
				case strings.Contains(r.URL.Path, "/_doc/"):
					response = map[string]any{"_id": id, "found": true, "_source": persisted}
				case strings.HasSuffix(r.URL.Path, "/_mget"):
					response = map[string]any{"docs": []any{map[string]any{"_id": id, "found": true, "_source": persisted}}}
				case strings.HasSuffix(r.URL.Path, "/_search"):
					response = map[string]any{"hits": map[string]any{"total": map[string]any{"value": 1, "relation": "eq"}}}
				default:
					t.Fatalf("unexpected ES request: %s", r.URL.Path)
				}
				body, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Elastic-Product": []string{"Elasticsearch"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			engine := &Engine{client: client}
			err = engine.CreateKnowledgeLexicalRows(context.Background(), "ragflow_synthetic", "synthetic", []map[string]any{row})
			if tc.field == "" {
				if err != nil || requests != 4 {
					t.Fatalf("equivalent nonzero ordinal: requests=%d err=%v", requests, err)
				}
			} else if err == nil || err.Error() != "knowledge row conflict is not equivalent" || requests != 2 {
				t.Fatalf("changed %s escaped strict conflict comparison: requests=%d err=%v", tc.field, requests, err)
			}
		})
	}
}
