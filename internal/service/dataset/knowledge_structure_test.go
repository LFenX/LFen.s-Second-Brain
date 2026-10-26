package dataset

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	structure "ragflow/internal/parser/structure"
)

type structureTaskFixture struct {
	directory *directoryFixture
	request   entity.KnowledgeStructureTaskRequest
}

func structureSetupTaskTest(t *testing.T, enabled bool) *structureTaskFixture {
	t.Helper()
	f := setupBatchTest(t)
	models := []any{&entity.KnowledgeStructurePolicyRecord{}, &entity.KnowledgeStructureOperationRecord{}, &entity.KnowledgeStructureTaskRecord{}, &entity.KnowledgeStructureLeaseRecord{}, &entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureHeadRecord{}, &entity.KnowledgeStructureNodeRecord{}, &entity.KnowledgeStructureRelationRecord{}, &entity.KnowledgeStructureArtifactRecord{}, &entity.KnowledgeStructureCursorRecord{}}
	if err := dao.DB.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	project, err := f.service.CreateKnowledgeGroup(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.CreateKnowledgeGroupRequest{Kind: "project", Name: "Synthetic structure project"})
	if err != nil {
		t.Fatal(err)
	}
	f.reg.DefaultClassification.GroupID = &project.ID
	f.write(t, "web/card.tsx", "\ufeffexport function Card() { return <div>合成 🧠</div> }\r\n")
	f.write(t, "web/readme.md", "# 合成结构\n\n一行记录。\n\n```ts\n# 非标题\n```\n")
	batch := runBatchTest(t, f, acceptBatchTest(t, f, prepareBatchTest(t, f, "web/card.tsx", "web/readme.md"), "structure-fixture-batch").TaskID)
	preview, err := f.service.PrepareKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPreviewRequest{ProjectID: project.ID, TaskID: batch.TaskID})
	if err != nil {
		t.Fatal(err)
	}
	receipt, _, err := f.service.PublishKnowledgeProjectManifest(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeProjectManifestPublicationRequest{ProjectID: project.ID, PreviewID: preview.PreviewID, ManifestSHA256: preview.ManifestSHA256, IdempotencyKey: "structure-fixture-manifest"})
	if err != nil {
		t.Fatal(err)
	}
	req := entity.KnowledgeStructureTaskRequest{ProjectID: project.ID, ManifestID: receipt.ManifestID, ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: 1, Aliases: []entity.KnowledgeStructureAlias{}, Members: []entity.KnowledgeStructureMember{}, Confirmed: true, IdempotencyKey: "structure-task"}
	for _, entry := range preview.Items {
		parser := "tsx"
		if strings.HasSuffix(entry.RelativePath, ".md") {
			parser = "markdown"
		}
		current := lifecycleTestDetail(t, f.service, entry.SourceID)
		req.Members = append(req.Members, entity.KnowledgeStructureMember{SourceID: entry.SourceID, SourceRevisionID: entry.SourceRevisionID, LifecycleVersion: current.Current.LifecycleVersion, Parser: parser})
	}
	if enabled {
		value, created, err := f.service.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeStructurePolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: 1, Enabled: true, Confirmed: true, IdempotencyKey: "structure-enable"})
		if err != nil || !created {
			t.Fatal(value, created, err)
		}
		req.ExpectedStructurePolicyVersion = 2
	}
	return &structureTaskFixture{directory: f, request: req}
}

func structureAssertError(t *testing.T, err error, status int, code string, durable bool) *StructureOperationError {
	t.Helper()
	var public *StructureOperationError
	if !errors.As(err, &public) || public.Status != status || public.Code != code || (public.RejectionID != nil) != durable {
		t.Fatalf("expected %d/%s durable=%t; received %v", status, code, durable, err)
	}
	return public
}

func structureAcceptTest(t *testing.T, f *structureTaskFixture, req entity.KnowledgeStructureTaskRequest) *entity.KnowledgeStructureTask {
	t.Helper()
	value, created, err := f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatal(value, created, err)
	}
	task, ok := value.(*entity.KnowledgeStructureTask)
	if !ok {
		t.Fatalf("unexpected outcome %T", value)
	}
	return task
}

