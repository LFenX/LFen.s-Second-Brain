package dataset

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
)

func (d *DatasetService) contentLexicalPublished(ctx context.Context, owner, domain string, index entity.KnowledgeLexicalIndexRef, ready bool) (*entity.Knowledgebase, *contentLexicalManifest, error) {
	kb, _, err := d.lexicalContext(ctx, owner, domain, ready)
	if err != nil {
		return nil, nil, err
	}
	policies, err := d.contentLexicalPolicies(ctx, owner, domain, ready)
	if err != nil {
		return nil, nil, err
	}
	if !entity.ValidKnowledgeLexicalHash(index.ID) || !entity.ValidKnowledgeLexicalHash(index.ManifestSHA256) {
		return nil, nil, lexicalError(400, "invalid_request")
	}
	var record entity.KnowledgeLexicalPublicationRecord
	if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", index.ID, owner, domain, entity.KnowledgeContentLexicalProtocol).Take(&record).Error; err != nil {
		return nil, nil, contentLexicalNormalize(err)
	}
	if record.ManifestSHA256 != index.ManifestSHA256 {
		return nil, nil, lexicalError(409, "integrity_error")
	}
	manifest, err := contentLexicalPublication(record)
	if err != nil {
		return nil, nil, err
	}
	if ready {
		if manifest.Policies != policies {
			return nil, nil, lexicalError(409, "index_stale")
		}
		producer, err := tokenizer.ProducerSHA256()
		if err != nil || producer != manifest.ProducerSHA256 || manifest.ProfileSHA256 != contentLexicalProfile(policies.Language, producer).SHA256 {
			return nil, nil, lexicalError(409, "index_stale")
		}
	}
	return kb, manifest, nil
}

func (d *DatasetService) contentLexicalCurrentness(ctx context.Context, owner, domain string, selection entity.KnowledgeContentLexicalSelection) (string, error) {
	current, historical := false, false
	for _, target := range selection.Targets {
		if _, err := d.GetKnowledgeContentRepresentation(ctx, owner, domain, target); err != nil {
			return "", contentLexicalNormalize(err)
		}
		var source entity.KnowledgeSourceRevision
		if err := dao.DB.WithContext(ctx).Select("is_current").Where("id = ? AND domain_id = ? AND document_id = ? AND sha256 = ?", target.Source.RevisionID, domain, target.Source.SourceID, target.Source.SHA256).Take(&source).Error; err != nil {
			return "", contentLexicalNormalize(err)
		}
		current, historical = current || source.IsCurrent, historical || !source.IsCurrent
	}
	if historical && current {
		return "mixed", nil
	}
	if historical {
		return "historical", nil
	}
	return "current", nil
}

func (d *DatasetService) contentLexicalReadRow(ctx context.Context, owner, domain string, row contentLexicalManifestRow) (string, string, error) {
	ref := row.Reference
	meta, err := d.GetKnowledgeContentRepresentation(ctx, owner, domain, ref.Address.Target)
	if err != nil {
		return "", "", contentLexicalNormalize(err)
	}
	node, err := d.GetKnowledgeContentNode(ctx, owner, domain, ref.Address)
	if err != nil {
		return "", "", contentLexicalNormalize(err)
	}
	n := node.Node
	if n.Text.Range == nil || n.Text.CoordinateSpace != ref.CoordinateSpace || lexicalJSON(n.Locator) != lexicalJSON(ref.Locator) || lexicalJSON(n.OriginalPosition) != lexicalJSON(ref.OriginalPosition) || n.CoordinatePrecision != ref.CoordinatePrecision {
		return "", "", lexicalError(409, "integrity_error")
	}
	if ref.Address.Target.Representation.Kind == "source_revision" {
		if meta.RootRef == nil || *meta.RootRef.ID != *ref.Address.Node.ID || ref.Assembly != "c1_root_subrange_exact_bytes" || ref.Range.StartByte < n.Text.Range.StartByte || ref.Range.EndByte > n.Text.Range.EndByte {
			return "", "", lexicalError(409, "integrity_error")
		}
	} else if n.Kind != "passage" || ref.Assembly != "existing_passage_full_range" || ref.Range.StartByte != n.Text.Range.StartByte || ref.Range.EndByte != n.Text.Range.EndByte {
		return "", "", lexicalError(409, "integrity_error")
	}
	if ref.CoordinateSpace == "derived_utf8" {
		if meta.RootRef == nil {
			return "", "", lexicalError(409, "integrity_error")
		}
		root, err := d.GetKnowledgeContentNode(ctx, owner, domain, entity.KnowledgeContentAddress{Target: ref.Address.Target, Node: *meta.RootRef})
		if err != nil || root.Node.Text.Range == nil || root.Node.Text.Range.SHA256 == nil || *root.Node.Text.Range.SHA256 != ref.ReadingSHA256 {
			return "", "", lexicalError(409, "integrity_error")
		}
	} else if ref.ReadingSHA256 != ref.Address.Target.Source.SHA256 {
		return "", "", lexicalError(409, "integrity_error")
	}
	window, err := d.ReadKnowledgeContentNode(ctx, owner, domain, entity.KnowledgeContentReadRequest{Address: ref.Address, StartByte: &ref.Range.StartByte, EndByte: &ref.Range.EndByte})
	if err != nil {
		return "", "", contentLexicalNormalize(err)
	}
	if ref.Range.SHA256 == nil || window.Range.SHA256 == nil || window.CoordinateSpace != ref.CoordinateSpace || window.Range.StartByte != ref.Range.StartByte || window.Range.EndByte != ref.Range.EndByte || *window.Range.SHA256 != *ref.Range.SHA256 || lexicalSHA([]byte(window.Text)) != *ref.Range.SHA256 || int64(len(window.Text)) != row.ByteLength {
		return "", "", lexicalError(409, "integrity_error")
	}
	current, err := d.contentLexicalCurrentness(ctx, owner, domain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{ref.Address.Target}})
	if err != nil {
		return "", "", err
	}
	return window.Text, current, nil
}

