package dataset

import (
	"context"
	"errors"
	"ragflow/internal/engine/types"

	"ragflow/internal/entity"
	"ragflow/internal/service/nlp"
)

func (d *DatasetService) accessSearch(ctx context.Context, actor, requestID string, req entity.KnowledgeAccessSearchRequest) (int, []byte, error) {
	if len(req.Query) > 4096 {
		return 0, nil, accessError(400, "invalid_request")
	}
	verified, err := d.accessOpenView(ctx, actor, req.ViewID, "search", requestID)
	if err != nil {
		return 0, nil, err
	}
	data := entity.KnowledgeAccessSearchData{ViewID: req.ViewID, Query: req.Query, RequestedLimit: req.Limit, Hits: []entity.KnowledgeAccessHit{}, Pagination: "none", Ranking: "scoped_lexical_stable_id", ParentExpansion: "disabled"}
	if len(verified.Rows) > 0 {
		if d.docEngine == nil || d.docEngine.GetType() != "elasticsearch" {
			return 0, nil, accessError(503, "unavailable")
		}
		keys := make([]lexicalSearchRowKey, 0, len(verified.Rows))
		allowed := map[string]contentLexicalManifestRow{}
		for _, row := range verified.Rows {
			keys = append(keys, lexicalSearchRowKey{ID: row.ID, Binding: row.Binding})
			allowed[row.ID] = row
		}
		fallback, highlight := false, false
		weight := 0.0
		result, err := nlp.NewRetrievalService(d.docEngine, d.documentDAO).Search(ctx, &nlp.RetrievalSearchRequest{KnowledgeLexical: true, KnowledgeAccessStable: true, Question: req.Query, TenantIDs: []string{verified.KB.TenantID}, KbIDs: []string{verified.KB.ID}, Page: 1, PageSize: int(req.Limit), Filter: lexicalSearchScope(keys), EmbeddingModel: nil, AllowDenseFallback: &fallback, Highlight: &highlight, VectorSimilarityWeight: &weight, RankFeature: map[string]float64{}, Language: verified.Capture.Policies.Language})
		if err != nil {
			if errors.Is(err, types.ErrKnowledgeAccessIntegrity) {
				return 0, nil, accessError(409, "integrity_error")
			}
			return 0, nil, accessError(503, "unavailable")
		}
		if result == nil || len(result.IDs) != len(result.Chunks) || len(result.IDs) > int(req.Limit) || len(result.Field) != len(result.IDs) {
			return 0, nil, accessError(409, "integrity_error")
		}
		cache := map[string]*contentLexicalSnapshot{}
		previous := ""
		for i, id := range result.IDs {
			row, ok := allowed[id]
			fields := result.Field[id]
			if !ok || fields == nil || id <= previous {
				return 0, nil, accessError(409, "integrity_error")
			}
			previous = id
			available, valid := lexicalNumber(fields["available_int"])
			if !valid || available != 0 || fields["id"] != id || fields["kb_id"] != verified.KB.ID || fields["doc_id"] != row.Reference.Address.Target.Source.SourceID || fields["knowledge_binding_kwd"] != row.Binding || fields["knowledge_ref"] != accessJSON(row.Reference) {
				return 0, nil, accessError(409, "integrity_error")
			}
			text, citation, _, err := d.accessReadRow(ctx, verified, row, cache)
			if err != nil {
				return 0, nil, err
			}
			if fields["content_with_weight"] != text {
				return 0, nil, accessError(409, "integrity_error")
			}
			data.Hits = append(data.Hits, entity.KnowledgeAccessHit{EvidenceRef: accessEvidence(req.ViewID, row), Citation: citation, Text: text, ByteLength: int64(len(text)), RankOrdinal: int64(i + 1)})
		}
	}
	data.ReturnedCount = int64(len(data.Hits))
	encoded, err := accessEncode("search", requestID, data)
	if err != nil {
		return 0, nil, err
	}
	if err = d.accessFinalPermit(ctx, verified, "search", requestID); err != nil {
		return 0, nil, err
	}
	return 200, encoded, nil
}
