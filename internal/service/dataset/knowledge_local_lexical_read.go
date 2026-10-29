package dataset

import (
	"context"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
)

func (d *DatasetService) lexicalPublished(ctx context.Context, user, domain string, index entity.KnowledgeLexicalIndexRef, ready bool) (*entity.Knowledgebase, *lexicalManifest, error) {
	kb, p, err := d.lexicalContext(ctx, user, domain, ready)
	if err != nil {
		return nil, nil, err
	}
	if !entity.ValidKnowledgeLexicalHash(index.ID) || !entity.ValidKnowledgeLexicalHash(index.ManifestSHA256) {
		return nil, nil, lexicalError(400, "invalid_request")
	}
	var publication entity.KnowledgeLexicalPublicationRecord
	if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", index.ID, user, domain, entity.KnowledgeLocalLexicalProtocol).Take(&publication).Error; err != nil {
		return nil, nil, lexicalNormalize(err)
	}
	if publication.ManifestSHA256 != index.ManifestSHA256 {
		return nil, nil, lexicalError(409, "integrity_error")
	}
	m, err := lexicalPublication(publication)
	if err != nil {
		return nil, nil, err
	}
	if ready {
		if m.PolicyVersion != p.Version || m.FoundationVersion != p.FoundationVersion {
			return nil, nil, lexicalError(409, "index_stale")
		}
		producer, err := tokenizer.ProducerSHA256()
		if err != nil {
			return nil, nil, lexicalNormalize(err)
		}
		if producer != m.Producer || lexicalLanguage(kb) != m.Language {
			return nil, nil, lexicalError(409, "index_stale")
		}
	}
	return kb, m, nil
}
func lexicalEvidence(index entity.KnowledgeLexicalIndexRef, row lexicalManifestRow) entity.KnowledgeLexicalEvidence {
	return entity.KnowledgeLexicalEvidence{Version: entity.KnowledgeLocalLexicalVersion, Index: index, RowID: row.ID, Source: row.Target.Source, Representation: row.Target.Representation, Node: row.Member.Node, CoordinateSpace: "source_utf8", Range: entity.KnowledgeLexicalRange{Start: row.Member.Range.Start, End: row.Member.Range.End}, FragmentSHA256: row.TextSHA256}
}

// ReadKnowledgeLocal uses immutable SQL evidence and the fixed source reader.
// Local search opt-in does not replace the foundation owner's read authority.
func (d *DatasetService) ReadKnowledgeLocal(ctx context.Context, user, domain string, req entity.KnowledgeLexicalReadRequest) (*entity.KnowledgeLexicalReadData, error) {
	if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeLocalLexicalVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if req.Evidence.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	_, m, err := d.lexicalPublished(ctx, user, domain, req.Evidence.Index, false)
	if err != nil {
		return nil, err
	}
	for _, row := range m.Rows {
		if row.ID != req.Evidence.RowID {
			continue
		}
		expected := lexicalEvidence(req.Evidence.Index, row)
		if lexicalJSON(expected) != lexicalJSON(req.Evidence) {
			return nil, lexicalError(409, "integrity_error")
		}
		text, current, err := d.lexicalReadRow(ctx, user, domain, row)
		if err != nil {
			return nil, err
		}
		current, err = d.lexicalCurrentness(ctx, user, domain, entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{row.Target}})
		if err != nil {
			return nil, err
		}
		if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
			return nil, err
		}
		return &entity.KnowledgeLexicalReadData{Evidence: expected, Text: text, Encoding: "utf-8", ByteLength: len(text), Currentness: current, Complete: true}, nil
	}
	return nil, lexicalError(404, "not_found")
}
