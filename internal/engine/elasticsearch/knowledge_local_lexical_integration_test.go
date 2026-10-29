//go:build integration

package elasticsearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	es "github.com/elastic/go-elasticsearch/v8"
	"net/http"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"strings"
	"testing"
	"time"
)

func TestKnowledgeLocalLexicalRealESIsolation(t *testing.T) {
	if os.Getenv("LFEN_POC_SYNTHETIC_ONLY") != "1" {
		t.Skip("isolated synthetic environment not selected")
	}
	if err := common.InitLogger("warn", common.FileOutput{}, ""); err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client, err := es.NewClient(es.Config{Addresses: []string{"http://kcls-es:9200"}, Transport: lexicalRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "kcls-es:9200" || r.URL.Scheme != "http" {
			return nil, errors.New("non-synthetic HTTP blocked")
		}
		return transport.RoundTrip(r)
	})})
	if err != nil {
		t.Fatal(err)
	}
	e := &Engine{client: client}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	domain := common.GenerateUUID()
	index := "ragflow_kcls_" + domain
	if err := e.PrepareKnowledgeLexical(ctx, index, domain, ""); err != nil {
		t.Fatal(err)
	}
	hash := func(v string) string { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:]) }
	row := map[string]any{"id": hash("synthetic index row"), "doc_id": common.GenerateUUID(), "kb_id": domain, "knowledge_binding_kwd": hash("synthetic binding"), "knowledge_ref": "{\"projection_version\":\"0.1.0-candidate\",\"synthetic_transport_probe\":true}", "content_with_weight": "合成运行笔记\r\nTEXT-SYN-07\r\n", "content_ltks": "合成 运行 笔记 text syn 07", "available_int": 0}
	if err := e.CreateKnowledgeLexicalRows(ctx, index, domain, []map[string]any{row}); err != nil {
		t.Fatal(err)
	}
	if err := e.CreateKnowledgeLexicalRows(ctx, index, domain, []map[string]any{row}); err != nil {
		t.Fatalf("equivalent create retry: %v", err)
	}
	ordinary := map[string]any{"id": row["id"], "doc_id": row["doc_id"], "content_with_weight": "合成覆盖尝试", "available_int": 1}
	if _, err := e.InsertChunks(ctx, []map[string]any{ordinary}, index, domain, ""); err == nil {
		t.Fatal("ordinary upsert overwrote immutable row")
	}
	if err := e.UpdateChunks(ctx, map[string]any{"id": row["id"]}, map[string]any{"available_int": 1}, index, domain); err == nil {
		t.Fatal("direct update activated immutable row")
	}
	if err := e.UpdateChunks(ctx, map[string]any{"doc_id": row["doc_id"]}, map[string]any{"available_int": 1}, index, domain); err != nil {
		t.Fatal(err)
	}
	if err := e.VerifyKnowledgeLexicalRows(ctx, index, domain, []map[string]any{row}); err != nil {
		t.Fatal(err)
	}
	negative := map[string]any{"id": "synthetic-negative-availability", "doc_id": row["doc_id"], "content_with_weight": "合成负值哨兵", "available_int": -1}
	if _, err := e.InsertChunks(ctx, []map[string]any{negative}, index, domain, ""); err != nil {
		t.Fatal(err)
	}
	result, err := e.Search(ctx, &types.SearchRequest{IndexNames: []string{index}, KbIDs: []string{domain}, Limit: 8, Filter: map[string]any{"available_int": 0, "knowledge_available_exact": 0}, SelectFields: []string{"id", "available_int", "knowledge_ref"}})
	if err != nil || result.Total != 1 {
		t.Fatalf("exact zero: %+v %v", result, err)
	}
	legacy, err := e.Search(ctx, &types.SearchRequest{IndexNames: []string{index}, KbIDs: []string{domain}, Limit: 8, Filter: map[string]any{"available_int": 0}, SelectFields: []string{"id"}})
	if err != nil || legacy.Total != 2 {
		t.Fatalf("legacy zero changed: %+v %v", legacy, err)
	}
	corrupt := copyFields(row)
	corrupt["knowledge_ref"] = "same binding, mismatched reference"
	if e.CreateKnowledgeLexicalRows(ctx, index, domain, []map[string]any{corrupt}) == nil {
		t.Fatal("non-equivalent conflict accepted")
	}
	if err := e.VerifyKnowledgeLexicalRows(ctx, index, domain, []map[string]any{row}); err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("LFEN_KCLS_EVIDENCE_DIR"); strings.HasPrefix(dir, "/tmp/lfen-kcls-be-20261028-01") {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{"kind": "RealElasticsearchIntegration", "index": index, "rows": 1, "exact_zero_hits": result.Total, "legacy_zero_hits": legacy.Total, "create_retry": "equivalent_only", "ordinary_insert": "rejected", "direct_update": "rejected", "bulk_activation": "excluded", "reference_bytes_sha256": hash(row["knowledge_ref"].(string)), "text_bytes_sha256": hash(row["content_with_weight"].(string)), "data_durability": "tmpfs_lifetime_only"})
		if err := os.WriteFile(dir+"/real-es-isolation.json", raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("real_es_index=%s exact0=1 legacy_lt1=2 ordinary_overwrite=blocked activation=blocked CRLF=preserved", index)
}
