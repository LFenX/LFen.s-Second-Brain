package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

func originalRevisionView(source *entity.KnowledgeSourceRevision) (*entity.KnowledgeOriginalRevision, error) {
	if source.RepresentationKind != "original_only" || source.CreateDate == nil || !isKnowledgeID(source.OriginalRegistrationID, 32) {
		return nil, originalError(409, "original_integrity")
	}
	return &entity.KnowledgeOriginalRevision{DomainID: source.DomainID, RegistrationID: source.OriginalRegistrationID,
		SourceID: source.DocumentID, SourceRevisionID: source.ID, OriginalFilename: source.OriginalFilename, RelativePath: source.RelativePath,
		Classification: entity.KnowledgeDirectoryClassification{GroupID: source.GroupID, ComponentSide: source.ComponentSide, Language: source.Language},
		SHA256:         source.SHA256, ByteLength: source.ByteLength, MediaType: source.OriginalMediaType, RepresentationKind: "original_only",
		TextAvailable: false, ParseStatus: "not_parsed", StructureStatus: "not_built", FullTextStatus: "not_built", VectorStatus: "not_built",
		IsCurrent: source.IsCurrent, CreatedAt: originalTime(*source.CreateDate)}, nil
}

func loadOriginalRevision(tx *gorm.DB, domain, id string) (*entity.KnowledgeSourceRevision, error) {
	if !isKnowledgeID(id, 32) {
		return nil, originalError(404, "knowledge_not_found")
	}
	var source entity.KnowledgeSourceRevision
	err := tx.Model(&entity.KnowledgeSourceRevision{}).
		Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").
		Where("knowledge_source_revision.id = ? AND knowledge_source_revision.domain_id = ? AND knowledge_source_revision.representation_kind = ? AND document.status = ?", id, domain, "original_only", "1").Take(&source).Error
	if err != nil {
		return nil, err
	}
	if _, err := originalRegistration(tx, domain, source.OriginalRegistrationID); err != nil {
		return nil, err
	}
	return &source, nil
}

// GetKnowledgeOriginalTarget resolves only a registered exact path.
func (d *DatasetService) GetKnowledgeOriginalTarget(ctx context.Context, user, domain, registration, relative string) (*entity.KnowledgeOriginalTarget, error) {
	var result *entity.KnowledgeOriginalTarget
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		reg, err := originalRegistration(tx, domain, registration)
		if err != nil {
			return err
		}
		found := false
		for _, p := range reg.IncludePaths {
			if relative == p {
				found = true
			}
		}
		if !found {
			return originalError(400, "knowledge_invalid_request")
		}
		_, current, err := originalCurrent(tx, domain, registration, relative)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeOriginalTarget{DomainID: domain, RegistrationID: registration, RelativePath: relative}
		if current != nil {
			result.Source, err = originalRevisionView(current)
		}
		return err
	})
	return result, err
}

// GetKnowledgeOriginalRevision reads metadata for one fixed original revision.
func (d *DatasetService) GetKnowledgeOriginalRevision(ctx context.Context, user, domain, id string) (*entity.KnowledgeOriginalRevision, error) {
	var result *entity.KnowledgeOriginalRevision
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		source, err := loadOriginalRevision(tx, domain, id)
		if err != nil {
			return err
		}
		result, err = originalRevisionView(source)
		return err
	})
	return result, err
}

type originalCursor struct {
	GroupID *string `json:"group_id"`
	LastID  string  `json:"last_id"`
}

