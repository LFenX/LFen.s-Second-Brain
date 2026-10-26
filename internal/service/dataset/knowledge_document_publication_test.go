package dataset

import (
	"encoding/json"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func documentEmptyPDFProjection() parser.DocumentStructureResult {
	empty := entity.KnowledgeDocumentReadingRange{TextSHA256: directorySHA(nil)}
	root := entity.KnowledgeDocumentNodePayload{Kind: "document", Attributes: json.RawMessage(`{}`), Locator: entity.KnowledgeDocumentLocator{Scheme: "document_v1"}, OriginalPosition: entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}, CoordinatePrecision: "document", TextStatus: "empty", ReadingRange: empty}
	parent := entity.KnowledgeDocumentNaturalRef{Kind: "document", Locator: root.Locator}
	reason := "no_extractable_text"
	page := entity.KnowledgeDocumentNodePayload{Kind: "page", Attributes: json.RawMessage(`{"page_number":1,"page_content_classification":"unknown"}`), Locator: entity.KnowledgeDocumentLocator{Scheme: "pdf_page_v1", PageNumber: 1}, ParentRef: &parent, OriginalPosition: entity.KnowledgeDocumentOriginalPosition{Kind: "pdf_page", PageNumber: 1}, CoordinatePrecision: "physical_page", TextStatus: "unsupported_or_requires_ocr", TextReason: &reason, ReadingRange: empty}
	count := int64(1)
	return parser.DocumentStructureResult{ReadingText: "", Payloads: []entity.KnowledgeDocumentNodePayload{root, page}, PageCount: &count, Quality: "complete", TextCoverage: "supported_profile_blocks"}
}

func TestKnowledgeDocumentPublicationRejectsCorruptReuse(t *testing.T) {
	s, req := documentSetupTest(t, true)
	prepare := func(req entity.KnowledgeDocumentTaskRequest) (*structureClaim, *documentBound) {
		t.Helper()
		v, created, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
		if err != nil || !created {
			t.Fatal(v, created, err)
		}
		id := v.(*entity.KnowledgeDocumentTask).TaskID
		var row entity.KnowledgeStructureTaskRecord
		if err := dao.DB.Where("id = ?", id).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
		if err != nil || claim == nil {
			t.Fatal(claim, err)
		}
		fixed, err := documentDecode[documentInputs](row.Inputs)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, documentEmptyPDFProjection())
		if err != nil {
			t.Fatal(err)
		}
		return claim, bound
	}
	claim, bound := prepare(req)
	if err := s.documentPublish(t.Context(), claim, bound); err != nil {
		t.Fatal(err)
	}
	repID := bound.Representation.RepresentationID
	req.ExpectedActiveRepresentationID = &repID
	req.IdempotencyKey = "reuse-corrupt"
	claim2, bound2 := prepare(req)
	if bound2.Representation.RepresentationID != repID {
		t.Fatal("task key/head entered content identity")
	}
	if err := dao.DB.Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("id = ?", repID).Update("reading_text", "synthetic-corruption").Error; err != nil {
		t.Fatal(err)
	}
	err := s.documentPublish(t.Context(), claim2, bound2)
	documentAssertError(t, err, 409, "document_integrity", false)
	var head entity.KnowledgeDocumentHeadRecord
	if err := dao.DB.Where("source_id = ?", req.Target.SourceID).Take(&head).Error; err != nil || head.RepresentationID != repID {
		t.Fatal("failed reuse changed head", head, err)
	}
	s.documentFailClaim(claim2, "document_integrity")
	if err := dao.DB.Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("id = ?", repID).Update("reading_text", "").Error; err != nil {
		t.Fatal(err)
	}
	req.IdempotencyKey = "reuse-restored"
	claim3, bound3 := prepare(req)
	if err := s.documentPublish(t.Context(), claim3, bound3); err != nil {
		t.Fatal(err)
	}
	fixed, err := s.GetKnowledgeDocumentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, repID)
	if err != nil || fixed.ReadingByteLength != 0 || fixed.NodeCount != 2 {
		t.Fatal(fixed, err)
	}
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", req.Target.SourceID).Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, claim3.TaskID)
	documentAssertError(t, err, 404, "not_found", false)
	_, err = s.GetKnowledgeDocumentOperation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, "task", req.IdempotencyKey)
	documentAssertError(t, err, 404, "not_found", false)
	_, _, err = s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	documentAssertError(t, err, 404, "not_found", false)
	_, err = s.GetKnowledgeDocumentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, repID)
	documentAssertError(t, err, 404, "not_found", false)
}
