package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"unicode/utf8"

	"ragflow/internal/entity"
)

func accessEvidence(viewID string, row contentLexicalManifestRow) entity.KnowledgeAccessEvidenceRef {
	return entity.KnowledgeAccessEvidenceRef{ContractVersion: entity.KnowledgeAccessVersion, ViewID: viewID, EvidenceID: lexicalSHA([]byte(accessJSON([]any{"knowledge-access-evidence/0.3.0", viewID, row.ID, row.Binding, row.ReferenceSHA256})))}
}
func accessUnitLocator(verified *verifiedAccessActor, row contentLexicalManifestRow) (entity.KnowledgeAccessUnitLocator, error) {
	for i, rows := range verified.ByTarget {
		for ordinal, candidate := range rows {
			if candidate.ID == row.ID {
				var next *int64
				if ordinal+1 < len(rows) {
					next = accessPtr(int64(ordinal + 1))
				}
				return entity.KnowledgeAccessUnitLocator{ViewID: verified.View.ID, TargetIndex: int64(i), UnitOrdinal: int64(ordinal), TargetUnitCount: int64(len(rows)), NextUnitOrdinal: next}, nil
			}
		}
	}
	return entity.KnowledgeAccessUnitLocator{}, accessError(409, "integrity_error")
}

func (d *DatasetService) accessRead(ctx context.Context, actor, requestID string, req entity.KnowledgeAccessReadRequest) (int, []byte, error) {
	var selector struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(req.Selector, &selector) != nil {
		return 0, nil, accessError(400, "invalid_request")
	}
	var unit entity.KnowledgeAccessUnitReadSelector
	var evidence entity.KnowledgeAccessEvidenceReadSelector
	viewID := ""
	if selector.Kind == "unit" {
		if entity.KnowledgeAccessDecode(req.Selector, "UnitReadSelector", &unit) != nil {
			return 0, nil, accessError(400, "invalid_request")
		}
		viewID = unit.ViewID
	} else {
		if entity.KnowledgeAccessDecode(req.Selector, "EvidenceReadSelector", &evidence) != nil {
			return 0, nil, accessError(400, "invalid_request")
		}
		viewID = evidence.EvidenceRef.ViewID
	}
	verified, err := d.accessOpenView(ctx, actor, viewID, "read", requestID)
	if err != nil {
		return 0, nil, err
	}
	var row *contentLexicalManifestRow
	if selector.Kind == "unit" {
		if unit.TargetIndex >= int64(len(verified.ByTarget)) || unit.UnitOrdinal >= int64(len(verified.ByTarget[unit.TargetIndex])) {
			return 0, nil, accessError(400, "invalid_request")
		}
		copy := verified.ByTarget[unit.TargetIndex][unit.UnitOrdinal]
		row = &copy
	} else {
		for _, candidate := range verified.Rows {
			if accessEvidence(viewID, candidate) == evidence.EvidenceRef {
				if row != nil {
					return 0, nil, accessError(409, "integrity_error")
				}
				copy := candidate
				row = &copy
			}
		}
		if row == nil {
			return 0, nil, accessError(404, "not_found")
		}
	}
	text, citation, typed, err := d.accessReadRow(ctx, verified, *row, nil)
	if err != nil {
		return 0, nil, err
	}
	locator, err := accessUnitLocator(verified, *row)
	if err != nil {
		return 0, nil, err
	}
	data := entity.KnowledgeAccessReadData{EvidenceRef: accessEvidence(viewID, *row), Citation: citation, Text: text, ByteLength: int64(len(text)), Encoding: "utf-8", Complete: true, TypedContext: typed, Attachments: "not_exposed", ParentExpansion: "disabled", UnitLocator: locator}
	encoded, err := accessEncode("read", requestID, data)
	if err != nil {
		return 0, nil, err
	}
	if err = d.accessFinalPermit(ctx, verified, "read", requestID); err != nil {
		return 0, nil, err
	}
	return 200, encoded, nil
}

