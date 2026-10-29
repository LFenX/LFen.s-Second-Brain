//go:build linux

package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func contentLexicalContextCopy(t *testing.T, original *contentLexicalSnapshot) *contentLexicalSnapshot {
	t.Helper()
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var copy contentLexicalSnapshot
	if json.Unmarshal(raw, &copy) != nil {
		t.Fatal("snapshot copy")
	}
	return &copy
}

func TestKnowledgeContentLexicalTypedContextScopeBudgetAndAssembly(t *testing.T) {
	contentLexicalTokenizer(t)
	s, ctx, selection, _ := contentLexicalMixedFixture(t)
	var target entity.KnowledgeContentTarget
	for _, candidate := range selection.Targets {
		if candidate.Representation.Kind == "parsed_representation" {
			target = candidate
		}
	}
	prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}, true)
	if err != nil || len(prepared.Rows) != 1 {
		t.Fatal("parsed table plan", err)
	}
	ref := prepared.Rows[0].Reference
	snapshot, err := s.contentLexicalSnapshot(ctx, knowledgeTestOwner, knowledgeTestDomain, target, prepared.Policies.Language, prepared.ProducerSHA256)
	if err != nil {
		t.Fatal(err)
	}
	node, err := s.GetKnowledgeContentNode(ctx, knowledgeTestOwner, knowledgeTestDomain, ref.Address)
	if err != nil {
		t.Fatal(err)
	}
	contextValue, err := contentLexicalBuildContext(snapshot, node.Node, ref)
	if err != nil || len(contextValue.TypedMembers) != 1 || !contextValue.MembersComplete || contextValue.TypedMembers[0].TextAssembly != "row_major_uncovered_cells_tab_lf_v1" {
		t.Fatal("whole table must retain verified assembly", err)
	}
	for _, test := range []string{"boundary", "grid_tamper", "metadata_budget", "member_limit", "partial"} {
		t.Run(test, func(t *testing.T) {
			copy := contentLexicalContextCopy(t, snapshot)
			bounded := ref
			switch test {
			case "boundary":
				bounded.Range.EndByte = bounded.Range.StartByte + 1
			case "grid_tamper":
				for i := range copy.TypedMembers {
					if copy.TypedMembers[i].Node.Kind == "table" {
						copy.TypedMembers[i].Node.Attributes.Parser.Table.Grid[0][0].Text += "tampered"
					}
				}
			case "metadata_budget":
				for i := range copy.TypedMembers {
					if copy.TypedMembers[i].Node.Kind == "table" {
						copy.TypedMembers[i].Node.Attributes.Parser.Table.Caption = strings.Repeat("<>&", 21000)
					}
				}
			case "member_limit":
				member := contextValue.TypedMembers[0]
				member.Node.Kind = "text_block"
				member.Node.Attributes = entity.KnowledgeContentAttributes{Parser: &entity.KnowledgeContentParserAttributes{ItemType: "text"}}
				copy.TypedMembers = nil
				for i := range 33 {
					item := member
					item.Node.Ref.ID = contentString(lexicalSHA([]byte(fmt.Sprint(i))))
					copy.TypedMembers = append(copy.TypedMembers, item)
				}
			case "partial":
				copy.Unsearchable = 1
				copy.Representation.State = "partial"
			}
			value, err := contentLexicalBuildContext(copy, node.Node, bounded)
			if test == "grid_tamper" {
				lexicalAssertCode(t, err, "integrity_error")
				return
			}
			if err != nil || value.MembersComplete || value.MetadataBytes > 65536 || value.MetadataBytes != len(lexicalJSON(value)) || len(value.TypedMembers) > 32 {
				t.Fatal("scope or cap lost", test, err)
			}
			want := map[string]string{"boundary": "whole_member_outside_evidence", "metadata_budget": "metadata_budget", "member_limit": "member_limit", "partial": "representation_contains_unsearchable_nodes"}[test]
			if !slices.Contains(value.OmissionReasons, want) {
				t.Fatal("omission missing", test, value.OmissionReasons)
			}
			if (test == "boundary" || test == "metadata_budget") && len(value.TypedMembers) != 0 {
				t.Fatal("partial or oversize table leaked")
			}
		})
	}
}

