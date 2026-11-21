package dataset

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/storage"
)

func parsedPersistenceFixture(t *testing.T) (*DatasetService, *parsedBound, entity.KnowledgeDocumentHeadRecord) {
	t.Helper()
	s, r2 := contentPublishedDocument(t, "")
	var oldHead entity.KnowledgeDocumentHeadRecord
	if err := dao.DB.Where("source_id = ?", r2.Representation.Original.SourceID).Take(&oldHead).Error; err != nil {
		t.Fatal(err)
	}
	original := []byte("A,B\n甲,乙\n1,2\n")
	id := strings.Repeat("d", 32)
	// The frozen C1 receiver admits this text carrier. Native .csv registration
	// needs its own explicit source adapter; this fixture does not widen C1.
	uploadKnowledgeTest(t, storage.GetStorageFactory().GetStorage(), id, "rows.txt", original)
	snapshot, err := s.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: id, RelativePath: "rows.txt", ComponentSide: "non_code", Language: ""})
	if err != nil {
		t.Fatal(err)
	}
	normalized := parser.NormalizeParseResult(parser.NewCSVParser().ParseWithResult(t.Context(), "rows.csv", original))
	bound, err := bindKnowledgeParsed(contentSource(&snapshot.Source), original, strings.Repeat("a", 64), normalized, 32)
	if err != nil {
		t.Fatal(err)
	}
	return s, bound, oldHead
}

func parsedStore(t *testing.T, bound *parsedBound, task string) {
	t.Helper()
	if err := dao.DB.Transaction(func(tx *gorm.DB) error {
		return persistKnowledgeParsed(tx, knowledgeTestOwner, knowledgeTestDomain, task, bound, time.Now().UTC())
	}); err != nil {
		t.Fatal(err)
	}
}

func TestParsedPersistenceAtomicRollbackReuseAndR2Head(t *testing.T) {
	s, bound, oldHead := parsedPersistenceFixture(t)
	abort := errors.New("synthetic abort before publication commit")
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := persistKnowledgeParsed(tx, knowledgeTestOwner, knowledgeTestDomain, strings.Repeat("b", 32), bound, time.Now().UTC()); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("id = ?", *bound.Representation.Ref.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := dao.DB.Model(&entity.KnowledgeDocumentNodeRecord{}).Where("representation_id = ?", *bound.Representation.Ref.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
	parsedStore(t, bound, strings.Repeat("b", 32))
	var before entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.Where("id = ?", *bound.Representation.Ref.ID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	parsedStore(t, bound, strings.Repeat("c", 32))
	var after entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.Where("id = ?", before.ID).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if before.TaskID != after.TaskID || before.PublishedAt != after.PublishedAt || before.Payload != after.Payload || after.Protocol != entity.KnowledgeParsedProtocol {
		t.Fatal("reuse rewrote fixed representation")
	}
	var head entity.KnowledgeDocumentHeadRecord
	if err := dao.DB.Where("source_id = ?", oldHead.SourceID).Take(&head).Error; err != nil || head != oldHead {
		t.Fatal("R2 head changed", head, err)
	}
	if _, err := s.GetKnowledgeDocumentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, after.ID); err == nil {
		t.Fatal("old frozen R2 reader accepted parsed protocol")
	}
	if _, err := s.GetKnowledgeDocumentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, oldHead.RepresentationID); err != nil {
		t.Fatal("old R2 path changed", err)
	}
}

func TestParsedPersistenceUsesUnifiedReadListAndChecksDamage(t *testing.T) {
	s, bound, _ := parsedPersistenceFixture(t)
	parsedStore(t, bound, strings.Repeat("b", 32))
	target := entity.KnowledgeContentTarget{Source: bound.Representation.Source, Representation: bound.Representation.Ref}
	meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, target)
	if err != nil || meta.Ref.Kind != "parsed_representation" || meta.RootRef.Kind != "parsed_node" || meta.CoordinateSpace != "derived_utf8" {
		t.Fatal(meta, err)
	}
	page, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Parent: meta.RootRef, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].Kind != "table" || page.Items[0].Locator.Kind != "parsed_item" || page.Items[0].OriginalPosition != nil {
		t.Fatal(page, err)
	}
	next, err := s.ListKnowledgeContentNodes(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentNodeQuery{Target: target, Parent: meta.RootRef, Limit: 1, Cursor: *page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].Kind != "passage" || next.NextCursor != nil {
		t.Fatal(next, err)
	}
	address := entity.KnowledgeContentAddress{Target: target, Node: next.Items[0].Ref}
	window, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: address})
	if err != nil || window.Text != "A\tB\n甲\t乙\n1\t2" || !window.Complete || window.CoordinateSpace != "derived_utf8" || *window.Range.SHA256 == target.Source.SHA256 {
		t.Fatal(window, err)
	}
	_, err = s.GetKnowledgeContentRepresentation(t.Context(), strings.Repeat("e", 32), knowledgeTestDomain, target)
	contentAssertStatus(t, err, 404)
	if err := dao.DB.Model(&entity.KnowledgeDocumentNodeRecord{}).Where("id = ?", *address.Node.ID).Update("payload", "{}").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.GetKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, address)
	contentAssertStatus(t, err, 409)
	err = dao.DB.Transaction(func(tx *gorm.DB) error {
		return persistKnowledgeParsed(tx, knowledgeTestOwner, knowledgeTestDomain, strings.Repeat("c", 32), bound, time.Now().UTC())
	})
	contentAssertStatus(t, err, 409)
}

