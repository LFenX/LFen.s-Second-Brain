package dataset

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"ragflow/internal/entity"
)

func contentLexicalContextDigest(snapshot *contentLexicalSnapshot) string {
	return lexicalSHA([]byte(lexicalJSON([]any{snapshot.Representation, snapshot.ReadingSHA256, snapshot.ManifestSHA256, snapshot.Root, snapshot.Passages, snapshot.TypedMembers})))
}

func contentLexicalContextSize(value *entity.KnowledgeContentLexicalContext) int {
	for range 5 {
		raw, err := json.Marshal(value)
		if err != nil {
			return 1 << 30
		}
		if value.MetadataBytes == len(raw) {
			return len(raw)
		}
		value.MetadataBytes = len(raw)
	}
	return 1 << 30
}

func contentLexicalContextOmit(value *entity.KnowledgeContentLexicalContext, reason string) {
	value.MembersComplete = false
	if !slices.Contains(value.OmissionReasons, reason) {
		value.OmissionReasons = append(value.OmissionReasons, reason)
	}
}

// The full owning tree is verified before selecting whole typed members. No
// parent text, partial grid, inferred page frame or diagnostic merge origins
// become authority through this bounded context.
func (d *DatasetService) contentLexicalContext(ctx context.Context, owner, domain string, manifest *contentLexicalManifest, row contentLexicalManifestRow) (*entity.KnowledgeContentLexicalContext, error) {
	ref := row.Reference
	snapshot, err := d.contentLexicalSnapshot(ctx, owner, domain, ref.Address.Target, manifest.Policies.Language, manifest.ProducerSHA256)
	if err != nil {
		return nil, err
	}
	if snapshot.ReadingSHA256 != ref.ReadingSHA256 {
		return nil, lexicalError(409, "integrity_error")
	}
	actual, err := d.GetKnowledgeContentNode(ctx, owner, domain, ref.Address)
	if err != nil {
		return nil, contentLexicalNormalize(err)
	}
	if attrs := actual.Node.Attributes.Structure; attrs != nil {
		c := attrs.Classification
		if !validKnowledgeSide(c.ComponentSide) || c.Language != strings.TrimSpace(c.Language) || c.Language != "" && !validKnowledgeText(c.Language, 32) || c.GroupID != nil && !isKnowledgeID(*c.GroupID, 32) {
			return nil, lexicalError(409, "integrity_error")
		}
	}
	value, err := contentLexicalBuildContext(snapshot, actual.Node, ref)
	if err != nil {
		return nil, err
	}
	// Verify the same source and manifest again after assembling metadata.
	latest, err := d.contentLexicalSnapshot(ctx, owner, domain, ref.Address.Target, manifest.Policies.Language, manifest.ProducerSHA256)
	if err != nil {
		return nil, err
	}
	sort.Slice(latest.TypedMembers, func(i, j int) bool {
		a, b := latest.TypedMembers[i].Node, latest.TypedMembers[j].Node
		if a.Text.Range.StartByte != b.Text.Range.StartByte {
			return a.Text.Range.StartByte < b.Text.Range.StartByte
		}
		if a.Text.Range.EndByte != b.Text.Range.EndByte {
			return a.Text.Range.EndByte < b.Text.Range.EndByte
		}
		return *a.Ref.ID < *b.Ref.ID
	})
	if contentLexicalContextDigest(latest) != contentLexicalContextDigest(snapshot) {
		return nil, lexicalError(409, "source_changed")
	}
	return value, nil
}

