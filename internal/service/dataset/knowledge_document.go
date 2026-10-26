package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

const documentTaskProtocol = "document_structure/0.1.0"

type DocumentOperationError struct {
	Status       int
	Code         string
	OutcomeKnown bool
	RejectionID  *string
}

func (e *DocumentOperationError) Error() string { return e.Code }

func documentError(status int, code string) error {
	return &DocumentOperationError{Status: status, Code: code}
}

func documentNormalize(err error) error {
	if err == nil {
		return nil
	}
	var own *DocumentOperationError
	if errors.As(err, &own) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return documentError(404, "not_found")
	}
	var old *KnowledgeError
	if errors.As(err, &old) && old.Status == 404 {
		return documentError(404, "not_found")
	}
	return documentError(503, "unavailable")
}

func documentDecode[T any](raw string) (*T, error) {
	var v T
	if json.Unmarshal([]byte(raw), &v) != nil {
		return nil, documentError(409, "document_integrity")
	}
	return &v, nil
}

func (d *DatasetService) documentTransaction(ctx context.Context, owner, domain string, fn func(*gorm.DB, *entity.Knowledgebase, time.Time) error) error {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return documentNormalize(err)
	}
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := dao.DB.WithContext(work).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(work, tx, owner, domain)
		if err != nil {
			return err
		}
		now, err := batchNow(tx)
		if err != nil {
			return err
		}
		return fn(tx, kb, now)
	})
	return documentNormalize(err)
}

func documentPolicyRecord(tx *gorm.DB, owner, domain string) (*entity.KnowledgeDocumentPolicyRecord, error) {
	var row entity.KnowledgeDocumentPolicyRecord
	err := tx.Where("domain_id = ?", domain).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &entity.KnowledgeDocumentPolicyRecord{DomainID: domain, OwnerID: owner, Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	if row.OwnerID != owner || !structureVersion(row.Version) {
		return nil, documentError(409, "document_integrity")
	}
	return &row, nil
}

func (d *DatasetService) KnowledgeDocumentPolicy(ctx context.Context, owner, domain string) (*entity.KnowledgeDocumentPolicy, error) {
	var value *entity.KnowledgeDocumentPolicy
	err := d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, _ time.Time) error {
		row, err := documentPolicyRecord(tx, owner, domain)
		if err != nil {
			return err
		}
		value = &entity.KnowledgeDocumentPolicy{DomainID: domain, FoundationPolicyVersion: kb.KnowledgePolicyVersion, DocumentPolicyVersion: row.Version, Enabled: row.Enabled, Mode: "document_structure_only", AllowedModelUses: []string{}, LexicalIndex: "not_created", VectorIndex: "not_created"}
		return nil
	})
	return value, err
}

func documentOperationID(owner, domain, kind, key string) string {
	id, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-document-operation/0.1.0", "owner_id": owner, "domain_id": domain, "operation": kind, "idempotency_key": key})
	return id
}

func documentFindOperation(tx *gorm.DB, owner, domain, kind, key string) (*entity.KnowledgeDocumentOperationRow, error) {
	var row entity.KnowledgeDocumentOperationRow
	err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", documentOperationID(owner, domain, kind, key), owner, domain, kind).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.Key != key {
		return nil, documentError(409, "document_integrity")
	}
	return &row, nil
}

func documentSaveOperation(tx *gorm.DB, owner, domain, kind, key, taskID, digest, outcome string, status int, value any, now time.Time) error {
	payload, err := structureJSON(value)
	if err != nil {
		return err
	}
	return tx.Create(&entity.KnowledgeDocumentOperationRow{ID: documentOperationID(owner, domain, kind, key), OwnerID: owner, DomainID: domain, Kind: kind, Key: key, TaskID: taskID, RequestSHA256: digest, Status: status, OutcomeKind: outcome, Payload: payload, CreatedAt: now}).Error
}