func structureRunTest(t *testing.T, f *structureTaskFixture, task *entity.KnowledgeStructureTask) *entity.KnowledgeStructureTask {
	t.Helper()
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil {
		t.Fatal(err)
	}
	if claim != nil {
		f.directory.service.runKnowledgeStructure(t.Context(), claim)
	}
	value, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return value.(*entity.KnowledgeStructureTask)
}

func TestKnowledgeStructurePolicyDefaultNoopAndFixedReceipt(t *testing.T) {
	f := structureSetupTaskTest(t, false)
	s := f.directory.service
	policy, err := s.KnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || policy.Enabled || policy.StructurePolicyVersion != 1 || len(policy.AllowedModelUses) != 0 || policy.Mode != "structure_only" {
		t.Fatal(policy, err)
	}
	enable := entity.KnowledgeStructurePolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: 1, Enabled: true, Confirmed: true, IdempotencyKey: "policy-first"}
	first, created, err := s.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, enable)
	if err != nil || !created || first.(*entity.KnowledgeStructurePolicyReceipt).After.StructurePolicyVersion != 2 {
		t.Fatal(first, created, err)
	}
	noop := enable
	noop.ExpectedStructurePolicyVersion, noop.IdempotencyKey = 2, "policy-noop"
	value, _, err := s.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, noop)
	if err != nil || value.(*entity.KnowledgeStructurePolicyReceipt).Outcome != "unchanged" || value.(*entity.KnowledgeStructurePolicyReceipt).After.StructurePolicyVersion != 2 {
		t.Fatal(value, err)
	}
	disable := noop
	disable.Enabled, disable.IdempotencyKey = false, "policy-disable"
	if _, _, err := s.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, disable); err != nil {
		t.Fatal(err)
	}
	replay, created, err := s.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, enable)
	if err != nil || created || replay.(*entity.KnowledgeStructurePolicyReceipt).CommandID != first.(*entity.KnowledgeStructurePolicyReceipt).CommandID {
		t.Fatal(replay, created, err)
	}
	policy, err = s.KnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || policy.Enabled || policy.StructurePolicyVersion != 3 || policy.FoundationPolicyVersion != 1 {
		t.Fatal("fixed receipt re-applied or modified foundation", policy, err)
	}
}

