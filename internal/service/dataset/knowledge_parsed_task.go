package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

func (d *DatasetService) parsedTransaction(ctx context.Context, owner, domain string, fn func(*gorm.DB, *entity.Knowledgebase, time.Time) error) error {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return documentNormalize(err)
	}
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return documentNormalize(dao.DB.WithContext(work).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(work, tx, owner, domain)
		if err != nil {
			return err
		}
		now, err := batchNow(tx)
		if err != nil {
			return err
		}
		return fn(tx, kb, now)
	}))
}

func parsedPolicyRecord(tx *gorm.DB, owner, domain string) (*entity.KnowledgeContentPolicyRecord, error) {
	var row entity.KnowledgeContentPolicyRecord
	err := tx.Where("domain_id = ?", domain).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &entity.KnowledgeContentPolicyRecord{DomainID: domain, OwnerID: owner, Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	if row.OwnerID != owner || !structureVersion(row.Version) {
		return nil, documentError(409, "content_integrity")
	}
	return &row, nil
}

func parsedPolicyView(kb *entity.Knowledgebase, row *entity.KnowledgeContentPolicyRecord) *entity.KnowledgeContentPolicy {
	return &entity.KnowledgeContentPolicy{DomainID: kb.ID, FoundationPolicyVersion: kb.KnowledgePolicyVersion, ContentPolicyVersion: row.Version, ParseEnabled: row.ParseEnabled, LexicalEnabled: row.LexicalEnabled}
}

func (d *DatasetService) KnowledgeContentPolicy(ctx context.Context, owner, domain string) (*entity.KnowledgeContentPolicy, error) {
	var value *entity.KnowledgeContentPolicy
	err := d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, _ time.Time) error {
		row, err := parsedPolicyRecord(tx, owner, domain)
		if err == nil {
			value = parsedPolicyView(kb, row)
		}
		return err
	})
	return value, err
}

func parsedOperationID(owner, domain, kind, key string) string {
	id, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "content-operation/0.2.0", "owner_id": owner, "domain_id": domain, "operation": kind, "idempotency_key": key})
	return id
}

func parsedFindOperation(tx *gorm.DB, owner, domain, kind, key, digest string) (*entity.KnowledgeStructureOperationRecord, error) {
	var row entity.KnowledgeStructureOperationRecord
	err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", parsedOperationID(owner, domain, kind, key), owner, domain, kind).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.Key != key {
		return nil, documentError(409, "content_integrity")
	}
	if row.RequestSHA256 != digest {
		return nil, documentError(409, "idempotency_conflict")
	}
	return &row, nil
}

func parsedSaveOperation(tx *gorm.DB, owner, domain, kind, key, digest, taskID, outcome string, status int, value any, now time.Time) error {
	payload, err := structureJSON(value)
	if err != nil {
		return err
	}
	return tx.Create(&entity.KnowledgeStructureOperationRecord{ID: parsedOperationID(owner, domain, kind, key), OwnerID: owner, DomainID: domain, Kind: kind, Key: key, TaskID: taskID, RequestSHA256: digest, OutcomeKind: outcome, Status: status, Payload: payload, CreatedAt: now}).Error
}

func parsedReject(tx *gorm.DB, owner, domain, kind, key, digest, code string, now time.Time) (*DocumentOperationError, error) {
	status := 409
	if code == "capacity_exceeded" {
		status = 429
	}
	id := parsedOperationID(owner, domain, kind, key)
	err := parsedSaveOperation(tx, owner, domain, kind, key, digest, "", "rejection", status, map[string]string{"code": code}, now)
	return &DocumentOperationError{Status: status, Code: code, OutcomeKnown: true, RejectionID: &id}, err
}

func parsedReplayError(row *entity.KnowledgeStructureOperationRecord) error {
	var value struct {
		Code string `json:"code"`
	}
	if entity.KnowledgeParsedDecode([]byte(row.Payload), &value) != nil || value.Code == "" {
		return documentError(409, "content_integrity")
	}
	return &DocumentOperationError{Status: row.Status, Code: value.Code, OutcomeKnown: true, RejectionID: &row.ID}
}