func documentOperationValue(tx *gorm.DB, row *entity.KnowledgeDocumentOperationRow) (any, error) {
	switch row.OutcomeKind {
	case "task":
		_, task, err := documentLoadTask(tx, row.OwnerID, row.DomainID, row.TaskID)
		if err == nil {
			err = documentAuthorizeTarget(tx, row.DomainID, task.Original.SourceID, task.Original.SourceRevisionID)
		}
		return task, err
	case "policy":
		return documentDecode[entity.KnowledgeDocumentPolicyReceipt](row.Payload)
	case "cancel":
		_, task, err := documentLoadTask(tx, row.OwnerID, row.DomainID, row.TaskID)
		if err != nil {
			return nil, err
		}
		if err := documentAuthorizeTarget(tx, row.DomainID, task.Original.SourceID, task.Original.SourceRevisionID); err != nil {
			return nil, err
		}
		return documentDecode[entity.KnowledgeDocumentCancelReceipt](row.Payload)
	case "rejection":
		value, err := documentDecode[entity.KnowledgeDocumentRejectionReceipt](row.Payload)
		if err != nil {
			return nil, err
		}
		if value.Operation == "task" {
			req, err := documentDecode[entity.KnowledgeDocumentTaskRequest](string(value.OriginalRequest))
			if err != nil {
				return nil, err
			}
			if err := documentAuthorizeTarget(tx, row.DomainID, req.Target.SourceID, req.Target.SourceRevisionID); err != nil {
				return nil, err
			}
		}
		if value.Operation == "cancel" {
			_, task, err := documentLoadTask(tx, row.OwnerID, row.DomainID, row.TaskID)
			if err != nil {
				return nil, err
			}
			if err := documentAuthorizeTarget(tx, row.DomainID, task.Original.SourceID, task.Original.SourceRevisionID); err != nil {
				return nil, err
			}
		}
		return value, nil
	default:
		return nil, documentError(409, "document_integrity")
	}
}

func documentAuthorizeTarget(tx *gorm.DB, domain, source, revision string) error {
	var row entity.KnowledgeSourceRevision
	return tx.Model(&entity.KnowledgeSourceRevision{}).Select("knowledge_source_revision.id").Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").Where("knowledge_source_revision.id = ? AND knowledge_source_revision.domain_id = ? AND knowledge_source_revision.document_id = ? AND document.status = ?", revision, domain, source, "1").Take(&row).Error
}

func documentReplay(tx *gorm.DB, row *entity.KnowledgeDocumentOperationRow, digest, taskID string) (any, error) {
	if row.RequestSHA256 != digest || row.Kind == "cancel" && row.TaskID != taskID {
		return nil, documentError(409, "idempotency_conflict")
	}
	value, err := documentOperationValue(tx, row)
	if err != nil {
		return nil, err
	}
	if rejected, ok := value.(*entity.KnowledgeDocumentRejectionReceipt); ok {
		return nil, &DocumentOperationError{Status: row.Status, Code: rejected.Reason, OutcomeKnown: true, RejectionID: &rejected.RejectionID}
	}
	return value, nil
}

func documentReject(tx *gorm.DB, owner, domain, kind, key, taskID, digest, reason string, target, original any, now time.Time) (*DocumentOperationError, error) {
	request, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	targetJSON, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}
	receipt := entity.KnowledgeDocumentRejectionReceipt{RecordKind: "rejection", RejectionID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, Operation: kind, Target: targetJSON, IdempotencyKey: key, OriginalRequest: request, RequestSHA256: digest, Reason: reason, RecordedAt: structureTimestamp(now)}
	status := 409
	if reason == "capacity_exceeded" {
		status = 429
	}
	if err := documentSaveOperation(tx, owner, domain, kind, key, taskID, digest, "rejection", status, &receipt, now); err != nil {
		return nil, err
	}
	return &DocumentOperationError{Status: status, Code: reason, OutcomeKnown: true, RejectionID: &receipt.RejectionID}, nil
}

