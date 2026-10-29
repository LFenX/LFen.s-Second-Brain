package dataset

import (
	"context"

	"ragflow/internal/entity"
)

func (d *DatasetService) SearchKnowledgeContentLexical(ctx context.Context, owner, domain string, req entity.KnowledgeContentLexicalSearchRequest) (*entity.KnowledgeContentLexicalSearchData, error) {
	if _, _, err := d.lexicalContext(ctx, owner, domain, true); err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeContentVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if req.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	kb, manifest, err := d.contentLexicalPublished(ctx, owner, domain, req.Index, true)
	if err != nil {
		return nil, err
	}
	selection, sha := contentLexicalSelection(req.Selection)
	if sha != manifest.SelectionSHA256 || lexicalJSON(selection) != lexicalJSON(manifest.Selection) {
		return nil, lexicalError(409, "integrity_error")
	}
	native, ok := d.docEngine.(lexicalEngine)
	if !ok || d.docEngine.GetType() != "elasticsearch" {
		return nil, lexicalError(503, "unavailable")
	}
	expected, err := d.contentLexicalExpectedRows(ctx, manifest)
	if err != nil {
		return nil, err
	}
	if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+kb.TenantID, domain, expected); err != nil {
		return nil, lexicalError(409, "integrity_error")
	}
	keys := make([]lexicalSearchRowKey, len(manifest.Rows))
	byID := make(map[string]contentLexicalManifestRow, len(manifest.Rows))
	for i, row := range manifest.Rows {
		keys[i], byID[row.ID] = lexicalSearchRowKey{ID: row.ID, Binding: row.Binding}, row
	}
	result, err := d.lexicalSearchRows(ctx, kb, req.Query, req.Limit, keys)
	if err != nil {
		return nil, contentLexicalNormalize(err)
	}
	if result == nil || len(result.IDs) > req.Limit || len(result.IDs) != len(result.Chunks) {
		return nil, lexicalError(409, "integrity_error")
	}
	hits := make([]entity.KnowledgeContentLexicalHit, 0, len(result.IDs))
	seen := make(map[string]bool, len(result.IDs))
	for _, id := range result.IDs {
		row, exists := byID[id]
		fields := result.Field[id]
		if !exists || seen[id] || fields == nil {
			return nil, lexicalError(409, "integrity_error")
		}
		seen[id] = true
		available, valid := lexicalNumber(fields["available_int"])
		if !valid || available != 0 || fields["knowledge_binding_kwd"] != row.Binding || fields["knowledge_ref"] != lexicalJSON(row.Reference) || fields["doc_id"] != row.Reference.Address.Target.Source.SourceID || fields["kb_id"] != domain {
			return nil, lexicalError(409, "integrity_error")
		}
		text, current, err := d.contentLexicalReadRow(ctx, owner, domain, row)
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
		hits = append(hits, entity.KnowledgeContentLexicalHit{Evidence: contentLexicalEvidence(req.Index, row), Text: text, Score: score, MatchType: "lexical", Currentness: current})
	}
	if _, err := d.contentLexicalCurrentness(ctx, owner, domain, manifest.Selection); err != nil {
		return nil, err
	}
	if _, _, err := d.contentLexicalPublished(ctx, owner, domain, req.Index, true); err != nil {
		return nil, err
	}
	return &entity.KnowledgeContentLexicalSearchData{Binding: entity.KnowledgeContentLexicalBinding{Index: req.Index, Selection: manifest.Selection, SelectionSHA256: manifest.SelectionSHA256, LocalVersion: manifest.Policies.LocalVersion, ContentVersion: manifest.Policies.ContentVersion, FoundationVersion: manifest.Policies.FoundationVersion, ProfileSHA256: manifest.ProfileSHA256, ProducerSHA256: manifest.ProducerSHA256, Engine: "elasticsearch", Execution: "local_lexical_no_models/0.2.0", ParentExpansion: "disabled", PublishedAt: manifest.PublishedAt}, Coverage: manifest.Coverage, Query: req.Query, RequestedLimit: req.Limit, ReturnedCount: len(hits), Hits: hits, TotalMatches: nil, Pagination: "none", Ranking: "native_lexical_only", ParentExpansion: "disabled"}, nil
}
