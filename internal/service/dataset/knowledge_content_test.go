package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

func contentAssertStatus(t *testing.T, err error, status int) {
	t.Helper()
	var own *KnowledgeError
	if !errors.As(err, &own) || own.Status != status {
		t.Fatalf("expected %d; got %#v", status, err)
	}
}

func TestKnowledgeContentC1R1FixedReadAndRanges(t *testing.T) {
	body := []byte(strings.Repeat("x", 65534) + "合成\r\n")
	s, view, rootID := setupStructureReadTest(t, body)
	source, err := s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, view.Members[0].SourceRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []entity.KnowledgeContentTarget{contentTarget(contentSource(source), "source_revision", source.ID, nil), contentTarget(contentSource(source), "structure_view", view.ViewID, &view.ProducerSHA256)} {
		meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, target)
		if err != nil || meta.RootRef == nil || meta.CoordinateSpace != "source_utf8" {
			t.Fatal(meta, err)
		}
		if target.Representation.Kind == "structure_view" && *meta.RootRef.ID != rootID {
			t.Fatal("R1 root replaced with C1 identity")
		}
		address := entity.KnowledgeContentAddress{Target: target, Node: *meta.RootRef}
		node, err := s.GetKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, address)
		if err != nil || node.Node.Ordinal != nil || node.Node.Text.Value != nil || node.Node.Text.OwnRange != nil || node.Node.Locator.SourceUTF8.EndByte != int64(len(body)) {
			t.Fatal(node, err)
		}
		first, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address})
		if err != nil || first.Complete || first.NextStartByte == nil || first.Range.EndByte != 65534 || first.Range.SHA256 == nil || *first.Range.SHA256 != directorySHA([]byte(first.Text)) {
			t.Fatal(first, err)
		}
		second, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address, StartByte: first.NextStartByte})
		if err != nil || !second.Complete || first.Text+second.Text != string(body) {
			t.Fatal(second, err)
		}
		page, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Limit: 5})
		if err != nil || len(page.Items) != 1 || *page.Items[0].Ref.ID != *meta.RootRef.ID {
			t.Fatal(page, err)
		}
		bad := int64(65535)
		_, err = s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address, StartByte: &bad})
		contentAssertStatus(t, err, 400)
		wrong := target
		wrong.Source.SHA256 = strings.Repeat("f", 64)
		_, err = s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, wrong)
		contentAssertStatus(t, err, 409)
	}
}

func TestKnowledgeContentProjectionRejectsMixedSourceAndCopiesMetadata(t *testing.T) {
	s, view, id := setupStructureReadTest(t, []byte("合成\n正文"))
	source, err := s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, view.Members[0].SourceRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	node, err := s.GetKnowledgeStructureNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, view.ViewID, id)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := contentProjectR1(source, view, node)
	if err != nil {
		t.Fatal(err)
	}
	*node.Classification.GroupID = "mutated local classification"
	if *projected.Attributes.Structure.Classification.GroupID == *node.Classification.GroupID {
		t.Fatal("classification aliases producer memory")
	}
	changed := *source
	changed.ID = strings.Repeat("e", 32)
	if _, err := contentProjectR1(&changed, view, node); err == nil {
		t.Fatal("accepted mixed revision")
	}
	changed = *source
	changed.RepresentationKind = "original_only"
	root := knowledgeRoot(&changed)
	if _, err := contentProjectC1(&changed, &root); err == nil {
		t.Fatal("invented UTF-8 node for binary original")
	}
}

type contentRevokingStorage struct {
	storage.Storage
	onGet func()
}

func (s *contentRevokingStorage) Get(ctx context.Context, bucket, name string, tenant ...string) ([]byte, error) {
	value, err := s.Storage.Get(ctx, bucket, name, tenant...)
	if s.onGet != nil {
		f := s.onGet
		s.onGet = nil
		f()
	}
	return value, err
}

func TestKnowledgeContentRechecksAuthorityAfterSourceRead(t *testing.T) {
	s, view, _ := setupStructureReadTest(t, []byte("合成撤权内容"))
	source, err := s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, view.Members[0].SourceRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	target := contentTarget(contentSource(source), "source_revision", source.ID, nil)
	address := entity.KnowledgeContentAddress{Target: target, Node: contentRef("c1_node", source.RootNodeID)}
	store := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(&contentRevokingStorage{Storage: store, onGet: func() {
		if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "0").Error; err != nil {
			t.Fatal(err)
		}
	}})
	defer storage.GetStorageFactory().SetStorage(store)
	window, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address})
	contentAssertStatus(t, err, 404)
	if window != nil {
		t.Fatal("content escaped after revoke during storage read")
	}
	_, err = s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, target)
	contentAssertStatus(t, err, 404)
	_, err = s.GetKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, address)
	contentAssertStatus(t, err, 404)
	_, err = s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Limit: 5})
	contentAssertStatus(t, err, 404)
}