func (d *DatasetService) ChangeKnowledgeDocumentPolicy(ctx context.Context, owner, domain string, req entity.KnowledgeDocumentPolicyChangeRequest) (any, bool, error) {
	if !structureKey(req.IdempotencyKey) || !req.Confirmed || !structureVersion(req.ExpectedPolicyVersion) || !structureVersion(req.ExpectedDocumentPolicyVersion) {
		return nil, false, documentError(400, "invalid_request")
	}
	digest, err := entity.KnowledgeDocumentPolicyDigest(owner, domain, req)
	if err != nil {
		return nil, false, documentError(400, "invalid_request")
	}
	var value any
	var rejected *DocumentOperationError
	created := false
	err = d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		existing, err := documentFindOperation(tx, owner, domain, "policy", req.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			value, err = documentReplay(tx, existing, digest, "")
			return err
		}
		row, err := documentPolicyRecord(tx, owner, domain)
		if err != nil {
			return err
		}
		reason := ""
		if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion {
			reason = "foundation_policy_changed"
		} else if row.Version != req.ExpectedDocumentPolicyVersion || row.Enabled != req.Enabled && row.Version == entity.KnowledgeMaxLifecycleVersion {
			reason = "document_policy_changed"
		}
		target := entity.KnowledgeDocumentDomainTarget{Kind: "domain", DomainID: domain}
		if reason != "" {
			rejected, err = documentReject(tx, owner, domain, "policy", req.IdempotencyKey, "", digest, reason, target, req, now)
			return err
		}
		before := entity.KnowledgeDocumentPolicySnapshot{DocumentPolicyVersion: row.Version, Enabled: row.Enabled}
		outcome := "unchanged"
		if row.Enabled != req.Enabled {
			row.Enabled = req.Enabled
			row.Version++
			outcome = "applied"
		}
		row.UpdatedAt = now
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		receipt := &entity.KnowledgeDocumentPolicyReceipt{RecordKind: "policy_receipt", CommandID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, Target: target, IdempotencyKey: req.IdempotencyKey, OriginalRequest: req, RequestSHA256: digest, Before: before, After: entity.KnowledgeDocumentPolicySnapshot{DocumentPolicyVersion: row.Version, Enabled: row.Enabled}, Outcome: outcome, CompletedAt: structureTimestamp(now)}
		if err := documentSaveOperation(tx, owner, domain, "policy", req.IdempotencyKey, "", digest, "policy", 201, receipt, now); err != nil {
			return err
		}
		value, created = receipt, true
		return nil
	})
	if err == nil && rejected != nil {
		err = rejected
	}
	return value, created, err
}

type documentInputs struct {
	Original                       entity.KnowledgeDocumentOriginalRef    `json:"original"`
	Snapshot                       entity.KnowledgeDocumentSourceSnapshot `json:"snapshot"`
	FoundationPolicyVersion        int64                                  `json:"foundation_policy_version"`
	DocumentPolicyVersion          int64                                  `json:"document_policy_version"`
	ExpectedActiveRepresentationID *string                                `json:"expected_active_representation_id"`
	Profile                        entity.KnowledgeDocumentProfile        `json:"profile"`
}

func documentActiveHead(tx *gorm.DB, owner, domain, source string) (*string, error) {
	var row entity.KnowledgeDocumentHeadRecord
	err := tx.Where("source_id = ? AND domain_id = ? AND owner_id = ?", source, domain, owner).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row.RepresentationID, nil
}