func TestKnowledgeStructureSemanticRejectionIsDurableAndAuthorized(t *testing.T) {
	f := structureSetupTaskTest(t, false)
	s := f.directory.service
	_, created, err := s.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.request)
	first := structureAssertError(t, err, 409, "policy_disabled", true)
	if created {
		t.Fatal("rejection admitted a task")
	}
	value, err := s.GetKnowledgeStructureTaskByKey(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.request.IdempotencyKey)
	if err != nil || value.(*entity.KnowledgeStructureRejectionReceipt).RejectionID != *first.RejectionID {
		t.Fatal(value, err)
	}
	if _, _, err := s.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeStructurePolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: 1, Enabled: true, Confirmed: true, IdempotencyKey: "enable-after-rejection"}); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.request)
	again := structureAssertError(t, err, 409, "policy_disabled", true)
	if *again.RejectionID != *first.RejectionID {
		t.Fatal("fixed rejection replaced")
	}
	changed := f.request
	changed.ExpectedStructurePolicyVersion = 2
	_, _, err = s.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, changed)
	structureAssertError(t, err, 409, "idempotency_conflict", false)
	_, err = s.GetKnowledgeStructureTaskByKey(t.Context(), knowledgeTestOther, knowledgeTestDomain, f.request.IdempotencyKey)
	structureAssertError(t, err, 404, "not_found", false)
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeStructureTaskRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestKnowledgeStructureRejectedReceiptRollbackRemainsUnknown(t *testing.T) {
	f := structureSetupTaskTest(t, false)
	name := "structure-rejection-rollback"
	if err := dao.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_structure_operation" {
			tx.AddError(errors.New("synthetic storage failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dao.DB.Callback().Create().Remove(name) })
	_, _, err := f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.request)
	public := structureAssertError(t, err, 503, "unavailable", false)
	if public.OutcomeKnown {
		t.Fatal("rollback claimed known outcome")
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeStructureOperationRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestKnowledgeStructureOriginalOrderDigestAndRoundTrip(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	req := f.request
	req.Members = slices.Clone(req.Members)
	slices.Reverse(req.Members)
	first := structureAcceptTest(t, f, req)
	replay, created, err := f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, f.request)
	if err != nil || created || replay.(*entity.KnowledgeStructureTask).TaskID != first.TaskID || replay.(*entity.KnowledgeStructureTask).OriginalRequest.Members[0].SourceID != req.Members[0].SourceID {
		t.Fatal("canonical reorder lost original order", replay, created, err)
	}
	done := structureRunTest(t, f, first)
	if done.State != "succeeded" || done.CompletedAt == nil || done.ResultViewID == nil || done.FinishedMembers != done.TotalMembers || done.ModelCalls != 0 || done.LocalInference != 0 || done.ExternalFetches != 0 {
		t.Fatal(done)
	}
	var viewRow entity.KnowledgeStructureViewRecord
	if err := dao.DB.Where("id = ? AND published = ?", *done.ResultViewID, true).Take(&viewRow).Error; err != nil {
		t.Fatal(err)
	}
	view, err := structureDecode[entity.KnowledgeStructureView](viewRow.Payload)
	if err != nil || view.SourceCount != 2 || view.NodeCount < 4 || view.RelationCount < 2 {
		t.Fatal(view, err)
	}
	var nodeRows []entity.KnowledgeStructureNodeRecord
	if err := dao.DB.Where("view_id = ?", view.ViewID).Find(&nodeRows).Error; err != nil || len(nodeRows) != int(view.NodeCount) {
		t.Fatal(len(nodeRows), err)
	}
	for _, node := range nodeRows {
		if node.FragmentSHA256 == "" || node.SourceRevisionID == "" {
			t.Fatal(node)
		}
	}
	policy, _ := f.directory.service.KnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if _, _, err := f.directory.service.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeStructurePolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: policy.StructurePolicyVersion, Enabled: false, Confirmed: true, IdempotencyKey: "disable-after-success"}); err != nil {
		t.Fatal(err)
	}
	value, err := f.directory.service.GetKnowledgeStructureTaskByKey(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req.IdempotencyKey)
	if err != nil || value.(*entity.KnowledgeStructureTask).State != "succeeded" {
		t.Fatal("history hidden by processing disable", value, err)
	}
}

func TestKnowledgeStructureCancelIdentityCASAndQueueRecovery(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	first := structureAcceptTest(t, f, f.request)
	otherReq := f.request
	otherReq.IdempotencyKey = "second-task"
	cmd := entity.KnowledgeStructureCancelRequest{ExpectedTaskRevision: first.Revision, Confirmed: true, IdempotencyKey: "cancel-original"}
	value, created, err := f.directory.service.CancelKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.TaskID, cmd)
	if err != nil || !created || value.(*entity.KnowledgeStructureCancelReceipt).AfterState != "cancel_requested" {
		t.Fatal(value, created, err)
	}
	second := structureAcceptTest(t, f, otherReq)
	_, _, err = f.directory.service.CancelKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, second.TaskID, cmd)
	structureAssertError(t, err, 409, "idempotency_conflict", false)
	stale := cmd
	stale.IdempotencyKey = "stale-cancel"
	_, _, err = f.directory.service.CancelKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.TaskID, stale)
	structureAssertError(t, err, 409, "task_revision_changed", true)
	done := structureRunTest(t, f, first)
	if done.State != "cancelled" || done.CompletedAt == nil || done.ResultViewID != nil {
		t.Fatal(done)
	}
	_, _, err = f.directory.service.CancelKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.TaskID, stale)
	structureAssertError(t, err, 409, "task_revision_changed", true)
	for _, member := range done.Members {
		if member.State != "cancelled" {
			t.Fatal(member)
		}
	}
}

