//go:build linux

package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func TestParsedSourceNativeRevisionReuseAndOldAPIScopes(t *testing.T) {
	firstBytes, nextBytes := []byte("A,B\n1,2\n"), []byte("A,B\n3,4\n")
	s, first, _, store := parsedExecutionFixture(t, "rows.csv", firstBytes, false)
	request := entity.KnowledgeParsedSnapshotRequest{DocumentID: first.DocumentID, ExpectedRevisionID: first.ID, ExpectedPolicyVersion: 1, RelativePath: "rows.csv", Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}}
	unchanged, err := s.SnapshotKnowledgeParsedSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, request)
	if err != nil || unchanged.ID != first.ID {
		t.Fatal("unchanged native input created another revision", err)
	}
	if err := store.Put(t.Context(), knowledgeTestDomain, "synthetic/"+first.DocumentID, nextBytes); err != nil {
		t.Fatal(err)
	}
	next, err := s.SnapshotKnowledgeParsedSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, request)
	if err != nil || next.ID == first.ID {
		t.Fatal("new native bytes did not advance revision", err)
	}
	if err := store.Put(t.Context(), knowledgeTestDomain, "synthetic/"+first.DocumentID, firstBytes); err != nil {
		t.Fatal(err)
	}
	request.ExpectedRevisionID = next.ID
	restored, err := s.SnapshotKnowledgeParsedSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, request)
	if err != nil || restored.ID != first.ID {
		t.Fatal("existing immutable native revision was duplicated", err)
	}
	state, err := s.GetKnowledgeParsedSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.DocumentID)
	if err != nil || state.LifecycleVersion != 3 || state.Source != contentSource(first) || state.Profile == nil || state.State != "active" {
		t.Fatal(state, err)
	}
	var commits int64
	if err := dao.DB.Model(&entity.FileCommit{}).Count(&commits).Error; err != nil || commits != 2 {
		t.Fatal("metadata reuse appended a native commit", commits, err)
	}
	old, err := s.ListKnowledgeOriginalRevisions(t.Context(), knowledgeTestOwner, knowledgeTestDomain, nil, "")
	if err != nil || len(old.Items) != 0 {
		t.Fatal("new format broke old original-document list", old, err)
	}
	lifecycles, err := s.ListKnowledgeSourceLifecycles(t.Context(), knowledgeTestOwner, knowledgeTestDomain, KnowledgeLifecycleListQuery{})
	if err != nil || len(lifecycles.Items) != 0 {
		t.Fatal("new kind leaked into existing lifecycle scope", lifecycles, err)
	}
	if _, err := s.GetKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.DocumentID); err == nil {
		t.Fatal("old lifecycle detail exposed new source family")
	}
	meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, contentTarget(contentSource(first), "source_revision", first.ID, nil))
	if err != nil || meta.State != "unavailable" || meta.RootRef != nil || meta.NodeCount == nil || *meta.NodeCount != 0 {
		t.Fatal("unparsed opaque source received a fake text root", meta, err)
	}
}

