package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

var lifecycleKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type lifecycleSourceData struct {
	head         *entity.KnowledgeSourceRevision
	state        *entity.KnowledgeSourceLifecycle
	binding      *entity.KnowledgeSourceBinding
	registration *entity.KnowledgeSourceRegistration
	native       entity.File
}

func lifecycleNormalize(err error) error {
	if err == nil {
		return nil
	}
	var public *KnowledgeError
	if errors.As(err, &public) {
		switch public.Code {
		case "knowledge_not_found", "knowledge_invalid_request", "policy_changed", "revision_changed", "idempotency_conflict", "lifecycle_source_archived", "lifecycle_action_unsupported", "lifecycle_position_occupied", "lifecycle_version_exhausted", "lifecycle_integrity", "lifecycle_budget_exceeded", "lifecycle_result_unknown":
			return err
		case "knowledge_conflict":
			return lifecycleConflict("policy_changed")
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return knowledgeNotFound()
	}
	return knowledgeError(503, "lifecycle_result_unknown", "Lifecycle result is unknown; recover by the original action and key")
}

func (d *DatasetService) lifecycleTransaction(ctx context.Context, user, domain string, fn func(*gorm.DB, *entity.Knowledgebase) error) error {
	if _, err := d.knowledgeBase(ctx, user, domain, true); err != nil {
		return lifecycleNormalize(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return lifecycleNormalize(dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(ctx, tx, user, domain)
		if err != nil {
			return err
		}
		return fn(tx, kb)
	}))
}

func loadLifecycleSource(tx *gorm.DB, kb *entity.Knowledgebase, source string) (*lifecycleSourceData, error) {
	if !isKnowledgeID(source, 32) {
		return nil, knowledgeNotFound()
	}
	var document entity.Document
	if err := tx.Where("id = ? AND kb_id = ? AND status = ?", source, kb.ID, "1").Take(&document).Error; err != nil {
		return nil, err
	}
	head, err := lifecycleHead(tx, kb.ID, source)
	if err != nil {
		return nil, err
	}
	state, err := lifecycleState(tx, kb.ID, source)
	if err != nil {
		return nil, err
	}
	data := &lifecycleSourceData{head: head, state: state}
	if err := tx.Model(&entity.File{}).Joins("JOIN file2document ON file2document.file_id = file.id").
		Where("file.id = ? AND file.parent_id = ? AND file.tenant_id = ? AND file.source_type = ? AND file2document.document_id = ?", head.FileID, head.FolderID, kb.TenantID, "knowledgebase", source).Take(&data.native).Error; err != nil {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	var binding entity.KnowledgeSourceBinding
	err = tx.Where("document_id = ?", source).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if head.RepresentationKind != "text" {
			return nil, lifecycleConflict("lifecycle_integrity")
		}
		return data, nil
	}
	if err != nil {
		return nil, err
	}
	var reg entity.KnowledgeSourceRegistration
	if err := tx.Where("id = ? AND domain_id = ?", binding.RegistrationID, kb.ID).Take(&reg).Error; err != nil {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	if binding.FileID != head.FileID || binding.RelativePath != head.RelativePath {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	valid := reg.SourceKind == "text_file" && head.RepresentationKind == "text" || reg.SourceKind == "controlled_directory" && head.RepresentationKind == "text" || (reg.SourceKind == "original_document" || reg.SourceKind == "parsed_file" && reg.InputMode == "native_snapshot") && head.RepresentationKind == "original_only" || (reg.SourceKind == "web_page" || reg.SourceKind == "web_feed") && head.RepresentationKind == "web_capture"
	if !valid || head.RepresentationKind != "text" && head.OriginalRegistrationID != reg.ID {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	data.binding, data.registration = &binding, &reg
	return data, nil
}

// The existing lifecycle API remains scoped to its original source families.
// Parsed-source state uses the explicit unified-content adapter.
func loadKnowledgeLifecycleSource(tx *gorm.DB, kb *entity.Knowledgebase, source string) (*lifecycleSourceData, error) {
	data, err := loadLifecycleSource(tx, kb, source)
	if err != nil {
		return nil, err
	}
	if data.registration != nil && data.registration.SourceKind == "parsed_file" {
		return nil, knowledgeNotFound()
	}
	return data, nil
}

func lifecycleSnapshot(source *entity.KnowledgeSourceRevision, state *entity.KnowledgeSourceLifecycle) entity.KnowledgeLifecycleSnapshot {
	return entity.KnowledgeLifecycleSnapshot{SourceRevisionID: source.ID, SourceInputSHA256: source.InputHash, LifecycleVersion: state.Version, State: state.State, RelativePath: source.RelativePath, Classification: entity.KnowledgeDirectoryClassification{GroupID: source.GroupID, ComponentSide: source.ComponentSide, Language: source.Language}, SHA256: source.SHA256, ByteLength: source.ByteLength}
}

func lifecycleCanMove(data *lifecycleSourceData) bool {
	return data.registration != nil && (data.registration.SourceKind == "text_file" || data.registration.SourceKind == "original_document")
}

func lifecycleDetail(kb *entity.Knowledgebase, data *lifecycleSourceData) entity.KnowledgeLifecycleDetail {
	result := entity.KnowledgeLifecycleDetail{OwnerID: kb.CreatedBy, DomainID: kb.ID, SourceID: data.head.DocumentID, NativeFileID: data.head.FileID, RepresentationKind: data.head.RepresentationKind, PolicyVersion: kb.KnowledgePolicyVersion, Current: lifecycleSnapshot(data.head, data.state), Actions: map[string]entity.KnowledgeLifecycleActionCapability{}}
	if data.registration != nil {
		result.RegistrationID = &data.registration.ID
		result.SourceKind = &data.registration.SourceKind
	}
	for _, action := range []string{"reclassify", "move", "archive", "restore"} {
		capability := entity.KnowledgeLifecycleActionCapability{Supported: true, Allowed: true}
		if action == "move" && !lifecycleCanMove(data) {
			capability.Supported, capability.Allowed, capability.Reason = false, false, directoryString("action_unsupported")
		} else if data.state.State == "archived" && (action == "move" || action == "reclassify") {
			capability.Allowed, capability.Reason = false, directoryString("source_archived")
		}
		result.Actions[action] = capability
	}
	return result
}

func (d *DatasetService) GetKnowledgeSourceLifecycle(ctx context.Context, user, domain, source string) (*entity.KnowledgeLifecycleDetail, error) {
	var result *entity.KnowledgeLifecycleDetail
	err := d.lifecycleTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		data, err := loadKnowledgeLifecycleSource(tx, kb, source)
		if err != nil {
			return err
		}
		view := lifecycleDetail(kb, data)
		result = &view
		return nil
	})
	return result, err
}

func lifecycleCheckExpected(kb *entity.Knowledgebase, data *lifecycleSourceData, request entity.KnowledgeLifecycleCommand) error {
	if !isKnowledgeID(request.ExpectedSourceRevisionID, 32) || request.ExpectedLifecycleVersion < 1 || request.ExpectedLifecycleVersion > entity.KnowledgeMaxLifecycleVersion || request.ExpectedPolicyVersion < 1 || request.ExpectedPolicyVersion > entity.KnowledgeMaxLifecycleVersion {
		return knowledgeInvalid("Invalid expected versions")
	}
	if request.ExpectedPolicyVersion != kb.KnowledgePolicyVersion {
		return lifecycleConflict("policy_changed")
	}
	if request.ExpectedSourceRevisionID != data.head.ID || request.ExpectedLifecycleVersion != data.state.Version {
		return lifecycleConflict("revision_changed")
	}
	return nil
}

func lifecycleMoveReason(tx *gorm.DB, data *lifecycleSourceData, relative string) (*string, error) {
	if !lifecycleCanMove(data) {
		return directoryString("action_unsupported"), nil
	}
	if data.state.State == "archived" {
		return directoryString("source_archived"), nil
	}
	if !isKnowledgeImportPath(relative) || strings.ContainsAny(relative, "*?[]{}") || strings.ContainsFunc(relative, unicode.IsControl) {
		return directoryString("invalid_target"), nil
	}
	if !slices.Contains(data.registration.IncludePaths, relative) {
		return directoryString("target_not_registered"), nil
	}
	if data.head.RepresentationKind == "original_only" {
		if _, err := inspectKnowledgeOriginalNames(data.head.OriginalFilename, relative, data.head.OriginalMediaType); err != nil {
			return directoryString("target_media_mismatch"), nil
		}
	}
	var binding entity.KnowledgeSourceBinding
	err := tx.Where("registration_id = ? AND relative_path = ?", data.registration.ID, relative).Take(&binding).Error
	if err == nil && (binding.DocumentID != data.head.DocumentID || binding.RelativePath != relative) {
		return directoryString("target_occupied"), nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	retired, err := lifecycleRetired(tx, data.registration.ID, relative)
	if err != nil {
		return nil, err
	}
	if retired != nil && retired.SourceID != data.head.DocumentID {
		return directoryString("target_occupied"), nil
	}
	return nil, nil
}

func (d *DatasetService) CheckKnowledgeSourceMoveTarget(ctx context.Context, user, domain, source, relative string, expected entity.KnowledgeLifecycleCommand) (*entity.KnowledgeLifecycleTarget, error) {
	if !utf8.ValidString(relative) || utf8.RuneCountInString(relative) < 1 || utf8.RuneCountInString(relative) > 512 {
		return nil, knowledgeInvalid("Invalid target path")
	}
	var result *entity.KnowledgeLifecycleTarget
	err := d.lifecycleTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		data, err := loadKnowledgeLifecycleSource(tx, kb, source)
		if err != nil {
			return err
		}
		if err := lifecycleCheckExpected(kb, data, expected); err != nil {
			return err
		}
		reason, err := lifecycleMoveReason(tx, data, relative)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeLifecycleTarget{DomainID: domain, SourceID: source, CurrentSourceRevisionID: data.head.ID, LifecycleVersion: data.state.Version, PolicyVersion: kb.KnowledgePolicyVersion, RelativePath: relative, Allowed: reason == nil, Reason: reason}
		if data.registration != nil {
			result.RegistrationID = &data.registration.ID
		}
		return nil
	})
	return result, err
}

func lifecycleCommandDigest(user, domain, source, action string, request entity.KnowledgeLifecycleCommand, target any) (string, error) {
	return entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-lifecycle-command/0.7.0", "owner_id": user, "domain_id": domain, "source_id": source, "action": action, "expected_source_revision_id": request.ExpectedSourceRevisionID, "expected_lifecycle_version": request.ExpectedLifecycleVersion, "expected_policy_version": request.ExpectedPolicyVersion, "confirmed": request.Confirmed, "target": target})
}

func lifecycleCommandKey(user, domain, action, key string) string {
	return entity.KnowledgeID("knowledge-lifecycle-command-key/0.7.0", user, domain, action, key)
}

func loadLifecycleReceipt(tx *gorm.DB, user, domain, id, action, key string) (*entity.KnowledgeLifecycleCommandRecord, *entity.KnowledgeLifecycleReceipt, error) {
	query := tx.Where("owner_id = ? AND domain_id = ?", user, domain)
	if id != "" {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("key_sha256 = ?", lifecycleCommandKey(user, domain, action, key))
	}
	var row entity.KnowledgeLifecycleCommandRecord
	if err := query.Take(&row).Error; err != nil {
		return nil, nil, err
	}
	var receipt entity.KnowledgeLifecycleReceipt
	if json.Unmarshal([]byte(row.Payload), &receipt) != nil || receipt.CommandID != row.ID || receipt.OwnerID != user || receipt.DomainID != domain || receipt.SourceID != row.SourceID || receipt.Action != row.Action || receipt.IdempotencyKey != row.IdempotencyKey || receipt.RequestSHA256 != row.RequestSHA256 || row.KeySHA256 != lifecycleCommandKey(user, domain, row.Action, row.IdempotencyKey) || id == "" && (row.Action != action || row.IdempotencyKey != key) {
		return nil, nil, lifecycleConflict("lifecycle_integrity")
	}
	var request entity.KnowledgeLifecycleCommand
	var target any
	var canonical []byte
	switch receipt.Action {
	case "reclassify":
		var value entity.KnowledgeLifecycleReclassify
		if json.Unmarshal(receipt.Request, &value) != nil {
			return nil, nil, lifecycleConflict("lifecycle_integrity")
		}
		request, target = value.KnowledgeLifecycleCommand, value.Classification
		canonical, _ = json.Marshal(value)
	case "move":
		var value entity.KnowledgeLifecycleMove
		if json.Unmarshal(receipt.Request, &value) != nil {
			return nil, nil, lifecycleConflict("lifecycle_integrity")
		}
		request, target = value.KnowledgeLifecycleCommand, map[string]any{"relative_path": value.RelativePath}
		canonical, _ = json.Marshal(value)
	case "archive", "restore":
		if json.Unmarshal(receipt.Request, &request) != nil {
			return nil, nil, lifecycleConflict("lifecycle_integrity")
		}
		canonical, _ = json.Marshal(request)
	default:
		return nil, nil, lifecycleConflict("lifecycle_integrity")
	}
	digest, err := lifecycleCommandDigest(user, domain, receipt.SourceID, receipt.Action, request, target)
	if err != nil || digest != receipt.RequestSHA256 || request.IdempotencyKey != receipt.IdempotencyKey || string(canonical) != string(receipt.Request) {
		return nil, nil, lifecycleConflict("lifecycle_integrity")
	}
	var document entity.Document
	if err := tx.Where("id = ? AND kb_id = ? AND status = ?", receipt.SourceID, domain, "1").Take(&document).Error; err != nil {
		return nil, nil, err
	}
	return &row, &receipt, nil
}

func (d *DatasetService) GetKnowledgeLifecycleCommand(ctx context.Context, user, domain, id, action, key string) (*entity.KnowledgeLifecycleReceipt, error) {
	if id != "" && !isKnowledgeID(id, 32) || id == "" && (!slices.Contains([]string{"reclassify", "move", "archive", "restore"}, action) || !lifecycleKeyPattern.MatchString(key)) {
		return nil, knowledgeInvalid("Invalid command identity")
	}
	var result *entity.KnowledgeLifecycleReceipt
	err := d.lifecycleTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		_, receipt, err := loadLifecycleReceipt(tx, user, domain, id, action, key)
		result = receipt
		return err
	})
	return result, err
}

