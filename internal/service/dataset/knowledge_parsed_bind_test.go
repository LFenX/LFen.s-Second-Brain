package dataset

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func parsedTestSource(original []byte) entity.KnowledgeContentSourceRef {
	return entity.KnowledgeContentSourceRef{DomainID: strings.Repeat("1", 32), SourceID: strings.Repeat("2", 32), RevisionID: strings.Repeat("3", 32), SHA256: directorySHA(original)}
}

func parsedAssertCoverage(t *testing.T, bound *parsedBound, maxUnit int64) {
	t.Helper()
	var cursor int64
	var body strings.Builder
	for _, node := range bound.Nodes {
		if node.NodeID == "" || node.RepresentationID != *bound.Representation.Ref.ID || node.Payload.Text.Value != nil || node.Payload.Text.CoordinateSpace != "derived_utf8" {
			t.Fatal("invalid fixed node", node)
		}
		if node.Payload.Kind != "passage" {
			continue
		}
		r := node.Payload.Text.Range
		if r.StartByte != cursor || r.EndByte-r.StartByte > maxUnit || *r.SHA256 != directorySHA([]byte(bound.ReadingText[r.StartByte:r.EndByte])) {
			t.Fatal("bad passage range", r)
		}
		body.WriteString(bound.ReadingText[r.StartByte:r.EndByte])
		cursor = r.EndByte
		var memberEnd = r.StartByte
		for _, m := range node.Payload.Members {
			if m.Range.StartByte != memberEnd || m.Range.EndByte > r.EndByte || *m.Range.SHA256 != directorySHA([]byte(bound.ReadingText[m.Range.StartByte:m.Range.EndByte])) {
				t.Fatal("bad member provenance", m)
			}
			if m.Kind == "assembly_separator" && (m.LocalIndex != 0 || bound.ReadingText[m.Range.StartByte:m.Range.EndByte] != "\n") {
				t.Fatal("fake source separator", m)
			}
			memberEnd = m.Range.EndByte
		}
		if memberEnd != r.EndByte {
			t.Fatal("member coverage gap")
		}
	}
	if cursor != int64(len(bound.ReadingText)) || body.String() != bound.ReadingText {
		t.Fatal("reading coverage changed")
	}
}

func TestBindKnowledgeParsedRealCSVAndStableFixedIDs(t *testing.T) {
	original := []byte("A,B\n甲,乙\n1,2\n")
	normalized := parser.NormalizeParseResult(t.Context(), "synthetic.csv", parser.NewCSVParser().ParseWithResult(t.Context(), "synthetic.csv", original))
	source := parsedTestSource(original)
	producer := directorySHA([]byte("synthetic local producer"))
	first, err := bindKnowledgeParsed(source, original, producer, normalized, 16)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bindKnowledgeParsed(source, original, producer, normalized, 16)
	if err != nil || *first.Representation.Ref.ID != *second.Representation.Ref.ID || first.Nodes[0].NodeID != second.Nodes[0].NodeID {
		t.Fatal(first, second, err)
	}
	if first.Representation.Ref.Kind != "parsed_representation" || first.Representation.RootRef.Kind != "parsed_node" || first.Representation.Source != source || first.Nodes[1].Payload.Locator.Kind != "parsed_item" || first.Nodes[1].Payload.CoordinatePrecision != "item_only" || first.Nodes[1].Payload.OriginalPosition != nil {
		t.Fatal("fabricated original location")
	}
	if first.Nodes[1].Payload.Attributes.Parser.Table == nil || first.Nodes[1].Payload.Attributes.Parser.Table.Grid[1][0].Text != "甲" {
		t.Fatal("lost real CSV grid")
	}
	parsedAssertCoverage(t, first, 16)
	source.RevisionID = strings.Repeat("8", 32)
	changed, err := bindKnowledgeParsed(source, original, producer, normalized, 16)
	if err != nil || *first.Representation.Ref.ID == *changed.Representation.Ref.ID || first.Nodes[1].NodeID == changed.Nodes[1].NodeID {
		t.Fatal("revision was not part of identity", err)
	}
}

