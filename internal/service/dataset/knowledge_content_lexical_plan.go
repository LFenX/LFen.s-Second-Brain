package dataset

import (
	"context"
	"errors"
	"sort"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/parser/chunk"
	"ragflow/internal/tokenizer"
)

// These private candidate types do not widen the frozen 0.1 selection/evidence
// codec. A 0.2 reference binds the original SHA and the derived reading SHA.
type contentLexicalReference struct {
	Version             string                                    `json:"projection_version"`
	Address             entity.KnowledgeContentAddress            `json:"address"`
	CoordinateSpace     string                                    `json:"coordinate_space"`
	ReadingSHA256       string                                    `json:"reading_text_sha256"`
	Range               entity.KnowledgeContentRange              `json:"range"`
	Locator             entity.KnowledgeContentLocator            `json:"locator"`
	OriginalPosition    *entity.KnowledgeDocumentOriginalPosition `json:"original_position"`
	CoordinatePrecision string                                    `json:"coordinate_precision"`
	Assembly            string                                    `json:"assembly"`
}

type contentLexicalManifestRow struct {
	ID              string                  `json:"row_id"`
	Binding         string                  `json:"binding_id"`
	Plan            string                  `json:"plan_sha256"`
	Reference       contentLexicalReference `json:"reference"`
	ReferenceSHA256 string                  `json:"reference_sha256"`
	ByteLength      int64                   `json:"byte_length"`
}

type contentLexicalPolicyCapture struct {
	FoundationVersion int64  `json:"foundation_policy_version"`
	LocalVersion      int64  `json:"local_policy_version"`
	ContentVersion    int64  `json:"content_policy_version"`
	Language          string `json:"language"`
	LocalEnabled      bool   `json:"local_enabled"`
	ContentEnabled    bool   `json:"content_enabled"`
}

type contentLexicalPrepared struct {
	Version         string                                   `json:"projection_version"`
	Selection       entity.KnowledgeContentLexicalSelection  `json:"selection"`
	SelectionSHA256 string                                   `json:"selection_sha256"`
	ProfileSHA256   string                                   `json:"profile_sha256"`
	ProducerSHA256  string                                   `json:"producer_sha256"`
	Policies        contentLexicalPolicyCapture              `json:"policies"`
	Rows            []contentLexicalManifestRow              `json:"rows"`
	Coverage        []entity.KnowledgeContentLexicalCoverage `json:"coverage"`
	Inputs          []lexicalTokenInput                      `json:"-"`
}

func contentLexicalNormalize(err error) error {
	if err == nil {
		return nil
	}
	var lexical *KnowledgeLexicalError
	if errors.As(err, &lexical) {
		return err
	}
	var content *KnowledgeError
	if errors.As(contentNormalize(err), &content) {
		switch content.Status {
		case 400:
			return lexicalError(400, "invalid_request")
		case 404:
			return lexicalError(404, "not_found")
		case 409:
			return lexicalError(409, "integrity_error")
		case 413:
			return lexicalError(413, "capacity_exceeded")
		case 415:
			return lexicalError(415, "unsupported_representation")
		}
	}
	return lexicalNormalize(err)
}

func (d *DatasetService) contentLexicalPolicies(ctx context.Context, owner, domain string, required bool) (contentLexicalPolicyCapture, error) {
	kb, local, err := d.lexicalContext(ctx, owner, domain, required)
	if err != nil {
		return contentLexicalPolicyCapture{}, err
	}
	content, err := parsedPolicyRecord(dao.DB.WithContext(ctx), owner, domain)
	if err != nil {
		return contentLexicalPolicyCapture{}, contentLexicalNormalize(err)
	}
	if required && !content.LexicalEnabled {
		return contentLexicalPolicyCapture{}, lexicalError(409, "policy_disabled")
	}
	if !utf8.ValidString(lexicalLanguage(kb)) || len(lexicalLanguage(kb)) > 4096 {
		return contentLexicalPolicyCapture{}, lexicalError(409, "integrity_error")
	}
	return contentLexicalPolicyCapture{FoundationVersion: local.FoundationVersion, LocalVersion: local.Version, ContentVersion: content.Version, Language: lexicalLanguage(kb), LocalEnabled: local.Enabled, ContentEnabled: content.LexicalEnabled}, nil
}

