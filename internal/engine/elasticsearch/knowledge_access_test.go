package elasticsearch

import (
	"encoding/json"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
)

func accessEngineRequest() *types.SearchRequest {
	return &types.SearchRequest{KnowledgeAccessStable: true, IndexNames: []string{"ragflow_11111111111111111111111111111111"}, KbIDs: []string{"synthetic"}, Limit: 2, SelectFields: []string{"id", "content_with_weight"}, Filter: map[string]any{"available_int": 0, "knowledge_available_exact": 0, "or": []map[string]any{{"knowledge_binding_kwd": strings.Repeat("b", 64), "id": []string{strings.Repeat("1", 64), strings.Repeat("2", 64)}}}}, MatchExprs: []any{&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "synthetic", TopN: 2}}, RankFeature: map[string]float64{}}
}

func TestKnowledgeAccessEngineStableIDBeforeLimitAndNoScoreReorder(t *testing.T) {
	requests := 0
	engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if strings.HasSuffix(r.URL.Path, "/_mapping") {
			return searchErrorResponse(200, `{"ragflow_11111111111111111111111111111111":{"mappings":{"properties":{"id":{"type":"keyword"}}}}}`), nil
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			t.Fatal("query JSON")
		}
		sortJSON, _ := json.Marshal(body["sort"])
		if string(sortJSON) != `[{"id":{"order":"asc"}}]` || body["size"] != float64(2) || !strings.Contains(string(raw), "knowledge_binding_kwd") || strings.Contains(string(raw), "id.keyword") {
			t.Fatalf("scope/order/limit missing: %s", raw)
		}
		return searchErrorResponse(200, `{"hits":{"total":{"value":1000},"hits":[{"_id":"`+strings.Repeat("1", 64)+`","_score":1,"_source":{"id":"`+strings.Repeat("1", 64)+`","content_with_weight":"first"}},{"_id":"`+strings.Repeat("2", 64)+`","_score":9999,"_source":{"id":"`+strings.Repeat("2", 64)+`","content_with_weight":"second"}}]}}`), nil
	})
	result, err := engine.Search(t.Context(), accessEngineRequest())
	if err != nil || len(result.Chunks) != 2 || result.Chunks[0]["content_with_weight"] != "first" || requests != 2 {
		t.Fatal(result, err, requests)
	}
}

func TestKnowledgeAccessEngineUnsupportedMappingsAndTransportFailClosed(t *testing.T) {
	for _, mapping := range []string{`{"id":{"type":"text","fields":{"keyword":{"type":"keyword"}}}}`, `{"id":{"type":"keyword","doc_values":false}}`, `{"id":{"type":"keyword","ignore_above":32}}`, `{}`} {
		t.Run(mapping, func(t *testing.T) {
			searches := 0
			engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(r.URL.Path, "/_mapping") {
					searches++
				}
				return searchErrorResponse(200, `{"ragflow_11111111111111111111111111111111":{"mappings":{"properties":`+mapping+`}}}`), nil
			})
			result, err := engine.Search(t.Context(), accessEngineRequest())
			if err == nil || result != nil || searches != 0 {
				t.Fatal("unsupported mapping searched", result, err)
			}
		})
	}
	engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/_mapping") {
			return searchErrorResponse(200, `{"ragflow_11111111111111111111111111111111":{"mappings":{"properties":{"id":{"type":"keyword"}}}}}`), nil
		}
		return searchErrorResponse(400, `{"error":{"reason":"PrivateQueryAndBodyMustNotEscape"}}`), nil
	})
	result, err := engine.Search(t.Context(), accessEngineRequest())
	if err == nil || result != nil || strings.Contains(err.Error(), "PrivateQueryAndBodyMustNotEscape") {
		t.Fatal("private backend error escaped", err)
	}
}

func TestKnowledgeAccessEnginePrivateLogsAndOrdinaryDiagnosticControl(t *testing.T) {
	engine := newSearchErrorEngine(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/_mapping") {
			return searchErrorResponse(200, `{"ragflow_11111111111111111111111111111111":{"mappings":{"properties":{"id":{"type":"keyword"}}}}}`), nil
		}
		return searchErrorResponse(400, `{"error":{"reason":"PrivateBackendSentinelMustNotEscape"}}`), nil
	})
	if err := common.SetLogLevel("debug"); err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.DebugLevel)
	previous, previousSugar := common.Logger, common.Sugar
	common.Logger = zap.New(core)
	common.Sugar = common.Logger.Sugar()
	t.Cleanup(func() { common.Logger, common.Sugar = previous, previousSugar })
	request := accessEngineRequest()
	request.MatchExprs[0].(*types.MatchTextExpr).MatchingText = "PrivateQuerySentinelMustNotEscape"
	result, err := engine.Search(t.Context(), request)
	if result != nil || err == nil || strings.Contains(err.Error(), "Sentinel") || logs.Len() != 0 {
		t.Fatal("private ES diagnostic escaped")
	}
	request.KnowledgeAccessStable = false
	_, _ = engine.Search(t.Context(), request)
	if logs.Len() == 0 {
		t.Fatal("ordinary ES logging globally disabled")
	}
}