func (d *DatasetService) accessReadRow(ctx context.Context, verified *verifiedAccessActor, row contentLexicalManifestRow, cache map[string]*contentLexicalSnapshot) (string, entity.KnowledgeAccessCitation, entity.KnowledgeAccessTypedContext, error) {
	invalid := func() (string, entity.KnowledgeAccessCitation, entity.KnowledgeAccessTypedContext, error) {
		return "", entity.KnowledgeAccessCitation{}, entity.KnowledgeAccessTypedContext{}, accessError(409, "integrity_error")
	}
	ref := row.Reference
	target := ref.Address.Target
	key := accessJSON(target)
	allowed := false
	for _, selected := range verified.Scope.Targets {
		allowed = allowed || key == accessJSON(accessContentTarget(selected))
	}
	if !allowed {
		return invalid()
	}
	// No content loader runs before the exact target intersection above.
	proofCtx := accessProofContext(ctx, accessDB(ctx))
	if err := d.accessDependencyClosure(proofCtx, verified.KB, verified.Scope.Targets); err != nil {
		return "", entity.KnowledgeAccessCitation{}, entity.KnowledgeAccessTypedContext{}, err
	}
	var snapshot *contentLexicalSnapshot
	if cache != nil {
		snapshot = cache[key]
	}
	if snapshot == nil {
		meta, err := d.contentStoredRepresentation(proofCtx, verified.KB, target)
		if err != nil {
			return "", entity.KnowledgeAccessCitation{}, entity.KnowledgeAccessTypedContext{}, err
		}
		snapshot, err = d.contentLexicalStoredSnapshot(proofCtx, verified.KB, target, meta, verified.Capture.Policies.Language, verified.Manifest.ProducerSHA256)
		if err != nil {
			return "", entity.KnowledgeAccessCitation{}, entity.KnowledgeAccessTypedContext{}, err
		}
		if cache != nil {
			cache[key] = snapshot
		}
	}
	if snapshot.ReadingSHA256 != ref.ReadingSHA256 || len(snapshot.Body) > 4<<20 || !utf8.ValidString(snapshot.Body) || !knowledgeRange([]byte(snapshot.Body), ref.Range.StartByte, ref.Range.EndByte) || ref.Range.SHA256 == nil || ref.Range.EndByte-ref.Range.StartByte != row.ByteLength || row.ByteLength < 1 || row.ByteLength > 8192 {
		return invalid()
	}
	var node *entity.KnowledgeContentNode
	if snapshot.Root != nil && snapshot.Root.Ref.ID != nil && *snapshot.Root.Ref.ID == *ref.Address.Node.ID {
		node = snapshot.Root
	}
	for i := range snapshot.Passages {
		candidate := &snapshot.Passages[i]
		if candidate.Ref.ID != nil && *candidate.Ref.ID == *ref.Address.Node.ID {
			if node != nil {
				return invalid()
			}
			node = candidate
		}
	}
	if node == nil || node.Text.Range == nil || node.Text.CoordinateSpace != ref.CoordinateSpace || accessJSON(node.Locator) != accessJSON(ref.Locator) || accessJSON(node.OriginalPosition) != accessJSON(ref.OriginalPosition) || node.CoordinatePrecision != ref.CoordinatePrecision {
		return invalid()
	}
	if target.Representation.Kind == "source_revision" {
		if ref.Assembly != "c1_root_subrange_exact_bytes" || ref.Range.StartByte < node.Text.Range.StartByte || ref.Range.EndByte > node.Text.Range.EndByte {
			return invalid()
		}
	} else {
		if node.Kind != "passage" || ref.Assembly != "existing_passage_full_range" || node.Text.Range.StartByte != ref.Range.StartByte || node.Text.Range.EndByte != ref.Range.EndByte {
			return invalid()
		}
	}
	text := snapshot.Body[ref.Range.StartByte:ref.Range.EndByte]
	if lexicalSHA([]byte(text)) != *ref.Range.SHA256 || row.ReferenceSHA256 != lexicalSHA([]byte(accessJSON(ref))) {
		return invalid()
	}
	var source entity.KnowledgeSourceRevision
	if err := contentProofStore(proofCtx).Select("id", "relative_path", "original_filename").Where("id = ? AND domain_id = ? AND document_id = ? AND sha256 = ?", target.Source.RevisionID, verified.KB.ID, target.Source.SourceID, target.Source.SHA256).Take(&source).Error; err != nil {
		return "", entity.KnowledgeAccessCitation{}, entity.KnowledgeAccessTypedContext{}, err
	}
	name := path.Base(source.RelativePath)
	if source.OriginalFilename != "" {
		name = path.Base(source.OriginalFilename)
	}
	if name == "" || name == "." || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 256 {
		return invalid()
	}
	citation := entity.KnowledgeAccessCitation{Target: accessTarget(target), NodeID: *ref.Address.Node.ID, CoordinateSpace: ref.CoordinateSpace, Range: entity.KnowledgeAccessRange{StartByte: ref.Range.StartByte, EndByte: ref.Range.EndByte, SHA256: *ref.Range.SHA256}, DisplayName: name, PositionLabel: fmt.Sprintf("UTF-8 bytes %d–%d", ref.Range.StartByte, ref.Range.EndByte), RevisionBinding: "fixed"}
	typed, err := accessTypedContext(snapshot, row)
	if err != nil {
		return "", entity.KnowledgeAccessCitation{}, entity.KnowledgeAccessTypedContext{}, err
	}
	return text, citation, typed, nil
}

