package entity

import (
	"strings"
	"testing"
)

func TestKnowledgeLocalLexicalSelectionAndEvidence(t *testing.T) {
	domain := strings.Repeat("1", 32)
	revision := strings.Repeat("3", 32)
	s := KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []KnowledgeLexicalTarget{{Source: KnowledgeLexicalSourceRef{DomainID: domain, SourceID: strings.Repeat("2", 32), RevisionID: revision, SHA256: strings.Repeat("4", 64)}, Representation: KnowledgeLexicalRepresentationRef{Kind: "source_revision", ID: revision}}}}
	if err := s.Validate(domain); err != nil {
		t.Fatal(err)
	}
	s.Targets = append(s.Targets, s.Targets[0])
	if s.Validate(domain) == nil {
		t.Fatal("duplicate source accepted")
	}
	s.Targets = s.Targets[:1]
	e := KnowledgeLexicalEvidence{Version: KnowledgeLocalLexicalVersion, Index: KnowledgeLexicalIndexRef{ID: strings.Repeat("5", 64), ManifestSHA256: strings.Repeat("6", 64)}, RowID: strings.Repeat("7", 64), Source: s.Targets[0].Source, Representation: s.Targets[0].Representation, Node: KnowledgeLexicalNodeRef{Kind: "c1_node", ID: strings.Repeat("8", 64)}, CoordinateSpace: "source_utf8", Range: KnowledgeLexicalRange{Start: 0, End: 8192}, FragmentSHA256: strings.Repeat("9", 64)}
	if err := e.Validate(domain); err != nil {
		t.Fatal(err)
	}
	e.Range.End++
	if e.Validate(domain) == nil {
		t.Fatal("oversize unit accepted")
	}
	e.Range.End--
	e.Node.Kind = "r1_node"
	if e.Validate(domain) == nil {
		t.Fatal("cross representation node accepted")
	}
	if ValidKnowledgeLexicalQuery(strings.Repeat("中", 1366)) || ValidKnowledgeLexicalQuery(" \n\t") || ValidKnowledgeLexicalQuery(string([]byte{0xff})) {
		t.Fatal("invalid UTF8/query byte limit accepted")
	}
	if !ValidKnowledgeLexicalQuery("合成 alpha") || !ValidKnowledgeLexicalVersion(0, true) || ValidKnowledgeLexicalVersion(0, false) || ValidKnowledgeLexicalVersion(9007199254740992, true) {
		t.Fatal("scalar contract mismatch")
	}
}
