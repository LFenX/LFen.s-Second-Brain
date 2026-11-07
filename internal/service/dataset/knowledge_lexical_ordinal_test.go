//go:build linux

package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
)

// These inputs exercise projection identity and the real tokenizer; the plan
// adapter is exercised separately against the in-memory source store below.
func ordinalTestInputs(source, revision int) []lexicalTokenInput {
	parts := []string{
		fmt.Sprintf("Synthetic source %d revision %d first passage.\r\n", source, revision),
		fmt.Sprintf("Synthetic source %d second reading passage.\n", source),
	}
	body := strings.Join(parts, "")
	reading := lexicalSHA([]byte(body))
	sourceRef := entity.KnowledgeContentSourceRef{DomainID: knowledgeTestDomain, SourceID: fmt.Sprintf("%032x", source+1), RevisionID: fmt.Sprintf("%032x", 1000+source*128+revision), SHA256: reading}
	target := contentTarget(sourceRef, "structure_view", lexicalSHA([]byte(lexicalJSON(sourceRef))), contentString(strings.Repeat("e", 64)))
	profile := contentLexicalProfile("English", strings.Repeat("f", 64)).SHA256
	binding := contentLexicalBinding(target, profile, "source_utf8", reading)
	inputs := make([]lexicalTokenInput, 0, len(parts))
	var start int64
	for i, text := range parts {
		end := start + int64(len(text))
		textSHA := lexicalSHA([]byte(text))
		node := contentRef("r1_node", lexicalSHA([]byte(fmt.Sprintf("%s:%d", *target.Representation.ID, i))))
		ref := contentLexicalReference{Version: entity.KnowledgeContentVersion, Address: entity.KnowledgeContentAddress{Target: target, Node: node}, CoordinateSpace: "source_utf8", ReadingSHA256: reading, Range: entity.KnowledgeContentRange{StartByte: start, EndByte: end, SHA256: contentString(textSHA)}, Locator: entity.KnowledgeContentLocator{Kind: "source_utf8", SourceUTF8: &entity.KnowledgeContentSourceLocator{StartByte: start, EndByte: end, StartLine: int64(i + 1), EndLine: int64(i + 1)}}, CoordinatePrecision: "source_bytes", Assembly: "existing_passage_full_range"}
		reference := lexicalJSON(ref)
		referenceSHA := lexicalSHA([]byte(reference))
		inputs = append(inputs, lexicalTokenInput{ID: contentLexicalRowID(binding, referenceSHA, end-start, textSHA), Binding: binding, Reference: reference, ReferenceSHA256: referenceSHA, TextSHA256: textSHA, SourceID: sourceRef.SourceID, ByteLength: end - start, StartByte: start, Text: text, Name: fmt.Sprintf("synthetic-%d.md", source)})
		start = end
	}
	return inputs
}

func ordinalTestSelection(revision int) []lexicalTokenInput {
	var inputs []lexicalTokenInput
	for source := range 6 {
		variant := 0
		if source == 0 {
			variant = revision
		}
		inputs = append(inputs, ordinalTestInputs(source, variant)...)
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].ID < inputs[j].ID })
	return inputs
}

func ordinalTestProjection(row map[string]any) map[string]any {
	projection := make(map[string]any, len(row))
	for key, value := range row {
		if key != "create_time" && key != "create_timestamp_flt" {
			projection[key] = value
		}
	}
	return projection
}

