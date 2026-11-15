package dataset

import (
	"context"
	"encoding/json"
	"ragflow/internal/entity"
	"sort"
	"unicode/utf8"
)

type lexicalUnit struct {
	Row  lexicalManifestRow
	Text string
	Name string
}

func (d *DatasetService) lexicalCurrentness(ctx context.Context, user, domain string, selection entity.KnowledgeLexicalSelection) (string, error) {
	current, historical := false, false
	for _, target := range selection.Targets {
		source, err := d.knowledgeSource(ctx, domain, target.Source.RevisionID)
		if err != nil {
			return "", lexicalNormalize(err)
		}
		if source.DocumentID != target.Source.SourceID || source.SHA256 != target.Source.SHA256 {
			return "", lexicalError(409, "integrity_error")
		}
		if target.Representation.Kind == "structure_view" {
			if _, err := d.structureReadView(ctx, user, domain, target.Representation.ID); err != nil {
				return "", lexicalNormalize(err)
			}
		}
		if source.IsCurrent {
			current = true
		} else {
			historical = true
		}
	}
	if historical {
		if current {
			return "mixed", nil
		}
		return "historical", nil
	}
	return "current", nil
}

func (d *DatasetService) lexicalSource(ctx context.Context, user, domain string, t entity.KnowledgeLexicalTarget) (*entity.KnowledgeSourceRevision, []byte, error) {
	if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
		return nil, nil, err
	}
	return d.lexicalStoredSource(ctx, domain, t)
}

func (d *DatasetService) lexicalStoredSource(ctx context.Context, domain string, t entity.KnowledgeLexicalTarget) (*entity.KnowledgeSourceRevision, []byte, error) {
	s, err := d.knowledgeSource(ctx, domain, t.Source.RevisionID)
	if err != nil {
		return nil, nil, lexicalNormalize(err)
	}
	if s.DocumentID != t.Source.SourceID || s.SHA256 != t.Source.SHA256 || s.RepresentationKind != "text" {
		return nil, nil, lexicalError(409, "integrity_error")
	}
	b, err := knowledgeSourceBytes(ctx, s)
	if err != nil {
		return nil, nil, lexicalNormalize(err)
	}
	if !utf8.Valid(b) {
		return nil, nil, lexicalError(409, "integrity_error")
	}
	return s, b, nil
}
func (d *DatasetService) lexicalUnits(ctx context.Context, user, domain, language, producer string, s entity.KnowledgeLexicalSelection) ([]lexicalUnit, entity.KnowledgeLexicalCoverage, string, error) {
	if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
		return nil, entity.KnowledgeLexicalCoverage{}, "", err
	}
	units, coverage, current, err := d.lexicalStoredUnits(ctx, user, domain, language, producer, s)
	if err != nil {
		return nil, coverage, "", err
	}
	if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
		return nil, coverage, "", err
	}
	return units, coverage, current, nil
}

