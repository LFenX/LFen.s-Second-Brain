package elasticsearch

import (
	"context"
	"encoding/json"
	es "github.com/elastic/go-elasticsearch/v8"
	"io"
	"net/http"
	"strings"
	"testing"
)

type lexicalRoundTripper func(*http.Request) (*http.Response, error)

func (f lexicalRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestKnowledgeLocalLexicalExactZeroAndAtomicOrdinaryWrite(t *testing.T) {
	raw, _ := json.Marshal(buildBoolQueryFromCondition(map[string]any{"available_int": 0, "knowledge_available_exact": 0}, []string{"synthetic"}, false, false))
	if !strings.Contains(string(raw), `"term":{"available_int":0}`) || !strings.Contains(string(raw), `"lt":1`) {
		t.Fatalf("zero semantics: %s", raw)
	}
	raw, _ = json.Marshal(buildBoolQueryFromCondition(map[string]any{"knowledge_available_exact": -1}, nil, false, false))
	if !strings.Contains(string(raw), `"match_none"`) {
		t.Fatalf("invalid exact filter was dropped: %s", raw)
	}
	update := knowledgeOrdinaryUpdate(map[string]any{"id": "ordinary", "available_int": 1})
	script := update["script"].(map[string]any)["source"].(string)
	if update["scripted_upsert"] != true || !strings.HasPrefix(script, knowledgeWriteGuard) || !strings.Contains(script, "ctx._source.clear()") || !knowledgeReserved(map[string]any{"knowledge_ref": nil}) {
		t.Fatal("ordinary write lost atomic immutable-row guard")
	}
}
func TestKnowledgeLocalLexicalConflictReplayAndCompleteRowVerification(t *testing.T) {
	id := strings.Repeat("a", 64)
	binding := strings.Repeat("b", 64)
	row := map[string]any{"id": id, "kb_id": "synthetic", "doc_id": "source", "knowledge_binding_kwd": binding, "knowledge_ref": "{\"synthetic\":true}", "content_with_weight": "原创合成\r\n", "available_int": 0}
	persisted := copyFields(row)
	requests := 0
	client, err := es.NewClient(es.Config{Addresses: []string{"http://synthetic.invalid"}, Transport: lexicalRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		var body any
		switch {
		case r.URL.Path == "/_bulk":
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"create"`) {
				t.Fatalf("not create-only: %s", b)
			}
			body = map[string]any{"items": []any{map[string]any{"create": map[string]any{"_id": id, "status": 409}}}}
		case strings.Contains(r.URL.Path, "/_doc/"):
			body = map[string]any{"_id": id, "found": true, "_source": persisted}
		case strings.HasSuffix(r.URL.Path, "/_mget"):
			body = map[string]any{"docs": []any{map[string]any{"_id": id, "found": true, "_source": persisted}}}
		case strings.HasSuffix(r.URL.Path, "/_search"):
			body = map[string]any{"hits": map[string]any{"total": map[string]any{"value": 1, "relation": "eq"}}}
		default:
			t.Fatalf("unexpected ES request: %s", r.URL.Path)
		}
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Elastic-Product": []string{"Elasticsearch"}}, Body: io.NopCloser(strings.NewReader(string(b))), Request: r}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{client: client}
	if err := engine.CreateKnowledgeLexicalRows(context.Background(), "ragflow_synthetic", "synthetic", []map[string]any{row}); err != nil {
		t.Fatal(err)
	}
	if requests < 4 {
		t.Fatal("complete verification not issued")
	}
	persisted["available_int"] = -1
	if engine.VerifyKnowledgeLexicalRows(context.Background(), "ragflow_synthetic", "synthetic", []map[string]any{row}) == nil {
		t.Fatal("negative availability passed exact-zero verification")
	}
	if engine.CreateKnowledgeLexicalRows(context.Background(), "ragflow_synthetic", "synthetic", []map[string]any{row}) == nil {
		t.Fatal("non-equivalent conflict replay passed")
	}
}