func contentLexicalBuildContext(snapshot *contentLexicalSnapshot, actual entity.KnowledgeContentNode, ref contentLexicalReference) (*entity.KnowledgeContentLexicalContext, error) {
	value := &entity.KnowledgeContentLexicalContext{Node: &actual, TypedMembers: []entity.KnowledgeContentLexicalTypedMember{}, Scope: "whole_nodes_contained_in_evidence_range", MembersComplete: true, OmissionReasons: []string{}, ManifestSHA256: snapshot.ManifestSHA256, RepresentationState: snapshot.Representation.State, ParentExpansion: "disabled"}
	switch ref.Address.Target.Representation.Kind {
	case "source_revision":
		value.Membership = "stored_c1_root"
	case "structure_view":
		value.Membership = "validated_r1_passage"
	case "document_representation":
		value.Membership = "validated_document_manifest"
	case "parsed_representation":
		value.Membership = "validated_parsed_manifest"
	}
	if value.ManifestSHA256 == nil {
		contentLexicalContextOmit(value, "typed_members_not_defined_for_source_family")
	}
	if snapshot.Unsearchable > 0 {
		contentLexicalContextOmit(value, "representation_contains_unsearchable_nodes")
	}
	if contentLexicalContextSize(value) > 65536-2048 {
		value.Node, value.NodeOmissionReason = nil, contentString("metadata_budget")
	}
	sort.Slice(snapshot.TypedMembers, func(i, j int) bool {
		a, b := snapshot.TypedMembers[i].Node, snapshot.TypedMembers[j].Node
		if a.Text.Range.StartByte != b.Text.Range.StartByte {
			return a.Text.Range.StartByte < b.Text.Range.StartByte
		}
		if a.Text.Range.EndByte != b.Text.Range.EndByte {
			return a.Text.Range.EndByte < b.Text.Range.EndByte
		}
		return *a.Ref.ID < *b.Ref.ID
	})
	for _, member := range snapshot.TypedMembers {
		n := member.Node
		if slices.Contains([]string{"document", "section", "page", "slide", "passage"}, n.Kind) {
			continue
		}
		r := n.Text.Range
		if r == nil || r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > int64(len(snapshot.Body)) {
			return nil, lexicalError(409, "integrity_error")
		}
		if contentLexicalUnsearchable(n.Text.Status) {
			if r.StartByte >= ref.Range.StartByte && r.StartByte <= ref.Range.EndByte {
				value.OmittedMembers++
				contentLexicalContextOmit(value, "unsearchable_member")
			}
			continue
		}
		empty := r.StartByte == r.EndByte
		if empty {
			if n.Text.Status != "empty" || r.SHA256 == nil || *r.SHA256 != lexicalSHA(nil) {
				return nil, lexicalError(409, "integrity_error")
			}
		}
		if !empty && (r.EndByte <= ref.Range.StartByte || r.StartByte >= ref.Range.EndByte) || empty && (r.StartByte < ref.Range.StartByte || r.StartByte > ref.Range.EndByte) {
			continue
		}
		if r.StartByte < ref.Range.StartByte || r.EndByte > ref.Range.EndByte {
			value.OmittedMembers++
			contentLexicalContextOmit(value, "whole_member_outside_evidence")
			continue
		}
		if r.SHA256 == nil || r.EndByte > int64(len(snapshot.Body)) || lexicalSHA([]byte(snapshot.Body[r.StartByte:r.EndByte])) != *r.SHA256 {
			return nil, lexicalError(409, "integrity_error")
		}
		if n.Kind == "table" && n.Attributes.Parser != nil && n.Attributes.Parser.Table != nil {
			attrs := n.Attributes.Parser
			const assembly = "row_major_uncovered_cells_tab_lf_v1"
			if attrs.TextAssembly == nil || *attrs.TextAssembly != assembly {
				return nil, lexicalError(409, "integrity_error")
			}
			rows := make([]string, len(attrs.Table.Grid))
			for i, cells := range attrs.Table.Grid {
				values := make([]string, 0, len(cells))
				for _, cell := range cells {
					if !cell.Covered {
						values = append(values, cell.Text)
					}
				}
				rows[i] = strings.Join(values, "\t")
			}
			if strings.Join(rows, "\n") != snapshot.Body[r.StartByte:r.EndByte] {
				return nil, lexicalError(409, "integrity_error")
			}
			member.TextAssembly = assembly
		}
		if len(value.TypedMembers) == 32 {
			value.OmittedMembers++
			contentLexicalContextOmit(value, "member_limit")
			continue
		}
		value.TypedMembers = append(value.TypedMembers, member)
		if contentLexicalContextSize(value) > 65536-2048 {
			value.TypedMembers = value.TypedMembers[:len(value.TypedMembers)-1]
			value.OmittedMembers++
			contentLexicalContextOmit(value, "metadata_budget")
		}
	}
	if contentLexicalContextSize(value) > 65536 {
		return nil, lexicalError(413, "capacity_exceeded")
	}
	return value, nil
}