// CommandKnowledgeSourceLifecycle publishes metadata, the generation and a fixed
// receipt in one native transaction. Successful key replays never reapply actions.
func (d *DatasetService) CommandKnowledgeSourceLifecycle(ctx context.Context, user, domain, source, action string, command any) (*entity.KnowledgeLifecycleReceipt, bool, error) {
	var request entity.KnowledgeLifecycleCommand
	var target any
	switch value := command.(type) {
	case entity.KnowledgeLifecycleReclassify:
		if action != "reclassify" {
			return nil, false, knowledgeInvalid("Invalid action")
		}
		request, target = value.KnowledgeLifecycleCommand, value.Classification
	case entity.KnowledgeLifecycleMove:
		if action != "move" {
			return nil, false, knowledgeInvalid("Invalid action")
		}
		request, target = value.KnowledgeLifecycleCommand, map[string]any{"relative_path": value.RelativePath}
	case entity.KnowledgeLifecycleCommand:
		if action != "archive" && action != "restore" {
			return nil, false, knowledgeInvalid("Invalid action")
		}
		request = value
	default:
		return nil, false, knowledgeInvalid("Invalid command")
	}
	if !lifecycleKeyPattern.MatchString(request.IdempotencyKey) || !request.Confirmed || !isKnowledgeID(source, 32) {
		return nil, false, knowledgeInvalid("Explicit confirmation and valid identity required")
	}
	digest, err := lifecycleCommandDigest(user, domain, source, action, request, target)
	if err != nil {
		return nil, false, knowledgeInvalid("Invalid command projection")
	}
	var result *entity.KnowledgeLifecycleReceipt
	created := false
	err = d.lifecycleTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		row, prior, err := loadLifecycleReceipt(tx, user, domain, "", action, request.IdempotencyKey)
		if err == nil {
			if row.SourceID != source || row.RequestSHA256 != digest {
				return lifecycleConflict("idempotency_conflict")
			}
			result = prior
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		data, err := loadKnowledgeLifecycleSource(tx, kb, source)
		if err != nil {
			return err
		}
		if err := lifecycleCheckExpected(kb, data, request); err != nil {
			return err
		}
		before := lifecycleSnapshot(data.head, data.state)
		changed := false
		switch action {
		case "reclassify":
			if data.state.State == "archived" {
				return lifecycleConflict("lifecycle_source_archived")
			}
			classification := command.(entity.KnowledgeLifecycleReclassify).Classification
			if err := knowledgeImportClassification(tx, domain, classification.GroupID, classification.ComponentSide, classification.Language); err != nil {
				return err
			}
			changed = directoryJSON(classification) != directoryJSON(before.Classification)
			if changed {
				if err := lifecycleMetadataRevision(tx, kb, user, data, data.head.RelativePath, classification); err != nil {
					return err
				}
			}
		case "move":
			relative := command.(entity.KnowledgeLifecycleMove).RelativePath
			reason, err := lifecycleMoveReason(tx, data, relative)
			if err != nil {
				return err
			}
			if reason != nil {
				switch *reason {
				case "source_archived":
					return lifecycleConflict("lifecycle_source_archived")
				case "action_unsupported":
					return lifecycleConflict("lifecycle_action_unsupported")
				case "target_occupied":
					return lifecycleConflict("lifecycle_position_occupied")
				default:
					return knowledgeInvalid(*reason)
				}
			}
			changed = relative != data.head.RelativePath
			if changed {
				if err := lifecycleMoveBinding(tx, data, relative); err != nil {
					return err
				}
				if err := lifecycleMetadataRevision(tx, kb, user, data, relative, before.Classification); err != nil {
					return err
				}
			}
		case "archive", "restore":
			next := "archived"
			if action == "restore" {
				next = "active"
			}
			if action == "restore" {
				if err := knowledgeImportClassification(tx, domain, data.head.GroupID, data.head.ComponentSide, data.head.Language); err != nil {
					return err
				}
				if data.registration != nil {
					if data.binding.RelativePath != data.head.RelativePath {
						return lifecycleConflict("lifecycle_integrity")
					}
					retired, err := lifecycleRetired(tx, data.registration.ID, data.head.RelativePath)
					if err != nil {
						return err
					}
					if retired != nil {
						return lifecycleConflict("lifecycle_position_occupied")
					}
				}
			}
			changed = data.state.State != next
			if changed {
				if err := lifecycleAdvance(tx, data.state, next); err != nil {
					return err
				}
			}
		}
		if changed && (action == "reclassify" || action == "move") {
			if err := lifecycleAdvance(tx, data.state, data.state.State); err != nil {
				return err
			}
		}
		status := "unchanged"
		if changed {
			status = "applied"
		}
		now := time.Now().UTC()
		raw, err := json.Marshal(command)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeLifecycleReceipt{CommandID: common.GenerateUUID(), OwnerID: user, DomainID: domain, SourceID: source, Action: action, IdempotencyKey: request.IdempotencyKey, RequestSHA256: digest, Request: raw, Result: status, Before: before, After: lifecycleSnapshot(data.head, data.state), PolicyVersion: kb.KnowledgePolicyVersion, CreatedAt: now.Format(time.RFC3339)}
		payload, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if len(payload)+512 > entity.KnowledgeLifecycleJSONBytes {
			return knowledgeError(413, "lifecycle_budget_exceeded", "Lifecycle result exceeds the JSON budget")
		}
		record := entity.KnowledgeLifecycleCommandRecord{ID: result.CommandID, KeySHA256: lifecycleCommandKey(user, domain, action, request.IdempotencyKey), OwnerID: user, DomainID: domain, SourceID: source, Action: action, IdempotencyKey: request.IdempotencyKey, RequestSHA256: digest, Payload: string(payload), CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}
