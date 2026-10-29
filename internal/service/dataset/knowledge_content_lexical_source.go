package dataset

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

type contentLexicalSnapshot struct {
	Representation entity.KnowledgeContentRepresentation
	ReadingSHA256  string
	Body           string
	Name           string
	Passages       []entity.KnowledgeContentNode
	Root           *entity.KnowledgeContentNode
	Unsearchable   int64
	ManifestSHA256 *string
	TypedMembers   []entity.KnowledgeContentLexicalTypedMember
}

// contentLexicalSnapshot loads each reading body once. The existing owning
// binder/validator checks the entire fixed tree before any units reach indexing.
func (d *DatasetService) contentLexicalSnapshot(ctx context.Context, owner, domain string, target entity.KnowledgeContentTarget, language, producer string) (*contentLexicalSnapshot, error) {
	meta, err := d.GetKnowledgeContentRepresentation(ctx, owner, domain, target)
	if err != nil {
		return nil, contentLexicalNormalize(err)
	}
	if meta.RootRef == nil || meta.Ref.ID == nil || meta.Source == nil {
		return nil, lexicalError(415, "unsupported_representation")
	}
	if meta.Ref.Kind == "source_revision" || meta.Ref.Kind == "structure_view" {
		return d.contentLexicalSourceText(ctx, owner, domain, target, meta, language, producer)
	}
	if meta.CoordinateSpace != "derived_utf8" {
		return nil, lexicalError(415, "unsupported_representation")
	}
	protocol, maxNodes := entity.KnowledgeParsedProtocol, 32768
	if meta.Ref.Kind == "document_representation" {
		protocol, maxNodes = documentTaskProtocol, 8192
	} else if meta.Ref.Kind != "parsed_representation" {
		return nil, lexicalError(415, "unsupported_representation")
	}
	var row entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.WithContext(ctx).Select("reading_text", "manifest").Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", *meta.Ref.ID, owner, domain, protocol).Take(&row).Error; err != nil {
		return nil, contentLexicalNormalize(err)
	}
	if len(row.ReadingText) > 4<<20 || !utf8.ValidString(row.ReadingText) {
		return nil, lexicalError(409, "integrity_error")
	}
	var nodes []entity.KnowledgeDocumentNodeRecord
	if err := dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND representation_id = ?", owner, domain, *meta.Ref.ID).Limit(maxNodes + 1).Find(&nodes).Error; err != nil {
		return nil, contentLexicalNormalize(err)
	}
	if len(nodes) < 1 || len(nodes) > maxNodes || meta.NodeCount == nil || int64(len(nodes)) != *meta.NodeCount {
		return nil, lexicalError(409, "integrity_error")
	}
	snapshot := &contentLexicalSnapshot{Representation: *meta, Body: row.ReadingText, ReadingSHA256: lexicalSHA([]byte(row.ReadingText)), Passages: []entity.KnowledgeContentNode{}}
	if protocol == entity.KnowledgeParsedProtocol {
		err = d.contentLexicalParsedNodes(ctx, owner, domain, snapshot, row.Manifest, nodes)
	} else {
		err = d.contentLexicalDocumentNodes(ctx, owner, domain, snapshot, row.Manifest, nodes)
	}
	if err != nil {
		return nil, contentLexicalNormalize(err)
	}
	if _, err := d.GetKnowledgeContentRepresentation(ctx, owner, domain, target); err != nil {
		return nil, contentLexicalNormalize(err)
	}
	return snapshot, nil
}