func (d *DatasetService) ReadKnowledgeContentLexical(ctx context.Context, owner, domain string, req entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error) {
	if _, _, err := d.lexicalContext(ctx, owner, domain, false); err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeContentVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if req.Evidence.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	policies, err := d.contentLexicalPolicies(ctx, owner, domain, false)
	if err != nil {
		return nil, err
	}
	_, manifest, err := d.contentLexicalPublished(ctx, owner, domain, req.Evidence.Index, false)
	if err != nil {
		return nil, err
	}
	for _, row := range manifest.Rows {
		if row.ID != req.Evidence.RowID {
			continue
		}
		expected := contentLexicalEvidence(req.Evidence.Index, row)
		if lexicalJSON(expected) != lexicalJSON(req.Evidence) {
			return nil, lexicalError(409, "integrity_error")
		}
		text, current, err := d.contentLexicalReadRow(ctx, owner, domain, row)
		if err != nil {
			return nil, err
		}
		enrichment, err := d.contentLexicalContext(ctx, owner, domain, manifest, row)
		if err != nil {
			return nil, err
		}
		_, latest, err := d.contentLexicalPublished(ctx, owner, domain, req.Evidence.Index, false)
		if err != nil {
			return nil, err
		}
		if lexicalJSON(latest) != lexicalJSON(manifest) {
			return nil, lexicalError(409, "integrity_error")
		}
		finalText, finalCurrent, err := d.contentLexicalReadRow(ctx, owner, domain, row)
		if err != nil {
			return nil, err
		}
		if finalText != text {
			return nil, lexicalError(409, "source_changed")
		}
		current = finalCurrent
		latestPolicies, err := d.contentLexicalPolicies(ctx, owner, domain, false)
		if err != nil {
			return nil, err
		}
		if latestPolicies != policies {
			return nil, lexicalError(409, "policy_changed")
		}
		return &entity.KnowledgeContentLexicalReadData{Evidence: expected, Text: text, Encoding: "utf-8", ByteLength: len(text), Currentness: current, Complete: true, Context: *enrichment}, nil
	}
	return nil, lexicalError(404, "not_found")
}

func (d *DatasetService) contentLexicalExpectedRows(ctx context.Context, manifest *contentLexicalManifest) ([]map[string]any, error) {
	prepared, err := d.prepareKnowledgeContentLexical(ctx, manifest.Owner, manifest.Domain, manifest.Selection, true)
	if err != nil {
		return nil, err
	}
	if prepared.Policies != manifest.Policies || prepared.ProfileSHA256 != manifest.ProfileSHA256 || prepared.ProducerSHA256 != manifest.ProducerSHA256 || prepared.SelectionSHA256 != manifest.SelectionSHA256 || lexicalJSON(prepared.Rows) != lexicalJSON(manifest.Rows) || lexicalJSON(prepared.Coverage) != lexicalJSON(manifest.Coverage) {
		return nil, lexicalError(409, "integrity_error")
	}
	rows := make([]map[string]any, len(manifest.Rows))
	for i, row := range manifest.Rows {
		input := prepared.Inputs[i]
		rows[i] = map[string]any{"id": row.ID, "kb_id": manifest.Domain, "doc_id": input.SourceID, "knowledge_binding_kwd": row.Binding, "knowledge_ref": input.Reference, "content_with_weight": input.Text, "available_int": 0}
	}
	return rows, nil
}
