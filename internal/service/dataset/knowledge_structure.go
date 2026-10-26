package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// StructureOperationError describes only this attempt. A durable rejection is
// identified separately; callers must recover the original key after unknowns.
type StructureOperationError struct {
	Status       int
	Code         string
	Message      string
	OutcomeKnown bool
	RejectionID  *string
}

func (e *StructureOperationError) Error() string { return e.Message }

func structureError(status int, code string) error {
	return &StructureOperationError{Status: status, Code: code, Message: code, OutcomeKnown: status != 503}
}

func structureNormalize(err error) error {
	if err == nil {
		return nil
	}
	var own *StructureOperationError
	if errors.As(err, &own) {
		return err
	}
	var old *KnowledgeError
	if errors.As(err, &old) && old.Status == 404 || errors.Is(err, gorm.ErrRecordNotFound) {
		return structureError(404, "not_found")
	}
	if old != nil && old.Code == "lifecycle_integrity" {
		return structureError(409, "lifecycle_integrity")
	}
	var directory *KnowledgeDirectoryError
	if errors.As(err, &directory) && directory.Status == 404 {
		return structureError(404, "not_found")
	}
	if directory != nil && directory.Reason != nil && *directory.Reason == "observation_mismatch" {
		return structureError(409, "structure_integrity")
	}
	return structureError(503, "unavailable")
}

func structureTimestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func structureVersion(v int64) bool { return v >= 1 && v <= entity.KnowledgeMaxLifecycleVersion }

func structureKey(key string) bool { return lifecycleKeyPattern.MatchString(key) }

var structureAliasPrefix = regexp.MustCompile(`^[A-Za-z@~_][A-Za-z0-9@~_./-]{0,62}/$`)

func structureValidateAliases(aliases []entity.KnowledgeStructureAlias) bool {
	if aliases == nil || len(aliases) > 8 {
		return false
	}
	for i, a := range aliases {
		if !structureAliasPrefix.MatchString(a.Prefix) || len(a.Prefix) > 64 || !structureAliasTarget(a.Target) {
			return false
		}
		for j := 0; j < i; j++ {
			if strings.HasPrefix(a.Prefix, aliases[j].Prefix) || strings.HasPrefix(aliases[j].Prefix, a.Prefix) {
				return false
			}
		}
	}
	return true
}