// ListKnowledgeOriginalRevisions is a scoped current-head view, not a snapshot.
func (d *DatasetService) ListKnowledgeOriginalRevisions(ctx context.Context, user, domain string, group *string, cursor string) (*entity.KnowledgeOriginalPage, error) {
	result := &entity.KnowledgeOriginalPage{DomainID: domain, Items: []entity.KnowledgeOriginalRevision{}}
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		if group != nil {
			if !isKnowledgeID(*group, 32) {
				return originalError(400, "knowledge_invalid_request")
			}
			var row entity.KnowledgeGroup
			if err := tx.Where("id = ? AND domain_id = ?", *group, domain).Take(&row).Error; err != nil {
				return err
			}
		}
		position := originalCursor{GroupID: group}
		if cursor != "" {
			if !isKnowledgeID(cursor, 32) {
				return originalError(400, "knowledge_invalid_request")
			}
			var row entity.KnowledgeDirectoryState
			if err := tx.Where("id = ? AND domain_id = ? AND user_id = ? AND kind = ? AND expires_at > ?", cursor, domain, user, "original_cursor", now).Take(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return originalError(400, "knowledge_invalid_request")
				}
				return err
			}
			if json.Unmarshal([]byte(row.Payload), &position) != nil || directoryJSON(position.GroupID) != directoryJSON(group) || !isKnowledgeID(position.LastID, 32) {
				return originalError(400, "knowledge_invalid_request")
			}
		}
		validDocuments := tx.Model(&entity.Document{}).Select("id").Where("kb_id = ? AND status = ?", domain, "1")
		query := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND is_current = ? AND representation_kind = ? AND document_id IN (?)", domain, true, "original_only", validDocuments)
		query = query.Where("original_registration_id IN (?)", tx.Model(&entity.KnowledgeSourceRegistration{}).Select("id").Where("domain_id = ? AND source_kind = ? AND input_mode = ?", domain, "original_document", "original_upload"))
		query = lifecycleActiveQuery(query)
		if group != nil {
			query = query.Where("group_id = ?", *group)
		}
		if position.LastID != "" {
			query = query.Where("id > ?", position.LastID)
		}
		var rows []entity.KnowledgeSourceRevision
		if err := query.Order("id ASC").Limit(21).Find(&rows).Error; err != nil {
			return err
		}
		responseBytes := 2
		for _, row := range rows[:min(len(rows), 20)] {
			if _, err := originalRegistration(tx, domain, row.OriginalRegistrationID); err != nil {
				return err
			}
			view, err := originalRevisionView(&row)
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(view)
			if err != nil {
				return err
			}
			if responseBytes+len(encoded)+1 > 30720 {
				break
			}
			responseBytes += len(encoded) + 1
			result.Items = append(result.Items, *view)
		}
		if len(rows) > len(result.Items) {
			if len(result.Items) == 0 {
				return originalError(500, "knowledge_internal")
			}
			position.LastID = rows[len(result.Items)-1].ID
			next := entity.KnowledgeDirectoryState{ID: common.GenerateUUID(), DomainID: domain, UserID: user, Kind: "original_cursor", Payload: directoryJSON(position), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
			if err := tx.Create(&next).Error; err != nil {
				return err
			}
			result.NextCursor = &next.ID
		}
		return nil
	})
	return result, err
}

func readOriginalReceipt(row *entity.KnowledgeOriginalImportRecord, domain, registration string) (*entity.KnowledgeOriginalReceipt, error) {
	var receipt entity.KnowledgeOriginalReceipt
	if json.Unmarshal([]byte(row.Receipt), &receipt) != nil || receipt.DomainID != domain || receipt.RegistrationID != registration || receipt.ImportTaskID != row.ID || receipt.UploadID != row.UploadID || receipt.IdempotencyKey != row.IdempotencyKey || receipt.ManifestSHA256 != row.ManifestSHA256 || receipt.Source.DomainID != domain || receipt.Source.RegistrationID != registration {
		return nil, errors.New("invalid original receipt")
	}
	return &receipt, nil
}

// GetKnowledgeOriginalImport recovers a fixed receipt independently of staging TTL.
func (d *DatasetService) GetKnowledgeOriginalImport(ctx context.Context, user, domain, registration, id, key string) (*entity.KnowledgeOriginalReceipt, error) {
	var result *entity.KnowledgeOriginalReceipt
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		if _, err := originalRegistration(tx, domain, registration); err != nil {
			return err
		}
		query := tx.Where("registration_id = ?", registration)
		if key != "" {
			if !originalKey(key, 128) {
				return originalError(400, "knowledge_invalid_request")
			}
			query = query.Where("idempotency_key = ?", key)
		} else {
			if !isKnowledgeID(id, 32) {
				return originalError(404, "knowledge_not_found")
			}
			query = query.Where("id = ?", id)
		}
		var row entity.KnowledgeOriginalImportRecord
		if err := query.Take(&row).Error; err != nil {
			return err
		}
		var err error
		result, err = readOriginalReceipt(&row, domain, registration)
		return err
	})
	return result, err
}

