package dataset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

func lifecycleTestDetail(t *testing.T, s *DatasetService, source string) *entity.KnowledgeLifecycleDetail {
	t.Helper()
	v, e := s.GetKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func lifecycleTestCommand(v *entity.KnowledgeLifecycleDetail, key string) entity.KnowledgeLifecycleCommand {
	return entity.KnowledgeLifecycleCommand{IdempotencyKey: key, ExpectedSourceRevisionID: v.Current.SourceRevisionID, ExpectedLifecycleVersion: v.Current.LifecycleVersion, ExpectedPolicyVersion: v.PolicyVersion, Confirmed: true}
}
func lifecycleTestAction(t *testing.T, s *DatasetService, source, action string, request any) *entity.KnowledgeLifecycleReceipt {
	t.Helper()
	v, created, e := s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source, action, request)
	if e != nil || !created {
		t.Fatalf("action=%s created=%t err=%v", action, created, e)
	}
	return v
}
func lifecycleTestSource(t *testing.T) (*DatasetService, *entity.KnowledgeSourceRegistration, *entity.KnowledgeImportReceipt) {
	t.Helper()
	s, _, reg := setupKnowledgeImportTest(t)
	r := commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, "// 合成\nexport const card = '🧠';\n"), "first")
	return s, reg, r
}

func TestKnowledgeLifecycleFourActionsAndRetiredPositions(t *testing.T) {
	s, reg, first := lifecycleTestSource(t)
	source := *first.Items[0].SourceID
	before := lifecycleTestDetail(t, s, source)
	if before.Current.LifecycleVersion != 1 || before.SourceKind == nil || *before.SourceKind != "text_file" {
		t.Fatal(before)
	}
	class := before.Current.Classification
	class.ComponentSide = "shared"
	reclassified := lifecycleTestAction(t, s, source, "reclassify", entity.KnowledgeLifecycleReclassify{KnowledgeLifecycleCommand: lifecycleTestCommand(before, "class"), Classification: class})
	if reclassified.After.LifecycleVersion != 2 || reclassified.After.SourceRevisionID == reclassified.Before.SourceRevisionID || reclassified.After.SHA256 != reclassified.Before.SHA256 {
		t.Fatal(reclassified)
	}
	moveReq := entity.KnowledgeLifecycleMove{KnowledgeLifecycleCommand: lifecycleTestCommand(lifecycleTestDetail(t, s, source), "move"), RelativePath: "docs/empty.md"}
	check, e := s.CheckKnowledgeSourceMoveTarget(t.Context(), knowledgeTestOwner, reg.DomainID, source, moveReq.RelativePath, moveReq.KnowledgeLifecycleCommand)
	if e != nil || !check.Allowed || check.Reserved {
		t.Fatal(check, e)
	}
	moved := lifecycleTestAction(t, s, source, "move", moveReq)
	if moved.After.LifecycleVersion != 3 || moved.After.RelativePath != "docs/empty.md" {
		t.Fatal(moved)
	}
	var doc entity.Document
	if e := dao.DB.Where("id = ?", source).Take(&doc).Error; e != nil || doc.Name == nil || *doc.Name != "card.tsx" {
		t.Fatal("logical move renamed native document", e)
	}
	_, e = s.PreviewKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.PreviewKnowledgeImportRequest{RelativePath: "web/card.tsx", Text: "same bytes do not merge", ComponentSide: "frontend", Language: "tsx"})
	assertOriginalCode(t, e, 409, "knowledge_conflict")
	back := lifecycleTestAction(t, s, source, "move", entity.KnowledgeLifecycleMove{KnowledgeLifecycleCommand: lifecycleTestCommand(lifecycleTestDetail(t, s, source), "back"), RelativePath: "web/card.tsx"})
	if back.After.LifecycleVersion != 4 || back.After.SourceRevisionID != reclassified.After.SourceRevisionID {
		t.Fatal("historical metadata reuse did not advance", back)
	}
	archiveReq := lifecycleTestCommand(lifecycleTestDetail(t, s, source), "archive")
	archived := lifecycleTestAction(t, s, source, "archive", archiveReq)
	if archived.After.SourceRevisionID != back.After.SourceRevisionID || archived.After.LifecycleVersion != 5 {
		t.Fatal(archived)
	}
	list, e := s.ListKnowledgeSources(t.Context(), knowledgeTestOwner, reg.DomainID, "", "", 20)
	if e != nil || len(list.Items) != 0 {
		t.Fatal("archived source in old current list", e)
	}
	archiveList, e := s.ListKnowledgeSourceLifecycles(t.Context(), knowledgeTestOwner, reg.DomainID, KnowledgeLifecycleListQuery{State: "archived"})
	if e != nil || len(archiveList.Items) != 1 || archiveList.NextCursor != nil {
		t.Fatal(archiveList, e)
	}
	old, e := s.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, reg.DomainID, *first.Items[0].RootNodeID, nil, nil)
	if e != nil || !strings.Contains(old.Text, "🧠") {
		t.Fatal("archive destroyed fixed text", e)
	}
	restored := lifecycleTestAction(t, s, source, "restore", lifecycleTestCommand(lifecycleTestDetail(t, s, source), "restore"))
	if restored.After.State != "active" || restored.After.LifecycleVersion != 6 {
		t.Fatal(restored)
	}
	replay, created, e := s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, reg.DomainID, source, "archive", archiveReq)
	if e != nil || created || replay.CommandID != archived.CommandID {
		t.Fatal("fixed replay failed", e)
	}
	if lifecycleTestDetail(t, s, source).Current.State != "active" {
		t.Fatal("old archive key re-applied")
	}
	archiveReq.ExpectedLifecycleVersion++
	_, _, e = s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, reg.DomainID, source, "archive", archiveReq)
	assertOriginalCode(t, e, 409, "idempotency_conflict")
}