func documentCapture(tx *gorm.DB, kb *entity.Knowledgebase, req entity.KnowledgeDocumentTaskRequest) (*documentInputs, string, error) {
	policy, err := documentPolicyRecord(tx, kb.CreatedBy, kb.ID)
	if err != nil {
		return nil, "", err
	}
	if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion {
		return nil, "foundation_policy_changed", nil
	}
	if policy.Version != req.ExpectedDocumentPolicyVersion {
		return nil, "document_policy_changed", nil
	}
	if !policy.Enabled {
		return nil, "document_policy_disabled", nil
	}
	data, err := loadLifecycleSource(tx, kb, req.Target.SourceID)
	if err != nil {
		return nil, "", err
	}
	if data.head.RepresentationKind != "original_only" || data.registration == nil || data.registration.SourceKind != "original_document" {
		return nil, "unsupported_source", nil
	}
	if data.head.ID != req.Target.SourceRevisionID || data.head.SHA256 != req.Target.OriginalSHA256 {
		return nil, "source_revision_changed", nil
	}
	if data.state.Version != req.Target.ExpectedLifecycleVersion {
		return nil, "lifecycle_changed", nil
	}
	if data.state.State != "active" {
		return nil, "source_archived", nil
	}
	profile, err := documentProfile(req.ProfileID)
	if err != nil {
		return nil, "producer_changed", nil
	}
	if !profile.RuntimeReady || profile.ProducerSHA256 != req.ExpectedProducerSHA256 {
		return nil, "producer_changed", nil
	}
	if data.head.ByteLength < 1 || data.head.ByteLength > 8<<20 || data.head.OriginalMediaType != profile.MediaType {
		return nil, "unsupported_source", nil
	}
	head, err := documentActiveHead(tx, kb.CreatedBy, kb.ID, req.Target.SourceID)
	if err != nil {
		return nil, "", err
	}
	if !sameOptionalID(head, req.ExpectedActiveRepresentationID) {
		return nil, "head_changed", nil
	}
	original := entity.KnowledgeDocumentOriginalRef{DomainID: kb.ID, RegistrationID: data.head.OriginalRegistrationID, SourceID: data.head.DocumentID, SourceRevisionID: data.head.ID, SHA256: data.head.SHA256, ByteLength: data.head.ByteLength, MediaType: data.head.OriginalMediaType, OriginalFilename: data.head.OriginalFilename, RepresentationKind: "original_only"}
	snapshot := entity.KnowledgeDocumentSourceSnapshot{LifecycleVersion: data.state.Version, RelativePath: data.head.RelativePath, Classification: entity.KnowledgeDirectoryClassification{GroupID: data.head.GroupID, ComponentSide: data.head.ComponentSide, Language: data.head.Language}}
	return &documentInputs{Original: original, Snapshot: snapshot, FoundationPolicyVersion: kb.KnowledgePolicyVersion, DocumentPolicyVersion: policy.Version, ExpectedActiveRepresentationID: head, Profile: *profile}, "", nil
}

func documentValidateTask(req entity.KnowledgeDocumentTaskRequest) bool {
	return req.Target.Kind == "original_document" && isKnowledgeID(req.Target.SourceID, 32) && isKnowledgeID(req.Target.SourceRevisionID, 32) && isKnowledgeID(req.Target.OriginalSHA256, 64) && structureVersion(req.Target.ExpectedLifecycleVersion) && structureVersion(req.ExpectedPolicyVersion) && structureVersion(req.ExpectedDocumentPolicyVersion) && (req.ExpectedActiveRepresentationID == nil || isKnowledgeID(*req.ExpectedActiveRepresentationID, 64)) && slices.Contains([]string{"pdf-plain-pages/1", "pptx-ooxml-slides/1", "docx-ooxml-blocks/1"}, req.ProfileID) && isKnowledgeID(req.ExpectedProducerSHA256, 64) && req.Confirmed && structureKey(req.IdempotencyKey)
}