func publishOriginalSource(ctx context.Context, tx *gorm.DB, kb *entity.Knowledgebase, row *entity.KnowledgeOriginalUploadRecord, upload *entity.KnowledgeOriginalUpload, binding *entity.KnowledgeSourceBinding, current *entity.KnowledgeSourceRevision) (*entity.KnowledgeSourceRevision, error) {
	if upload.ByteLength == nil || upload.SHA256 == nil || upload.MediaType == nil || upload.Action == nil || upload.ReceivedBytes != *upload.ByteLength || *upload.ByteLength != upload.DeclaredByteLength || !isKnowledgeID(row.NativeHash, 32) || row.ObjectFolderID != *kb.KnowledgeFolderID || row.ObjectLocation != storage.KnowledgeOriginalObjectPrefix+*upload.SHA256 {
		return nil, originalError(409, "original_integrity")
	}
	input := originalInputHash(*upload.SHA256, upload.OriginalFilename, *upload.MediaType, upload.RelativePath, upload.Classification)
	if current != nil {
		if err := lifecyclePublishState(tx, kb.ID, current.DocumentID, false, current.InputHash != input); err != nil {
			return nil, originalError(409, "revision_changed")
		}
	}
	if current != nil && current.InputHash == input {
		return current, nil
	}
	var folder entity.File
	if err := tx.Where("id = ? AND tenant_id = ? AND type = ? AND source_type = ?", *kb.KnowledgeFolderID, kb.TenantID, "folder", "knowledgebase").Take(&folder).Error; err != nil {
		return nil, err
	}
	name, location, valid := upload.OriginalFilename, row.ObjectLocation, "1"
	kind := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	if *upload.MediaType == "text/plain" {
		kind = "txt"
	}
	var doc entity.Document
	var native entity.File
	if binding == nil {
		binding = &entity.KnowledgeSourceBinding{ID: common.GenerateUUID(), RegistrationID: upload.RegistrationID, RelativePath: upload.RelativePath, DocumentID: common.GenerateUUID(), FileID: common.GenerateUUID()}
		if err := lifecyclePublishState(tx, kb.ID, binding.DocumentID, true, true); err != nil {
			return nil, err
		}
		doc = entity.Document{ID: binding.DocumentID, KbID: kb.ID, CreatedBy: upload.OwnerID, Name: &name, Location: &location, Size: *upload.ByteLength, ContentHash: &row.NativeHash, Status: &valid, ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: kind, Suffix: strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")}
		native = entity.File{ID: binding.FileID, ParentID: folder.ID, TenantID: kb.TenantID, CreatedBy: upload.OwnerID, Name: name, Location: &location, Size: *upload.ByteLength, Type: kind, SourceType: "knowledgebase"}
		for _, record := range []any{&doc, &native, &entity.File2Document{ID: common.GenerateUUID(), FileID: &native.ID, DocumentID: &doc.ID}, binding} {
			if err := tx.Create(record).Error; err != nil {
				return nil, err
			}
		}
		if err := tx.Model(kb).Update("doc_num", gorm.Expr("doc_num + 1")).Error; err != nil {
			return nil, err
		}
	} else {
		if err := tx.Where("id = ? AND kb_id = ? AND status = ?", binding.DocumentID, kb.ID, "1").Take(&doc).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("id = ? AND parent_id = ? AND tenant_id = ? AND source_type = ?", binding.FileID, folder.ID, kb.TenantID, "knowledgebase").Take(&native).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&doc).Updates(map[string]any{"name": name, "location": location, "size": *upload.ByteLength, "content_hash": row.NativeHash}).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&native).Updates(map[string]any{"name": name, "location": location, "size": *upload.ByteLength}).Error; err != nil {
			return nil, err
		}
		native.Name = name
	}
	var source entity.KnowledgeSourceRevision
	err := tx.Where("domain_id = ? AND document_id = ? AND input_hash = ?", kb.ID, doc.ID, input).Take(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		commit, item, err := file.NewFileCommitService().RecordSourceSnapshotObject(ctx, tx, &native, upload.OwnerID, *upload.SHA256, location, *upload.ByteLength)
		if err != nil {
			return nil, err
		}
		source = entity.KnowledgeSourceRevision{ID: item.ID, DomainID: kb.ID, DocumentID: doc.ID, FileID: native.ID, CommitID: commit.ID, FolderID: folder.ID,
			GroupID: upload.Classification.GroupID, RelativePath: upload.RelativePath, ComponentSide: upload.Classification.ComponentSide, Language: upload.Classification.Language,
			SHA256: *upload.SHA256, InputHash: input, ByteLength: *upload.ByteLength, LineCount: 0, IsCurrent: true,
			RepresentationKind: "original_only", OriginalFilename: name, OriginalMediaType: *upload.MediaType, OriginalRegistrationID: upload.RegistrationID}
		if err := tx.Create(&source).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if source.RepresentationKind != "original_only" || source.OriginalRegistrationID != upload.RegistrationID || source.FileID != native.ID || source.FolderID != folder.ID {
		return nil, originalError(409, "original_integrity")
	}
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND id <> ?", kb.ID, doc.ID, source.ID).Update("is_current", false).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("id = ?", source.ID).Update("is_current", true).Error; err != nil {
		return nil, err
	}
	source.IsCurrent = true
	return &source, nil
}

