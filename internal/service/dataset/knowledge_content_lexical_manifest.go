package dataset

import (
	"sort"
	"unicode/utf8"

	"ragflow/internal/entity"
)

type contentLexicalManifest struct {
	Version         string                                   `json:"version"`
	Protocol        string                                   `json:"protocol"`
	Owner           string                                   `json:"owner"`
	Domain          string                                   `json:"domain"`
	Selection       entity.KnowledgeContentLexicalSelection  `json:"selection"`
	SelectionSHA256 string                                   `json:"selection_sha256"`
	Policies        contentLexicalPolicyCapture              `json:"policies"`
	ProducerSHA256  string                                   `json:"producer_sha256"`
	ProfileSHA256   string                                   `json:"profile_sha256"`
	Coverage        []entity.KnowledgeContentLexicalCoverage `json:"coverage"`
	TokenlessBytes  int64                                    `json:"tokenless_reading_bytes"`
	Rows            []contentLexicalManifestRow              `json:"rows"`
	Sources         []contentLexicalSourceFence              `json:"sources"`
	PublishedAt     string                                   `json:"published_at"`
}

func contentLexicalPublication(record entity.KnowledgeLexicalPublicationRecord) (*contentLexicalManifest, error) {
	invalid := func() (*contentLexicalManifest, error) { return nil, lexicalError(409, "integrity_error") }
	if record.Protocol != entity.KnowledgeContentLexicalProtocol || len(record.Payload) > 32<<20 || lexicalSHA([]byte(record.Payload)) != record.ManifestSHA256 || lexicalDigest("knowledge-content-index-publication/0.2.0", record.OwnerID, record.DomainID, record.ManifestSHA256) != record.ID {
		return invalid()
	}
	var manifest contentLexicalManifest
	if entity.KnowledgeParsedDecode([]byte(record.Payload), &manifest) != nil || lexicalJSON(manifest) != record.Payload || manifest.Version != entity.KnowledgeContentVersion || manifest.Protocol != entity.KnowledgeContentLexicalProtocol || manifest.Owner != record.OwnerID || manifest.Domain != record.DomainID || manifest.Selection.Validate(record.DomainID) != nil || manifest.PublishedAt == "" {
		return invalid()
	}
	p := manifest.Policies
	if !p.LocalEnabled || !p.ContentEnabled || !entity.ValidKnowledgeLexicalVersion(p.LocalVersion, false) || !entity.ValidKnowledgeLexicalVersion(p.ContentVersion, false) || !entity.ValidKnowledgeLexicalVersion(p.FoundationVersion, false) || !utf8.ValidString(p.Language) || len(p.Language) > 4096 || !entity.ValidKnowledgeLexicalHash(manifest.ProducerSHA256) || manifest.ProfileSHA256 != contentLexicalProfile(p.Language, manifest.ProducerSHA256).SHA256 {
		return invalid()
	}
	selection, selectionSHA := contentLexicalSelection(manifest.Selection)
	if selectionSHA != record.SelectionSHA256 || selectionSHA != manifest.SelectionSHA256 || lexicalJSON(selection) != lexicalJSON(manifest.Selection) || len(manifest.Coverage) != len(selection.Targets) || len(manifest.Sources) != len(selection.Targets) || len(manifest.Rows) < 1 || len(manifest.Rows) > 32768 {
		return invalid()
	}
	rowsBySource := make(map[string][]contentLexicalManifestRow, len(selection.Targets))
	targets := make(map[string]string, len(selection.Targets))
	for _, target := range selection.Targets {
		targets[target.Source.SourceID] = lexicalJSON(target)
	}
	seen := make(map[string]bool, len(manifest.Rows))
	for i, row := range manifest.Rows {
		ref := row.Reference
		evidence := contentLexicalEvidence(entity.KnowledgeLexicalIndexRef{ID: record.ID, ManifestSHA256: record.ManifestSHA256}, row)
		if seen[row.ID] || i > 0 && manifest.Rows[i-1].ID >= row.ID || targets[ref.Address.Target.Source.SourceID] != lexicalJSON(ref.Address.Target) || evidence.Validate(record.DomainID) != nil || ref.Version != entity.KnowledgeContentVersion || row.Plan != manifest.ProfileSHA256 || row.Binding != contentLexicalBinding(ref.Address.Target, manifest.ProfileSHA256, ref.CoordinateSpace, ref.ReadingSHA256) || row.ReferenceSHA256 != lexicalSHA([]byte(lexicalJSON(ref))) || len(lexicalJSON(ref)) > 24<<10 || row.ByteLength != ref.Range.EndByte-ref.Range.StartByte || row.ID != contentLexicalRowID(row.Binding, row.ReferenceSHA256, row.ByteLength, *ref.Range.SHA256) {
			return invalid()
		}
		if ref.CoordinateSpace == "source_utf8" && ref.ReadingSHA256 != ref.Address.Target.Source.SHA256 {
			return invalid()
		}
		seen[row.ID] = true
		rowsBySource[ref.Address.Target.Source.SourceID] = append(rowsBySource[ref.Address.Target.Source.SourceID], row)
	}
	var total int64
	for i, target := range selection.Targets {
		coverage := manifest.Coverage[i]
		space, original := "derived_utf8", "not_asserted"
		if target.Representation.Kind == "source_revision" || target.Representation.Kind == "structure_view" {
			space, original = "source_utf8", "source_utf8_exact"
		}
		if lexicalJSON(coverage.Target) != lexicalJSON(target) || coverage.Scope != "selected_representation_reading_utf8" || coverage.CoordinateSpace != space || coverage.OriginalCoverage != original || !entity.ValidKnowledgeLexicalHash(coverage.ReadingSHA256) || coverage.ExpectedBytes < 0 || coverage.ExpectedBytes > 4<<20 || coverage.CoveredBytes != coverage.ExpectedBytes || !coverage.ReadingTextComplete || coverage.UnsearchableNodes < 0 || coverage.RepresentationState != "ready" && coverage.RepresentationState != "empty" && coverage.RepresentationState != "partial" || manifest.Sources[i].SourceID != target.Source.SourceID || !entity.ValidKnowledgeLexicalHash(manifest.Sources[i].SHA256) {
			return invalid()
		}
		rows := rowsBySource[target.Source.SourceID]
		if coverage.IndexedUnits != int64(len(rows)) || coverage.ExpectedBytes == 0 && coverage.TextIndexState != "empty" || coverage.ExpectedBytes > 0 && coverage.TextIndexState != "available" {
			return invalid()
		}
		sort.Slice(rows, func(a, b int) bool { return rows[a].Reference.Range.StartByte < rows[b].Reference.Range.StartByte })
		var cursor int64
		root := ""
		for _, row := range rows {
			ref := row.Reference
			if ref.ReadingSHA256 != coverage.ReadingSHA256 || ref.Range.StartByte != cursor || ref.Range.EndByte > coverage.ExpectedBytes {
				return invalid()
			}
			if target.Representation.Kind == "source_revision" {
				if root == "" {
					root = *ref.Address.Node.ID
				}
				if *ref.Address.Node.ID != root || ref.Locator.SourceUTF8.StartByte != 0 || ref.Locator.SourceUTF8.EndByte != coverage.ExpectedBytes {
					return invalid()
				}
			} else if target.Representation.Kind == "structure_view" && (ref.Locator.SourceUTF8.StartByte != ref.Range.StartByte || ref.Locator.SourceUTF8.EndByte != ref.Range.EndByte) {
				return invalid()
			}
			cursor = ref.Range.EndByte
		}
		if cursor != coverage.ExpectedBytes {
			return invalid()
		}
		total += coverage.ExpectedBytes
	}
	if total > 16<<20 || manifest.TokenlessBytes < 0 || manifest.TokenlessBytes > total {
		return invalid()
	}
	return &manifest, nil
}