func accessTypedContext(snapshot *contentLexicalSnapshot, row contentLexicalManifestRow) (entity.KnowledgeAccessTypedContext, error) {
	value := entity.KnowledgeAccessTypedContext{Members: []entity.KnowledgeAccessTypedMember{}, MembersComplete: true, OmissionReasons: []string{}}
	reasons := map[string]bool{}
	for _, member := range snapshot.TypedMembers {
		node := member.Node
		r := node.Text.Range
		if contentLexicalUnsearchable(node.Text.Status) {
			reasons["unsearchable_member"] = true
			continue
		}
		if r == nil || r.SHA256 == nil || node.Ref.ID == nil || r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > int64(len(snapshot.Body)) {
			return value, accessError(409, "integrity_error")
		}
		if r.StartByte < row.Reference.Range.StartByte || r.EndByte > row.Reference.Range.EndByte {
			reasons["whole_member_outside_evidence"] = true
			continue
		}
		if !knowledgeRange([]byte(snapshot.Body), r.StartByte, r.EndByte) || lexicalSHA([]byte(snapshot.Body[r.StartByte:r.EndByte])) != *r.SHA256 || !entity.ValidKnowledgeLexicalHash(member.PayloadSHA256) {
			return value, accessError(409, "integrity_error")
		}
		if len(value.Members) >= 32 {
			reasons["member_budget"] = true
			continue
		}
		status := "available"
		if r.StartByte == r.EndByte {
			status = "empty"
		}
		item := entity.KnowledgeAccessTypedMember{NodeID: *node.Ref.ID, Kind: node.Kind, Range: entity.KnowledgeAccessRange{StartByte: r.StartByte, EndByte: r.EndByte, SHA256: *r.SHA256}, TextStatus: status, PayloadSHA256: member.PayloadSHA256}
		candidate := append(append([]entity.KnowledgeAccessTypedMember(nil), value.Members...), item)
		if len(accessJSON(candidate)) > 65536 {
			reasons["metadata_budget"] = true
			continue
		}
		value.Members = candidate
		reasons["typed_payload_not_exposed_in_access_v03"] = true
	}
	for reason := range reasons {
		value.OmissionReasons = append(value.OmissionReasons, reason)
	}
	sort.Strings(value.OmissionReasons)
	value.MembersComplete = len(value.OmissionReasons) == 0
	return value, nil
}
