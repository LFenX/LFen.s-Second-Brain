package dataset

import (
	"context"
	"math"
	"ragflow/internal/entity"
	"ragflow/internal/service/nlp"
	"strconv"
)

type lexicalSearchRowKey struct{ ID, Binding string }

func lexicalSearchScope(rows []lexicalSearchRowKey) map[string]any {
	byBinding := map[string][]string{}
	for _, row := range rows {
		byBinding[row.Binding] = append(byBinding[row.Binding], row.ID)
	}
	clauses := []map[string]any{}
	for binding, ids := range byBinding {
		clauses = append(clauses, map[string]any{"knowledge_binding_kwd": binding, "id": ids})
	}
	if len(clauses) == 0 {
		clauses = append(clauses, map[string]any{"knowledge_binding_kwd": "no-published-rows"})
	}
	return map[string]any{"available_int": 0, "knowledge_available_exact": 0, "or": clauses}
}

func (d *DatasetService) lexicalSearchRows(ctx context.Context, kb *entity.Knowledgebase, query string, limit int, rows []lexicalSearchRowKey) (*nlp.RetrievalSearchResult, error) {
	weight := 0.0
	fallback, highlight := false, false
	return nlp.NewRetrievalService(d.docEngine, d.documentDAO).Search(ctx, &nlp.RetrievalSearchRequest{KnowledgeLexical: true, Question: query, TenantIDs: []string{kb.TenantID}, KbIDs: []string{kb.ID}, Page: 1, PageSize: limit, Filter: lexicalSearchScope(rows), EmbeddingModel: nil, VectorSimilarityWeight: &weight, AllowDenseFallback: &fallback, Highlight: &highlight, RankFeature: map[string]float64{}, Language: lexicalLanguage(kb)})
}
func lexicalNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case int:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return 0, false
}
func (d *DatasetService) SearchKnowledgeLocal(ctx context.Context, user, domain string, req entity.KnowledgeLexicalSearchRequest) (*entity.KnowledgeLexicalSearchData, error) {
	if _, _, err := d.lexicalContext(ctx, user, domain, true); err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeLocalLexicalVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if req.Selection.Validate(domain) != nil || !entity.ValidKnowledgeLexicalQuery(req.Query) || req.Limit < 1 || req.Limit > 8 {
		return nil, lexicalError(400, "invalid_request")
	}
	kb, m, err := d.lexicalPublished(ctx, user, domain, req.Index, true)
	if err != nil {
		return nil, err
	}
	s, sha := lexicalSelection(req.Selection)
	if sha != m.SelectionSHA256 || lexicalJSON(s) != lexicalJSON(m.Selection) {
		return nil, lexicalError(409, "integrity_error")
	}
	native, ok := d.docEngine.(lexicalEngine)
	if !ok || d.docEngine.GetType() != "elasticsearch" {
		return nil, lexicalError(503, "unavailable")
	}
	expected, err := d.lexicalExpectedRows(ctx, m)
	if err != nil {
		return nil, err
	}
	if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+kb.TenantID, domain, expected); err != nil {
		return nil, lexicalError(409, "integrity_error")
	}
	keys := make([]lexicalSearchRowKey, len(m.Rows))
	for i, row := range m.Rows {
		keys[i] = lexicalSearchRowKey{ID: row.ID, Binding: row.Binding}
	}
	result, err := d.lexicalSearchRows(ctx, kb, req.Query, req.Limit, keys)
	if err != nil {
		return nil, lexicalNormalize(err)
	}
	if result == nil || len(result.IDs) > req.Limit || len(result.IDs) != len(result.Chunks) {
		return nil, lexicalError(409, "integrity_error")
	}
	manifest := map[string]lexicalManifestRow{}
	for _, r := range m.Rows {
		manifest[r.ID] = r
	}
	hits := []entity.KnowledgeLexicalHit{}
	seen := map[string]bool{}
	for _, id := range result.IDs {
		row, ok := manifest[id]
		fields := result.Field[id]
		if !ok || seen[id] || fields == nil {
			return nil, lexicalError(409, "integrity_error")
		}
		seen[id] = true
		available, valid := lexicalNumber(fields["available_int"])
		if !valid || available != 0 || fields["knowledge_binding_kwd"] != row.Binding || fields["knowledge_ref"] != row.Reference || fields["doc_id"] != row.Target.Source.SourceID || fields["kb_id"] != domain {
			return nil, lexicalError(409, "integrity_error")
		}
		text, current, err := d.lexicalReadRow(ctx, user, domain, row)
		if err != nil {
			return nil, err
		}
		if fields["content_with_weight"] != text || int64(len(text)) != row.ByteLength {
			return nil, lexicalError(409, "integrity_error")
		}
		score, valid := lexicalNumber(fields["_score"])
		if !valid {
			return nil, lexicalError(409, "integrity_error")
		}
		hits = append(hits, entity.KnowledgeLexicalHit{Evidence: lexicalEvidence(req.Index, row), Text: text, Score: score, MatchType: "lexical", Currentness: current})
	}
	if _, err := d.lexicalCurrentness(ctx, user, domain, m.Selection); err != nil {
		return nil, err
	}
	if _, _, err := d.lexicalPublished(ctx, user, domain, req.Index, true); err != nil {
		return nil, err
	}
	return &entity.KnowledgeLexicalSearchData{Binding: entity.KnowledgeLexicalBinding{Index: req.Index, Selection: m.Selection, SelectionSHA256: m.SelectionSHA256, PolicyVersion: m.PolicyVersion, FoundationVersion: m.FoundationVersion, ProfileSHA256: m.Profile, Engine: "elasticsearch", ExecutionProfile: "local_lexical_no_models_v1", ParentExpansion: "disabled", UnitPolicy: "existing_r1_passage_or_bounded_c1_root", PublishedAt: m.PublishedAt}, Coverage: m.Coverage, Readiness: "ready", Query: req.Query, RequestedLimit: req.Limit, ReturnedCount: len(hits), Hits: hits, Pagination: "not_supported_in_first_slice", Ranking: "native_lexical_only", ParentExpansion: "disabled"}, nil
}
