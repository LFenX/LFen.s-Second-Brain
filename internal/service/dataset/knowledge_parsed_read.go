package dataset

import (
	"context"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type parsedReadState struct {
	Representation entity.KnowledgeParsedRepresentation
	Manifest       []entity.KnowledgeParsedManifestEntry
}

func (d *DatasetService) parsedReadRepresentation(ctx context.Context, owner, domain, id string) (*parsedReadState, error) {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return nil, err
	}
	if !isKnowledgeID(id, 64) {
		return nil, contentError(404, "not_found")
	}
	var row entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.WithContext(ctx).Omit("reading_text").Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", id, owner, domain, entity.KnowledgeParsedProtocol).Take(&row).Error; err != nil {
		return nil, err
	}
	state := &parsedReadState{}
	if entity.KnowledgeParsedDecode([]byte(row.Payload), &state.Representation) != nil || entity.KnowledgeParsedDecode([]byte(row.Manifest), &state.Manifest) != nil {
		return nil, contentError(409, "content_integrity")
	}
	rep := state.Representation
	calculated, err := entity.KnowledgeParsedRepresentationDigest(rep)
	manifestSHA, manifestErr := entity.KnowledgeParsedManifestDigest(state.Manifest)
	if err != nil || manifestErr != nil || calculated != id || rep.Ref.ID == nil || *rep.Ref.ID != id || manifestSHA != rep.ManifestSHA256 || rep.NodeCount != int64(len(state.Manifest)) || rep.Source.DomainID != domain || rep.Source.SourceID != row.SourceID || rep.Source.RevisionID != row.SourceRevisionID {
		return nil, contentError(409, "content_integrity")
	}
	root, err := entity.KnowledgeParsedNodeDigest(id, 0, state.Manifest[0].PayloadSHA256)
	if err != nil || rep.RootRef.Kind != "parsed_node" || rep.RootRef.ID == nil || *rep.RootRef.ID != root || rep.RootRef.ItemIndex != nil {
		return nil, contentError(409, "content_integrity")
	}
	source, err := d.parsedAuthorizedSource(ctx, owner, domain, rep.Source)
	if err != nil {
		return nil, err
	}
	if !contentSameSource(rep.Source, source) {
		return nil, contentError(409, "content_integrity")
	}
	return state, nil
}

func parsedReadNode(ctx context.Context, owner, domain string, state *parsedReadState, id string) (*entity.KnowledgeParsedNode, error) {
	if !isKnowledgeID(id, 64) {
		return nil, contentError(404, "not_found")
	}
	rep := state.Representation
	var row entity.KnowledgeDocumentNodeRecord
	if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND representation_id = ?", id, owner, domain, *rep.Ref.ID).Take(&row).Error; err != nil {
		return nil, err
	}
	var node entity.KnowledgeParsedNode
	if entity.KnowledgeParsedDecode([]byte(row.Payload), &node) != nil || node.RepresentationID != *rep.Ref.ID || node.NodeID != id || node.Payload.LocalIndex < 0 || node.Payload.LocalIndex >= int64(len(state.Manifest)) {
		return nil, contentError(409, "content_integrity")
	}
	p := node.Payload
	entry := state.Manifest[p.LocalIndex]
	sha, err := entity.KnowledgeParsedPayloadDigest(p)
	calculated, idErr := entity.KnowledgeParsedNodeDigest(*rep.Ref.ID, p.LocalIndex, node.PayloadSHA256)
	if err != nil || idErr != nil || sha != node.PayloadSHA256 || entry.PayloadSHA256 != sha || calculated != id || p.Text.Range == nil || p.Text.Range.SHA256 == nil || p.Text.Validate("derived_utf8") != nil || p.Text.Range.EndByte > rep.ReadingByteLength || row.Ordinal != p.Ordinal {
		return nil, contentError(409, "content_integrity")
	}
	if p.LocalIndex == 0 {
		if p.ParentLocalIndex != nil || entry.ParentLocalIndex != nil || row.ParentID != "" || p.Kind != "document" {
			return nil, contentError(409, "content_integrity")
		}
	} else if p.ParentLocalIndex == nil || *p.ParentLocalIndex != 0 || entry.ParentLocalIndex == nil || *entry.ParentLocalIndex != 0 || row.ParentID != *rep.RootRef.ID {
		return nil, contentError(409, "content_integrity")
	}
	return &node, nil
}