func structureAliasTarget(value string) bool {
	if value == "" {
		return true
	}
	if !utf8.ValidString(value) || len(value) > 512 || strings.HasPrefix(value, "/") || !strings.HasSuffix(value, "/") || strings.ContainsAny(value, "\\:") {
		return false
	}
	for _, character := range value {
		if character <= 31 || character == 127 {
			return false
		}
	}
	for _, part := range strings.Split(strings.TrimSuffix(value, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func structureValidateTask(req entity.KnowledgeStructureTaskRequest) bool {
	if !isKnowledgeID(req.ProjectID, 32) || !isKnowledgeID(req.ManifestID, 32) || req.ExpectedActiveViewID != nil && !isKnowledgeID(*req.ExpectedActiveViewID, 64) || !structureVersion(req.ExpectedPolicyVersion) || !structureVersion(req.ExpectedStructurePolicyVersion) || !req.Confirmed || !structureKey(req.IdempotencyKey) || len(req.Members) < 1 || len(req.Members) > 32 || !structureValidateAliases(req.Aliases) {
		return false
	}
	seen := map[string]bool{}
	for _, member := range req.Members {
		if !isKnowledgeID(member.SourceID, 32) || !isKnowledgeID(member.SourceRevisionID, 32) || !structureVersion(member.LifecycleVersion) || !slices.Contains([]string{"typescript", "tsx", "go", "markdown"}, member.Parser) || seen[member.SourceID] {
			return false
		}
		seen[member.SourceID] = true
	}
	return true
}

func structureTaskDigest(owner, domain string, req entity.KnowledgeStructureTaskRequest) (string, error) {
	members := slices.Clone(req.Members)
	slices.SortFunc(members, func(a, b entity.KnowledgeStructureMember) int { return strings.Compare(a.SourceID, b.SourceID) })
	aliases := slices.Clone(req.Aliases)
	if aliases == nil {
		aliases = []entity.KnowledgeStructureAlias{}
	}
	slices.SortFunc(aliases, func(a, b entity.KnowledgeStructureAlias) int { return strings.Compare(a.Prefix, b.Prefix) })
	return entity.KnowledgeLifecycleDigest(map[string]any{
		"kind": "knowledge-structure-task/0.1.0", "owner_id": owner, "domain_id": domain,
		"project_id": req.ProjectID, "manifest_id": req.ManifestID,
		"expected_active_view_id": req.ExpectedActiveViewID, "expected_policy_version": req.ExpectedPolicyVersion,
		"expected_structure_policy_version": req.ExpectedStructurePolicyVersion,
		"members":                           members, "aliases": aliases, "confirmed": req.Confirmed,
	})
}

func structurePolicyDigest(owner, domain string, req entity.KnowledgeStructurePolicyChangeRequest) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-policy/0.1.0", "owner_id": owner, "domain_id": domain, "expected_policy_version": req.ExpectedPolicyVersion, "expected_structure_policy_version": req.ExpectedStructurePolicyVersion, "enabled": req.Enabled, "confirmed": req.Confirmed})
}

func structureCancelDigest(owner, domain, taskID string, req entity.KnowledgeStructureCancelRequest) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-cancel/0.1.0", "owner_id": owner, "domain_id": domain, "task_id": taskID, "expected_task_revision": req.ExpectedTaskRevision, "confirmed": req.Confirmed})
}

func structureOperationID(owner, domain, kind, key string) string {
	id, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-structure-operation-scope/0.1.0", "owner_id": owner, "domain_id": domain, "operation": kind, "idempotency_key": key})
	return id
}

func structureJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

func structureDecode[T any](payload string) (*T, error) {
	var value T
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return nil, structureError(409, "structure_integrity")
	}
	return &value, nil
}

func structurePolicyRecord(tx *gorm.DB, owner, domain string) (*entity.KnowledgeStructurePolicyRecord, error) {
	var row entity.KnowledgeStructurePolicyRecord
	err := tx.Where("domain_id = ?", domain).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &entity.KnowledgeStructurePolicyRecord{DomainID: domain, OwnerID: owner, Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	if row.OwnerID != owner || !structureVersion(row.Version) {
		return nil, structureError(409, "structure_integrity")
	}
	return &row, nil
}

func structurePolicy(kb *entity.Knowledgebase, row *entity.KnowledgeStructurePolicyRecord) *entity.KnowledgeStructurePolicy {
	return &entity.KnowledgeStructurePolicy{DomainID: kb.ID, FoundationPolicyVersion: kb.KnowledgePolicyVersion, StructurePolicyVersion: row.Version, Enabled: row.Enabled, Mode: "structure_only", AllowedModelUses: []string{}, LexicalIndex: "not_created", VectorIndex: "not_created"}
}

func (d *DatasetService) KnowledgeStructurePolicy(ctx context.Context, owner, domain string) (*entity.KnowledgeStructurePolicy, error) {
	kb, err := d.knowledgeBase(ctx, owner, domain, true)
	if err != nil {
		return nil, structureNormalize(err)
	}
	row, err := structurePolicyRecord(dao.DB.WithContext(ctx), owner, domain)
	if err != nil {
		return nil, structureNormalize(err)
	}
	return structurePolicy(kb, row), nil
}

// structureTransaction rechecks current owner membership under the domain lock
// before original-key lookup or any observable mutation.
func (d *DatasetService) structureTransaction(ctx context.Context, owner, domain string, fn func(*gorm.DB, *entity.Knowledgebase, time.Time) error) error {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return structureNormalize(err)
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
	return structureNormalize(err)
}

func structureFindOperation(tx *gorm.DB, owner, domain, kind, key string) (*entity.KnowledgeStructureOperationRecord, error) {
	var row entity.KnowledgeStructureOperationRecord
	err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", structureOperationID(owner, domain, kind, key), owner, domain, kind).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.Key != key {
		return nil, structureError(409, "structure_integrity")
	}
	return &row, nil
}