func TestKnowledgeStructureLeaseTakeoverFencesOldAttempt(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	task := structureAcceptTest(t, f, f.request)
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	first, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	if again, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID()); err != nil || again != nil {
		t.Fatal("live lease double claim", again, err)
	}
	if err := dao.DB.Model(&entity.KnowledgeStructureLeaseRecord{}).Where("domain_id = ?", knowledgeTestDomain).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	second, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || second == nil || second.Epoch <= first.Epoch {
		t.Fatal(second, err)
	}
	if err := f.directory.service.structureCheckpoint(t.Context(), first, true, nil); !errors.Is(err, errStructureLeaseLost) {
		t.Fatal("expired worker progressed", err)
	}
	f.directory.service.runKnowledgeStructure(t.Context(), second)
	value, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || value.(*entity.KnowledgeStructureTask).AttemptCount != 2 || value.(*entity.KnowledgeStructureTask).State != "succeeded" {
		t.Fatal(value, err)
	}
}

func TestKnowledgeStructurePolicyABABlocksAndKeepsOldView(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	first := structureRunTest(t, f, structureAcceptTest(t, f, f.request))
	if first.ResultViewID == nil {
		t.Fatal(first)
	}
	req := f.request
	req.IdempotencyKey, req.ExpectedActiveViewID = "after-first", first.ResultViewID
	next := structureAcceptTest(t, f, req)
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", next.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	for i, enabled := range []bool{false, true} {
		_, _, err := f.directory.service.ChangeKnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeStructurePolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedStructurePolicyVersion: int64(2 + i), Enabled: enabled, Confirmed: true, IdempotencyKey: []string{"off", "on"}[i]})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.directory.service.structureCheckpoint(t.Context(), claim, false, nil); !errors.Is(err, errStructureStopped) {
		t.Fatal(err)
	}
	current, err := structureActiveHead(dao.DB, knowledgeTestOwner, knowledgeTestDomain, req.ProjectID)
	if err != nil || current == nil || *current != *first.ResultViewID {
		t.Fatal("policy ABA changed visible head", current, err)
	}
	value, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, next.TaskID)
	if err != nil || value.(*entity.KnowledgeStructureTask).State != "failed" || *value.(*entity.KnowledgeStructureTask).FailureCode != "policy_changed" {
		t.Fatal(value, err)
	}
}

func TestKnowledgeStructureAliasAndDuplicateSelectionValidation(t *testing.T) {
	for _, target := range []string{"", "frontend/", "src/组件/"} {
		if !structureValidateAliases([]entity.KnowledgeStructureAlias{{Prefix: "@/", Target: target}}) {
			t.Fatal("valid frozen directory target rejected", target)
		}
	}
	for _, target := range []string{"src", "/src/", "../src/", "src//", "src/./", "src\n/", "C:/src/"} {
		if structureValidateAliases([]entity.KnowledgeStructureAlias{{Prefix: "@/", Target: target}}) {
			t.Fatal("invalid target accepted", target)
		}
	}
	if structureValidateAliases([]entity.KnowledgeStructureAlias{{Prefix: "@/", Target: ""}, {Prefix: "@/lib/", Target: "lib/"}}) {
		t.Fatal("overlapping aliases accepted")
	}
	f := structureSetupTaskTest(t, true)
	req := f.request
	req.Members = append(slices.Clone(req.Members), req.Members[0])
	_, _, err := f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	structureAssertError(t, err, 400, "invalid_request", false)
}