func contentPublishedDocument(t *testing.T, text string) (*DatasetService, *documentBound) {
	t.Helper()
	s, req := documentSetupTest(t, true)
	v, created, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatal(v, created, err)
	}
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", v.(*entity.KnowledgeDocumentTask).TaskID).Take(&row).Error; err != nil {
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
	result := documentEmptyPDFProjection()
	if text != "" {
		reading := entity.KnowledgeDocumentReadingRange{EndByte: int64(len(text)), TextSHA256: directorySHA([]byte(text))}
		result.ReadingText = text
		for i := range result.Payloads {
			result.Payloads[i].TextStatus, result.Payloads[i].TextReason, result.Payloads[i].ReadingRange = "available", nil, reading
		}
		root := result.Payloads[0]
		passage := root
		passage.Kind, passage.Attributes, passage.Ordinal, passage.CoordinatePrecision = "passage", json.RawMessage(`{"chunk_index":0}`), 1, "derived_range"
		passage.ParentRef = &entity.KnowledgeDocumentNaturalRef{Kind: root.Kind, Locator: root.Locator}
		passage.Locator = entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &root.Locator, ChunkIndex: 0}
		passage.OwnReadingRange = &reading
		result.Payloads = append(result.Payloads, passage)
	}
	// This supplies a constructed projection to the real binding/publication code;
	// it tests fixed read integrity, not PDF extraction quality.
	bound, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.documentPublish(t.Context(), claim, bound); err != nil {
		t.Fatal(err)
	}
	return s, bound
}

func TestKnowledgeContentR2PersistedDerivedReadAndOriginalUnavailable(t *testing.T) {
	s, bound := contentPublishedDocument(t, "合成派生正文\n")
	rep := bound.Representation
	target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: rep.DomainID, SourceID: rep.Original.SourceID, RevisionID: rep.Original.SourceRevisionID, SHA256: rep.Original.SHA256}, "document_representation", rep.RepresentationID, &rep.ProducerSHA256)
	meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, target)
	if err != nil || meta.CoordinateSpace != "derived_utf8" || meta.NodeCount == nil || *meta.NodeCount != 3 {
		t.Fatal(meta, err)
	}
	page, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Limit: 5})
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "document" {
		t.Fatal(page, err)
	}
	children, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Parent: meta.RootRef, Limit: 5})
	if err != nil || len(children.Items) != 2 || children.Items[0].Locator.Document.Scheme != "pdf_page_v1" || children.Items[1].Text.OwnRange == nil {
		t.Fatal(children, err)
	}
	address := entity.KnowledgeContentAddress{Target: target, Node: children.Items[1].Ref}
	negative := int64(-65537)
	_, err = s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address, StartByte: &negative})
	contentAssertStatus(t, err, 400)
	window, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address})
	if err != nil || window.Text != "合成派生正文\n" || window.CoordinateSpace != "derived_utf8" || window.Address.Target.Source.SHA256 == *window.Range.SHA256 || *window.Range.SHA256 != directorySHA([]byte(window.Text)) {
		t.Fatal(window, err)
	}
	originalTarget := contentTarget(target.Source, "source_revision", target.Source.RevisionID, nil)
	original, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, originalTarget)
	if err != nil || original.RootRef != nil || original.State != "unavailable" || original.CoordinateSpace != "" {
		t.Fatal(original, err)
	}
	if err := dao.DB.Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("id = ?", rep.RepresentationID).Update("reading_text", "tampered").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address})
	contentAssertStatus(t, err, 409)
}

func TestKnowledgeContentR2EmptyPhysicalPageIsExplicit(t *testing.T) {
	s, bound := contentPublishedDocument(t, "")
	rep := bound.Representation
	target := contentTarget(entity.KnowledgeContentSourceRef{DomainID: rep.DomainID, SourceID: rep.Original.SourceID, RevisionID: rep.Original.SourceRevisionID, SHA256: rep.Original.SHA256}, "document_representation", rep.RepresentationID, &rep.ProducerSHA256)
	root := contentRef("r2_node", rep.RootDocumentNodeID)
	page, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Parent: &root, Limit: 5})
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	node := page.Items[0]
	if node.Text.Status != "unsupported_or_requires_ocr" || node.Text.Reason == nil || node.Text.Value != nil || node.AttachmentRef != nil || node.CoordinatePrecision != "physical_page" || node.OriginalPosition.PageNumber != 1 {
		t.Fatal("empty physical page misrepresented", node)
	}
	window, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: entity.KnowledgeContentAddress{Target: target, Node: node.Ref}})
	if err != nil || window.Text != "" || window.TextStatus != "unsupported_or_requires_ocr" || !window.Complete {
		t.Fatal(window, err)
	}
}