func parsedAssertNodePages100(t *testing.T, s *DatasetService, target entity.KnowledgeContentTarget, root entity.KnowledgeContentNodeRef, expected int) {
	t.Helper()
	query := entity.KnowledgeContentNodeQuery{Target: target, Parent: &root, Limit: 100}
	seen := map[string]bool{}
	firstCursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
		if err != nil {
			t.Fatal("unified limit100 page failed", err)
		}
		if len(page.Items) == 0 || len(page.Items) > 20 {
			t.Fatal("unified page size", len(page.Items))
		}
		for _, node := range page.Items {
			if node.Ref.ID == nil || seen[*node.Ref.ID] {
				t.Fatal("duplicate node across cursor pages")
			}
			seen[*node.Ref.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		if firstCursor == "" {
			firstCursor = *page.NextCursor
		}
		query.Cursor = *page.NextCursor
	}
	if len(seen) != expected || firstCursor == "" {
		t.Fatal("node pagination coverage", len(seen), expected)
	}
	query.Cursor, query.Limit = firstCursor, 99
	_, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
	contentAssertStatus(t, err, 400)
	query.Limit = 100
	query.Parent = nil
	_, err = s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
	contentAssertStatus(t, err, 400)
}

func TestParsedUnifiedNodeLimit100R1R2AndParsedPages(t *testing.T) {
	t.Run("R1", func(t *testing.T) {
		s, view, rootID := setupStructureReadTest(t, []byte("synthetic pagination"))
		root, err := s.GetKnowledgeStructureNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, view.ViewID, rootID)
		if err != nil {
			t.Fatal(err)
		}
		// Constructed fixed R1 nodes exercise pagination, not parser quality.
		for i := 0; i < 25; i++ {
			node := *root
			node.StructureNodeID = entity.KnowledgeID("synthetic content pagination", fmt.Sprint(i))
			node.Kind = "passage"
			node.ParentID = &rootID
			node.Name = fmt.Sprint(i)
			raw, _ := json.Marshal(node)
			if err := dao.DB.Create(&entity.KnowledgeStructureNodeRecord{ID: node.StructureNodeID, OwnerID: knowledgeTestOwner, DomainID: knowledgeTestDomain, ViewID: view.ViewID, SourceID: node.SourceID, SourceRevisionID: node.SourceRevisionID, ParentID: rootID, Kind: node.Kind, StartByte: node.StartByte, EndByte: node.EndByte, Payload: string(raw)}).Error; err != nil {
				t.Fatal(err)
			}
		}
		target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: knowledgeTestDomain, SourceID: view.Members[0].SourceID, RevisionID: view.Members[0].SourceRevisionID, SHA256: view.Members[0].SHA256}, "structure_view", view.ViewID, &view.ProducerSHA256)
		if page, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Limit: 100}); err != nil || len(page.Items) != 1 {
			t.Fatal("R1 root limit100", err)
		}
		parsedAssertNodePages100(t, s, target, contentRef("r1_node", rootID), 25)
	})
	t.Run("R2", func(t *testing.T) {
		s, req := documentSetupTest(t, true)
		value, _, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
		if err != nil {
			t.Fatal(err)
		}
		var row entity.KnowledgeStructureTaskRecord
		if err := dao.DB.Where("id = ?", value.(*entity.KnowledgeDocumentTask).TaskID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
		if err != nil || claim == nil {
			t.Fatal(err)
		}
		fixed, err := documentDecode[documentInputs](row.Inputs)
		if err != nil {
			t.Fatal(err)
		}
		projection := documentEmptyPDFProjection()
		page := projection.Payloads[1]
		projection.Payloads = projection.Payloads[:1]
		count := int64(25)
		projection.PageCount = &count
		for i := int64(1); i <= 25; i++ {
			value := page
			value.Locator.PageNumber = i
			value.OriginalPosition.PageNumber = i
			value.Ordinal = i - 1
			value.Attributes = json.RawMessage(fmt.Sprintf(`{"page_number":%d,"page_content_classification":"unknown"}`, i))
			projection.Payloads = append(projection.Payloads, value)
		}
		bound, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, projection)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.documentPublish(t.Context(), claim, bound); err != nil {
			t.Fatal(err)
		}
		rep := bound.Representation
		target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: knowledgeTestDomain, SourceID: rep.Original.SourceID, RevisionID: rep.Original.SourceRevisionID, SHA256: rep.Original.SHA256}, "document_representation", rep.RepresentationID, &rep.ProducerSHA256)
		parsedAssertNodePages100(t, s, target, contentRef("r2_node", rep.RootDocumentNodeID), 25)
	})
	t.Run("parsed", func(t *testing.T) {
		var body strings.Builder
		body.WriteString("A,B\n")
		for i := 0; i < 60; i++ {
			fmt.Fprintf(&body, "row%d,value%d\n", i, i)
		}
		s, _, req, _ := parsedExecutionFixture(t, "rows.csv", []byte(body.String()), true)
		req.MaxUnitBytes = 32
		task, claim := parsedExecutionClaim(t, s, req)
		s.runKnowledgeParsed(t.Context(), claim)
		done := parsedExecutionCurrent(t, s, task.TaskID)
		if done.State != "succeeded" || done.Result == nil {
			t.Fatal(done.State, done.FailureCode)
		}
		meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *done.Result)
		if err != nil || meta.NodeCount == nil {
			t.Fatal(err)
		}
		parsedAssertNodePages100(t, s, *done.Result, *meta.RootRef, int(*meta.NodeCount)-1)
	})
}