func structureOperationValue(tx *gorm.DB, row *entity.KnowledgeStructureOperationRecord) (any, error) {
	switch row.OutcomeKind {
	case "rejection":
		return structureDecode[entity.KnowledgeStructureRejectionReceipt](row.Payload)
	case "policy":
		return structureDecode[entity.KnowledgeStructurePolicyReceipt](row.Payload)
	case "cancel":
		return structureDecode[entity.KnowledgeStructureCancelReceipt](row.Payload)
	case "task":
		_, task, err := structureLoadTask(tx, row.OwnerID, row.DomainID, row.TaskID)
		return task, err
	default:
		return nil, structureError(409, "structure_integrity")
	}
}

func structureReplay(tx *gorm.DB, row *entity.KnowledgeStructureOperationRecord, digest, target string) (any, error) {
	if row.RequestSHA256 != digest || row.Kind == "cancel" && row.TaskID != target {
		return nil, structureError(409, "idempotency_conflict")
	}
	value, err := structureOperationValue(tx, row)
	if err != nil {
		return nil, err
	}
	if rejected, ok := value.(*entity.KnowledgeStructureRejectionReceipt); ok {
		return nil, &StructureOperationError{Status: row.Status, Code: rejected.Reason, Message: rejected.Reason, OutcomeKnown: true, RejectionID: &rejected.RejectionID}
	}
	return value, nil
}

func structureSaveOperation(tx *gorm.DB, owner, domain, kind, key, taskID, digest, outcome string, status int, value any, now time.Time) error {
	payload, err := structureJSON(value)
	if err != nil {
		return err
	}
	return tx.Create(&entity.KnowledgeStructureOperationRecord{ID: structureOperationID(owner, domain, kind, key), OwnerID: owner, DomainID: domain, Kind: kind, Key: key, TaskID: taskID, RequestSHA256: digest, Status: status, OutcomeKind: outcome, Payload: payload, CreatedAt: now}).Error
}

// structureReject is called from a domain-locked transaction. The caller keeps
// this error out of the transaction return so the immutable receipt commits.
func structureReject(tx *gorm.DB, owner, domain, kind, key, taskID, digest, reason string, original any, now time.Time) (*StructureOperationError, error) {
	body, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var target *string
	if taskID != "" {
		target = &taskID
	}
	rejection := &entity.KnowledgeStructureRejectionReceipt{RejectionID: common.GenerateUUID(), Operation: kind, OwnerID: owner, DomainID: domain, TaskID: target, IdempotencyKey: key, OriginalRequest: body, RequestSHA256: digest, Reason: reason, RecordedAt: structureTimestamp(now)}
	status := 409
	if reason == "capacity_exceeded" {
		status = 429
	}
	if err := structureSaveOperation(tx, owner, domain, kind, key, taskID, digest, "rejection", status, rejection, now); err != nil {
		return nil, err
	}
	return &StructureOperationError{Status: status, Code: reason, Message: reason, OutcomeKnown: true, RejectionID: &rejection.RejectionID}, nil
}