func TestKnowledgeLexicalOrdinalSharedSixSourceProjection(t *testing.T) {
	contentLexicalTokenizer(t)
	oldResolver, attempts := component.DefaultEmbedderResolver, 0
	component.DefaultEmbedderResolver = func(context.Context, string, string) (component.Embedder, string, error) {
		attempts++
		return nil, "", errors.New("model call forbidden")
	}
	t.Cleanup(func() { component.DefaultEmbedderResolver = oldResolver })
	before := ordinalTestSelection(0)
	positions := make(map[string]int, len(before))
	for i, input := range before {
		positions[input.ID] = i
	}
	var after []lexicalTokenInput
	shifted := false
	for revision := 1; revision <= 64 && !shifted; revision++ {
		after = ordinalTestSelection(revision)
		for i, input := range after {
			if old, shared := positions[input.ID]; shared && old != i {
				shifted = true
			}
		}
	}
	if !shifted {
		t.Fatal("fixture must move an unchanged row in the global row-ID order")
	}
	job := entity.KnowledgeLexicalJobRecord{DomainID: knowledgeTestDomain, Language: "English", CreatedAt: time.Unix(1000, 0)}
	oldRows, _, err := lexicalTokenizeInputs(t.Context(), job, before)
	if err != nil {
		t.Fatal(err)
	}
	job.CreatedAt = job.CreatedAt.Add(time.Hour)
	newRows, _, err := lexicalTokenizeInputs(t.Context(), job, after)
	if err != nil {
		t.Fatal(err)
	}
	oldByID := make(map[string]map[string]any, len(oldRows))
	for _, row := range oldRows {
		oldByID[row["id"].(string)] = row
	}
	sources := make(map[string]bool)
	shared, nonzero := 0, 0
	for i, input := range after {
		sources[input.SourceID] = true
		if newRows[i]["id"] != input.ID {
			t.Fatal("tokenizer changed manifest order")
		}
		want := 0
		if input.StartByte > 0 {
			want = 1
			nonzero++
		}
		if newRows[i]["chunk_order_int"] != want {
			t.Fatalf("reading order for %s = %v, want %d", input.ID, newRows[i]["chunk_order_int"], want)
		}
		if old, ok := oldByID[input.ID]; ok {
			shared++
			if before[positions[input.ID]] != input || !reflect.DeepEqual(ordinalTestProjection(old), ordinalTestProjection(newRows[i])) {
				t.Fatalf("unchanged target changed its immutable row: %s", input.ID)
			}
		}
	}
	if len(sources) != 6 || shared != 10 || nonzero != 6 || attempts != 0 {
		t.Fatalf("sources=%d shared=%d nonzero=%d model_attempts=%d", len(sources), shared, nonzero, attempts)
	}
	// Selecting only one of the five unchanged bindings keeps both row projections.
	companion := ordinalTestInputs(1, 0)
	companion[0], companion[1] = companion[1], companion[0]
	rows, _, err := lexicalTokenizeInputs(t.Context(), job, companion)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if !reflect.DeepEqual(ordinalTestProjection(row), ordinalTestProjection(oldByID[row["id"].(string)])) {
			t.Fatal("subset or input permutation changed a fixed binding")
		}
	}
	if attempts != 0 {
		t.Fatalf("model_attempts=%d", attempts)
	}
}

func TestKnowledgeContentLexicalOrdinalPlanUsesReadingRanges(t *testing.T) {
	contentLexicalTokenizer(t)
	s, store := setupKnowledgeTest(t)
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	ctx := contentLexicalSession(t, s, true, true)
	docID := strings.Repeat("2", 32)
	body := []byte(strings.Repeat("Synthetic original reading passage.\r\n", 1000))
	uploadKnowledgeTest(t, store, docID, "long-source.txt", body)
	snapshot, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID, RelativePath: "long-source.txt", ComponentSide: "non_code"})
	if err != nil {
		t.Fatal(err)
	}
	target := contentTarget(contentSource(&snapshot.Source), "source_revision", snapshot.Source.ID, nil)
	prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Inputs) < 3 || len(prepared.Inputs) != len(prepared.Rows) {
		t.Fatal("expected several exact C1 ranges")
	}
	job := entity.KnowledgeLexicalJobRecord{DomainID: knowledgeTestDomain, Language: prepared.Policies.Language, CreatedAt: time.Unix(1000, 0)}
	rows, _, err := lexicalTokenizeInputs(ctx, job, prepared.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	byStart := make(map[int64]int, len(prepared.Inputs))
	for i, input := range prepared.Inputs {
		var ref contentLexicalReference
		if err := json.Unmarshal([]byte(input.Reference), &ref); err != nil {
			t.Fatal(err)
		}
		if input.StartByte != ref.Range.StartByte || input.ID != prepared.Rows[i].ID || rows[i]["id"] != input.ID || i > 0 && prepared.Inputs[i-1].ID >= input.ID {
			t.Fatal("plan lost its immutable range or paired row-ID order")
		}
		byStart[input.StartByte] = rows[i]["chunk_order_int"].(int)
	}
	var cursor int64
	for order := 0; order < len(prepared.Inputs); order++ {
		if got, ok := byStart[cursor]; !ok || got != order {
			t.Fatalf("range at %d has order %d, want %d", cursor, got, order)
		}
		for _, input := range prepared.Inputs {
			if input.StartByte == cursor {
				cursor += input.ByteLength
				break
			}
		}
	}
	if cursor != int64(len(body)) {
		t.Fatal("reading ranges do not cover the source")
	}
}

