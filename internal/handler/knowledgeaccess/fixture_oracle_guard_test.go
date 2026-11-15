//go:build integration && linux

package knowledgeaccess

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func fixtureGuardParsedSnapshot(t *testing.T) (fixtureTypedSnapshot, string) {
	t.Helper()
	body := "ab"
	zero := int64(0)
	sha := digest([]byte(body))
	r := entity.KnowledgeContentRange{EndByte: 2, SHA256: &sha}
	rootLocator := entity.KnowledgeDocumentLocator{Scheme: "document_v1"}
	position := entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}
	text := entity.KnowledgeContentText{CoordinateSpace: "derived_utf8", Range: &r, Status: "available"}
	payloads := []entity.KnowledgeParsedNodePayload{
		{Kind: "document", Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &rootLocator}, OriginalPosition: &position, CoordinatePrecision: "document", Text: text, Attributes: entity.KnowledgeContentAttributes{Document: &entity.KnowledgeContentDocumentAttributes{Empty: &entity.KnowledgeDocumentEmptyAttributes{}}}, Members: []entity.KnowledgeParsedMember{}},
		{LocalIndex: 1, ParentLocalIndex: &zero, Kind: "text_block", Locator: entity.KnowledgeContentLocator{Kind: "parsed_item", ItemIndex: &zero}, CoordinatePrecision: "item_only", Text: text, Attributes: entity.KnowledgeContentAttributes{Parser: &entity.KnowledgeContentParserAttributes{ItemType: "text", Positions: [][]float64{}}}, Members: []entity.KnowledgeParsedMember{}},
		{LocalIndex: 2, ParentLocalIndex: &zero, Kind: "passage", Ordinal: 1, Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &rootLocator}}, OriginalPosition: &position, CoordinatePrecision: "derived_range", Text: text, Attributes: entity.KnowledgeContentAttributes{Document: &entity.KnowledgeContentDocumentAttributes{Passage: &entity.KnowledgeDocumentPassageAttributes{}}}, Members: []entity.KnowledgeParsedMember{{Kind: "content", LocalIndex: 1, Range: r}}},
	}
	manifest := make([]entity.KnowledgeParsedManifestEntry, len(payloads))
	for i, p := range payloads {
		hash, err := entity.KnowledgeParsedPayloadDigest(p)
		if err != nil {
			t.Fatal("payload")
		}
		manifest[i] = entity.KnowledgeParsedManifestEntry{LocalIndex: int64(i), ParentLocalIndex: p.ParentLocalIndex, PayloadSHA256: hash}
	}
	manifestSHA, err := entity.KnowledgeParsedManifestDigest(manifest)
	if err != nil {
		t.Fatal("manifest")
	}
	producer := strings.Repeat("4", 64)
	rep := entity.KnowledgeParsedRepresentation{Version: entity.KnowledgeContentVersion, Protocol: entity.KnowledgeParsedProtocol, Source: entity.KnowledgeContentSourceRef{DomainID: strings.Repeat("1", 32), SourceID: strings.Repeat("2", 32), RevisionID: strings.Repeat("3", 32), SHA256: sha}, Ref: entity.KnowledgeContentRepresentationRef{Kind: "parsed_representation", ProducerSHA256: &producer}, RootRef: entity.KnowledgeContentNodeRef{Kind: "parsed_node"}, ReadingTextSHA256: sha, ReadingByteLength: 2, ManifestSHA256: manifestSHA, NodeCount: 3, State: "ready", Assembly: entity.KnowledgeParsedAssembly, RangePlanVersion: "guard/1", MaxUnitBytes: 8192}
	id, err := entity.KnowledgeParsedRepresentationDigest(rep)
	if err != nil {
		t.Fatal("representation")
	}
	rep.Ref.ID = &id
	ids := make([]string, len(payloads))
	for i := range payloads {
		ids[i], err = entity.KnowledgeParsedNodeDigest(id, int64(i), manifest[i].PayloadSHA256)
		if err != nil {
			t.Fatal("node")
		}
	}
	rep.RootRef.ID = &ids[0]
	snap := fixtureTypedSnapshot{OwnerID: strings.Repeat("5", 32), DomainID: rep.Source.DomainID, Protocol: entity.KnowledgeParsedProtocol, Target: entity.KnowledgeContentTarget{Source: rep.Source, Representation: rep.Ref}, Payload: string(fixtureJSONBytes(rep)), Manifest: string(fixtureJSONBytes(manifest))}
	for i, p := range payloads {
		node := entity.KnowledgeParsedNode{RepresentationID: id, NodeID: ids[i], PayloadSHA256: manifest[i].PayloadSHA256, Payload: p}
		parent := ""
		if i > 0 {
			parent = ids[0]
		}
		snap.Nodes = append(snap.Nodes, fixtureStoredTypedNode{ID: ids[i], ParentID: parent, Ordinal: p.Ordinal, Payload: string(fixtureJSONBytes(node))})
	}
	return snap, body
}

