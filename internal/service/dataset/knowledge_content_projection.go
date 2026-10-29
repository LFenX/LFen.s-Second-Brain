package dataset

import (
	"encoding/json"

	"ragflow/internal/entity"
)

func contentString(value string) *string { return &value }
func contentInt(value int64) *int64      { return &value }

func contentRef(kind, id string) entity.KnowledgeContentNodeRef {
	return entity.KnowledgeContentNodeRef{Kind: kind, ID: contentString(id)}
}

func contentParent(kind string, id *string) *entity.KnowledgeContentNodeRef {
	if id == nil {
		return nil
	}
	value := contentRef(kind, *id)
	return &value
}

func contentTarget(source entity.KnowledgeContentSourceRef, kind, id string, producer *string) entity.KnowledgeContentTarget {
	ref := entity.KnowledgeContentRepresentationRef{Kind: kind, ID: contentString(id)}
	if producer != nil {
		ref.ProducerSHA256 = contentString(*producer)
	}
	return entity.KnowledgeContentTarget{Source: source, Representation: ref}
}

func contentSource(source *entity.KnowledgeSourceRevision) entity.KnowledgeContentSourceRef {
	return entity.KnowledgeContentSourceRef{DomainID: source.DomainID, SourceID: source.DocumentID, RevisionID: source.ID, SHA256: source.SHA256}
}

func contentSourceText(start, end int64) entity.KnowledgeContentText {
	status := "available"
	if start == end {
		status = "empty"
	}
	return entity.KnowledgeContentText{CoordinateSpace: "source_utf8", Range: &entity.KnowledgeContentRange{StartByte: start, EndByte: end}, Status: status}
}

func contentProjectC1(source *entity.KnowledgeSourceRevision, node *entity.KnowledgeNode) (*entity.KnowledgeContentNode, error) {
	if source.RepresentationKind == "original_only" || source.RepresentationKind == "web_capture" || node.DomainID != source.DomainID || node.SourceRevisionID != source.ID || node.StartByte < 0 || node.EndByte > source.ByteLength {
		return nil, contentError(409, "content_integrity")
	}
	target := contentTarget(contentSource(source), "source_revision", source.ID, nil)
	value := &entity.KnowledgeContentNode{
		Ref: contentRef("c1_node", node.ID), ParentRef: contentParent("c1_node", node.ParentID), Kind: node.Kind, Name: node.Name,
		Locator:             entity.KnowledgeContentLocator{Kind: "source_utf8", SourceUTF8: &entity.KnowledgeContentSourceLocator{StartByte: node.StartByte, EndByte: node.EndByte, StartLine: node.StartLine, EndLine: node.EndLine}},
		CoordinatePrecision: "source_bytes", Text: contentSourceText(node.StartByte, node.EndByte),
		Attributes: entity.KnowledgeContentAttributes{Source: &entity.KnowledgeContentSourceAttributes{LogicalID: node.LogicalID, LogicalKey: node.LogicalKey, StructureVersion: node.StructureVersion}},
	}
	if value.Validate(&target) != nil {
		return nil, contentError(409, "content_integrity")
	}
	return value, nil
}

func contentProjectR1(source *entity.KnowledgeSourceRevision, view *entity.KnowledgeStructureView, node *entity.KnowledgeStructureNode) (*entity.KnowledgeContentNode, error) {
	member, err := structureMember(view, source.DocumentID)
	if err != nil || view.DomainID != source.DomainID || node.ViewID != view.ViewID || node.SourceID != source.DocumentID || node.SourceRevisionID != source.ID || node.ProducerSHA256 != view.ProducerSHA256 || node.CoordinateSpace != "source_utf8" || member.SourceRevisionID != source.ID || member.SHA256 != source.SHA256 || node.StartByte < 0 || node.EndByte > source.ByteLength {
		return nil, contentError(409, "content_integrity")
	}
	target := contentTarget(contentSource(source), "structure_view", view.ViewID, &view.ProducerSHA256)
	classification := node.Classification
	if classification.GroupID != nil {
		classification.GroupID = contentString(*classification.GroupID)
	}
	value := &entity.KnowledgeContentNode{
		Ref: contentRef("r1_node", node.StructureNodeID), ParentRef: contentParent("r1_node", node.ParentID), Kind: node.Kind, Name: node.Name,
		Locator:             entity.KnowledgeContentLocator{Kind: "source_utf8", SourceUTF8: &entity.KnowledgeContentSourceLocator{StartByte: node.StartByte, EndByte: node.EndByte, StartLine: node.StartLine, EndLine: node.EndLine}},
		CoordinatePrecision: "source_bytes", Text: contentSourceText(node.StartByte, node.EndByte),
		Attributes: entity.KnowledgeContentAttributes{Structure: &entity.KnowledgeContentStructureAttributes{LogicalNodeID: node.LogicalNodeID, LogicalKey: node.LogicalKey, IdentityStability: node.IdentityStability, Parser: node.Parser, Classification: classification}},
	}
	if value.Validate(&target) != nil {
		return nil, contentError(409, "content_integrity")
	}
	return value, nil
}

