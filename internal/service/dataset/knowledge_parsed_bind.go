package dataset

import (
	"fmt"
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/parser/chunk"
	parser "ragflow/internal/parser/parser"
)

type parsedBound struct {
	Representation entity.KnowledgeParsedRepresentation
	ReadingText    string
	Nodes          []entity.KnowledgeParsedNode
	Manifest       []entity.KnowledgeParsedManifestEntry
}

type parsedPlan struct {
	start, end int64
	member     entity.KnowledgeParsedMember
}

// bindKnowledgeParsed is pure. Admission, execution provenance, current authority
// and publication fences belong to the DatasetService task/transaction boundary.
func bindKnowledgeParsed(source entity.KnowledgeContentSourceRef, original []byte, producer string, normalized parser.NormalizedParseResult, maxUnit int64) (*parsedBound, error) {
	if normalized.Diagnostic != nil || normalized.Result.Err != nil || normalized.Status != "json" {
		return nil, fmt.Errorf("normalization_not_publishable")
	}
	projection, err := parser.ProjectKnowledgeParseResult(normalized.Result)
	if err != nil {
		return nil, err
	}
	return bindKnowledgeParsedProjection(source, original, producer, projection, maxUnit)
}

func bindKnowledgeParsedProjection(source entity.KnowledgeContentSourceRef, original []byte, producer string, projection *entity.KnowledgeContentProjection, maxUnit int64) (*parsedBound, error) {
	if source.Validate(source.DomainID) != nil || int64(len(original)) > entity.KnowledgeMaxSourceBytes || directorySHA(original) != source.SHA256 {
		return nil, fmt.Errorf("source_binding_mismatch")
	}
	if maxUnit < 8 || maxUnit > 8192 {
		return nil, fmt.Errorf("invalid_unit_budget")
	}
	if projection == nil || projection.Validate() != nil || projection.Binding != "unbound" {
		return nil, fmt.Errorf("invalid_projection")
	}
	if projection.State != "ready" && projection.State != "empty" && projection.State != "partial" {
		return nil, fmt.Errorf("no_publishable_content")
	}
	rootPosition := &entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}
	rootLocator := entity.KnowledgeDocumentLocator{Scheme: "document_v1"}
	payloads := []entity.KnowledgeParsedNodePayload{{Kind: "document", Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &rootLocator}, OriginalPosition: rootPosition, CoordinatePrecision: "document", Attributes: entity.KnowledgeContentAttributes{Document: &entity.KnowledgeContentDocumentAttributes{Empty: &entity.KnowledgeDocumentEmptyAttributes{}}}, Members: []entity.KnowledgeParsedMember{}}}
	var reading strings.Builder
	var unsupported, unavailable int64
	nonempty := false
	for i, node := range projection.Nodes {
		index := int64(i + 1)
		item := int64(i)
		text := node.Text
		text.CoordinateSpace = "derived_utf8"
		text.Value = nil
		value := ""
		if node.Text.Value != nil {
			value = *node.Text.Value
		}
		if value != "" && nonempty {
			reading.WriteByte('\n')
		}
		start := int64(reading.Len())
		reading.WriteString(value)
		if value != "" {
			nonempty = true
		}
		r := entity.KnowledgeContentRange{StartByte: start, EndByte: int64(reading.Len()), SHA256: contentString(directorySHA([]byte(value)))}
		text.Range = &r
		if text.Status == "available" || text.Status == "empty" {
			own := r
			text.OwnRange = &own
		}
		if text.Status == "unsupported" || text.Status == "unsupported_or_requires_ocr" {
			unsupported++
		}
		if text.Status == "unavailable" {
			unavailable++
		}
		payloads = append(payloads, entity.KnowledgeParsedNodePayload{LocalIndex: index, ParentLocalIndex: contentInt(0), Kind: node.Kind, Name: node.Name, Ordinal: int64(i), Locator: entity.KnowledgeContentLocator{Kind: "parsed_item", ItemIndex: &item}, CoordinatePrecision: "item_only", Text: text, Attributes: node.Attributes, Members: []entity.KnowledgeParsedMember{}})
		if reading.Len() > 4<<20 {
			return nil, fmt.Errorf("reading_capacity_exceeded")
		}
	}
	body := reading.String()
	rootStatus := "available"
	if body == "" {
		rootStatus = "empty"
	}
	payloads[0].Text = entity.KnowledgeContentText{CoordinateSpace: "derived_utf8", Range: &entity.KnowledgeContentRange{EndByte: int64(len(body)), SHA256: contentString(directorySHA([]byte(body)))}, Status: rootStatus}
	plans := []parsedPlan{}
	lastText := -1
	for i, node := range projection.Nodes {
		if node.Text.Value != nil && *node.Text.Value != "" {
			lastText = i
		}
	}
	for i, node := range projection.Nodes {
		if node.Text.Value == nil {
			continue
		}
		budget := maxUnit
		if i != lastText {
			budget--
		}
		input := chunk.ContentRangeInput{Kind: node.Kind, CoordinateSpace: "item_utf8", Text: *node.Text.Value, MaxBytes: budget}
		if node.Attributes.Parser != nil {
			input.Table = node.Attributes.Parser.Table
		}
		ranges, err := chunk.PlanContentRanges(input)
		if err != nil {
			return nil, err
		}
		base := payloads[i+1].Text.Range.StartByte
		for _, r := range ranges {
			start, end := base+r.StartByte, base+r.EndByte
			plans = append(plans, parsedPlan{start: start, end: end, member: entity.KnowledgeParsedMember{Kind: "content", LocalIndex: int64(i + 1), Range: entity.KnowledgeContentRange{StartByte: start, EndByte: end, SHA256: contentString(r.SHA256)}, FirstRow: r.FirstRow, EndRow: r.EndRow}})
		}
	}
	if len(plans) == 0 && body != "" {
		return nil, fmt.Errorf("reading_coverage_gap")
	}
	for i, plan := range plans {
		end := int64(len(body))
		if i+1 < len(plans) {
			end = plans[i+1].start
		}
		if i == 0 && plan.start != 0 || end < plan.end || end-plan.start > maxUnit {
			return nil, fmt.Errorf("reading_coverage_gap")
		}
		members := []entity.KnowledgeParsedMember{plan.member}
		if end > plan.end {
			if body[plan.end:end] != "\n" {
				return nil, fmt.Errorf("unexpected_assembly_gap")
			}
			members = append(members, entity.KnowledgeParsedMember{Kind: "assembly_separator", LocalIndex: 0, Range: entity.KnowledgeContentRange{StartByte: plan.end, EndByte: end, SHA256: contentString(directorySHA([]byte(body[plan.end:end])))}})
		}
		r := entity.KnowledgeContentRange{StartByte: plan.start, EndByte: end, SHA256: contentString(directorySHA([]byte(body[plan.start:end])))}
		locator := entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &rootLocator, ChunkIndex: int64(i)}
		payloads = append(payloads, entity.KnowledgeParsedNodePayload{LocalIndex: int64(len(payloads)), ParentLocalIndex: contentInt(0), Kind: "passage", Ordinal: int64(len(projection.Nodes) + i), Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &locator}, OriginalPosition: &entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}, CoordinatePrecision: "derived_range", Text: entity.KnowledgeContentText{CoordinateSpace: "derived_utf8", Range: &r, OwnRange: &r, Status: "available"}, Attributes: entity.KnowledgeContentAttributes{Document: &entity.KnowledgeContentDocumentAttributes{Passage: &entity.KnowledgeDocumentPassageAttributes{ChunkIndex: int64(i)}}}, Members: members})
	}
	if len(payloads) > 32768 {
		return nil, fmt.Errorf("node_capacity_exceeded")
	}
	bound := &parsedBound{ReadingText: body, Nodes: []entity.KnowledgeParsedNode{}, Manifest: []entity.KnowledgeParsedManifestEntry{}}
	for _, payload := range payloads {
		sha, err := entity.KnowledgeParsedPayloadDigest(payload)
		if err != nil {
			return nil, err
		}
		bound.Manifest = append(bound.Manifest, entity.KnowledgeParsedManifestEntry{LocalIndex: payload.LocalIndex, ParentLocalIndex: payload.ParentLocalIndex, PayloadSHA256: sha})
		bound.Nodes = append(bound.Nodes, entity.KnowledgeParsedNode{PayloadSHA256: sha, Payload: payload})
	}
	manifest, err := entity.KnowledgeParsedManifestDigest(bound.Manifest)
	if err != nil {
		return nil, err
	}
	rep := entity.KnowledgeParsedRepresentation{Version: entity.KnowledgeContentVersion, Protocol: entity.KnowledgeParsedProtocol, Source: source, Ref: entity.KnowledgeContentRepresentationRef{Kind: "parsed_representation", ProducerSHA256: contentString(producer)}, ReadingTextSHA256: directorySHA([]byte(body)), ReadingByteLength: int64(len(body)), ManifestSHA256: manifest, NodeCount: int64(len(payloads)), State: projection.State, UnsupportedNodeCount: unsupported, UnavailableNodeCount: unavailable, Assembly: entity.KnowledgeParsedAssembly, RangePlanVersion: chunk.ContentRangePlanVersion, MaxUnitBytes: maxUnit}
	id, err := entity.KnowledgeParsedRepresentationDigest(rep)
	if err != nil {
		return nil, err
	}
	rep.Ref.ID = contentString(id)
	for i := range bound.Nodes {
		node := &bound.Nodes[i]
		node.RepresentationID = id
		node.NodeID, err = entity.KnowledgeParsedNodeDigest(id, int64(i), node.PayloadSHA256)
		if err != nil {
			return nil, err
		}
	}
	rep.RootRef = contentRef("parsed_node", bound.Nodes[0].NodeID)
	bound.Representation = rep
	return bound, nil
}