func (d *DatasetService) lexicalStoredUnits(ctx context.Context, user, domain, language, producer string, s entity.KnowledgeLexicalSelection) ([]lexicalUnit, entity.KnowledgeLexicalCoverage, string, error) {
	coverage := entity.KnowledgeLexicalCoverage{SelectedSources: len(s.Targets), Complete: true, Missing: []entity.KnowledgeLexicalMissingRange{}}
	units := []lexicalUnit{}
	current, historical := false, false
	for _, target := range s.Targets {
		source, body, err := d.lexicalStoredSource(ctx, domain, target)
		if err != nil {
			return nil, coverage, "", err
		}
		if source.IsCurrent {
			current = true
		} else {
			historical = true
		}
		coverage.ExpectedBytes += int64(len(body))
		if coverage.ExpectedBytes > 16777216 {
			return nil, coverage, "", lexicalError(413, "capacity_exceeded")
		}
		members := []lexicalStoredMember{}
		strategy := "c1_complete_root"
		if target.Representation.Kind == "source_revision" {
			if len(body) > 8192 {
				return nil, coverage, "", lexicalError(413, "unit_oversize")
			}
			root := knowledgeRoot(source)
			members = append(members, lexicalStoredMember{Node: entity.KnowledgeLexicalNodeRef{Kind: "c1_node", ID: root.ID}, Range: lexicalStoredRange{Start: 0, End: int64(len(body)), SHA256: lexicalSHA(body)}})
		} else {
			strategy = "r1_existing_passage"
			view, err := d.structureStoredView(ctx, user, domain, target.Representation.ID)
			if err != nil {
				return nil, coverage, "", lexicalNormalize(err)
			}
			if target.Representation.ProducerSHA256 == nil || view.ProducerSHA256 != *target.Representation.ProducerSHA256 {
				return nil, coverage, "", lexicalError(409, "integrity_error")
			}
			member, err := structureMember(view, source.DocumentID)
			if err != nil || member.SourceRevisionID != source.ID || member.SHA256 != source.SHA256 {
				return nil, coverage, "", lexicalError(409, "integrity_error")
			}
			var nodes []entity.KnowledgeStructureNodeRecord
			if err := contentProofStore(ctx).Where("owner_id = ? AND domain_id = ? AND view_id = ? AND source_id = ? AND kind = ?", user, domain, view.ViewID, source.DocumentID, "passage").Order("start_byte ASC, id ASC").Limit(32769).Find(&nodes).Error; err != nil {
				return nil, coverage, "", lexicalNormalize(err)
			}
			if len(nodes) > 32768 {
				return nil, coverage, "", lexicalError(413, "capacity_exceeded")
			}
			for _, row := range nodes {
				node, err := structureReadNode(ctx, user, domain, view, row.ID)
				if err != nil {
					return nil, coverage, "", lexicalNormalize(err)
				}
				if node.Kind != "passage" || node.CoordinateSpace != "source_utf8" || node.SourceRevisionID != source.ID || row.SourceRevisionID != source.ID || node.StartByte != row.StartByte || node.EndByte != row.EndByte || node.ProducerSHA256 != view.ProducerSHA256 || !knowledgeRange(body, node.StartByte, node.EndByte) {
					return nil, coverage, "", lexicalError(409, "integrity_error")
				}
				if node.EndByte-node.StartByte > 8192 {
					return nil, coverage, "", lexicalError(413, "unit_oversize")
				}
				sha := lexicalSHA(body[node.StartByte:node.EndByte])
				if sha != row.FragmentSHA256 {
					return nil, coverage, "", lexicalError(409, "integrity_error")
				}
				members = append(members, lexicalStoredMember{Node: entity.KnowledgeLexicalNodeRef{Kind: "r1_node", ID: node.StructureNodeID}, Range: lexicalStoredRange{Start: node.StartByte, End: node.EndByte, SHA256: sha}})
			}
		}
		var end int64
		complete := true
		plan := lexicalPlan(strategy, language, producer)
		binding := lexicalBinding(target, plan)
		for _, member := range members {
			if member.Range.Start < end {
				return nil, coverage, "", lexicalError(409, "integrity_error")
			}
			if member.Range.Start > end {
				coverage.Missing = append(coverage.Missing, entity.KnowledgeLexicalMissingRange{Source: target.Source, Range: entity.KnowledgeLexicalRange{Start: end, End: member.Range.Start}})
				complete = false
			}
			end = member.Range.End
			length := end - member.Range.Start
			coverage.CoveredBytes += length
			reference := lexicalJSON(lexicalStoredReference{Version: entity.KnowledgeLocalLexicalVersion, Source: target.Source, Representation: target.Representation, Members: []lexicalStoredMember{member}})
			r := entity.KnowledgeLexicalRange{Start: member.Range.Start, End: member.Range.End}
			row := lexicalManifestRow{ID: lexicalRowID(binding, member.Node, r, member.Range.SHA256), Binding: binding, Plan: plan, Reference: reference, ReferenceSHA256: lexicalSHA([]byte(reference)), TextSHA256: member.Range.SHA256, ByteLength: length, Member: member, Target: target}
			units = append(units, lexicalUnit{Row: row, Text: string(body[r.Start:r.End]), Name: source.RelativePath})
		}
		if end < int64(len(body)) {
			coverage.Missing = append(coverage.Missing, entity.KnowledgeLexicalMissingRange{Source: target.Source, Range: entity.KnowledgeLexicalRange{Start: end, End: int64(len(body))}})
			complete = false
		}
		if complete {
			coverage.CoveredSources++
		} else {
			coverage.Complete = false
		}
		if len(units) > 32768 {
			return nil, coverage, "", lexicalError(413, "capacity_exceeded")
		}
	}
	coverage.IndexedUnits = len(units)
	currentness := "current"
	if historical {
		currentness = "historical"
		if current {
			currentness = "mixed"
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Row.ID < units[j].Row.ID })
	return units, coverage, currentness, nil
}

func (d *DatasetService) lexicalReadRow(ctx context.Context, user, domain string, row lexicalManifestRow) (string, string, error) {
	source, body, err := d.lexicalSource(ctx, user, domain, row.Target)
	if err != nil {
		return "", "", err
	}
	member := row.Member
	r := entity.KnowledgeLexicalRange{Start: member.Range.Start, End: member.Range.End}
	if !knowledgeRange(body, r.Start, r.End) || r.End-r.Start > 8192 || row.ByteLength != r.End-r.Start {
		return "", "", lexicalError(409, "integrity_error")
	}
	if row.Target.Representation.Kind == "source_revision" {
		if member.Node.Kind != "c1_node" || member.Node.ID != knowledgeRoot(source).ID || r.Start != 0 || r.End != int64(len(body)) {
			return "", "", lexicalError(409, "integrity_error")
		}
	} else {
		view, err := d.structureReadView(ctx, user, domain, row.Target.Representation.ID)
		if err != nil {
			return "", "", lexicalNormalize(err)
		}
		node, err := structureReadNode(ctx, user, domain, view, member.Node.ID)
		if err != nil {
			return "", "", lexicalNormalize(err)
		}
		if row.Target.Representation.ProducerSHA256 == nil || view.ProducerSHA256 != *row.Target.Representation.ProducerSHA256 || member.Node.Kind != "r1_node" || node.Kind != "passage" || node.StartByte != r.Start || node.EndByte != r.End || node.SourceRevisionID != source.ID {
			return "", "", lexicalError(409, "integrity_error")
		}
	}
	text := string(body[r.Start:r.End])
	if lexicalSHA([]byte(text)) != row.TextSHA256 || row.TextSHA256 != member.Range.SHA256 || lexicalSHA([]byte(row.Reference)) != row.ReferenceSHA256 {
		return "", "", lexicalError(409, "integrity_error")
	}
	var ref lexicalStoredReference
	if json.Unmarshal([]byte(row.Reference), &ref) != nil || row.Reference != lexicalJSON(lexicalStoredReference{Version: entity.KnowledgeLocalLexicalVersion, Source: row.Target.Source, Representation: row.Target.Representation, Members: []lexicalStoredMember{member}}) {
		return "", "", lexicalError(409, "integrity_error")
	}
	if lexicalBinding(row.Target, row.Plan) != row.Binding || lexicalRowID(row.Binding, member.Node, r, row.TextSHA256) != row.ID {
		return "", "", lexicalError(409, "integrity_error")
	}
	current := "current"
	if !source.IsCurrent {
		current = "historical"
	}
	return text, current, nil
}