func contentDocumentAttributes(payload entity.KnowledgeDocumentNodePayload) (*entity.KnowledgeContentDocumentAttributes, error) {
	attrs := &entity.KnowledgeContentDocumentAttributes{}
	var target any
	switch payload.Kind {
	case "document", "section", "paragraph", "text_block", "text_box":
		attrs.Empty = &entity.KnowledgeDocumentEmptyAttributes{}
		target = attrs.Empty
	case "page":
		attrs.Page = &entity.KnowledgeDocumentPageAttributes{}
		target = attrs.Page
	case "slide":
		attrs.Slide = &entity.KnowledgeDocumentSlideAttributes{}
		target = attrs.Slide
	case "heading":
		attrs.Heading = &entity.KnowledgeDocumentHeadingAttributes{}
		target = attrs.Heading
	case "table":
		attrs.Table = &entity.KnowledgeDocumentTableAttributes{}
		target = attrs.Table
	case "row":
		attrs.Row = &entity.KnowledgeDocumentRowAttributes{}
		target = attrs.Row
	case "cell":
		attrs.Cell = &entity.KnowledgeDocumentCellAttributes{}
		target = attrs.Cell
	case "list":
		attrs.List = &entity.KnowledgeDocumentListAttributes{}
		target = attrs.List
	case "list_item":
		attrs.ListItem = &entity.KnowledgeDocumentListItemAttributes{}
		target = attrs.ListItem
	case "media":
		attrs.Media = &entity.KnowledgeDocumentMediaAttributes{}
		target = attrs.Media
	case "passage":
		attrs.Passage = &entity.KnowledgeDocumentPassageAttributes{}
		target = attrs.Passage
	case "unsupported_block":
		attrs.UnsupportedBlock = &entity.KnowledgeDocumentUnsupportedBlockAttributes{}
		target = attrs.UnsupportedBlock
	default:
		return nil, contentError(409, "content_integrity")
	}
	if json.Unmarshal(payload.Attributes, target) != nil {
		return nil, contentError(409, "content_integrity")
	}
	return attrs, nil
}

func contentProjectR2(rep *entity.KnowledgeDocumentRepresentation, node *entity.KnowledgeDocumentNode) (*entity.KnowledgeContentNode, error) {
	if node.RepresentationID != rep.RepresentationID || node.SourceID != rep.Original.SourceID || node.SourceRevisionID != rep.Original.SourceRevisionID || node.OriginalSHA256 != rep.Original.SHA256 || node.ProducerSHA256 != rep.ProducerSHA256 || node.Payload.ReadingRange.EndByte > rep.ReadingByteLength {
		return nil, contentError(409, "content_integrity")
	}
	// The existing codec validates the frozen payload and copies every pointer.
	raw, err := json.Marshal(node.Payload)
	if err != nil {
		return nil, contentError(409, "content_integrity")
	}
	var payload entity.KnowledgeDocumentNodePayload
	if json.Unmarshal(raw, &payload) != nil {
		return nil, contentError(409, "content_integrity")
	}
	attrs, err := contentDocumentAttributes(payload)
	if err != nil {
		return nil, err
	}
	target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: rep.DomainID, SourceID: node.SourceID, RevisionID: node.SourceRevisionID, SHA256: node.OriginalSHA256}, "document_representation", rep.RepresentationID, &rep.ProducerSHA256)
	value := &entity.KnowledgeContentNode{
		Ref: contentRef("r2_node", node.DocumentNodeID), ParentRef: contentParent("r2_node", node.ParentID), Kind: payload.Kind, Name: payload.Name, NameTruncated: payload.NameTruncated,
		Ordinal: contentInt(payload.Ordinal), Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &payload.Locator}, OriginalPosition: &payload.OriginalPosition,
		CoordinatePrecision: payload.CoordinatePrecision,
		Text:                entity.KnowledgeContentText{CoordinateSpace: "derived_utf8", Status: payload.TextStatus, Reason: payload.TextReason, Range: &entity.KnowledgeContentRange{StartByte: payload.ReadingRange.StartByte, EndByte: payload.ReadingRange.EndByte, SHA256: contentString(payload.ReadingRange.TextSHA256)}},
		Attributes:          entity.KnowledgeContentAttributes{Document: attrs},
	}
	if payload.OwnReadingRange != nil {
		r := payload.OwnReadingRange
		value.Text.OwnRange = &entity.KnowledgeContentRange{StartByte: r.StartByte, EndByte: r.EndByte, SHA256: contentString(r.TextSHA256)}
	}
	if value.Validate(&target) != nil {
		return nil, contentError(409, "content_integrity")
	}
	return value, nil
}