func TestKnowledgeLifecycleNoopCASPolicyAndMax(t *testing.T) {
	s, _, first := lifecycleTestSource(t)
	source := *first.Items[0].SourceID
	before := lifecycleTestDetail(t, s, source)
	stale := lifecycleTestCommand(before, "stale")
	stale.ExpectedLifecycleVersion++
	_, _, e := s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source, "restore", stale)
	assertOriginalCode(t, e, 409, "revision_changed")
	stale = lifecycleTestCommand(before, "policy")
	stale.ExpectedPolicyVersion++
	_, _, e = s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source, "restore", stale)
	assertOriginalCode(t, e, 409, "policy_changed")
	if e := dao.DB.Model(&entity.KnowledgeSourceLifecycle{}).Where("source_id = ?", source).Update("version", entity.KnowledgeMaxLifecycleVersion).Error; e != nil {
		t.Fatal(e)
	}
	maxed := lifecycleTestDetail(t, s, source)
	noop := lifecycleTestAction(t, s, source, "restore", lifecycleTestCommand(maxed, "max_noop"))
	if noop.Result != "unchanged" || noop.After.LifecycleVersion != entity.KnowledgeMaxLifecycleVersion {
		t.Fatal(noop)
	}
	_, _, e = s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source, "archive", lifecycleTestCommand(maxed, "overflow"))
	assertOriginalCode(t, e, 409, "lifecycle_version_exhausted")
	if lifecycleTestDetail(t, s, source).Current.State != "active" {
		t.Fatal("overflow mutated source")
	}
}

