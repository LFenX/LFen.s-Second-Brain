//go:build linux

package dataset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

func init() {
	if parser.IsDocumentStructureChild(os.Args[1:]) {
		os.Exit(parser.DocumentStructureChildMain(context.Background(), ParseKnowledgeDocumentChild))
	}
}

func parsedExecutionFixture(t *testing.T, name string, input []byte, enabled bool) (*DatasetService, *entity.KnowledgeSourceRevision, entity.KnowledgeParsedTaskRequest, storage.Storage) {
	t.Helper()
	if !parser.DocumentStructureChildAvailable() {
		t.Fatal("owned child unavailable on the isolated Linux validation host")
	}
	s, store := setupKnowledgeTest(t)
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeContentPolicyRecord{}, &entity.KnowledgeStructureOperationRecord{}, &entity.KnowledgeStructureTaskRecord{}, &entity.KnowledgeStructureLeaseRecord{}, &entity.KnowledgeStructureCursorRecord{}, &entity.KnowledgeDocumentRepresentationRecord{}, &entity.KnowledgeDocumentNodeRecord{}, &entity.KnowledgeDocumentHeadRecord{}, &entity.KnowledgeDocumentPolicyRecord{}, &entity.KnowledgeStructurePolicyRecord{}); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("d", 32)
	uploadKnowledgeTest(t, store, id, name, input)
	source, err := s.SnapshotKnowledgeParsedSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeParsedSnapshotRequest{DocumentID: id, ExpectedPolicyVersion: 1, RelativePath: name, Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}})
	if err != nil {
		t.Fatal("native source snapshot", err)
	}
	policyVersion := int64(1)
	if enabled {
		policy, created, err := s.ChangeKnowledgeContentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, ParseEnabled: true, Confirmed: true, IdempotencyKey: "enable-local-parse"})
		if err != nil || !created || policy.ContentPolicyVersion != 2 || policy.LexicalEnabled {
			t.Fatal(policy, created, err)
		}
		policyVersion = 2
	}
	life, err := s.GetKnowledgeParsedSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, id)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := parsedLocalProfile(string(utility.GetFileType(name)))
	if err != nil {
		t.Fatal(err)
	}
	request := entity.KnowledgeParsedTaskRequest{Source: contentSource(source), ExpectedLifecycleVersion: life.LifecycleVersion, ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: policyVersion, ExpectedProducerSHA256: profile.ProducerSHA256, MaxUnitBytes: 128, Confirmed: true, IdempotencyKey: "parse-local"}
	return s, source, request, store
}

func parsedExecutionClaim(t *testing.T, s *DatasetService, req entity.KnowledgeParsedTaskRequest) (*entity.KnowledgeParsedTask, *structureClaim) {
	t.Helper()
	task, created, err := s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatal(task, created, err)
	}
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || claim == nil || claim.Protocol != entity.KnowledgeParsedProtocol {
		t.Fatal(claim, err)
	}
	return task, claim
}

func parsedExecutionCurrent(t *testing.T, s *DatasetService, task string) *entity.KnowledgeParsedTask {
	t.Helper()
	value, err := s.GetKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestParsedExecutionActualCSVSourceTaskChildPublicationReadReuse(t *testing.T) {
	s, source, req, _ := parsedExecutionFixture(t, "rows.csv", []byte("A,B\n甲,乙\n1,2\n"), true)
	if source.RepresentationKind != "original_only" || source.RootNodeID != "" {
		t.Fatal("fabricated C1 source root")
	}
	if _, err := s.GetKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, source.ID); err == nil {
		t.Fatal("new binary registration widened C1")
	}
	task, claim := parsedExecutionClaim(t, s, req)
	s.runKnowledgeParsed(t.Context(), claim)
	done := parsedExecutionCurrent(t, s, task.TaskID)
	if done.State != "succeeded" || done.Result == nil || done.AttemptCount != 1 || done.CompletedAt == nil {
		t.Fatal("actual parser task did not publish", done.State, done.FailureCode)
	}
	meta, err := s.GetKnowledgeContentRepresentation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, *done.Result)
	if err != nil || meta.RootRef == nil || meta.CoordinateSpace != "derived_utf8" {
		t.Fatal(meta, err)
	}
	window, err := s.ReadKnowledgeContentNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentReadRequest{Address: entity.KnowledgeContentAddress{Target: *done.Result, Node: *meta.RootRef}})
	if err != nil || !strings.Contains(window.Text, "甲\t乙") {
		t.Fatal(window, err)
	}
	page, err := s.ListKnowledgeParsedRepresentations(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeParsedRepresentationQuery{Source: req.Source, Limit: 20})
	if err != nil || len(page.Items) != 1 || page.Items[0].Source != done.Result.Source || *page.Items[0].Representation.ID != *done.Result.Representation.ID {
		t.Fatal(page, err)
	}
	var before entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.Where("id = ?", *done.Result.Representation.ID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	req.IdempotencyKey = "same-fixed-source-second-task"
	second, secondClaim := parsedExecutionClaim(t, s, req)
	s.runKnowledgeParsed(t.Context(), secondClaim)
	secondDone := parsedExecutionCurrent(t, s, second.TaskID)
	if secondDone.State != "succeeded" || secondDone.Result == nil || *secondDone.Result.Representation.ID != *done.Result.Representation.ID {
		t.Fatal("fixed producer/source reuse failed", secondDone.State, secondDone.FailureCode)
	}
	var after entity.KnowledgeDocumentRepresentationRecord
	if err := dao.DB.Where("id = ?", before.ID).Take(&after).Error; err != nil || before.TaskID != after.TaskID || before.PublishedAt != after.PublishedAt {
		t.Fatal("immutable publication overwritten", err)
	}
	var lease entity.KnowledgeStructureLeaseRecord
	if err := dao.DB.Where("domain_id = ?", knowledgeTestDomain).Take(&lease).Error; err != nil || lease.TaskID != "" {
		t.Fatal("successful publication retained domain lease", err)
	}
}