func (d *DatasetService) ChangeKnowledgeContentPolicy(ctx context.Context, owner, domain string, req entity.KnowledgeContentPolicyRequest) (*entity.KnowledgeContentPolicy, bool, error) {
	if !structureVersion(req.ExpectedPolicyVersion) || !structureVersion(req.ExpectedContentPolicyVersion) || !req.Confirmed || !structureKey(req.IdempotencyKey) {
		return nil, false, documentError(400, "invalid_request")
	}
	digest, err := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "content-policy/0.2.0", "owner_id": owner, "domain_id": domain, "request": req})
	if err != nil {
		return nil, false, err
	}
	var value *entity.KnowledgeContentPolicy
	var rejected error
	created := false
	err = d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		op, err := parsedFindOperation(tx, owner, domain, "parsed_policy", req.IdempotencyKey, digest)
		if err != nil {
			return err
		}
		if op != nil {
			if op.OutcomeKind == "rejection" {
				rejected = parsedReplayError(op)
				return nil
			}
			value = &entity.KnowledgeContentPolicy{}
			if op.OutcomeKind != "policy" || entity.KnowledgeParsedDecode([]byte(op.Payload), value) != nil {
				return documentError(409, "content_integrity")
			}
			return nil
		}
		row, err := parsedPolicyRecord(tx, owner, domain)
		if err != nil {
			return err
		}
		changed := row.ParseEnabled != req.ParseEnabled || row.LexicalEnabled != req.LexicalEnabled
		code := ""
		if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion {
			code = "foundation_policy_changed"
		} else if row.Version != req.ExpectedContentPolicyVersion || changed && row.Version == entity.KnowledgeMaxLifecycleVersion {
			code = "content_policy_changed"
		}
		if code != "" {
			rejected, err = parsedReject(tx, owner, domain, "parsed_policy", req.IdempotencyKey, digest, code, now)
			return err
		}
		if changed {
			row.Version++
			row.ParseEnabled, row.LexicalEnabled, row.UpdatedAt = req.ParseEnabled, req.LexicalEnabled, now
			if err := tx.Save(row).Error; err != nil {
				return err
			}
		}
		value = parsedPolicyView(kb, row)
		if err := parsedSaveOperation(tx, owner, domain, "parsed_policy", req.IdempotencyKey, digest, "", "policy", 200, value, now); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err == nil {
		err = rejected
	}
	return value, created, err
}

type parsedInputs struct {
	Source                  entity.KnowledgeContentSourceRef        `json:"source_ref"`
	NativeFileID            string                                  `json:"native_file_id"`
	CommitID                string                                  `json:"commit_id"`
	FolderID                string                                  `json:"folder_id"`
	ByteLength              int64                                   `json:"byte_length"`
	SourceInputSHA256       string                                  `json:"source_input_sha256"`
	RelativePath            string                                  `json:"relative_path"`
	Classification          entity.KnowledgeDirectoryClassification `json:"classification"`
	LifecycleVersion        int64                                   `json:"lifecycle_version"`
	FoundationPolicyVersion int64                                   `json:"foundation_policy_version"`
	ContentPolicyVersion    int64                                   `json:"content_policy_version"`
	Filename                string                                  `json:"filename"`
	Profile                 entity.KnowledgeParsedProfile           `json:"profile"`
}

func parsedTaskDigest(owner, domain string, req entity.KnowledgeParsedTaskRequest) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-task/0.2.0", "owner_id": owner, "domain_id": domain, "request": req})
}

func parsedCapture(tx *gorm.DB, kb *entity.Knowledgebase, req entity.KnowledgeParsedTaskRequest) (*parsedInputs, string, error) {
	policy, err := parsedPolicyRecord(tx, kb.CreatedBy, kb.ID)
	if err != nil {
		return nil, "", err
	}
	if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion {
		return nil, "foundation_policy_changed", nil
	}
	if policy.Version != req.ExpectedContentPolicyVersion {
		return nil, "content_policy_changed", nil
	}
	if !policy.ParseEnabled {
		return nil, "content_policy_disabled", nil
	}
	source, err := parsedSourceRow(tx, kb, req.Source)
	if err != nil {
		return nil, "", err
	}
	data, err := loadLifecycleSource(tx, kb, source.DocumentID)
	if err != nil {
		return nil, "", err
	}
	if data.head.ID != source.ID || !source.IsCurrent {
		return nil, "source_changed", nil
	}
	if data.state.Version != req.ExpectedLifecycleVersion {
		return nil, "lifecycle_changed", nil
	}
	if data.state.State != "active" {
		return nil, "source_archived", nil
	}
	filename := source.OriginalFilename
	if source.RepresentationKind == "text" {
		filename = path.Base(source.RelativePath)
	}
	profile, err := parsedLocalProfile(string(utility.GetFileType(filename)))
	if err != nil || !profile.RuntimeReady || profile.ProducerSHA256 != req.ExpectedProducerSHA256 {
		return nil, "producer_changed", nil
	}
	return &parsedInputs{Source: contentSource(source), NativeFileID: source.FileID, CommitID: source.CommitID, FolderID: source.FolderID, ByteLength: source.ByteLength, SourceInputSHA256: source.InputHash, RelativePath: source.RelativePath, Classification: entity.KnowledgeDirectoryClassification{GroupID: source.GroupID, ComponentSide: source.ComponentSide, Language: source.Language}, Filename: filename, Profile: *profile, LifecycleVersion: data.state.Version, FoundationPolicyVersion: kb.KnowledgePolicyVersion, ContentPolicyVersion: policy.Version}, "", nil
}