func parsedContentNode(rep entity.KnowledgeParsedRepresentation, node *entity.KnowledgeParsedNode) (*entity.KnowledgeContentNode, error) {
	p := node.Payload
	value := &entity.KnowledgeContentNode{Ref: contentRef("parsed_node", node.NodeID), Kind: p.Kind, Name: p.Name, Ordinal: contentInt(p.Ordinal), Locator: p.Locator, OriginalPosition: p.OriginalPosition, CoordinatePrecision: p.CoordinatePrecision, Text: p.Text, Attributes: p.Attributes}
	if p.ParentLocalIndex != nil {
		parent := rep.RootRef
		value.ParentRef = &parent
	}
	target := entity.KnowledgeContentTarget{Source: rep.Source, Representation: rep.Ref}
	if value.Validate(&target) != nil {
		return nil, contentError(409, "content_integrity")
	}
	return value, nil
}

func (d *DatasetService) parsedReadContent(ctx context.Context, owner, domain string, address entity.KnowledgeContentAddress, start, end *int64) (*entity.KnowledgeContentWindow, error) {
	state, err := d.parsedReadRepresentation(ctx, owner, domain, *address.Target.Representation.ID)
	if err != nil {
		return nil, err
	}
	node, err := parsedReadNode(ctx, owner, domain, state, *address.Node.ID)
	if err != nil {
		return nil, err
	}
	var row entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.WithContext(ctx).Select("reading_text").Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", *state.Representation.Ref.ID, owner, domain, entity.KnowledgeParsedProtocol).Take(&row).Error; err != nil {
		return nil, err
	}
	body := row.ReadingText
	rep := state.Representation
	p := node.Payload
	if !utf8.ValidString(body) || int64(len(body)) != rep.ReadingByteLength || directorySHA([]byte(body)) != rep.ReadingTextSHA256 || !documentRangeValid(entity.KnowledgeDocumentReadingRange{StartByte: p.Text.Range.StartByte, EndByte: p.Text.Range.EndByte, TextSHA256: *p.Text.Range.SHA256}, body) {
		return nil, contentError(409, "content_integrity")
	}
	a, b := p.Text.Range.StartByte, p.Text.Range.EndByte
	if start != nil {
		a = *start
	}
	if end != nil {
		b = *end
	} else {
		b = min(b, a+65536)
		for b > a && b >= 0 && b < int64(len(body)) && !utf8.RuneStart(body[b]) {
			b--
		}
	}
	if a < p.Text.Range.StartByte || b > p.Text.Range.EndByte || b-a > 65536 || !knowledgeRange([]byte(body), a, b) {
		return nil, contentError(400, "invalid_request")
	}
	fragment := body[a:b]
	complete := b == p.Text.Range.EndByte
	var next *int64
	if !complete {
		next = contentInt(b)
	}
	if _, err := d.parsedReadRepresentation(ctx, owner, domain, *rep.Ref.ID); err != nil {
		return nil, err
	}
	return &entity.KnowledgeContentWindow{Version: entity.KnowledgeContentVersion, Address: address, Locator: p.Locator, CoordinateSpace: "derived_utf8", Range: entity.KnowledgeContentRange{StartByte: a, EndByte: b, SHA256: contentString(directorySHA([]byte(fragment)))}, TextStatus: p.Text.Status, TextReason: p.Text.Reason, Text: fragment, Complete: complete, NextStartByte: next}, nil
}