func TestParsedExecutionPolicyDefaultCASFixedRejectionAndOwner(t *testing.T) {
	s, _, req, _ := parsedExecutionFixture(t, "rows.csv", []byte("A,B\n1,2\n"), false)
	policy, err := s.KnowledgeContentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || policy.ContentPolicyVersion != 1 || policy.ParseEnabled || policy.LexicalEnabled {
		t.Fatal(policy, err)
	}
	_, _, err = s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	documentAssertError(t, err, 409, "content_policy_disabled", true)
	enable := entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 1, ParseEnabled: true, Confirmed: true, IdempotencyKey: "enable"}
	if _, _, err := s.ChangeKnowledgeContentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, enable); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	documentAssertError(t, err, 409, "content_policy_disabled", true)
	req.ExpectedContentPolicyVersion, req.IdempotencyKey = 2, "new-key"
	task, _, err := s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil {
		t.Fatal(err)
	}
	replay, created, err := s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || created || replay.TaskID != task.TaskID {
		t.Fatal(replay, created, err)
	}
	req.MaxUnitBytes++
	_, _, err = s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	documentAssertError(t, err, 409, "idempotency_conflict", false)
	_, err = s.GetKnowledgeParsedTask(t.Context(), knowledgeTestOther, knowledgeTestDomain, task.TaskID)
	documentAssertError(t, err, 404, "not_found", false)
	for _, model := range []any{&entity.KnowledgeDocumentPolicyRecord{}, &entity.KnowledgeStructurePolicyRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("content permission altered prior protocol permissions", err)
		}
	}
}

func TestParsedExecutionAtomicPublicationRollback(t *testing.T) {
	s, _, req, _ := parsedExecutionFixture(t, "rows.csv", []byte("A,B\n1,2\n"), true)
	task, claim := parsedExecutionClaim(t, s, req)
	const hook = "parsed-test-publication-sql-failure"
	if err := dao.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if tx.Statement.Table == "knowledge_structure_task" && ok && values["state"] == "succeeded" {
			tx.AddError(errors.New("synthetic task-success SQL failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dao.DB.Callback().Update().Remove(hook) })
	s.runKnowledgeParsed(t.Context(), claim)
	done := parsedExecutionCurrent(t, s, task.TaskID)
	if done.State != "failed" || done.Result != nil {
		t.Fatal("SQL failure left a success result", done.State)
	}
	for _, model := range []any{&entity.KnowledgeDocumentRepresentationRecord{}, &entity.KnowledgeDocumentNodeRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("partial publication escaped transaction", count, err)
		}
	}
}

func TestParsedExecutionPolicyRevocationCheckpoint(t *testing.T) {
	s, _, req, _ := parsedExecutionFixture(t, "rows.csv", []byte("A,B\n1,2\n"), true)
	task, claim := parsedExecutionClaim(t, s, req)
	if _, _, err := s.ChangeKnowledgeContentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 2, Confirmed: true, IdempotencyKey: "revoke-before-child"}); err != nil {
		t.Fatal(err)
	}
	s.runKnowledgeParsed(t.Context(), claim)
	done := parsedExecutionCurrent(t, s, task.TaskID)
	if done.State != "failed" || done.FailureCode == nil || *done.FailureCode != "content_policy_changed" {
		t.Fatal(done.State, done.FailureCode)
	}
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil || row.Execution != "" {
		t.Fatal("revoked task started child", err)
	}
}