// prepareKnowledgeContentLexical is an adapter into the existing tokenizer and
// publication core. It admits no queue item and never writes a publication.
func (d *DatasetService) prepareKnowledgeContentLexical(ctx context.Context, owner, domain string, selection entity.KnowledgeContentLexicalSelection, required bool) (*contentLexicalPrepared, error) {
	policies, err := d.contentLexicalPolicies(ctx, owner, domain, required)
	if err != nil {
		return nil, err
	}
	if selection.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	selection, selectionSHA := contentLexicalSelection(selection)
	producer, err := tokenizer.ProducerSHA256()
	if err != nil {
		return nil, lexicalNormalize(err)
	}
	profile := contentLexicalProfile(policies.Language, producer)
	prepared := &contentLexicalPrepared{Version: entity.KnowledgeContentVersion, Selection: selection, SelectionSHA256: selectionSHA, ProfileSHA256: profile.SHA256, ProducerSHA256: producer, Policies: policies, Rows: []contentLexicalManifestRow{}, Coverage: []entity.KnowledgeContentLexicalCoverage{}, Inputs: []lexicalTokenInput{}}
	var total int64
	for _, target := range selection.Targets {
		snapshot, err := d.contentLexicalSnapshot(ctx, owner, domain, target, policies.Language, producer)
		if err != nil {
			return nil, err
		}
		total += int64(len(snapshot.Body))
		if total > 16<<20 {
			return nil, lexicalError(413, "capacity_exceeded")
		}
		space := snapshot.Representation.CoordinateSpace
		originalCoverage := "not_asserted"
		if space == "source_utf8" {
			originalCoverage = "source_utf8_exact"
		}
		coverage := entity.KnowledgeContentLexicalCoverage{Target: target, Scope: "selected_representation_reading_utf8", CoordinateSpace: space, ReadingSHA256: snapshot.ReadingSHA256, ExpectedBytes: int64(len(snapshot.Body)), ReadingTextComplete: true, RepresentationState: snapshot.Representation.State, UnsearchableNodes: snapshot.Unsearchable, OriginalCoverage: originalCoverage, TextIndexState: "empty"}
		sort.Slice(snapshot.Passages, func(i, j int) bool {
			return snapshot.Passages[i].Text.Range.StartByte < snapshot.Passages[j].Text.Range.StartByte
		})
		binding := contentLexicalBinding(target, profile.SHA256, space, snapshot.ReadingSHA256)
		var cursor int64
		emit := func(node entity.KnowledgeContentNode, r entity.KnowledgeContentRange, assembly string) error {
			address := entity.KnowledgeContentAddress{Target: target, Node: node.Ref}
			if address.Validate(domain) != nil || node.Text.CoordinateSpace != space || r.StartByte != cursor || r.EndByte <= r.StartByte || r.EndByte-r.StartByte > 8192 || r.EndByte > int64(len(snapshot.Body)) || !utf8.ValidString(snapshot.Body[r.StartByte:r.EndByte]) {
				return lexicalError(409, "integrity_error")
			}
			fragmentSHA := lexicalSHA([]byte(snapshot.Body[r.StartByte:r.EndByte]))
			if r.SHA256 != nil && *r.SHA256 != fragmentSHA {
				return lexicalError(409, "integrity_error")
			}
			r.SHA256 = contentString(fragmentSHA)
			ref := contentLexicalReference{Version: entity.KnowledgeContentVersion, Address: address, CoordinateSpace: space, ReadingSHA256: snapshot.ReadingSHA256, Range: r, Locator: node.Locator, OriginalPosition: node.OriginalPosition, CoordinatePrecision: node.CoordinatePrecision, Assembly: assembly}
			reference := lexicalJSON(ref)
			if len(reference) > 24<<10 {
				return lexicalError(413, "capacity_exceeded")
			}
			referenceSHA := lexicalSHA([]byte(reference))
			id := contentLexicalRowID(binding, referenceSHA, r.EndByte-r.StartByte, fragmentSHA)
			prepared.Rows = append(prepared.Rows, contentLexicalManifestRow{ID: id, Binding: binding, Plan: profile.SHA256, Reference: ref, ReferenceSHA256: referenceSHA, ByteLength: r.EndByte - r.StartByte})
			prepared.Inputs = append(prepared.Inputs, lexicalTokenInput{ID: id, Binding: binding, Reference: reference, ReferenceSHA256: referenceSHA, TextSHA256: *r.SHA256, SourceID: target.Source.SourceID, ByteLength: r.EndByte - r.StartByte, Text: snapshot.Body[r.StartByte:r.EndByte], Name: snapshot.Name})
			cursor = r.EndByte
			coverage.IndexedUnits++
			if len(prepared.Rows) > 32768 {
				return lexicalError(413, "capacity_exceeded")
			}
			return nil
		}
		if target.Representation.Kind == "source_revision" {
			if snapshot.Root == nil || snapshot.Root.Text.Range == nil || snapshot.Root.Text.Range.StartByte != 0 || snapshot.Root.Text.Range.EndByte != int64(len(snapshot.Body)) {
				return nil, lexicalError(409, "integrity_error")
			}
			ranges, err := chunk.PlanContentRanges(chunk.ContentRangeInput{Kind: "text_block", CoordinateSpace: "source_utf8", Text: snapshot.Body, MaxBytes: 8192})
			if err != nil {
				return nil, lexicalError(409, "integrity_error")
			}
			for _, r := range ranges {
				if err := emit(*snapshot.Root, entity.KnowledgeContentRange{StartByte: r.StartByte, EndByte: r.EndByte, SHA256: contentString(r.SHA256)}, "c1_root_subrange_exact_bytes"); err != nil {
					return nil, err
				}
			}
		} else {
			for _, node := range snapshot.Passages {
				if node.Kind != "passage" || node.Text.Range == nil {
					return nil, lexicalError(409, "integrity_error")
				}
				if err := emit(node, *node.Text.Range, "existing_passage_full_range"); err != nil {
					return nil, err
				}
			}
		}
		if cursor != int64(len(snapshot.Body)) {
			return nil, lexicalError(409, "coverage_gap")
		}
		coverage.CoveredBytes = cursor
		if cursor > 0 {
			coverage.TextIndexState = "available"
		}
		prepared.Coverage = append(prepared.Coverage, coverage)
	}
	current, err := d.contentLexicalPolicies(ctx, owner, domain, required)
	if err != nil {
		return nil, err
	}
	if current != policies {
		return nil, lexicalError(409, "policy_changed")
	}
	// A common order keeps retries and row manifests deterministic across the
	// tokenizer, engine visibility check and later immutable publication.
	order := make([]int, len(prepared.Rows))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return prepared.Rows[order[i]].ID < prepared.Rows[order[j]].ID })
	rows, inputs := prepared.Rows, prepared.Inputs
	prepared.Rows, prepared.Inputs = make([]contentLexicalManifestRow, len(rows)), make([]lexicalTokenInput, len(inputs))
	for i, source := range order {
		prepared.Rows[i], prepared.Inputs[i] = rows[source], inputs[source]
	}
	if len(lexicalJSON(prepared)) > 32<<20 {
		return nil, lexicalError(413, "capacity_exceeded")
	}
	return prepared, nil
}

func (d *DatasetService) verifyKnowledgeContentLexical(ctx context.Context, owner, domain string, prepared *contentLexicalPrepared) error {
	if prepared == nil || prepared.Version != entity.KnowledgeContentVersion {
		return lexicalError(409, "integrity_error")
	}
	current, err := d.prepareKnowledgeContentLexical(ctx, owner, domain, prepared.Selection, true)
	if err != nil {
		return err
	}
	if current.Policies != prepared.Policies {
		return lexicalError(409, "policy_changed")
	}
	if lexicalJSON(current) != lexicalJSON(prepared) || len(current.Inputs) != len(prepared.Inputs) {
		return lexicalError(409, "integrity_error")
	}
	for i, input := range current.Inputs {
		if input != prepared.Inputs[i] {
			return lexicalError(409, "integrity_error")
		}
	}
	return nil
}