func (d *DatasetService) ChangeKnowledgeStructurePolicy(ctx context.Context, owner, domain string, req entity.KnowledgeStructurePolicyChangeRequest) (any, bool, error) {
	if !structureKey(req.IdempotencyKey) || !req.Confirmed || !structureVersion(req.ExpectedPolicyVersion) || !structureVersion(req.ExpectedStructurePolicyVersion) {
		return nil, false, structureError(400, "invalid_request")
	}
	digest, err := structurePolicyDigest(owner, domain, req)
	if err != nil {
		return nil, false, structureError(400, "invalid_request")
	}
	var value any
	var rejected *StructureOperationError
	created := false
	err = d.structureTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		existing, err := structureFindOperation(tx, owner, domain, "policy", req.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			value, err = structureReplay(tx, existing, digest, "")
			return err
		}
		row, err := structurePolicyRecord(tx, owner, domain)
		if err != nil {
			return err
		}
		if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion || row.Version != req.ExpectedStructurePolicyVersion || row.Enabled != req.Enabled && row.Version == entity.KnowledgeMaxLifecycleVersion {
			rejected, err = structureReject(tx, owner, domain, "policy", req.IdempotencyKey, "", digest, "policy_changed", req, now)
			return err
		}
		before := entity.KnowledgeStructurePolicySnapshot{StructurePolicyVersion: row.Version, Enabled: row.Enabled}
		outcome := "unchanged"
		if row.Enabled != req.Enabled {
			row.Version++
			row.Enabled = req.Enabled
			outcome = "applied"
		}
		row.UpdatedAt = now
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		receipt := &entity.KnowledgeStructurePolicyReceipt{CommandID: common.GenerateUUID(), Kind: "policy", DomainID: domain, OwnerID: owner, IdempotencyKey: req.IdempotencyKey, OriginalRequest: req, RequestSHA256: digest, Before: before, After: entity.KnowledgeStructurePolicySnapshot{StructurePolicyVersion: row.Version, Enabled: row.Enabled}, Outcome: outcome, CompletedAt: structureTimestamp(now)}
		if err := structureSaveOperation(tx, owner, domain, "policy", req.IdempotencyKey, "", digest, "policy", 201, receipt, now); err != nil {
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

type structureInputs struct {
	ManifestID              string                                `json:"manifest_id"`
	ManifestSHA256          string                                `json:"manifest_sha256"`
	FoundationPolicyVersion int64                                 `json:"foundation_policy_version"`
	StructurePolicyVersion  int64                                 `json:"structure_policy_version"`
	ExpectedActiveViewID    *string                               `json:"expected_active_view_id"`
	Members                 []entity.KnowledgeStructureViewMember `json:"members"`
	Aliases                 []entity.KnowledgeStructureAlias      `json:"aliases"`
}

func structureActiveHead(tx *gorm.DB, owner, domain, project string) (*string, error) {
	var row entity.KnowledgeStructureHeadRecord
	err := tx.Where("project_id = ? AND domain_id = ? AND owner_id = ?", project, domain, owner).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row.ViewID, nil
}

func structureParserMatches(parser, relative string) bool {
	switch parser {
	case "typescript":
		return path.Ext(relative) == ".ts"
	case "tsx":
		return path.Ext(relative) == ".tsx"
	case "go":
		return path.Ext(relative) == ".go"
	case "markdown":
		return path.Ext(relative) == ".md" || path.Ext(relative) == ".markdown"
	}
	return false
}

// structureCapture reads metadata only for explicitly selected entries. It
// neither opens unselected source objects nor changes fixed manifest coverage.
func structureCapture(tx *gorm.DB, kb *entity.Knowledgebase, req entity.KnowledgeStructureTaskRequest) (*structureInputs, string, error) {
	policy, err := structurePolicyRecord(tx, kb.CreatedBy, kb.ID)
	if err != nil {
		return nil, "", err
	}
	if kb.KnowledgePolicyVersion != req.ExpectedPolicyVersion || policy.Version != req.ExpectedStructurePolicyVersion {
		return nil, "policy_changed", nil
	}
	if !policy.Enabled {
		return nil, "policy_disabled", nil
	}
	if err := knowledgeProject(tx, kb.ID, req.ProjectID); err != nil {
		return nil, "", err
	}
	head, err := projectManifestHead(tx, kb.ID, req.ProjectID)
	if err != nil {
		return nil, "", err
	}
	if head == nil || *head != req.ManifestID {
		return nil, "manifest_changed", nil
	}
	_, manifest, receipt, err := loadProjectManifest(tx, kb.ID, req.ProjectID, req.ManifestID)
	if err != nil {
		return nil, "", err
	}
	active, err := structureActiveHead(tx, kb.CreatedBy, kb.ID, req.ProjectID)
	if err != nil {
		return nil, "", err
	}
	if !sameOptionalID(active, req.ExpectedActiveViewID) {
		return nil, "view_changed", nil
	}
	bySource := map[string]entity.KnowledgeProjectManifestEntry{}
	for _, entry := range manifest.Items {
		if _, duplicate := bySource[entry.SourceID]; duplicate {
			return nil, "", structureError(409, "structure_integrity")
		}
		bySource[entry.SourceID] = entry
	}
	inputs := &structureInputs{ManifestID: req.ManifestID, ManifestSHA256: receipt.ManifestSHA256, FoundationPolicyVersion: req.ExpectedPolicyVersion, StructurePolicyVersion: req.ExpectedStructurePolicyVersion, ExpectedActiveViewID: req.ExpectedActiveViewID, Members: []entity.KnowledgeStructureViewMember{}, Aliases: slices.Clone(req.Aliases)}
	var total int64
	for _, selected := range req.Members {
		entry, ok := bySource[selected.SourceID]
		if !ok || entry.SourceRevisionID != selected.SourceRevisionID {
			return nil, "manifest_changed", nil
		}
		current, err := loadLifecycleSource(tx, kb, selected.SourceID)
		if err != nil {
			return nil, "", err
		}
		if current.state.State != "active" || current.state.Version != selected.LifecycleVersion || current.head.ID != selected.SourceRevisionID {
			return nil, "revision_changed", nil
		}
		if current.head.RepresentationKind != "text" || current.registration == nil || current.registration.SourceKind != "text_file" && current.registration.SourceKind != "controlled_directory" || !structureParserMatches(selected.Parser, entry.RelativePath) {
			return nil, "unsupported_source", nil
		}
		classification := entity.KnowledgeStructureClassification{GroupID: current.head.GroupID, ComponentSide: current.head.ComponentSide, Language: current.head.Language}
		expectedClass := entity.KnowledgeStructureClassification{GroupID: entry.Classification.GroupID, ComponentSide: entry.Classification.ComponentSide, Language: entry.Classification.Language}
		if current.head.RelativePath != entry.RelativePath || current.head.SHA256 != entry.SHA256 || current.head.ByteLength != entry.ByteLength || directoryJSON(classification) != directoryJSON(expectedClass) || knowledgeRoot(current.head).ID != entry.RootNodeID {
			return nil, "revision_changed", nil
		}
		var root entity.KnowledgeNode
		if err := tx.Where("id = ? AND domain_id = ? AND source_revision_id = ? AND kind = ?", entry.RootNodeID, kb.ID, selected.SourceRevisionID, "file").Take(&root).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, "unsupported_source", nil
			}
			return nil, "", err
		}
		if current.head.ByteLength < 0 || current.head.ByteLength > 2<<20 {
			return nil, "unsupported_source", nil
		}
		total += current.head.ByteLength
		if total > 16<<20 {
			return nil, "unsupported_source", nil
		}
		inputs.Members = append(inputs.Members, entity.KnowledgeStructureViewMember{SourceID: selected.SourceID, SourceRevisionID: selected.SourceRevisionID, SourceNodeID: root.ID, LifecycleVersion: selected.LifecycleVersion, Parser: selected.Parser, SHA256: entry.SHA256, ByteLength: entry.ByteLength, RelativePath: entry.RelativePath, Classification: classification})
	}
	slices.SortFunc(inputs.Members, func(a, b entity.KnowledgeStructureViewMember) int { return strings.Compare(a.SourceID, b.SourceID) })
	slices.SortFunc(inputs.Aliases, func(a, b entity.KnowledgeStructureAlias) int { return strings.Compare(a.Prefix, b.Prefix) })
	return inputs, "", nil
}