func TestKnowledgeAccessFixtureCompleteTypedMetadata(t *testing.T) {
	snap, body := fixtureGuardParsedSnapshot(t)
	members, err := fixtureParsedTyped(snap, body)
	if err != nil || len(members) != 3 || members[0].Node.Kind != "document" || members[2].Node.Kind != "passage" {
		t.Fatal("complete_root_and_passage_missing")
	}
	r := byteRange{End: len(body), SHA: digest([]byte(body))}
	raw, err := fixtureSummarizeTyped(body, r, members)
	var full typedContext
	if err != nil || json.Unmarshal(raw, &full) != nil || len(full.Members) != 3 {
		t.Fatal("complete_projection")
	}
	filtered := []entity.KnowledgeContentLexicalTypedMember{members[1]}
	wrong, err := fixtureSummarizeTyped(body, r, filtered)
	if err != nil || sameJSON(raw, wrong) {
		t.Fatal("filtered_owner_context_masqueraded_as_full_oracle")
	}
	missing := snap
	missing.Nodes = missing.Nodes[:2]
	if _, err := fixtureParsedTyped(missing, body); err == nil {
		t.Fatal("missing_manifest_node_accepted")
	}
	corrupt := snap
	corrupt.Nodes = append([]fixtureStoredTypedNode(nil), snap.Nodes...)
	corrupt.Nodes[0].Payload = corrupt.Nodes[1].Payload
	if _, err := fixtureParsedTyped(corrupt, body); err == nil {
		t.Fatal("wrong_bound_node_accepted")
	}
}

func fixtureGuardCells() (string, []entity.KnowledgeContentLexicalTypedMember) {
	body := "A\t\tB"
	result := []entity.KnowledgeContentLexicalTypedMember{}
	for i, pair := range [][2]int64{{0, 1}, {2, 2}, {3, 4}} {
		sha := digest([]byte(body[pair[0]:pair[1]]))
		status := "available"
		if i == 1 {
			status = "empty"
		}
		id := strings.Repeat(string(rune('a'+i)), 64)
		result = append(result, entity.KnowledgeContentLexicalTypedMember{Node: entity.KnowledgeContentNode{Ref: entity.KnowledgeContentNodeRef{Kind: "r2_node", ID: &id}, Kind: "cell", Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &entity.KnowledgeDocumentLocator{Scheme: "office_package_v1", Part: "word/document.xml"}}, Text: entity.KnowledgeContentText{CoordinateSpace: "derived_utf8", Range: &entity.KnowledgeContentRange{StartByte: pair[0], EndByte: pair[1], SHA256: &sha}, Status: status}, Attributes: entity.KnowledgeContentAttributes{Document: &entity.KnowledgeContentDocumentAttributes{Cell: &entity.KnowledgeDocumentCellAttributes{CellIndex: int64(i)}}}}, PayloadSHA256: strings.Repeat("d", 64)})
	}
	return body, result
}

func TestKnowledgeAccessFixtureExactDOCXEmptyCell(t *testing.T) {
	body, cells := fixtureGuardCells()
	if fixtureEmptyCellWitness(body, cells) != nil {
		t.Fatal("valid_cell_witness")
	}
	cases := []struct {
		name   string
		change func([]entity.KnowledgeContentLexicalTypedMember)
	}{
		{"global_empty_paragraph", func(x []entity.KnowledgeContentLexicalTypedMember) { x[1].Node.Kind = "paragraph" }},
		{"wrong_cell_index", func(x []entity.KnowledgeContentLexicalTypedMember) { x[1].Node.Attributes.Document.Cell.CellIndex = 3 }},
		{"wrong_locator", func(x []entity.KnowledgeContentLexicalTypedMember) { x[1].Node.Locator.Document.Scheme = "document_v1" }},
		{"wrong_empty_sha", func(x []entity.KnowledgeContentLexicalTypedMember) {
			sha := strings.Repeat("e", 64)
			x[1].Node.Text.Range.SHA256 = &sha
		}},
		{"available_cell_missing", func(x []entity.KnowledgeContentLexicalTypedMember) { x[0].Node.Kind = "paragraph" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, x := fixtureGuardCells()
			tc.change(x)
			if fixtureEmptyCellWitness(body, x) == nil {
				t.Fatal("non_equivalent_empty_cell_witness_accepted")
			}
		})
	}
}