func TestKnowledgeLifecycleFencesRejectStateAndBodyABA(t *testing.T) {
	for _, mode := range []string{"state", "body", "unfenced"} {
		t.Run(mode, func(t *testing.T) {
			s, reg, first := lifecycleTestSource(t)
			source := *first.Items[0].SourceID
			pending := previewKnowledgeTest(t, s, reg, "// old pending\n")
			if mode == "state" {
				lifecycleTestAction(t, s, source, "archive", lifecycleTestCommand(lifecycleTestDetail(t, s, source), "archive"))
				lifecycleTestAction(t, s, source, "restore", lifecycleTestCommand(lifecycleTestDetail(t, s, source), "restore"))
			}
			if mode == "body" {
				commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, "// changed\n"), "change")
				returned := commitKnowledgeTest(t, s, reg, previewKnowledgeTest(t, s, reg, "// 合成\nexport const card = '🧠';\n"), "return")
				if *returned.Items[0].SourceRevisionID != *first.Items[0].SourceRevisionID {
					t.Fatal("ABA setup did not reuse historical ID")
				}
			}
			if mode == "unfenced" {
				if e := dao.DB.Where("preparation_id = ?", pending.ID).Delete(&entity.KnowledgeSourceFence{}).Error; e != nil {
					t.Fatal(e)
				}
			}
			_, _, e := s.CommitKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.CommitKnowledgeImportRequest{PreviewID: pending.ID, ManifestSHA256: pending.ManifestSHA256, IdempotencyKey: "stale"})
			assertOriginalCode(t, e, 409, "knowledge_conflict")
			fixed, e := s.GetKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, first.ID)
			if e != nil || fixed.ID != first.ID {
				t.Fatal("lost successful historical receipt", e)
			}
		})
	}
}

func TestKnowledgeLifecycleMetadataRollbackAtEachNativeWrite(t *testing.T) {
	for _, table := range []string{"file_commit", "file_commit_item", "knowledge_source_revision", "knowledge_node", "knowledge_lifecycle_command"} {
		t.Run(table, func(t *testing.T) {
			s, _, first := lifecycleTestSource(t)
			source := *first.Items[0].SourceID
			before := lifecycleTestDetail(t, s, source)
			counts := map[string]int64{}
			for _, name := range []string{"file_commit", "file_commit_item", "knowledge_source_revision", "knowledge_node", "knowledge_lifecycle_command"} {
				var n int64
				dao.DB.Table(name).Count(&n)
				counts[name] = n
			}
			name := "lifecycle_fail_create"
			if e := dao.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(errors.New("synthetic write failure"))
				}
			}); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { dao.DB.Callback().Create().Remove(name) })
			class := before.Current.Classification
			class.ComponentSide = "backend"
			_, _, e := s.CommandKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source, "reclassify", entity.KnowledgeLifecycleReclassify{KnowledgeLifecycleCommand: lifecycleTestCommand(before, "fail"), Classification: class})
			assertOriginalCode(t, e, 503, "lifecycle_result_unknown")
			after := lifecycleTestDetail(t, s, source)
			if directoryJSON(before) != directoryJSON(after) {
				t.Fatal("failed transaction changed head or generation")
			}
			for name, want := range counts {
				var n int64
				dao.DB.Table(name).Count(&n)
				if n != want {
					t.Fatalf("rollback %s %d != %d", name, n, want)
				}
			}
		})
	}
}

type lifecycleNoObjectIO struct{ storage.Storage }

func (lifecycleNoObjectIO) Get(context.Context, string, string, ...string) ([]byte, error) {
	return nil, errors.New("metadata must not GET objects")
}
func (lifecycleNoObjectIO) Put(context.Context, string, string, []byte, ...string) error {
	return errors.New("metadata must not PUT objects")
}

func TestKnowledgeLifecycleOriginalMetadataNoBodyIO(t *testing.T) {
	s, reg, store := originalFlowFixture(t)
	body := []byte("%PDF-1.7\nsynthetic\n%%EOF")
	ready := prepareOriginalFlow(t, s, reg, originalTestRequest("original"), body)
	first := confirmOriginalFlow(t, s, reg, ready, "confirm")
	storage.GetStorageFactory().SetStorage(lifecycleNoObjectIO{Storage: store})
	defer storage.GetStorageFactory().SetStorage(store)
	source := first.Source.SourceID
	v := lifecycleTestDetail(t, s, source)
	class := v.Current.Classification
	class.ComponentSide = "shared"
	changed := lifecycleTestAction(t, s, source, "reclassify", entity.KnowledgeLifecycleReclassify{KnowledgeLifecycleCommand: lifecycleTestCommand(v, "class"), Classification: class})
	if changed.After.SHA256 != first.Source.SHA256 || changed.After.ByteLength != int64(len(body)) {
		t.Fatal(changed)
	}
	var nodes int64
	dao.DB.Model(&entity.KnowledgeNode{}).Count(&nodes)
	if nodes != 0 {
		t.Fatal("original metadata invented text root")
	}
	fixed, e := s.GetKnowledgeOriginalRevision(t.Context(), knowledgeTestOwner, reg.DomainID, first.Source.SourceRevisionID)
	if e != nil || fixed.OriginalFilename != first.Source.OriginalFilename {
		t.Fatal(e)
	}
}