func (d *DatasetService) contentLexicalSourceText(ctx context.Context, owner, domain string, target entity.KnowledgeContentTarget, meta *entity.KnowledgeContentRepresentation, language, producer string) (*contentLexicalSnapshot, error) {
	old := entity.KnowledgeLexicalTarget{Source: entity.KnowledgeLexicalSourceRef{DomainID: target.Source.DomainID, SourceID: target.Source.SourceID, RevisionID: target.Source.RevisionID, SHA256: target.Source.SHA256}, Representation: entity.KnowledgeLexicalRepresentationRef{Kind: target.Representation.Kind, ID: *target.Representation.ID, ProducerSHA256: target.Representation.ProducerSHA256}}
	snapshot := &contentLexicalSnapshot{Representation: *meta, ReadingSHA256: target.Source.SHA256, Passages: []entity.KnowledgeContentNode{}}
	if target.Representation.Kind == "source_revision" {
		source, body, err := d.lexicalSource(ctx, owner, domain, old)
		if err != nil {
			return nil, err
		}
		root, err := d.GetKnowledgeContentNode(ctx, owner, domain, entity.KnowledgeContentAddress{Target: target, Node: *meta.RootRef})
		if err != nil {
			return nil, contentLexicalNormalize(err)
		}
		snapshot.Body, snapshot.Name, snapshot.Root = string(body), source.RelativePath, &root.Node
	} else {
		units, coverage, _, err := d.lexicalUnits(ctx, owner, domain, language, producer, entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{old}})
		if err != nil {
			return nil, err
		}
		if !coverage.Complete {
			return nil, lexicalError(409, "coverage_gap")
		}
		sort.Slice(units, func(i, j int) bool { return units[i].Row.Member.Range.Start < units[j].Row.Member.Range.Start })
		var body strings.Builder
		for _, unit := range units {
			if int64(body.Len()) != unit.Row.Member.Range.Start {
				return nil, lexicalError(409, "integrity_error")
			}
			body.WriteString(unit.Text)
			address := entity.KnowledgeContentAddress{Target: target, Node: contentRef("r1_node", unit.Row.Member.Node.ID)}
			node, err := d.GetKnowledgeContentNode(ctx, owner, domain, address)
			if err != nil {
				return nil, contentLexicalNormalize(err)
			}
			snapshot.Passages = append(snapshot.Passages, node.Node)
			snapshot.Name = unit.Name
		}
		snapshot.Body = body.String()
		if int64(body.Len()) != coverage.ExpectedBytes {
			return nil, lexicalError(409, "integrity_error")
		}
	}
	if lexicalSHA([]byte(snapshot.Body)) != target.Source.SHA256 || !utf8.ValidString(snapshot.Body) {
		return nil, lexicalError(409, "integrity_error")
	}
	if _, err := d.GetKnowledgeContentRepresentation(ctx, owner, domain, target); err != nil {
		return nil, contentLexicalNormalize(err)
	}
	return snapshot, nil
}

func (d *DatasetService) contentLexicalParsedNodes(ctx context.Context, owner, domain string, snapshot *contentLexicalSnapshot, rawManifest string, rows []entity.KnowledgeDocumentNodeRecord) error {
	state, err := d.parsedReadRepresentation(ctx, owner, domain, *snapshot.Representation.Ref.ID)
	if err != nil {
		return err
	}
	rep := state.Representation
	snapshot.ManifestSHA256 = contentString(rep.ManifestSHA256)
	source, err := d.parsedAuthorizedSource(ctx, owner, domain, rep.Source)
	if err != nil {
		return err
	}
	snapshot.Name = source.RelativePath
	var manifest []entity.KnowledgeParsedManifestEntry
	if entity.KnowledgeParsedDecode([]byte(rawManifest), &manifest) != nil || lexicalJSON(manifest) != lexicalJSON(state.Manifest) || rep.ReadingByteLength != int64(len(snapshot.Body)) || rep.ReadingTextSHA256 != snapshot.ReadingSHA256 {
		return lexicalError(409, "integrity_error")
	}
	nodes := make([]entity.KnowledgeParsedNode, len(manifest))
	seen := make(map[int64]bool, len(rows))
	for _, row := range rows {
		var node entity.KnowledgeParsedNode
		if entity.KnowledgeParsedDecode([]byte(row.Payload), &node) != nil {
			return lexicalError(409, "integrity_error")
		}
		i := node.Payload.LocalIndex
		if i < 0 || i >= int64(len(nodes)) || seen[i] || node.NodeID != row.ID || row.Ordinal != node.Payload.Ordinal || node.RepresentationID != *rep.Ref.ID {
			return lexicalError(409, "integrity_error")
		}
		parent := ""
		if node.Payload.ParentLocalIndex != nil {
			if *node.Payload.ParentLocalIndex != 0 {
				return lexicalError(409, "integrity_error")
			}
			parent = *rep.RootRef.ID
		}
		if row.ParentID != parent {
			return lexicalError(409, "integrity_error")
		}
		seen[i], nodes[i] = true, node
	}
	if len(seen) != len(nodes) || entity.KnowledgeParsedValidateBound(rep, snapshot.Body, nodes, manifest) != nil {
		return lexicalError(409, "integrity_error")
	}
	for _, node := range nodes {
		if contentLexicalUnsearchable(node.Payload.Text.Status) {
			snapshot.Unsearchable++
		}
		projected, err := parsedContentNode(rep, &node)
		if err != nil {
			return err
		}
		snapshot.TypedMembers = append(snapshot.TypedMembers, entity.KnowledgeContentLexicalTypedMember{Node: *projected, PayloadSHA256: node.PayloadSHA256, TextAssembly: "existing_reading_range"})
		if node.Payload.Kind == "passage" {
			snapshot.Passages = append(snapshot.Passages, *projected)
		}
	}
	return nil
}

