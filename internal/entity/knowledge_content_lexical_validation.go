package entity

import (
	"encoding/json"
	"fmt"
)

func (s KnowledgeContentLexicalSelection) Validate(domain string) error {
	if s.Kind != "fixed_representations" || len(s.Targets) < 1 || len(s.Targets) > 32 {
		return fmt.Errorf("invalid fixed representation selection")
	}
	seen := make(map[string]bool, len(s.Targets))
	for _, target := range s.Targets {
		if target.Validate(domain) != nil || seen[target.Source.SourceID] {
			return fmt.Errorf("invalid or duplicate selected source")
		}
		seen[target.Source.SourceID] = true
	}
	return nil
}

func (e KnowledgeContentLexicalEvidence) Validate(domain string) error {
	if e.Version != KnowledgeContentVersion || e.Address.Validate(domain) != nil || !ValidKnowledgeLexicalHash(e.Index.ID) || !ValidKnowledgeLexicalHash(e.Index.ManifestSHA256) || !ValidKnowledgeLexicalHash(e.RowID) || !ValidKnowledgeLexicalHash(e.ReadingSHA256) || e.Range.Validate() != nil || e.Range.SHA256 == nil || e.Range.EndByte <= e.Range.StartByte || e.Range.EndByte-e.Range.StartByte > KnowledgeLocalLexicalMaxUnit {
		return fmt.Errorf("invalid fixed content lexical evidence")
	}
	source := e.Address.Target.Representation.Kind == "source_revision" || e.Address.Target.Representation.Kind == "structure_view"
	space, maximum := "derived_utf8", int64(4<<20)
	if source {
		space, maximum = "source_utf8", KnowledgeMaxSourceBytes
	}
	if e.CoordinateSpace != space || e.Range.EndByte > maximum {
		return fmt.Errorf("evidence coordinate space mismatch")
	}
	if e.Address.Target.Representation.Kind == "source_revision" {
		if e.Assembly != "c1_root_subrange_exact_bytes" {
			return fmt.Errorf("invalid bounded C1 assembly")
		}
	} else if e.Assembly != "existing_passage_full_range" {
		return fmt.Errorf("invalid fixed passage assembly")
	}
	if source {
		p := e.Locator.SourceUTF8
		if e.Locator.Kind != "source_utf8" || p == nil || e.Locator.Document != nil || e.Locator.ItemIndex != nil || p.StartByte < 0 || p.EndByte < p.StartByte || e.Range.StartByte < p.StartByte || e.Range.EndByte > p.EndByte || e.OriginalPosition != nil || e.CoordinatePrecision != "source_bytes" {
			return fmt.Errorf("invalid source evidence location")
		}
	} else {
		if e.Locator.Kind != "document" || e.Locator.Document == nil || e.Locator.SourceUTF8 != nil || e.Locator.ItemIndex != nil || e.Locator.Document.Scheme != "derived_passage_v1" || e.CoordinatePrecision != "derived_range" {
			return fmt.Errorf("invalid derived evidence location")
		}
		if _, err := json.Marshal(e.Locator.Document); err != nil {
			return fmt.Errorf("invalid frozen document locator")
		}
		if e.OriginalPosition != nil {
			if _, err := json.Marshal(e.OriginalPosition); err != nil {
				return fmt.Errorf("invalid frozen original position")
			}
		}
	}
	return nil
}

func (r KnowledgeContentLexicalCreateRequest) Validate(domain string) error {
	if r.Selection.Validate(domain) != nil || !ValidKnowledgeLexicalKey(r.Key) || !ValidKnowledgeLexicalVersion(r.ExpectedLocalVersion, false) || !ValidKnowledgeLexicalVersion(r.ExpectedContentVersion, false) || !ValidKnowledgeLexicalVersion(r.ExpectedFoundationVersion, false) || !ValidKnowledgeLexicalHash(r.ExpectedProfileSHA256) {
		return fmt.Errorf("invalid unified lexical index request")
	}
	return nil
}

func (r KnowledgeContentLexicalSearchRequest) Validate(domain string) error {
	if r.Selection.Validate(domain) != nil || !ValidKnowledgeLexicalHash(r.Index.ID) || !ValidKnowledgeLexicalHash(r.Index.ManifestSHA256) || !ValidKnowledgeLexicalQuery(r.Query) || r.Limit < 1 || r.Limit > 20 {
		return fmt.Errorf("invalid unified lexical search request")
	}
	return nil
}