func TestKnowledgeStructurePrivateCacheRebindsRevisionAndEvidence(t *testing.T) {
	structureSetupTaskTest(t, true)
	member := entity.KnowledgeStructureViewMember{SourceID: strings.Repeat("2", 32), SourceRevisionID: strings.Repeat("3", 32), Parser: "markdown", RelativePath: "web/sample.md", ByteLength: 8, SHA256: directorySHA([]byte("# Title\n"))}
	inputs := &structureInputs{Members: []entity.KnowledgeStructureViewMember{member}, Aliases: []entity.KnowledgeStructureAlias{}}
	selection, _ := structureSelectionDigest(inputs)
	file := structure.File{SourceID: member.SourceID, RevisionID: member.SourceRevisionID, Path: member.RelativePath, Parser: member.Parser, SHA256: member.SHA256, ByteLength: int(member.ByteLength), Nodes: []structure.Node{}, Relations: []structure.Relation{{Kind: "calls_local", From: structure.Ref{SourceID: member.SourceID, Key: "file"}, Resolution: "unresolved", Reason: "dynamic_call", Evidence: structure.Evidence{SourceID: member.SourceID, RevisionID: member.SourceRevisionID}}}, Diagnostics: []structure.Diagnostic{}}
	producer, _ := entity.KnowledgeStructureProducerDigest(inputs.Aliases)
	if err := structureSaveArtifact(dao.DB, knowledgeTestOwner, knowledgeTestDomain, producer, selection, member, file, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	inputs.Members[0].SourceRevisionID = strings.Repeat("4", 32)
	inputs.Members[0].LifecycleVersion = 9
	value, err := structureCachedBuild(t.Context(), knowledgeTestOwner, knowledgeTestDomain, producer, inputs)
	if err != nil || value == nil || value.Files[0].RevisionID != inputs.Members[0].SourceRevisionID || value.Files[0].Relations[0].Evidence.RevisionID != inputs.Members[0].SourceRevisionID {
		t.Fatal(value, err)
	}
	if other, err := structureCachedBuild(t.Context(), knowledgeTestOther, knowledgeTestDomain, producer, inputs); err != nil || other != nil {
		t.Fatal("private cache crossed owner scope", other, err)
	}
	inputs.Members[0].RelativePath = "web/moved.md"
	if other, err := structureCachedBuild(t.Context(), knowledgeTestOwner, knowledgeTestDomain, producer, inputs); err != nil || other != nil {
		t.Fatal("selection/path cache mismatch reused", other, err)
	}
}

func TestKnowledgeStructureCapacityRejectionSurvivesCapacityChange(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	first := structureAcceptTest(t, f, f.request)
	var firstRow entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", first.TaskID).Take(&firstRow).Error; err != nil {
		t.Fatal(err)
	}
	firstClaim, err := claimKnowledgeStructure(t.Context(), firstRow, common.GenerateUUID())
	if err != nil || firstClaim == nil {
		t.Fatal(firstClaim, err)
	}
	current, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	first = current.(*entity.KnowledgeStructureTask)
	second := f.request
	second.IdempotencyKey = "capacity-second"
	secondTask := structureAcceptTest(t, f, second)
	third := f.request
	third.IdempotencyKey = "capacity-third"
	_, _, err = f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, third)
	rejection := structureAssertError(t, err, 429, "capacity_exceeded", true)
	_, _, err = f.directory.service.CancelKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.TaskID, entity.KnowledgeStructureCancelRequest{ExpectedTaskRevision: first.Revision, Confirmed: true, IdempotencyKey: "free-capacity"})
	if err != nil {
		t.Fatal(err)
	}
	f.directory.service.runKnowledgeStructure(t.Context(), firstClaim)
	done, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, first.TaskID)
	if err != nil || done.(*entity.KnowledgeStructureTask).State != "cancelled" {
		t.Fatal(done, err)
	}
	_, _, err = f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, third)
	replay := structureAssertError(t, err, 429, "capacity_exceeded", true)
	if *replay.RejectionID != *rejection.RejectionID {
		t.Fatal("capacity change replaced fixed rejection")
	}
	third.IdempotencyKey = "capacity-explicit-new-key"
	var secondRow entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", secondTask.TaskID).Take(&secondRow).Error; err != nil {
		t.Fatal(err)
	}
	if claim, err := claimKnowledgeStructure(t.Context(), secondRow, common.GenerateUUID()); err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	structureAcceptTest(t, f, third)
}

func TestKnowledgeStructureAllowsOnlyOneWaitingTask(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	first := structureAcceptTest(t, f, f.request)
	next := f.request
	next.IdempotencyKey = "second-waiting"
	_, _, err := f.directory.service.CreateKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, next)
	structureAssertError(t, err, 429, "capacity_exceeded", true)
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", first.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID()); err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	next.IdempotencyKey = "explicit-one-waiting-after-running"
	structureAcceptTest(t, f, next)
}