func (d *DatasetService) CreateKnowledgeParsedTask(ctx context.Context, owner, domain string, req entity.KnowledgeParsedTaskRequest) (*entity.KnowledgeParsedTask, bool, error) {
	if req.Source.Validate(domain) != nil || !structureVersion(req.ExpectedLifecycleVersion) || !structureVersion(req.ExpectedPolicyVersion) || !structureVersion(req.ExpectedContentPolicyVersion) || !isKnowledgeID(req.ExpectedProducerSHA256, 64) || req.MaxUnitBytes < 8 || req.MaxUnitBytes > 8192 || !req.Confirmed || !structureKey(req.IdempotencyKey) {
		return nil, false, documentError(400, "invalid_request")
	}
	digest, err := parsedTaskDigest(owner, domain, req)
	if err != nil {
		return nil, false, err
	}
	var value *entity.KnowledgeParsedTask
	var rejected error
	created := false
	err = d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		// Exact source authority is required for replays and durable rejections too.
		if _, err := parsedSourceRow(tx, kb, req.Source); err != nil {
			return err
		}
		op, err := parsedFindOperation(tx, owner, domain, "parsed_task", req.IdempotencyKey, digest)
		if err != nil {
			return err
		}
		if op != nil {
			if op.OutcomeKind == "rejection" {
				rejected = parsedReplayError(op)
				return nil
			}
			if op.OutcomeKind != "task" {
				return documentError(409, "content_integrity")
			}
			_, value, err = parsedLoadTask(tx, owner, domain, op.TaskID)
			return err
		}
		fixed, code, err := parsedCapture(tx, kb, req)
		if err != nil {
			return err
		}
		if code == "" {
			full, err := structureTaskCapacity(tx, domain)
			if err != nil {
				return err
			}
			if full {
				code = "capacity_exceeded"
			}
		}
		if code != "" {
			rejected, err = parsedReject(tx, owner, domain, "parsed_task", req.IdempotencyKey, digest, code, now)
			return err
		}
		value = &entity.KnowledgeParsedTask{TaskID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, OriginalRequest: req, RequestSHA256: digest, ProducerSHA256: fixed.Profile.ProducerSHA256, Revision: 1, State: "queued", Phase: "queued", CreatedAt: structureTimestamp(now), UpdatedAt: structureTimestamp(now), DeadlineAt: structureTimestamp(now.Add(120 * time.Second))}
		payload, err := structureJSON(value)
		if err != nil {
			return err
		}
		inputs, err := structureJSON(fixed)
		if err != nil {
			return err
		}
		row := entity.KnowledgeStructureTaskRecord{ID: value.TaskID, Protocol: entity.KnowledgeParsedProtocol, TargetKind: "source_revision", TargetID: req.Source.RevisionID, OwnerID: owner, DomainID: domain, RequestSHA256: digest, ProducerSHA256: fixed.Profile.ProducerSHA256, Revision: 1, State: "queued", Payload: payload, Inputs: inputs, CreatedAt: now, UpdatedAt: now, DeadlineAt: now.Add(120 * time.Second)}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := parsedSaveOperation(tx, owner, domain, "parsed_task", req.IdempotencyKey, digest, row.ID, "task", 201, value, now); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err == nil {
		err = rejected
	}
	return value, created, err
}