func (d *DatasetService) CreateKnowledgeStructureTask(ctx context.Context, owner, domain string, req entity.KnowledgeStructureTaskRequest) (any, bool, error) {
	if !structureValidateTask(req) {
		return nil, false, structureError(400, "invalid_request")
	}
	digest, err := structureTaskDigest(owner, domain, req)
	if err != nil {
		return nil, false, structureError(400, "invalid_request")
	}
	producer, err := entity.KnowledgeStructureProducerDigest(req.Aliases)
	if err != nil {
		return nil, false, structureError(400, "invalid_request")
	}
	var value any
	var rejected *StructureOperationError
	created := false
	err = d.structureTransaction(ctx, owner, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		existing, err := structureFindOperation(tx, owner, domain, "task", req.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			value, err = structureReplay(tx, existing, digest, "")
			return err
		}
		if !entity.KnowledgeStructureProducerCompatible() {
			return structureError(503, "unavailable")
		}
		inputs, reason, err := structureCapture(tx, kb, req)
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
			rejected, err = structureReject(tx, owner, domain, "task", req.IdempotencyKey, "", digest, reason, req, now)
			return err
		}
		task := &entity.KnowledgeStructureTask{TaskID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, ProjectID: req.ProjectID, OriginalRequest: req, RequestSHA256: digest, ProducerSHA256: producer, Revision: 1, State: "queued", CreatedAt: structureTimestamp(now), UpdatedAt: structureTimestamp(now), DeadlineAt: structureTimestamp(now.Add(120 * time.Second)), TotalMembers: int64(len(req.Members)), Members: []entity.KnowledgeStructureTaskMember{}}
		for _, member := range req.Members {
			task.Members = append(task.Members, entity.KnowledgeStructureTaskMember{SourceID: member.SourceID, SourceRevisionID: member.SourceRevisionID, LifecycleVersion: member.LifecycleVersion, Parser: member.Parser, State: "pending", Diagnostics: []entity.KnowledgeStructureDiagnostic{}})
		}
		payload, err := structureJSON(task)
		if err != nil {
			return err
		}
		fixedInputs, err := structureJSON(inputs)
		if err != nil {
			return err
		}
		row := entity.KnowledgeStructureTaskRecord{ID: task.TaskID, OwnerID: owner, DomainID: domain, ProjectID: req.ProjectID, RequestSHA256: digest, ProducerSHA256: producer, Revision: 1, State: task.State, Payload: payload, Inputs: fixedInputs, CreatedAt: now, UpdatedAt: now, DeadlineAt: now.Add(120 * time.Second)}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := structureSaveOperation(tx, owner, domain, "task", req.IdempotencyKey, task.TaskID, digest, "task", 201, task, now); err != nil {
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

func structureLoadTask(tx *gorm.DB, owner, domain, id string) (*entity.KnowledgeStructureTaskRecord, *entity.KnowledgeStructureTask, error) {
	var row entity.KnowledgeStructureTaskRecord
	if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol IN ?", id, owner, domain, []string{"", "code_structure/0.1.0"}).Take(&row).Error; err != nil {
		return nil, nil, err
	}
	task, err := structureDecode[entity.KnowledgeStructureTask](row.Payload)
	if err != nil {
		return nil, nil, err
	}
	if task.TaskID != row.ID || task.OwnerID != owner || task.DomainID != domain || task.State != row.State || task.Revision != row.Revision || task.AttemptCount != row.AttemptCount || task.RequestSHA256 != row.RequestSHA256 || task.ProducerSHA256 != row.ProducerSHA256 {
		return nil, nil, structureError(409, "structure_integrity")
	}
	return &row, task, nil
}

func (d *DatasetService) GetKnowledgeStructureTask(ctx context.Context, owner, domain, id string) (any, error) {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return nil, structureNormalize(err)
	}
	if !isKnowledgeID(id, 32) {
		return nil, structureError(404, "not_found")
	}
	_, task, err := structureLoadTask(dao.DB.WithContext(ctx), owner, domain, id)
	return task, structureNormalize(err)
}