func TestKnowledgeStructureAtomicPublicationRollback(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	task := structureAcceptTest(t, f, f.request)
	name := "structure-node-publication-rollback"
	if err := dao.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_structure_node" {
			tx.AddError(errors.New("synthetic node insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dao.DB.Callback().Create().Remove(name) })
	done := structureRunTest(t, f, task)
	if done.State != "failed" || done.ResultViewID != nil || done.FailureCode == nil || *done.FailureCode != "publication_failed" {
		t.Fatal(done)
	}
	for _, model := range []any{&entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureNodeRecord{}, &entity.KnowledgeStructureRelationRecord{}, &entity.KnowledgeStructureHeadRecord{}} {
		var count int64
		if err := dao.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("atomic rollback left %T count=%d err=%v", model, count, err)
		}
	}
}

func TestKnowledgeStructureSourceGenerationABABlocksOldTask(t *testing.T) {
	f := structureSetupTaskTest(t, true)
	first := structureRunTest(t, f, structureAcceptTest(t, f, f.request))
	if first.ResultViewID == nil {
		t.Fatal(first)
	}
	req := f.request
	req.IdempotencyKey, req.ExpectedActiveViewID = "source-before-aba", first.ResultViewID
	task := structureAcceptTest(t, f, req)
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	source := req.Members[0].SourceID
	before := lifecycleTestDetail(t, f.directory.service, source)
	lifecycleTestAction(t, f.directory.service, source, "archive", lifecycleTestCommand(before, "structure-archive"))
	archived := lifecycleTestDetail(t, f.directory.service, source)
	lifecycleTestAction(t, f.directory.service, source, "restore", lifecycleTestCommand(archived, "structure-restore"))
	current := lifecycleTestDetail(t, f.directory.service, source)
	if current.Current.SourceRevisionID != before.Current.SourceRevisionID || current.Current.LifecycleVersion != before.Current.LifecycleVersion+2 {
		t.Fatal("fixture did not make generation ABA")
	}
	if err := f.directory.service.structureCheckpoint(t.Context(), claim, false, nil); !errors.Is(err, errStructureStopped) {
		t.Fatal(err)
	}
	value, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || value.(*entity.KnowledgeStructureTask).State != "failed" || value.(*entity.KnowledgeStructureTask).FailureCode == nil || *value.(*entity.KnowledgeStructureTask).FailureCode != "revision_changed" {
		t.Fatal(value, err)
	}
	head, err := structureActiveHead(dao.DB, knowledgeTestOwner, knowledgeTestDomain, req.ProjectID)
	if err != nil || head == nil || *head != *first.ResultViewID {
		t.Fatal("generation ABA changed the old view", head, err)
	}
}

func TestKnowledgeStructureAttemptAndDeadlineBoundaries(t *testing.T) {
	t.Run("attempt-limit", func(t *testing.T) {
		f := structureSetupTaskTest(t, true)
		task := structureAcceptTest(t, f, f.request)
		var row entity.KnowledgeStructureTaskRecord
		if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= 3; attempt++ {
			claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
			if err != nil || claim == nil {
				t.Fatal(attempt, claim, err)
			}
			if err := dao.DB.Model(&entity.KnowledgeStructureLeaseRecord{}).Where("domain_id = ?", row.DomainID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
				t.Fatal(err)
			}
		}
		claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
		if err != nil || claim != nil {
			t.Fatal(claim, err)
		}
		value, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
		if err != nil || value.(*entity.KnowledgeStructureTask).State != "failed" || value.(*entity.KnowledgeStructureTask).AttemptCount != 3 {
			t.Fatal(value, err)
		}
	})
	t.Run("creation-deadline", func(t *testing.T) {
		f := structureSetupTaskTest(t, true)
		task := structureAcceptTest(t, f, f.request)
		if err := dao.DB.Model(&entity.KnowledgeStructureTaskRecord{}).Where("id = ?", task.TaskID).Update("deadline_at", time.Now().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
		var row entity.KnowledgeStructureTaskRecord
		if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
		if err != nil || claim != nil {
			t.Fatal(claim, err)
		}
		value, err := f.directory.service.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
		if err != nil || value.(*entity.KnowledgeStructureTask).State != "failed" || value.(*entity.KnowledgeStructureTask).CompletedAt == nil {
			t.Fatal(value, err)
		}
	})
}