func parsedLoadTask(tx *gorm.DB, owner, domain, id string) (*entity.KnowledgeStructureTaskRecord, *entity.KnowledgeParsedTask, error) {
	var row entity.KnowledgeStructureTaskRecord
	if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", id, owner, domain, entity.KnowledgeParsedProtocol).Take(&row).Error; err != nil {
		return nil, nil, err
	}
	var task entity.KnowledgeParsedTask
	if entity.KnowledgeParsedDecode([]byte(row.Payload), &task) != nil || task.TaskID != row.ID || task.OwnerID != owner || task.DomainID != domain || row.TargetKind != "source_revision" || row.TargetID != task.OriginalRequest.Source.RevisionID || task.State != row.State || task.Revision != row.Revision || task.AttemptCount != row.AttemptCount || task.RequestSHA256 != row.RequestSHA256 || task.ProducerSHA256 != row.ProducerSHA256 {
		return nil, nil, documentError(409, "content_integrity")
	}
	return &row, &task, nil
}

func parsedSaveTask(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeParsedTask, now time.Time) error {
	if structureTerminal(row.State) || row.Revision >= entity.KnowledgeMaxLifecycleVersion {
		return documentError(409, "content_integrity")
	}
	before := row.Revision
	task.Revision, task.UpdatedAt = before+1, structureTimestamp(now)
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
	result := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("id = ? AND protocol = ? AND owner_id = ? AND domain_id = ? AND revision = ? AND state = ?", row.ID, entity.KnowledgeParsedProtocol, row.OwnerID, row.DomainID, before, row.State).Updates(map[string]any{"revision": task.Revision, "state": task.State, "attempt_count": task.AttemptCount, "payload": payload, "updated_at": now, "completed_at": completed})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return documentError(409, "content_integrity")
	}
	row.Revision, row.State, row.AttemptCount, row.Payload, row.UpdatedAt, row.CompletedAt = task.Revision, task.State, task.AttemptCount, payload, now, completed
	return nil
}

func (d *DatasetService) GetKnowledgeParsedTask(ctx context.Context, owner, domain, id string) (*entity.KnowledgeParsedTask, error) {
	if !isKnowledgeID(id, 32) {
		return nil, documentError(404, "not_found")
	}
	var value *entity.KnowledgeParsedTask
	err := d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, _ time.Time) error {
		_, task, err := parsedLoadTask(tx, owner, domain, id)
		if err != nil {
			return err
		}
		if _, err := parsedSourceRow(tx, kb, task.OriginalRequest.Source); err != nil {
			return err
		}
		value = task
		return nil
	})
	return value, err
}

func (d *DatasetService) CancelKnowledgeParsedTask(ctx context.Context, owner, domain, id string, req entity.KnowledgeParsedCancelRequest) (*entity.KnowledgeParsedTask, bool, error) {
	if !isKnowledgeID(id, 32) || !structureVersion(req.ExpectedTaskRevision) || !structureKey(req.IdempotencyKey) {
		return nil, false, documentError(400, "invalid_request")
	}
	digest, err := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "parsed-cancel/0.2.0", "task_id": id, "request": req})
	if err != nil {
		return nil, false, err
	}
	var value *entity.KnowledgeParsedTask
	created := false
	err = d.parsedTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, task, err := parsedLoadTask(tx, owner, domain, id)
		if err != nil {
			return err
		}
		if _, err := parsedSourceRow(tx, kb, task.OriginalRequest.Source); err != nil {
			return err
		}
		op, err := parsedFindOperation(tx, owner, domain, "parsed_cancel", req.IdempotencyKey, digest)
		if err != nil {
			return err
		}
		if op != nil {
			value = &entity.KnowledgeParsedTask{}
			if op.TaskID != id || op.OutcomeKind != "cancel" || json.Unmarshal([]byte(op.Payload), value) != nil {
				return documentError(409, "content_integrity")
			}
			return nil
		}
		if task.Revision != req.ExpectedTaskRevision {
			return documentError(409, "task_changed")
		}
		if !structureTerminal(task.State) && task.State != "cancel_requested" {
			if task.State == "queued" {
				task.State = "cancelled"
			} else {
				task.State, task.Phase = "cancel_requested", "cleaning"
			}
			if err := parsedSaveTask(tx, row, task, now); err != nil {
				return err
			}
		}
		if err := parsedSaveOperation(tx, owner, domain, "parsed_cancel", req.IdempotencyKey, digest, id, "cancel", 200, task, now); err != nil {
			return err
		}
		value, created = task, true
		return nil
	})
	return value, created, err
}
