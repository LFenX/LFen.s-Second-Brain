package dataset

import (
	"encoding/json"
	"errors"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func documentSetupTest(t *testing.T, enabled bool) (*DatasetService, entity.KnowledgeDocumentTaskRequest) {
	t.Helper()
	s, reg, _ := originalFlowFixture(t)
	models := []any{&entity.KnowledgeDocumentPolicyRecord{}, &entity.KnowledgeDocumentOperationRow{}, &entity.KnowledgeDocumentRepresentationRecord{}, &entity.KnowledgeDocumentHeadRecord{}, &entity.KnowledgeDocumentNodeRecord{}, &entity.KnowledgeStructurePolicyRecord{}, &entity.KnowledgeStructureOperationRecord{}, &entity.KnowledgeStructureTaskRecord{}, &entity.KnowledgeStructureLeaseRecord{}, &entity.KnowledgeStructureCursorRecord{}}
	if err := dao.DB.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	ready := prepareOriginalFlow(t, s, reg, originalTestRequest("doc-original-ready"), []byte("%PDF-1.7\nsynthetic request fixture, not parsing evidence\n%%EOF"))
	receipt := confirmOriginalFlow(t, s, reg, ready, "doc-original-confirm")
	life, err := s.GetKnowledgeSourceLifecycle(t.Context(), knowledgeTestOwner, knowledgeTestDomain, receipt.Source.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := documentProfile(entity.KnowledgeDocumentPDFProfile)
	if err != nil {
		t.Fatal(err)
	}
	req := entity.KnowledgeDocumentTaskRequest{Target: entity.KnowledgeDocumentOriginalTarget{Kind: "original_document", SourceID: receipt.Source.SourceID, SourceRevisionID: receipt.Source.SourceRevisionID, OriginalSHA256: receipt.Source.SHA256, ExpectedLifecycleVersion: life.Current.LifecycleVersion}, ExpectedPolicyVersion: 1, ExpectedDocumentPolicyVersion: 1, ProfileID: profile.ProfileID, ExpectedProducerSHA256: profile.ProducerSHA256, Confirmed: true, IdempotencyKey: "document-task"}
	if enabled {
		value, created, err := s.ChangeKnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeDocumentPolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedDocumentPolicyVersion: 1, Enabled: true, Confirmed: true, IdempotencyKey: "enable-documents"})
		if err != nil || !created || value.(*entity.KnowledgeDocumentPolicyReceipt).After.DocumentPolicyVersion != 2 {
			t.Fatal(value, created, err)
		}
		req.ExpectedDocumentPolicyVersion = 2
	}
	return s, req
}

func documentAssertError(t *testing.T, err error, status int, code string, durable bool) {
	t.Helper()
	var own *DocumentOperationError
	if !errors.As(err, &own) || own.Status != status || own.Code != code || own.OutcomeKnown != durable || (own.RejectionID != nil) != durable {
		t.Fatalf("expected %d/%s durable=%t; got %#v", status, code, durable, err)
	}
}

func TestKnowledgeDocumentIndependentPolicyCASAndFixedReceipt(t *testing.T) {
	s, _ := documentSetupTest(t, false)
	policy, err := s.KnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || policy.Enabled || policy.DocumentPolicyVersion != 1 || policy.Mode != "document_structure_only" || policy.CodeStructurePermissionRequired {
		t.Fatal(policy, err)
	}
	req := entity.KnowledgeDocumentPolicyChangeRequest{ExpectedPolicyVersion: 1, ExpectedDocumentPolicyVersion: 1, Enabled: true, Confirmed: true, IdempotencyKey: "first-enable"}
	first, created, err := s.ChangeKnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatal(first, created, err)
	}
	noop := req
	noop.IdempotencyKey = "stale-noop"
	_, _, err = s.ChangeKnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, noop)
	documentAssertError(t, err, 409, "document_policy_changed", true)
	noop.ExpectedDocumentPolicyVersion = 2
	noop.IdempotencyKey = "proper-noop"
	v, _, err := s.ChangeKnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, noop)
	if err != nil || v.(*entity.KnowledgeDocumentPolicyReceipt).Outcome != "unchanged" || v.(*entity.KnowledgeDocumentPolicyReceipt).After.DocumentPolicyVersion != 2 {
		t.Fatal(v, err)
	}
	code, err := s.KnowledgeStructurePolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || code.Enabled || code.StructurePolicyVersion != 1 {
		t.Fatal("document policy changed code permission", code, err)
	}
	disable := noop
	disable.IdempotencyKey = "disable"
	disable.Enabled = false
	if _, _, err := s.ChangeKnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, disable); err != nil {
		t.Fatal(err)
	}
	replay, created, err := s.ChangeKnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || created || replay.(*entity.KnowledgeDocumentPolicyReceipt).CommandID != first.(*entity.KnowledgeDocumentPolicyReceipt).CommandID {
		t.Fatal(replay, created, err)
	}
	policy, err = s.KnowledgeDocumentPolicy(t.Context(), knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || policy.Enabled || policy.DocumentPolicyVersion != 3 || policy.FoundationPolicyVersion != 1 {
		t.Fatal(policy, err)
	}
}