func (d *DatasetService) CreateKnowledgeDocumentTask(ctx context.Context, owner, domain string, req entity.KnowledgeDocumentTaskRequest) (any, bool, error) {
	if !documentValidateTask(req) {
		return nil, false, documentError(400, "invalid_request")
	}
	digest, err := entity.KnowledgeDocumentTaskDigest(owner, domain, req)
	if err != nil {
		return nil, false, documentError(400, "invalid_request")
	}
	var value any
	var rejected *DocumentOperationError
	created := false
	err = d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		existing, err := documentFindOperation(tx, owner, domain, "task", req.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			value, err = documentReplay(tx, existing, digest, "")
			return err
		}
		if err := documentAuthorizeTarget(tx, domain, req.Target.SourceID, req.Target.SourceRevisionID); err != nil {
			return err
		}
		fixed, reason, err := documentCapture(tx, kb, req)
		if err != nil {
			return err
		}
		if reason == "" {
			full, err := structureTaskCapacity(tx, domain)
			if err != nil {
				return err
			}
			if full {
				reason = "capacity_exceeded"
			}
		}
		if reason != "" {
			rejected, err = documentReject(tx, owner, domain, "task", req.IdempotencyKey, "", digest, reason, req.Target, req, now)
			return err
		}
		task := &entity.KnowledgeDocumentTask{RecordKind: "task", TaskID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, OriginalRequest: req, Original: fixed.Original, SourceSnapshot: fixed.Snapshot, RequestSHA256: digest, ProducerSHA256: fixed.Profile.ProducerSHA256, Revision: 1, State: "queued", Phase: "queued", CreatedAt: structureTimestamp(now), UpdatedAt: structureTimestamp(now), DeadlineAt: structureTimestamp(now.Add(120 * time.Second)), Diagnostics: []entity.KnowledgeDocumentTaskDiagnostic{}}
		payload, err := structureJSON(task)
		if err != nil {
			return err
		}
		inputs, err := structureJSON(fixed)
		if err != nil {
			return err
		}
		row := entity.KnowledgeStructureTaskRecord{ID: task.TaskID, Protocol: documentTaskProtocol, TargetKind: "original_document", TargetID: req.Target.SourceID, OwnerID: owner, DomainID: domain, Revision: 1, State: "queued", RequestSHA256: digest, ProducerSHA256: task.ProducerSHA256, Payload: payload, Inputs: inputs, CreatedAt: now, UpdatedAt: now, DeadlineAt: now.Add(120 * time.Second)}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := documentSaveOperation(tx, owner, domain, "task", req.IdempotencyKey, task.TaskID, digest, "task", 201, task, now); err != nil {
			return err
		}
		value, created = task, true
		return nil
	})
	if err == nil && rejected != nil {
		err = rejected
	}
	return value, created, err
}

func documentLoadTask(tx *gorm.DB, owner, domain, id string) (*entity.KnowledgeStructureTaskRecord, *entity.KnowledgeDocumentTask, error) {
	var row entity.KnowledgeStructureTaskRecord
	err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", id, owner, domain, documentTaskProtocol).Take(&row).Error
	if err != nil {
		return nil, nil, err
	}
	task, err := documentDecode[entity.KnowledgeDocumentTask](row.Payload)
	if err != nil {
		return nil, nil, err
	}
	if task.TaskID != row.ID || task.OwnerID != owner || task.DomainID != domain || task.State != row.State || task.Revision != row.Revision || task.AttemptCount != row.AttemptCount || task.RequestSHA256 != row.RequestSHA256 || task.ProducerSHA256 != row.ProducerSHA256 || row.TargetKind != "original_document" || row.TargetID != task.Original.SourceID {
		return nil, nil, documentError(409, "document_integrity")
	}
	return &row, task, nil
}

func documentSaveTask(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, now time.Time) error {
	if structureTerminal(row.State) || row.Revision >= entity.KnowledgeMaxLifecycleVersion {
		return documentError(409, "document_integrity")
	}
	before := row.Revision
	task.Revision = before + 1
	task.UpdatedAt = structureTimestamp(now)
	var completed *time.Time
	if structureTerminal(task.State) {
		task.Phase = "done"
		stamp := structureTimestamp(now)
		task.CompletedAt = &stamp
		completed = &now
	}
	payload, err := structureJSON(task)
	if err != nil {
		return err
	}
	result := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ? AND revision = ? AND state = ?", row.ID, row.OwnerID, row.DomainID, documentTaskProtocol, before, row.State).Updates(map[string]any{"revision": task.Revision, "state": task.State, "attempt_count": task.AttemptCount, "payload": payload, "updated_at": now, "completed_at": completed})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return documentError(409, "document_integrity")
	}
	row.Revision, row.State, row.AttemptCount, row.Payload, row.UpdatedAt, row.CompletedAt = task.Revision, task.State, task.AttemptCount, payload, now, completed
	return nil
}