func TestParsedPersistenceRechecksAuthorityAfterDerivedRead(t *testing.T) {
	s, bound, _ := parsedPersistenceFixture(t)
	parsedStore(t, bound, strings.Repeat("b", 32))
	target := entity.KnowledgeContentTarget{Source: bound.Representation.Source, Representation: bound.Representation.Ref}
	callback := "synthetic:revoke_after_derived_read"
	revoked := false
	if err := dao.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if revoked || tx.Statement.Table != "knowledge_document_representation" || !slices.Contains(tx.Statement.Selects, "reading_text") {
			return
		}
		revoked = true
		if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("status", "0").Error; err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer dao.DB.Callback().Query().Remove(callback)
	window, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: entity.KnowledgeContentAddress{Target: target, Node: bound.Representation.RootRef}})
	if !revoked || window != nil {
		t.Fatal("derived content escaped revoke", window, revoked)
	}
	contentAssertStatus(t, err, 404)
}

func TestParsedPersistenceDiscoveryBindsSourceAndOpaqueCursor(t *testing.T) {
	s, bound, _ := parsedPersistenceFixture(t)
	parsedStore(t, bound, strings.Repeat("b", 32))
	original := []byte("A,B\n甲,乙\n1,2\n")
	normalized := parser.NormalizeParseResult(parser.NewCSVParser().ParseWithResult(t.Context(), "rows.csv", original))
	second, err := bindKnowledgeParsed(bound.Representation.Source, original, strings.Repeat("b", 64), normalized, 32)
	if err != nil {
		t.Fatal(err)
	}
	parsedStore(t, second, strings.Repeat("c", 32))
	query := entity.KnowledgeParsedRepresentationQuery{Source: bound.Representation.Source, Limit: 1}
	first, err := s.ListKnowledgeParsedRepresentations(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatal(first, err)
	}
	query.Cursor = *first.NextCursor
	next, err := s.ListKnowledgeParsedRepresentations(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
	if err != nil || len(next.Items) != 1 || next.NextCursor != nil || *first.Items[0].Representation.ID == *next.Items[0].Representation.ID {
		t.Fatal(next, err)
	}
	if first.Items[0].Source != query.Source || next.Items[0].Source != query.Source {
		t.Fatal("discovery changed source binding")
	}
	query.Limit = 2
	_, err = s.ListKnowledgeParsedRepresentations(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
	contentAssertStatus(t, err, 400)
	query.Limit, query.Cursor = 1, ""
	query.Source.SHA256 = strings.Repeat("f", 64)
	_, err = s.ListKnowledgeParsedRepresentations(t.Context(), knowledgeTestOwner, knowledgeTestDomain, query)
	contentAssertStatus(t, err, 409)
	query.Source = bound.Representation.Source
	_, err = s.ListKnowledgeParsedRepresentations(t.Context(), strings.Repeat("e", 32), knowledgeTestDomain, query)
	contentAssertStatus(t, err, 404)
}