func TestKnowledgeLifecycleDirectC1IdentityAndRegisteredBypass(t *testing.T) {
	s, store := setupKnowledgeTest(t)
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	doc := strings.Repeat("c", 32)
	uploadKnowledgeTest(t, store, doc, "sample.ts", []byte("export const example=1;\n"))
	first, e := s.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: doc})
	if e != nil {
		t.Fatal(e)
	}
	v := lifecycleTestDetail(t, s, doc)
	if v.RegistrationID != nil || v.SourceKind != nil || v.Actions["move"].Supported {
		t.Fatal("invented C1 registration", v)
	}
	class := v.Current.Classification
	class.ComponentSide = "frontend"
	lifecycleTestAction(t, s, doc, "reclassify", entity.KnowledgeLifecycleReclassify{KnowledgeLifecycleCommand: lifecycleTestCommand(v, "direct"), Classification: class})
	fixed, e := s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.Source.ID)
	if e != nil || fixed.DocumentID != doc {
		t.Fatal(e)
	}
	t.Run("registered_bypass", func(t *testing.T) {
		s2, reg, r := lifecycleTestSource(t)
		_, e := s2.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, reg.DomainID, entity.SnapshotKnowledgeSourceRequest{DocumentID: *r.Items[0].SourceID, ExpectedRevisionID: *r.Items[0].SourceRevisionID, RelativePath: "docs/empty.md", ComponentSide: "frontend", Language: "tsx"})
		assertOriginalCode(t, e, 409, "knowledge_conflict")
	})
}

func TestKnowledgeLifecycleSQLFiltersCursorsAndPermission(t *testing.T) {
	s, reg, first := lifecycleTestSource(t)
	source := *first.Items[0].SourceID
	for i, path := range []string{"backend/answer.go", "docs/empty.md"} {
		p, e := s.PreviewKnowledgeImport(t.Context(), knowledgeTestOwner, reg.DomainID, reg.ID, entity.PreviewKnowledgeImportRequest{RelativePath: path, Text: fmt.Sprintf("// %d", i), ComponentSide: "backend", Language: ""})
		if e != nil {
			t.Fatal(e)
		}
		commitKnowledgeTest(t, s, reg, p, fmt.Sprintf("file%d", i))
	}
	lifecycleTestAction(t, s, source, "archive", lifecycleTestCommand(lifecycleTestDetail(t, s, source), "archive"))
	q := KnowledgeLifecycleListQuery{Limit: 1, GroupScope: "ungrouped", ComponentSide: "backend", Language: directoryString("")}
	page, e := s.ListKnowledgeSourceLifecycles(t.Context(), knowledgeTestOwner, reg.DomainID, q)
	if e != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal(page, e)
	}
	q.Cursor = *page.NextCursor
	page2, e := s.ListKnowledgeSourceLifecycles(t.Context(), knowledgeTestOwner, reg.DomainID, q)
	if e != nil || len(page2.Items) != 1 || page2.Items[0].SourceID == page.Items[0].SourceID || page2.NextCursor != nil {
		t.Fatal(page2, e)
	}
	q.Limit = 2
	_, e = s.ListKnowledgeSourceLifecycles(t.Context(), knowledgeTestOwner, reg.DomainID, q)
	assertOriginalCode(t, e, 400, "knowledge_invalid_request")
	_, e = s.GetKnowledgeSourceLifecycle(t.Context(), knowledgeTestOther, reg.DomainID, source)
	assertOriginalCode(t, e, 404, "knowledge_not_found")
	if e := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ?", knowledgeTestOwner).Update("status", "0").Error; e != nil {
		t.Fatal(e)
	}
	_, e = s.GetKnowledgeLifecycleCommand(t.Context(), knowledgeTestOwner, reg.DomainID, "", "archive", "archive")
	assertOriginalCode(t, e, 404, "knowledge_not_found")
}
