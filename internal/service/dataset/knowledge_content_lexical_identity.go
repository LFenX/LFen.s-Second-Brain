package dataset

import (
	"sort"

	"ragflow/internal/entity"
	"ragflow/internal/parser/chunk"
)

func contentLexicalSelection(selection entity.KnowledgeContentLexicalSelection) (entity.KnowledgeContentLexicalSelection, string) {
	copy := entity.KnowledgeContentLexicalSelection{Kind: selection.Kind, Targets: make([]entity.KnowledgeContentTarget, len(selection.Targets))}
	for i, target := range selection.Targets {
		copy.Targets[i] = contentTarget(target.Source, target.Representation.Kind, *target.Representation.ID, target.Representation.ProducerSHA256)
	}
	sort.Slice(copy.Targets, func(i, j int) bool { return copy.Targets[i].Source.SourceID < copy.Targets[j].Source.SourceID })
	return copy, lexicalSHA([]byte(lexicalJSON(copy)))
}

func contentLexicalProfile(language, producer string) entity.KnowledgeContentLexicalProfile {
	strategies := []string{"c1_exact_utf8_ranges", "r1_existing_passage", "r2_existing_passage", "parsed_existing_passage"}
	sha := lexicalDigest("knowledge-content-index-plan/0.2.0", []any{strategies[0], strategies[1], strategies[2], strategies[3]}, chunk.ContentRangePlanVersion, int64(8192), "exact_bytes_no_trim_no_merge_no_parent", language, producer, "full_text")
	return entity.KnowledgeContentLexicalProfile{SHA256: sha, ProducerSHA256: producer, Language: language, Strategies: strategies, Engine: "elasticsearch", Execution: "local_lexical_no_models/0.2.0", MaxUnitBytes: 8192, ParentExpansion: "disabled", ModelsEnabled: false, RemoteFetches: false}
}

func contentLexicalBinding(target entity.KnowledgeContentTarget, profile, space, reading string) string {
	return lexicalDigest("knowledge-index-binding/0.2.0", lexicalSHA([]byte(lexicalJSON(target))), profile, space, reading)
}

func contentLexicalRowID(binding, reference string, length int64, fragment string) string {
	return lexicalDigest("knowledge-index-row/0.2.0", binding, reference, length, fragment)
}

func contentLexicalEvidence(index entity.KnowledgeLexicalIndexRef, row contentLexicalManifestRow) entity.KnowledgeContentLexicalEvidence {
	ref := row.Reference
	return entity.KnowledgeContentLexicalEvidence{Version: entity.KnowledgeContentVersion, Index: index, RowID: row.ID, Address: ref.Address, CoordinateSpace: ref.CoordinateSpace, ReadingSHA256: ref.ReadingSHA256, Range: ref.Range, Locator: ref.Locator, OriginalPosition: ref.OriginalPosition, CoordinatePrecision: ref.CoordinatePrecision, Assembly: ref.Assembly}
}