func contentLexicalDOCXWithEmptyCell(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>合成 DOCX</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p/></w:tc><w:tc><w:p><w:r><w:t>B</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`},
	}
	for _, part := range parts {
		file, err := writer.Create(part.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(part.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func contentLexicalPublishForRead(t *testing.T, s *DatasetService, ctx context.Context, selection entity.KnowledgeContentLexicalSelection) (*entity.KnowledgeContentLexicalJob, *contentLexicalManifest) {
	t.Helper()
	inspect, err := s.InspectKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalInspectRequest{Version: entity.KnowledgeContentVersion, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := s.CreateKnowledgeContentLexicalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalCreateRequest{Version: entity.KnowledgeContentVersion, Key: "context_read_fixture", ExpectedLocalVersion: inspect.Policy.LocalVersion, ExpectedContentVersion: inspect.Policy.ContentVersion, ExpectedFoundationVersion: inspect.Policy.FoundationVersion, ExpectedProfileSHA256: inspect.Profile.SHA256, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	runtime, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	s.startKnowledgeLocalLexicalRuntime(runtime)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err = s.GetKnowledgeContentLexicalJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == "published" || job.State == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if job.State != "published" || job.Index == nil {
		t.Fatal("context fixture publication", job.State, job.Readiness.Diagnostics)
	}
	_, manifest, err := s.contentLexicalPublished(ctx, knowledgeTestOwner, knowledgeTestDomain, *job.Index, false)
	if err != nil {
		t.Fatal(err)
	}
	return job, manifest
}

func TestKnowledgeContentLexicalActualR2DOCXEmptyCellContext(t *testing.T) {
	contentLexicalTokenizer(t)
	s, _ := documentSetupTest(t, true)
	reg, err := s.GetKnowledgeOriginalRegistration(t.Context(), knowledgeTestOwner, knowledgeTestDomain, "", "original:synthetic")
	if err != nil {
		t.Fatal(err)
	}
	input := contentLexicalDOCXWithEmptyCell(t)
	upload := originalTestRequest("actual_docx_empty_cell")
	upload.OriginalFilename = "empty-cell.docx"
	upload.RelativePath = "docs/sample.docx"
	upload.DeclaredMediaType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	ready := prepareOriginalFlow(t, s, reg, upload, input)
	receipt := confirmOriginalFlow(t, s, reg, ready, "actual_docx_empty_confirm")
	lifecycle, err := s.GetKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, receipt.Source.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := documentProfile(entity.KnowledgeDocumentDOCXProfile)
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeDocumentTaskRequest{Target: entity.KnowledgeDocumentOriginalTarget{Kind: "original_document", SourceID: receipt.Source.SourceID, SourceRevisionID: receipt.Source.SourceRevisionID, OriginalSHA256: receipt.Source.SHA256, ExpectedLifecycleVersion: lifecycle.Current.LifecycleVersion}, ExpectedPolicyVersion: 1, ExpectedDocumentPolicyVersion: 2, ProfileID: profile.ProfileID, ExpectedProducerSHA256: profile.ProducerSHA256, Confirmed: true, IdempotencyKey: "actual_docx_empty_task"})
	if err != nil {
		t.Fatal(err)
	}
	var task entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", value.(*entity.KnowledgeDocumentTask).TaskID).Take(&task).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), task, common.GenerateUUID())
	if err != nil || claim == nil {
		t.Fatal("claim", err)
	}
	s.runKnowledgeDocument(t.Context(), claim)
	doneValue, err := s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	done := doneValue.(*entity.KnowledgeDocumentTask)
	if done.State != "succeeded" || done.ResultRepresentationID == nil {
		t.Fatal("actual DOCX child failed", done.State, done.FailureCode)
	}
	rep, err := s.GetKnowledgeDocumentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *done.ResultRepresentationID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := contentLexicalSession(t, s, true, true)
	s.docEngine = &lexicalTestEngine{rows: map[string]map[string]any{}}
	target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: knowledgeTestDomain, SourceID: rep.Original.SourceID, RevisionID: rep.Original.SourceRevisionID, SHA256: rep.Original.SHA256}, "document_representation", rep.RepresentationID, &rep.ProducerSHA256)
	job, manifest := contentLexicalPublishForRead(t, s, ctx, entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}})
	var emptyCells int
	reads := []entity.KnowledgeContentLexicalReadData{}
	for _, row := range manifest.Rows {
		read, err := s.ReadKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: contentLexicalEvidence(*job.Index, row)})
		if err != nil {
			t.Fatal(err)
		}
		if !read.Context.MembersComplete {
			t.Fatal("complete DOCX typed context reported partial", read.Context.OmissionReasons)
		}
		for _, member := range read.Context.TypedMembers {
			n := member.Node
			if n.Kind == "cell" && n.Text.Status == "empty" {
				emptyCells++
				if n.Text.Range == nil || n.Text.Range.StartByte != n.Text.Range.EndByte || n.Text.Range.SHA256 == nil || *n.Text.Range.SHA256 != lexicalSHA(nil) || n.Attributes.Document.Cell == nil || n.Attributes.Document.Cell.CellIndex != 1 || n.Locator.Document == nil || n.Locator.Document.Scheme != "office_package_v1" {
					t.Fatal("empty cell semantics lost", member)
				}
			}
		}
		reads = append(reads, *read)
	}
	if emptyCells != 1 {
		t.Fatal("actual manifest empty cell silently omitted", emptyCells)
	}
	proof, err := json.MarshalIndent(map[string]any{"scope": "Actual_synthetic_DOCX_original_owned_child_publication_and_owning_context", "source_sha256": lexicalSHA(input), "reads": reads, "empty_cells": emptyCells, "quality": "NotStarted"}, "", "  ")
	if err != nil || os.WriteFile("lexical-docx-empty-context-proof.json", append(proof, '\n'), 0600) != nil {
		t.Fatal("empty-cell proof", err)
	}
}

func TestKnowledgeContentLexicalSharesLegacyActiveJobQuota(t *testing.T) {
	contentLexicalTokenizer(t)
	s, ctx, selection, _ := contentLexicalMixedFixture(t)
	var target entity.KnowledgeContentTarget
	for _, candidate := range selection.Targets {
		if candidate.Representation.Kind == "structure_view" {
			target = candidate
		}
	}
	oldSelection := entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{{Source: entity.KnowledgeLexicalSourceRef{DomainID: knowledgeTestDomain, SourceID: target.Source.SourceID, RevisionID: target.Source.RevisionID, SHA256: target.Source.SHA256}, Representation: entity.KnowledgeLexicalRepresentationRef{Kind: "structure_view", ID: *target.Representation.ID, ProducerSHA256: target.Representation.ProducerSHA256}}}}
	for i := range 3 {
		_, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: fmt.Sprintf("old_quota_%d", i), ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: oldSelection})
		if err != nil {
			t.Fatal(err)
		}
	}
	inspect, err := s.InspectKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalInspectRequest{Version: entity.KnowledgeContentVersion, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	request := entity.KnowledgeContentLexicalCreateRequest{Version: entity.KnowledgeContentVersion, Key: "new_quota_4", ExpectedLocalVersion: inspect.Policy.LocalVersion, ExpectedContentVersion: inspect.Policy.ContentVersion, ExpectedFoundationVersion: inspect.Policy.FoundationVersion, ExpectedProfileSHA256: inspect.Profile.SHA256, Selection: selection}
	if _, created, err := s.CreateKnowledgeContentLexicalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, request); err != nil || !created {
		t.Fatal("shared fourth slot", err)
	}
	request.Key = "new_quota_5"
	_, _, err = s.CreateKnowledgeContentLexicalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, request)
	lexicalAssertCode(t, err, "capacity_exceeded")
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeLexicalJobRecord{}).Count(&count).Error; err != nil || count != 4 {
		t.Fatal("parallel quota created another queue", count, err)
	}
}
