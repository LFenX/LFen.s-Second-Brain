package mcp

import (
	"encoding/json"
	"os"
	"testing"

	"ragflow/internal/entity"
)

// The unchanged frozen locator codec is the oracle. These are grammar vectors,
// not existing source locations, authority grants or owning service calls.
func TestContentKnowledgeCandidateFrozenOfficePartProjection(t *testing.T) {
	fixture := contentCandidateActual(t)
	evidence := fixture.Reads[len(fixture.Reads)-1].Evidence
	parent := evidence.Locator.Document.ParentLocator
	arguments := contentCandidateArguments("read", evidence.Address.Target.Source.DomainID, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: evidence})
	baseline, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	tool := contentCandidateTools(t)["knowledge_read_local"]
	parts := []string{
		"word/document.xml", "目录/正文.xml", "...", "a b/c.d", "", ".", "..", "/word", "word/", "word//doc", "word/./doc", "word/../doc", "word\\doc", "word:doc",
		"x\u2028//z", "x\u2029/../z", "x\u2028/./z", "x\u2029/z/", "x\u2028/z", "x/\u2029z", "word/\u2028", "word/..\u2029", "x\u2028/word/\u2029", "x\u2028../z", "\u2028", "\u2029", "x/\u2028/../z", "x\u2029//", "x\u2028/z\u2029/", "/x\u2028/z", "x\u2028/..\u2029/z",
		"word/document.xml\n", "word/document.xml\r", "word/document.xml\x00", "word/document.xml\x1f", "word/document.xml\x7f",
	}
	rows := make([]map[string]any, 0, len(parts))
	for i, part := range parts {
		frozenRaw, err := json.Marshal(map[string]any{"scheme": "office_package_v1", "part": part, "segments": parent.Segments})
		if err != nil {
			t.Fatal(err)
		}
		frozen := entity.KnowledgeDocumentValidateJSON("Locator", frozenRaw) == nil
		var value map[string]any
		if err := json.Unmarshal(baseline, &value); err != nil {
			t.Fatal(err)
		}
		body := value["body"].(map[string]any)["evidence_ref"].(map[string]any)
		document := body["locator"].(map[string]any)["document"].(map[string]any)
		document["parent_locator"].(map[string]any)["part"] = part
		raw, _ := json.Marshal(value)
		normalized, checked, normalizeErr := normalizeContentToolInput(raw)
		projected := normalizeErr == nil && tool.input.Validate(&checked) == nil
		var typed contentToolArguments[entity.KnowledgeContentLexicalReadRequest]
		typedOK := normalizeErr == nil && decodeContentTypedInput(normalized, &typed) == nil
		if projected != frozen || typedOK != frozen {
			t.Fatalf("case %d: frozen=%v, projected=%v, typed=%v", i, frozen, projected, typedOK)
		}
		rows = append(rows, map[string]any{"part": part, "frozen_codec_accepts": frozen, "Go_projected_schema_accepts": projected, "same_entity_decode_accepts": typedOK})
	}
	if output := os.Getenv("LFEN_MCP_CANDIDATE_OFFICE_OUTPUT"); output != "" {
		raw, err := json.MarshalIndent(map[string]any{"status": "Passed_ActualFrozenCodecAndGoSchemaConformance", "cases": rows, "new_MCP_network_calls": 0, "owning_calls": 0}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