func TestBindKnowledgeParsedAssemblyUnsupportedAndDecimalPositions(t *testing.T) {
	original := []byte("synthetic typed carrier")
	result := parser.ParseResult{OutputFormat: "json", JSON: []parser.DocItem{
		parser.TextItem{Text: "合成\r\n"},
		parser.ImageItem{Text: "alt is not OCR", Image: "SECRET_PAYLOAD", Positions: [][]float64{{1.25, 2.5}}},
		parser.RawItem{M: map[string]any{"text": "FORGED_TEXT", "owner_id": "FORGED_OWNER"}},
		parser.TextItem{Text: "第二段"},
	}}
	normalized := parser.NormalizeParseResult(t.Context(), "synthetic", result)
	bound, err := bindKnowledgeParsed(parsedTestSource(original), original, strings.Repeat("a", 64), normalized, 16)
	if err != nil {
		t.Fatal(err)
	}
	if bound.ReadingText != "合成\r\n\n第二段" || bound.Representation.State != "partial" || bound.Representation.UnsupportedNodeCount != 1 || bound.Representation.UnavailableNodeCount != 1 {
		t.Fatal(bound.Representation, bound.ReadingText)
	}
	if bound.Nodes[2].Payload.Text.Status != "unsupported" || bound.Nodes[3].Payload.Text.Status != "unavailable" || bound.Nodes[2].Payload.Attributes.Parser.Positions[0][0] != 1.25 {
		t.Fatal("unsupported facts changed")
	}
	parsedAssertCoverage(t, bound, 16)
	raw, err := json.Marshal(bound.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET_PAYLOAD") || strings.Contains(string(raw), "FORGED_") {
		t.Fatal("untrusted metadata escaped")
	}
	var restored []entity.KnowledgeParsedNode
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for i, node := range restored {
		sha, err := entity.KnowledgeParsedPayloadDigest(node.Payload)
		if err != nil || sha != bound.Nodes[i].PayloadSHA256 {
			t.Fatal("payload hash changed after storage JSON", i, err)
		}
	}
}

func TestBindKnowledgeParsedRejectsUnsafeAndDoesNotFabricateEmpty(t *testing.T) {
	original := []byte("synthetic")
	source := parsedTestSource(original)
	normalized := parser.NormalizeParseResult(t.Context(), "synthetic.txt", parser.ParseResult{OutputFormat: "text", Text: "safe"})
	wrong := source
	wrong.SHA256 = strings.Repeat("f", 64)
	if bound, err := bindKnowledgeParsed(wrong, original, strings.Repeat("a", 64), normalized, 16); err == nil || bound != nil {
		t.Fatal(bound, err)
	}
	for _, status := range []string{"fallback_text", "no_output", "failed"} {
		changed := normalized
		changed.Status = status
		if bound, err := bindKnowledgeParsed(source, original, strings.Repeat("a", 64), changed, 16); err == nil || bound != nil {
			t.Fatal(status, bound, err)
		}
	}
	audio := parser.NormalizeParseResult(t.Context(), "synthetic.wav", parser.NewAudioParser().ParseWithResult(t.Context(), "synthetic.wav", original))
	if bound, err := bindKnowledgeParsed(source, original, strings.Repeat("a", 64), audio, 16); err == nil || bound != nil {
		t.Fatal("audio config published", bound, err)
	}
	empty := parser.NormalizeParseResult(t.Context(), "synthetic.json", parser.NewJSONParser().ParseWithResult(t.Context(), "synthetic.json", nil))
	bound, err := bindKnowledgeParsed(parsedTestSource(nil), nil, strings.Repeat("a", 64), empty, 16)
	if err != nil || bound.Representation.State != "empty" || bound.ReadingText != "" || len(bound.Nodes) != 2 {
		t.Fatal(bound, err)
	}
}