// ConfirmKnowledgeOriginalImport publishes already stored bytes in one SQL transaction.
func (d *DatasetService) ConfirmKnowledgeOriginalImport(ctx context.Context, user, domain, registration string, req entity.KnowledgeOriginalConfirmRequest) (*entity.KnowledgeOriginalReceipt, bool, error) {
	var result *entity.KnowledgeOriginalReceipt
	created := false
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		if _, err := originalRegistration(tx, domain, registration); err != nil {
			return err
		}
		if !originalKey(req.IdempotencyKey, 128) || !isKnowledgeID(req.UploadID, 32) || !isKnowledgeID(req.ManifestSHA256, 64) || !req.Confirmed {
			return originalError(400, "knowledge_invalid_request")
		}
		var replay entity.KnowledgeOriginalImportRecord
		err := tx.Where("registration_id = ? AND idempotency_key = ?", registration, req.IdempotencyKey).Take(&replay).Error
		if err == nil {
			if replay.UploadID != req.UploadID || replay.ManifestSHA256 != req.ManifestSHA256 {
				return originalError(409, "idempotency_conflict")
			}
			result, err = readOriginalReceipt(&replay, domain, registration)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row, view, err := loadOriginalUpload(tx, kb, user, registration, req.UploadID, "", now)
		if err != nil {
			return err
		}
		if view.State == "committed" {
			return originalError(409, "idempotency_conflict")
		}
		if view.State != "ready" {
			return &originalCommitError{originalStateWriteError(view.State)}
		}
		if view.ManifestSHA256 == nil || req.ManifestSHA256 != *view.ManifestSHA256 || originalPreviewDigest(view) != req.ManifestSHA256 {
			return originalError(409, "manifest_changed")
		}
		binding, current, err := originalCurrent(tx, domain, registration, view.RelativePath)
		if err != nil {
			return err
		}
		if (view.ExpectedRevisionID == nil && current != nil) || (view.ExpectedRevisionID != nil && (current == nil || *view.ExpectedRevisionID != current.ID)) {
			return originalError(409, "revision_changed")
		}
		if err := lifecycleCheckPreparation(tx, domain, "original_upload", view.UploadID, view.RelativePath, lifecycleSourceValue(view.SourceID)); err != nil {
			return originalError(409, "revision_changed")
		}
		source, err := publishOriginalSource(tx.Statement.Context, tx, kb, row, view, binding, current)
		if err != nil {
			return err
		}
		sourceView, err := originalRevisionView(source)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeOriginalReceipt{ImportTaskID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration,
			UploadID: req.UploadID, IdempotencyKey: req.IdempotencyKey, ManifestSHA256: req.ManifestSHA256, Status: "succeeded",
			SaveStatus: *view.Action, Source: *sourceView, CreatedAt: originalTime(now)}
		record := entity.KnowledgeOriginalImportRecord{ID: result.ImportTaskID, RegistrationID: registration, IdempotencyKey: req.IdempotencyKey,
			UploadID: req.UploadID, ManifestSHA256: req.ManifestSHA256, Receipt: directoryJSON(result), CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		view.State, view.ImportTaskID, view.Code = "committed", directoryString(result.ImportTaskID), nil
		if err := saveOriginalUpload(tx, row, view, now); err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}