func TestKnowledgeDocumentFixedRejectionRollbackAndPermission(t *testing.T) {
	s, req := documentSetupTest(t, false)
	_, _, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	documentAssertError(t, err, 409, "document_policy_disabled", true)
	v, err := s.GetKnowledgeDocumentOperation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, "task", req.IdempotencyKey)
	if err != nil || v.(*entity.KnowledgeDocumentRejectionReceipt).Reason != "document_policy_disabled" {
		t.Fatal(v, err)
	}
	_, err = s.GetKnowledgeDocumentOperation(t.Context(), knowledgeTestOther, knowledgeTestDomain, "task", req.IdempotencyKey)
	documentAssertError(t, err, 404, "not_found", false)
	name := "document-rejection-failure"
	if err := dao.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_document_operation" {
			tx.AddError(errors.New("synthetic SQL outage"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dao.DB.Callback().Create().Remove(name) })
	req.IdempotencyKey = "rejection-rollback"
	_, _, err = s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	documentAssertError(t, err, 503, "unavailable", false)
	if err := dao.DB.Callback().Create().Remove(name); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetKnowledgeDocumentOperation(t.Context(), knowledgeTestOwner, knowledgeTestDomain, "task", req.IdempotencyKey)
	documentAssertError(t, err, 404, "not_found", false)
}

func TestKnowledgeDocumentSharedQuotaCancelAndProtocolIsolation(t *testing.T) {
	s, req := documentSetupTest(t, true)
	v, created, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || !created {
		t.Fatal(v, created, err)
	}
	task := v.(*entity.KnowledgeDocumentTask)
	_, err = s.GetKnowledgeStructureTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	structureAssertError(t, err, 404, "not_found", false)
	waiting := req
	waiting.IdempotencyKey = "waiting-task-rejected"
	_, _, err = s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, waiting)
	documentAssertError(t, err, 429, "capacity_exceeded", true)
	var firstRow entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&firstRow).Error; err != nil {
		t.Fatal(err)
	}
	firstClaim, err := claimKnowledgeStructure(t.Context(), firstRow, common.GenerateUUID())
	if err != nil || firstClaim == nil {
		t.Fatal(firstClaim, err)
	}
	running, err := s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	task = running.(*entity.KnowledgeDocumentTask)
	req2 := req
	req2.IdempotencyKey = "second-task"
	if _, _, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req2); err != nil {
		t.Fatal(err)
	}
	req3 := req
	req3.IdempotencyKey = "third-task"
	_, _, err = s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req3)
	documentAssertError(t, err, 429, "capacity_exceeded", true)
	cancel := entity.KnowledgeDocumentCancelRequest{ExpectedTaskRevision: task.Revision, Confirmed: true, IdempotencyKey: "cancel-one"}
	receipt, _, err := s.CancelKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID, cancel)
	if err != nil || receipt.(*entity.KnowledgeDocumentCancelReceipt).Outcome != "requested" {
		t.Fatal(receipt, err)
	}
	current, err := s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || current.(*entity.KnowledgeDocumentTask).State != "cancel_requested" || current.(*entity.KnowledgeDocumentTask).CompletedAt != nil {
		t.Fatal(current, err)
	}
	cancel.IdempotencyKey = "stale-cancel-noop"
	_, _, err = s.CancelKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID, cancel)
	documentAssertError(t, err, 409, "task_revision_changed", true)
	// This unit attempt has not started an OS child. The separate child tests
	// prove actual stop/wait/cleanup before production invokes this retirement.
	s.documentFailClaim(firstClaim, "native_timeout")
	current, err = s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || current.(*entity.KnowledgeDocumentTask).State != "cancelled" || current.(*entity.KnowledgeDocumentTask).CompletedAt == nil {
		t.Fatal(current, err)
	}
}