func TestKnowledgeLocalLexicalOrdinalAdapterUsesMemberRanges(t *testing.T) {
	contentLexicalTokenizer(t)
	unitsFor := func(source int) []lexicalUnit {
		parts := []string{"Synthetic first source passage.\r\n", "Synthetic second source passage.\n"}
		sourceRef := entity.KnowledgeLexicalSourceRef{DomainID: knowledgeTestDomain, SourceID: fmt.Sprintf("%032x", source), RevisionID: fmt.Sprintf("%032x", source+100), SHA256: lexicalSHA([]byte(strings.Join(parts, "")))}
		target := entity.KnowledgeLexicalTarget{Source: sourceRef, Representation: entity.KnowledgeLexicalRepresentationRef{Kind: "structure_view", ID: lexicalSHA([]byte(lexicalJSON(sourceRef))), ProducerSHA256: contentString(strings.Repeat("e", 64))}}
		plan := lexicalPlan("r1_existing_passage", "English", strings.Repeat("f", 64))
		binding := lexicalBinding(target, plan)
		var units []lexicalUnit
		var start int64
		for i, text := range parts {
			end, textSHA := start+int64(len(text)), lexicalSHA([]byte(text))
			member := lexicalStoredMember{Node: entity.KnowledgeLexicalNodeRef{Kind: "r1_node", ID: lexicalSHA([]byte(fmt.Sprintf("%s:%d", target.Representation.ID, i)))}, Range: lexicalStoredRange{Start: start, End: end, SHA256: textSHA}}
			reference := lexicalJSON(lexicalStoredReference{Version: entity.KnowledgeLocalLexicalVersion, Source: sourceRef, Representation: target.Representation, Members: []lexicalStoredMember{member}})
			id := lexicalRowID(binding, member.Node, entity.KnowledgeLexicalRange{Start: start, End: end}, textSHA)
			units = append(units, lexicalUnit{Row: lexicalManifestRow{ID: id, Binding: binding, Plan: plan, Reference: reference, ReferenceSHA256: lexicalSHA([]byte(reference)), TextSHA256: textSHA, ByteLength: end - start, Member: member, Target: target}, Text: text, Name: "synthetic.md"})
			start = end
		}
		return units
	}
	units := unitsFor(1)
	for source := 2; source <= 64; source++ {
		companion := unitsFor(source)
		if companion[0].Row.ID > companion[1].Row.ID {
			units = append(units, companion...)
			break
		}
	}
	if len(units) != 4 {
		t.Fatal("fixture must distinguish member reading order from row-ID order")
	}
	// Reverse the second binding so dropping StartByte cannot pass by input order.
	units[2], units[3] = units[3], units[2]
	job := entity.KnowledgeLexicalJobRecord{DomainID: knowledgeTestDomain, Language: "English", CreatedAt: time.Unix(1000, 0)}
	rows, _, err := lexicalTokenize(t.Context(), job, units)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{0, 1, 1, 0} {
		if rows[i]["id"] != units[i].Row.ID || rows[i]["chunk_order_int"] != want {
			t.Fatalf("member %d order=%v, want %d", i, rows[i]["chunk_order_int"], want)
		}
	}
}