func (d *DatasetService) GetKnowledgeStructureTaskByKey(ctx context.Context, owner, domain, key string) (any, error) {
	return d.structureGetByKey(ctx, owner, domain, "task", key)
}

func (d *DatasetService) GetKnowledgeStructureCommandByKey(ctx context.Context, owner, domain, kind, key string) (any, error) {
	if kind != "policy" && kind != "cancel" {
		return nil, structureError(400, "invalid_request")
	}
	return d.structureGetByKey(ctx, owner, domain, kind, key)
}

func (d *DatasetService) structureGetByKey(ctx context.Context, owner, domain, kind, key string) (any, error) {
	if _, err := d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return nil, structureNormalize(err)
	}
	if !structureKey(key) {
		return nil, structureError(400, "invalid_request")
	}
	tx := dao.DB.WithContext(ctx)
	row, err := structureFindOperation(tx, owner, domain, kind, key)
	if err != nil {
		return nil, structureNormalize(err)
	}
	if row == nil {
		return nil, structureError(404, "not_found")
	}
	value, err := structureOperationValue(tx, row)
	return value, structureNormalize(err)
}

func (d *DatasetService) CancelKnowledgeStructureTask(ctx context.Context, owner, domain, taskID string, req entity.KnowledgeStructureCancelRequest) (any, bool, error) {
	if !isKnowledgeID(taskID, 32) || !structureVersion(req.ExpectedTaskRevision) || !req.Confirmed || !structureKey(req.IdempotencyKey) {
		return nil, false, structureError(400, "invalid_request")
	}
	digest, err := structureCancelDigest(owner, domain, taskID, req)
	if err != nil {
		return nil, false, structureError(400, "invalid_request")
	}
	var value any
	var rejected *StructureOperationError
	created := false
	err = d.structureTransaction(ctx, owner, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		existing, err := structureFindOperation(tx, owner, domain, "cancel", req.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			value, err = structureReplay(tx, existing, digest, taskID)
			return err
		}
		row, task, err := structureLoadTask(tx, owner, domain, taskID)
		if err != nil {
			return err
		}
		if task.Revision != req.ExpectedTaskRevision || !structureTerminal(task.State) && task.State != "cancel_requested" && task.Revision == entity.KnowledgeMaxLifecycleVersion {
			rejected, err = structureReject(tx, owner, domain, "cancel", req.IdempotencyKey, taskID, digest, "task_revision_changed", req, now)
			return err
		}
		before, outcome := task.State, "unchanged"
		if !structureTerminal(task.State) && task.State != "cancel_requested" {
			task.State = "cancel_requested"
			if err := structureSaveTask(tx, row, task, now); err != nil {
				return err
			}
			outcome = "requested"
		}
		receipt := &entity.KnowledgeStructureCancelReceipt{CommandID: common.GenerateUUID(), Kind: "cancel", DomainID: domain, OwnerID: owner, TaskID: taskID, IdempotencyKey: req.IdempotencyKey, OriginalRequest: req, RequestSHA256: digest, BeforeState: before, AfterState: task.State, TaskRevision: task.Revision, Outcome: outcome, CompletedAt: structureTimestamp(now)}
		if err := structureSaveOperation(tx, owner, domain, "cancel", req.IdempotencyKey, taskID, digest, "cancel", 201, receipt, now); err != nil {
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

func structureTerminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}

// structureSaveTask is always called under the domain lock. The row revision
// fences stale in-process snapshots; heartbeat updates do not use this method.
func structureSaveTask(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeStructureTask, now time.Time) error {
	if structureTerminal(row.State) || row.Revision >= entity.KnowledgeMaxLifecycleVersion {
		return structureError(409, "structure_integrity")
	}
	before := row.Revision
	task.Revision = before + 1
	task.UpdatedAt = structureTimestamp(now)
	task.FinishedMembers = 0
	for _, member := range task.Members {
		if member.State != "pending" && member.State != "processing" {
			task.FinishedMembers++
		}
	}
	var completed *time.Time
	if structureTerminal(task.State) {
		task.CompletedAt = new(string)
		*task.CompletedAt = structureTimestamp(now)
		completed = &now
	}
	payload, err := structureJSON(task)
	if err != nil {
		return err
	}
	result := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("id = ? AND domain_id = ? AND owner_id = ? AND revision = ? AND state = ?", row.ID, row.DomainID, row.OwnerID, before, row.State).Updates(map[string]any{"revision": task.Revision, "state": task.State, "attempt_count": task.AttemptCount, "payload": payload, "updated_at": now, "completed_at": completed})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return structureError(409, "structure_integrity")
	}
	row.Revision, row.State, row.AttemptCount, row.Payload, row.UpdatedAt, row.CompletedAt = task.Revision, task.State, task.AttemptCount, payload, now, completed
	return nil
}