func (d *DatasetService) contentLexicalDocumentNodes(ctx context.Context, owner, domain string, snapshot *contentLexicalSnapshot, rawManifest string, rows []entity.KnowledgeDocumentNodeRecord) error {
	rep, err := d.documentReadRepresentation(ctx, owner, domain, *snapshot.Representation.Ref.ID)
	if err != nil {
		return err
	}
	manifest, err := documentDecode[entity.KnowledgeDocumentStructureManifest](rawManifest)
	if err != nil || rep.ReadingByteLength != int64(len(snapshot.Body)) || rep.ReadingTextSHA256 != snapshot.ReadingSHA256 || len(manifest.Nodes) != len(rows) {
		return lexicalError(409, "integrity_error")
	}
	manifestSHA, err := entity.KnowledgeDocumentManifestDigest(manifest.Nodes)
	if err != nil || manifestSHA != rep.StructureManifestSHA256 {
		return lexicalError(409, "integrity_error")
	}
	snapshot.ManifestSHA256 = contentString(manifestSHA)
	snapshot.Name = rep.SourceSnapshot.RelativePath
	byID := make(map[string]entity.KnowledgeDocumentNodeRecord, len(rows))
	for _, row := range rows {
		if _, exists := byID[row.ID]; exists {
			return lexicalError(409, "integrity_error")
		}
		byID[row.ID] = row
	}
	result := parser.DocumentStructureResult{ReadingText: snapshot.Body, Payloads: []entity.KnowledgeDocumentNodePayload{}, PageCount: rep.PageCount, SlideCount: rep.SlideCount, UnsupportedBlockCount: rep.UnsupportedBlockCount, Quality: rep.Quality, TextCoverage: rep.TextCoverage}
	for _, entry := range manifest.Nodes {
		id, err := entity.KnowledgeDocumentNodeDigest(rep.RepresentationID, rep.Original.SourceRevisionID, entry.NodePayloadSHA256)
		if err != nil {
			return lexicalError(409, "integrity_error")
		}
		row, exists := byID[id]
		if !exists {
			return lexicalError(409, "integrity_error")
		}
		node, err := documentDecode[entity.KnowledgeDocumentNode](row.Payload)
		if err != nil {
			return err
		}
		result.Payloads = append(result.Payloads, node.Payload)
	}
	fixed := &documentInputs{Original: rep.Original, Snapshot: rep.SourceSnapshot, FoundationPolicyVersion: rep.FoundationPolicyVersion, DocumentPolicyVersion: rep.DocumentPolicyVersion, Profile: entity.KnowledgeDocumentProfile{ProfileID: rep.ProfileID, Format: rep.Format, Configuration: rep.Configuration, Producer: rep.Producer, ProducerSHA256: rep.ProducerSHA256}}
	bound, err := documentBind(owner, domain, fixed, result)
	if err != nil || bound.Representation.RepresentationID != rep.RepresentationID || bound.Representation.RootDocumentNodeID != rep.RootDocumentNodeID || bound.Representation.StructureManifestSHA256 != rep.StructureManifestSHA256 {
		return lexicalError(409, "integrity_error")
	}
	for _, expected := range bound.Nodes {
		stored, exists := byID[expected.ID]
		if !exists || stored.ParentID != expected.ParentID || stored.Ordinal != expected.Ordinal || stored.Payload != expected.Payload {
			return lexicalError(409, "integrity_error")
		}
		node, err := documentDecode[entity.KnowledgeDocumentNode](expected.Payload)
		if err != nil {
			return err
		}
		if contentLexicalUnsearchable(node.Payload.TextStatus) {
			snapshot.Unsearchable++
		}
		projected, err := contentProjectR2(rep, node)
		if err != nil {
			return err
		}
		snapshot.TypedMembers = append(snapshot.TypedMembers, entity.KnowledgeContentLexicalTypedMember{Node: *projected, PayloadSHA256: node.NodePayloadSHA256, TextAssembly: "existing_reading_range"})
		if node.Payload.Kind == "passage" {
			snapshot.Passages = append(snapshot.Passages, *projected)
		}
	}
	return nil
}

func contentLexicalUnsearchable(status string) bool {
	return status == "unsupported" || status == "unsupported_or_requires_ocr" || status == "unavailable"
}
