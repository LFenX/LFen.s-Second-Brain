package dataset

import (
	"encoding/json"
	"testing"

	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func TestKnowledgeDocumentEmptyDerivedIdentityScopes(t *testing.T) {
	profile, err := documentProfile(entity.KnowledgeDocumentDOCXProfile)
	if err != nil {
		t.Fatal(err)
	}
	root := entity.KnowledgeDocumentNodePayload{Kind: "document", Attributes: json.RawMessage(`{}`), Locator: entity.KnowledgeDocumentLocator{Scheme: "document_v1"}, Name: "", OriginalPosition: entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}, CoordinatePrecision: "document", TextStatus: "empty", ReadingRange: entity.KnowledgeDocumentReadingRange{StartByte: 0, EndByte: 0, TextSHA256: directorySHA(nil)}}
	result := parser.DocumentStructureResult{ReadingText: "", Payloads: []entity.KnowledgeDocumentNodePayload{root}, Quality: "complete", TextCoverage: "supported_profile_blocks"}
	fixed := documentInputs{Original: entity.KnowledgeDocumentOriginalRef{DomainID: knowledgeTestDomain, RegistrationID: "11111111111111111111111111111111", SourceID: "22222222222222222222222222222222", SourceRevisionID: "33333333333333333333333333333333", SHA256: directorySHA([]byte("synthetic-empty-original-A")), ByteLength: 26, MediaType: profile.MediaType, OriginalFilename: "empty.docx", RepresentationKind: "original_only"}, Snapshot: entity.KnowledgeDocumentSourceSnapshot{LifecycleVersion: 1, RelativePath: "empty.docx", Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code", Language: ""}}, FoundationPolicyVersion: 1, DocumentPolicyVersion: 2, Profile: *profile}
	first, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, &fixed, result)
	if err != nil {
		t.Fatal(err)
	}
	seenRep := map[string]bool{first.Representation.RepresentationID: true}
	seenRoot := map[string]bool{first.Representation.RootDocumentNodeID: true}
	cases := []struct {
		name   string
		change func(*documentInputs)
	}{
		{"different_source", func(v *documentInputs) { v.Original.SourceID = "44444444444444444444444444444444" }},
		{"different_revision", func(v *documentInputs) { v.Original.SourceRevisionID = "55555555555555555555555555555555" }},
		{"different_original", func(v *documentInputs) { v.Original.SHA256 = directorySHA([]byte("synthetic-empty-original-B")) }},
		{"different_generation", func(v *documentInputs) { v.Snapshot.LifecycleVersion = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy := fixed
			tc.change(&copy)
			bound, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, &copy, result)
			if err != nil {
				t.Fatal(err)
			}
			if seenRep[bound.Representation.RepresentationID] || seenRoot[bound.Representation.RootDocumentNodeID] {
				t.Fatal("empty text collapsed independent source identity")
			}
			seenRep[bound.Representation.RepresentationID] = true
			seenRoot[bound.Representation.RootDocumentNodeID] = true
			if bound.Representation.ReadingTextSHA256 != first.Representation.ReadingTextSHA256 {
				t.Fatal("equal empty reading text gained synthetic content")
			}
		})
	}
}
