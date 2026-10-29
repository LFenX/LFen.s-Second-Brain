package entity

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

var lexicalID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var lexicalHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
var lexicalKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func ValidKnowledgeLexicalHash(s string) bool { return lexicalHash.MatchString(s) }
func ValidKnowledgeLexicalID(s string) bool   { return lexicalID.MatchString(s) }
func ValidKnowledgeLexicalKey(s string) bool  { return lexicalKey.MatchString(s) }
func ValidKnowledgeLexicalVersion(v int64, zero bool) bool {
	return v >= 1 && v <= 9007199254740991 || zero && v == 0
}
func (s KnowledgeLexicalSelection) Validate(domain string) error {
	if !lexicalID.MatchString(domain) || s.Kind != "fixed_revisions" || len(s.Targets) < 1 || len(s.Targets) > 32 {
		return errors.New("invalid fixed selection")
	}
	seen := map[string]bool{}
	for _, t := range s.Targets {
		if t.Source.DomainID != domain || !lexicalID.MatchString(t.Source.SourceID) || !lexicalID.MatchString(t.Source.RevisionID) || !lexicalHash.MatchString(t.Source.SHA256) || seen[t.Source.SourceID] {
			return errors.New("invalid source")
		}
		seen[t.Source.SourceID] = true
		if t.Representation.Kind == "source_revision" {
			if t.Representation.ID != t.Source.RevisionID || t.Representation.ProducerSHA256 != nil {
				return errors.New("invalid source representation")
			}
		} else if t.Representation.Kind == "structure_view" {
			if !lexicalHash.MatchString(t.Representation.ID) || t.Representation.ProducerSHA256 == nil || !lexicalHash.MatchString(*t.Representation.ProducerSHA256) {
				return errors.New("invalid structure representation")
			}
		} else {
			return errors.New("unsupported representation")
		}
	}
	return nil
}
func (e KnowledgeLexicalEvidence) Validate(domain string) error {
	s := KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []KnowledgeLexicalTarget{{Source: e.Source, Representation: e.Representation}}}
	if s.Validate(domain) != nil || e.Version != KnowledgeLocalLexicalVersion || !lexicalHash.MatchString(e.Index.ID) || !lexicalHash.MatchString(e.Index.ManifestSHA256) || !lexicalHash.MatchString(e.RowID) || !lexicalHash.MatchString(e.Node.ID) || !lexicalHash.MatchString(e.FragmentSHA256) || e.CoordinateSpace != "source_utf8" || e.Range.Start < 0 || e.Range.End < e.Range.Start || e.Range.End > 2097152 || e.Range.End-e.Range.Start > KnowledgeLocalLexicalMaxUnit {
		return errors.New("invalid evidence")
	}
	if e.Representation.Kind == "source_revision" && e.Node.Kind != "c1_node" || e.Representation.Kind == "structure_view" && e.Node.Kind != "r1_node" {
		return errors.New("invalid node kind")
	}
	return nil
}
func ValidKnowledgeLexicalQuery(s string) bool {
	if !utf8.ValidString(s) || len(s) == 0 || len(s) > 4096 {
		return false
	}
	for _, r := range s {
		space := r >= 9 && r <= 13 || r == 32 || r == 160 || r == 0x1680 || r >= 0x2000 && r <= 0x200a || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff
		if !space {
			return true
		}
	}
	return false
}