func (d *DatasetService) GetKnowledgeDocumentTask(ctx context.Context, owner, domain, id string) (any, error) {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return nil, documentNormalize(err)
	}
	_, task, err := documentLoadTask(dao.DB.WithContext(ctx), owner, domain, id)
	if err != nil {
		return nil, documentNormalize(err)
	}
	if _, err := d.GetKnowledgeOriginalRevision(ctx, owner, domain, task.Original.SourceRevisionID); err != nil {
		return nil, documentNormalize(err)
	}
	return task, nil
}

func (d *DatasetService) GetKnowledgeDocumentOperation(ctx context.Context, owner, domain, kind, key string) (any, error) {
	var value any
	err := d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		if !slices.Contains([]string{"task", "policy", "cancel"}, kind) || !structureKey(key) {
			return documentError(400, "invalid_request")
		}
		row, err := documentFindOperation(tx, owner, domain, kind, key)
		if err != nil {
			return err
		}
		if row == nil {
			return documentError(404, "not_found")
		}
		value, err = documentOperationValue(tx, row)
		if err != nil {
			return err
		}
		if row.TaskID != "" {
			_, task, err := documentLoadTask(tx, owner, domain, row.TaskID)
			if err != nil {
				return err
			}
			_, err = loadOriginalRevision(tx, domain, task.Original.SourceRevisionID)
			return err
		}
		return nil
	})
	return value, err
}

func (d *DatasetService) CancelKnowledgeDocumentTask(ctx context.Context, owner, domain, taskID string, req entity.KnowledgeDocumentCancelRequest) (any, bool, error) {
	if !isKnowledgeID(taskID, 32) || !structureVersion(req.ExpectedTaskRevision) || !req.Confirmed || !structureKey(req.IdempotencyKey) {
		return nil, false, documentError(400, "invalid_request")
	}
	digest, err := entity.KnowledgeDocumentCancelDigest(owner, domain, taskID, req)
	if err != nil {
		return nil, false, documentError(400, "invalid_request")
	}
	var value any
	var rejected *DocumentOperationError
	created := false
	err = d.documentTransaction(ctx, owner, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		existing, err := documentFindOperation(tx, owner, domain, "cancel", req.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			value, err = documentReplay(tx, existing, digest, taskID)
			return err
		}
		row, task, err := documentLoadTask(tx, owner, domain, taskID)
		if err != nil {
			return err
		}
		if _, err := loadOriginalRevision(tx, domain, task.Original.SourceRevisionID); err != nil {
			return err
		}
		target := entity.KnowledgeDocumentTaskTarget{Kind: "task", TaskID: taskID}
		if task.Revision != req.ExpectedTaskRevision || !structureTerminal(task.State) && task.State != "cancel_requested" && task.Revision == entity.KnowledgeMaxLifecycleVersion {
			rejected, err = documentReject(tx, owner, domain, "cancel", req.IdempotencyKey, taskID, digest, "task_revision_changed", target, req, now)
			return err
		}
		before, outcome := task.State, "unchanged"
		if !structureTerminal(task.State) && task.State != "cancel_requested" {
			task.State, task.Phase = "cancel_requested", "stopping"
			if err := documentSaveTask(tx, row, task, now); err != nil {
				return err
			}
			outcome = "requested"
		}
		receipt := &entity.KnowledgeDocumentCancelReceipt{RecordKind: "cancel_receipt", CommandID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, Target: target, IdempotencyKey: req.IdempotencyKey, OriginalRequest: req, RequestSHA256: digest, BeforeState: before, AfterState: task.State, TaskRevision: task.Revision, Outcome: outcome, CompletedAt: structureTimestamp(now)}
		if err := documentSaveOperation(tx, owner, domain, "cancel", req.IdempotencyKey, taskID, digest, "cancel", 201, receipt, now); err != nil {
			return err
		}
		value, created = receipt, true
		return nil
	})
	if err == nil && rejected != nil {
		err = rejected
	}
	return value, created, err
}