func parsedExecutionOwnedChild(t *testing.T, s *DatasetService, source *entity.KnowledgeSourceRevision, claim *structureClaim, req entity.KnowledgeParsedTaskRequest, onStarted func(parser.DocumentChildOwnership) error) error {
	t.Helper()
	input, err := parsedOriginalBytes(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	control := parser.DocumentChildControl{TaskID: claim.TaskID, Attempt: 1, Format: "csv", Filename: "rows.csv", ProfileID: parser.KnowledgeParsedLocalProfile, ProducerSHA256: req.ExpectedProducerSHA256, InputSHA256: req.Source.SHA256, InputBytes: int64(len(input))}
	_, err = parser.RunDocumentStructureChild(t.Context(), input, control, func(owner parser.DocumentChildOwnership) error {
		if err := parsedExecutionStoreOwner(t.Context(), s, claim, owner); err != nil {
			return err
		}
		return onStarted(owner)
	})
	return err
}

func parsedExecutionStoreOwner(ctx context.Context, s *DatasetService, claim *structureClaim, owner parser.DocumentChildOwnership) error {
	return s.parsedCheckpoint(ctx, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeParsedTask, _ *parsedInputs, now time.Time) error {
		payload, err := structureJSON(owner)
		if err != nil {
			return err
		}
		if err := tx.Model(row).Where("id = ? AND revision = ?", row.ID, row.Revision).Update("execution", payload).Error; err != nil {
			return err
		}
		task.Phase = "extracting"
		return parsedSaveTask(tx, row, task, now)
	})
}

func TestParsedExecutionCleanupUncertaintyKeepsDomainFence(t *testing.T) {
	s, source, req, _ := parsedExecutionFixture(t, "rows.csv", []byte("A,B\n1,2\n"), true)
	task, claim := parsedExecutionClaim(t, s, req)
	var obstruction string
	err := parsedExecutionOwnedChild(t, s, source, claim, req, func(owner parser.DocumentChildOwnership) error {
		obstruction = filepath.Join(owner.Directory, "synthetic-unknown-file")
		return os.WriteFile(obstruction, []byte("synthetic"), 0600)
	})
	if !errors.Is(err, parser.ErrDocumentChildUnresolved) {
		t.Fatal("unknown private file did not keep cleanup unproven", err)
	}
	s.parsedFinishClaim(claim, "cleanup_failed")
	current := parsedExecutionCurrent(t, s, task.TaskID)
	if structureTerminal(current.State) || current.Phase != "cleaning" {
		t.Fatal("unproven cleanup retired execution", current.State, current.Phase)
	}
	if _, _, err := s.CancelKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID, entity.KnowledgeParsedCancelRequest{ExpectedTaskRevision: current.Revision, IdempotencyKey: "cancel-uncertain"}); err != nil {
		t.Fatal(err)
	}
	req.IdempotencyKey = "waiting-behind-uncertain"
	second, _, err := s.CreateKnowledgeParsedTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil {
		t.Fatal(err)
	}
	var secondRow entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", second.TaskID).Take(&secondRow).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.KnowledgeStructureLeaseRecord{}).Where("domain_id = ?", knowledgeTestDomain).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if next, err := claimKnowledgeStructure(t.Context(), secondRow, common.GenerateUUID()); err != nil || next != nil {
		t.Fatal("queued task bypassed uncertain prior attempt", next, err)
	}
	var oldRow entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&oldRow).Error; err != nil {
		t.Fatal(err)
	}
	if next, err := claimKnowledgeStructure(t.Context(), oldRow, common.GenerateUUID()); err != nil || next != nil {
		t.Fatal("unproven owner was retried", next, err)
	}
	if err := os.Remove(obstruction); err != nil {
		t.Fatal(err)
	}
	if next, err := claimKnowledgeStructure(t.Context(), oldRow, common.GenerateUUID()); err != nil || next != nil {
		t.Fatal("cancelled recovery unexpectedly executed parser", next, err)
	}
	if final := parsedExecutionCurrent(t, s, task.TaskID); final.State != "cancelled" {
		t.Fatal("confirmed cleanup did not retire cancellation", final.State)
	}
}