func TestKnowledgeDocumentStopKeepsFenceUntilCleanup(t *testing.T) {
	s, req := documentSetupTest(t, true)
	v, _, err := s.CreateKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil {
		t.Fatal(err)
	}
	task := v.(*entity.KnowledgeDocumentTask)
	var row entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(t.Context(), row, common.GenerateUUID())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	current, err := s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.CancelKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID, entity.KnowledgeDocumentCancelRequest{ExpectedTaskRevision: current.(*entity.KnowledgeDocumentTask).Revision, Confirmed: true, IdempotencyKey: "stop-owned"})
	if err != nil {
		t.Fatal(err)
	}
	err = s.documentCheckpoint(t.Context(), claim, false, nil)
	var stop *documentStopError
	if !errors.As(err, &stop) || !stop.cancelled {
		t.Fatal(err)
	}
	current, err = s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || current.(*entity.KnowledgeDocumentTask).State != "cancel_requested" {
		t.Fatal(current, err)
	}
	var lease entity.KnowledgeStructureLeaseRecord
	if err := dao.DB.Where("domain_id = ?", knowledgeTestDomain).Take(&lease).Error; err != nil || lease.TaskID != task.TaskID {
		t.Fatal("checkpoint released execution fence", lease, err)
	}
	s.documentFailClaim(claim, "native_timeout")
	current, err = s.GetKnowledgeDocumentTask(t.Context(), knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || current.(*entity.KnowledgeDocumentTask).State != "cancelled" {
		t.Fatal(current, err)
	}
}

func TestKnowledgeDocumentBindingChecksNaturalManifestAndCoverage(t *testing.T) {
	text := "合成\n正文"
	rootRef := entity.KnowledgeDocumentNaturalRef{Kind: "document", Locator: entity.KnowledgeDocumentLocator{Scheme: "document_v1"}}
	rangeOf := func(a, b int64) entity.KnowledgeDocumentReadingRange {
		return entity.KnowledgeDocumentReadingRange{StartByte: a, EndByte: b, TextSHA256: directorySHA([]byte(text[a:b]))}
	}
	root := entity.KnowledgeDocumentNodePayload{Kind: "document", Attributes: json.RawMessage(`{}`), Locator: rootRef.Locator, Name: "", OriginalPosition: entity.KnowledgeDocumentOriginalPosition{Kind: "document_only"}, CoordinatePrecision: "document", TextStatus: "available", ReadingRange: rangeOf(0, int64(len(text)))}
	paragraph := root
	paragraph.Kind = "paragraph"
	paragraph.ParentRef = &rootRef
	paragraph.Locator = entity.KnowledgeDocumentLocator{Scheme: "office_package_v1", Part: "word/document.xml", Segments: []entity.KnowledgeDocumentOfficeSegment{{Namespace: "http://schemas.openxmlformats.org/wordprocessingml/2006/main", Name: "document", Index: 0}, {Namespace: "http://schemas.openxmlformats.org/wordprocessingml/2006/main", Name: "body", Index: 0}, {Namespace: "http://schemas.openxmlformats.org/wordprocessingml/2006/main", Name: "p", Index: 0}}}
	paragraph.CoordinatePrecision = "source_path"
	own := paragraph.ReadingRange
	paragraph.OwnReadingRange = &own
	passage := root
	passage.Kind = "passage"
	passage.Attributes = json.RawMessage(`{"chunk_index":0}`)
	passage.ParentRef = &rootRef
	passage.Ordinal = 1
	passage.Locator = entity.KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &rootRef.Locator, ChunkIndex: 0}
	passage.CoordinatePrecision = "derived_range"
	passage.OwnReadingRange = &own
	result := parser.DocumentStructureResult{ReadingText: text, Payloads: []entity.KnowledgeDocumentNodePayload{root, paragraph, passage}, Quality: "complete", TextCoverage: "supported_profile_blocks"}
	profile, err := documentProfile(entity.KnowledgeDocumentDOCXProfile)
	if err != nil {
		t.Fatal(err)
	}
	fixed := &documentInputs{Original: entity.KnowledgeDocumentOriginalRef{DomainID: knowledgeTestDomain, RegistrationID: "11111111111111111111111111111111", SourceID: "22222222222222222222222222222222", SourceRevisionID: "33333333333333333333333333333333", SHA256: directorySHA([]byte("original")), ByteLength: 8, MediaType: profile.MediaType, OriginalFilename: "synthetic.docx", RepresentationKind: "original_only"}, Snapshot: entity.KnowledgeDocumentSourceSnapshot{LifecycleVersion: 1, RelativePath: "synthetic.docx", Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code", Language: ""}}, FoundationPolicyVersion: 1, DocumentPolicyVersion: 2, Profile: *profile}
	first, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, result)
	if err != nil || len(first.Nodes) != 3 {
		t.Fatal(first, err)
	}
	second, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, result)
	if err != nil || first.Representation.RepresentationID != second.Representation.RepresentationID {
		t.Fatal("unstable identity", err)
	}
	fixed.Snapshot.LifecycleVersion++
	changed, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, result)
	if err != nil || first.Representation.RepresentationID == changed.Representation.RepresentationID {
		t.Fatal("ABA generation not in identity", err)
	}
	result.Payloads[2].ReadingRange.StartByte = 3
	if _, err := documentBind(knowledgeTestOwner, knowledgeTestDomain, fixed, result); err == nil {
		t.Fatal("accepted incomplete passage coverage")
	}
}