func TestParsedExecutionActualMergedHTMLAndEmptyJSON(t *testing.T) {
	for _, fixture := range []struct{ name, input, state string }{
		{"table.html", `<table><tr><td>A</td><td>B</td><td>C</td></tr><tr><td>D</td><td rowspan="2">MERGED</td><td>E</td></tr><tr><td>F</td><td>G</td></tr></table>`, "succeeded"},
		{"empty.json", "", "succeeded"},
		{"empty.csv", "", "failed"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			s, _, req, _ := parsedExecutionFixture(t, fixture.name, []byte(fixture.input), true)
			task, claim := parsedExecutionClaim(t, s, req)
			s.runKnowledgeParsed(t.Context(), claim)
			done := parsedExecutionCurrent(t, s, task.TaskID)
			if done.State != fixture.state {
				t.Fatal("unexpected actual producer publication state", done.State, done.FailureCode)
			}
			if done.Result == nil {
				if fixture.state != "failed" {
					t.Fatal("successful task lost fixed target")
				}
				return
			}
			meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *done.Result)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.name == "empty.json" && meta.State != "empty" {
				t.Fatal("explicit empty JSON was changed", meta.State)
			}
			if fixture.name == "table.html" {
				var rows []entity.KnowledgeDocumentNodeRecord
				if err := dao.DB.Where("representation_id = ?", *done.Result.Representation.ID).Find(&rows).Error; err != nil {
					t.Fatal(err)
				}
				found := false
				for _, row := range rows {
					var node entity.KnowledgeParsedNode
					if entity.KnowledgeParsedDecode([]byte(row.Payload), &node) != nil {
						t.Fatal("stored typed node failed decode")
					}
					if strings.Contains(row.Payload, "table_origins") {
						t.Fatal("diagnostic-only geometry became a storage contract")
					}
					if node.Payload.Attributes.Parser == nil || node.Payload.Attributes.Parser.Table == nil {
						continue
					}
					table := node.Payload.Attributes.Parser.Table
					if len(table.Grid) > 2 && len(table.Grid[2]) > 1 && table.Grid[2][1].Covered && table.Grid[2][1].OriginRow == 1 && table.Grid[2][1].OriginCol == 1 {
						found = true
					}
				}
				if !found {
					t.Fatal("fixed publication lost explicit merged geometry")
				}
			}
		})
	}
}

func TestParsedExecutionCancelDuringOwnedStart(t *testing.T) {
	s, source, req, _ := parsedExecutionFixture(t, "rows.csv", []byte("A,B\n1,2\n"), true)
	task, claim := parsedExecutionClaim(t, s, req)
	input, err := parsedOriginalBytes(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithCancel(t.Context())
	defer cancel()
	control := parser.DocumentChildControl{TaskID: task.TaskID, Attempt: 1, Format: "csv", Filename: "rows.csv", ProfileID: parser.KnowledgeParsedLocalProfile, ProducerSHA256: req.ExpectedProducerSHA256, InputSHA256: req.Source.SHA256, InputBytes: int64(len(input))}
	var observed parser.DocumentChildOwnership
	_, err = parser.RunDocumentStructureChild(work, input, control, func(owner parser.DocumentChildOwnership) error {
		observed = owner
		if err := parsedExecutionStoreOwner(t.Context(), s, claim, owner); err != nil {
			return err
		}
		current := parsedExecutionCurrent(t, s, task.TaskID)
		if _, _, err := s.CancelKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID, entity.KnowledgeParsedCancelRequest{ExpectedTaskRevision: current.Revision, IdempotencyKey: "cancel-owned-start"}); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || errors.Is(err, parser.ErrDocumentChildUnresolved) {
		t.Fatal("owned cancellation exit/cleanup was unproven", err)
	}
	if _, err := os.Lstat(observed.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled input directory remains", err)
	}
	if err := parser.RecoverDocumentStructureChild(t.Context(), observed); err != nil {
		t.Fatal("cancelled child ownership remains", err)
	}
	s.parsedFinishClaim(claim, "native_process_failed")
	if done := parsedExecutionCurrent(t, s, task.TaskID); done.State != "cancelled" || done.Result != nil {
		t.Fatal("confirmed owned cancellation did not retire task", done.State)
	}
}
