package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"slices"
	"time"
	"unicode/utf8"
)

type directoryPreviewPrivate struct {
	Identity string `json:"identity"`
}

func directoryPreviewDigest(view *entity.KnowledgeDirectoryPreview) string {
	if len(view.Items) != 1 {
		return ""
	}
	i := view.Items[0]
	c := i.Classification
	return directorySHA([]byte(directoryTokens("C2-DIR-PREVIEW-1", view.PreviewID, view.DomainID, view.RegistrationID, view.RootID, view.RootVersion, view.PolicyVersion, view.ObservationID, view.ObservationSHA256, view.CapturedAt, view.ExpiresAt, i.RelativePath, c.GroupID, c.ComponentSide, c.Language, i.Action, i.SourceID, i.ExpectedRevisionID, i.SHA256, i.ByteLength)))
}
func directoryVerifyHead(tx *gorm.DB, domain, registration, relative string, source, revision *string) (*entity.KnowledgeSourceBinding, error) {
	binding, current, err := directoryCurrent(tx, domain, registration, relative)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		if source != nil || revision != nil {
			return nil, directoryConflict("revision_changed")
		}
		return &entity.KnowledgeSourceBinding{ID: common.GenerateUUID(), RegistrationID: registration, RelativePath: relative, DocumentID: common.GenerateUUID(), FileID: common.GenerateUUID()}, nil
	}
	if source == nil || revision == nil || binding.DocumentID != *source || current.ID != *revision {
		return nil, directoryConflict("revision_changed")
	}
	return binding, nil
}
func (d *DatasetService) CreateKnowledgeDirectoryPreview(ctx context.Context, user, domain, registration string, req entity.KnowledgeDirectoryPreviewRequest) (*entity.KnowledgeDirectoryPreview, error) {
	if !isKnowledgeID(req.ObservationID, 32) || !isKnowledgeID(req.ObservationSHA256, 64) || !utility.IsKnowledgeDirectoryPath(req.RelativePath, false) {
		return nil, directoryInvalid()
	}
	var result *entity.KnowledgeDirectoryPreview
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		if !scope.contains(req.RelativePath) {
			return directoryInvalid()
		}
		obs, private, err := loadDirectoryObservation(tx, scope, user, req.ObservationID, kb)
		if err != nil {
			return err
		}
		if obs.ObservationSHA256 != req.ObservationSHA256 {
			return directoryConflict("observation_mismatch")
		}
		if !obs.Complete {
			return directoryConflict("observation_incomplete")
		}
		index := slices.IndexFunc(obs.Items, func(i entity.KnowledgeDirectoryObservationItem) bool { return i.RelativePath == req.RelativePath })
		if index < 0 {
			return directoryConflict("observation_mismatch")
		}
		i := obs.Items[index]
		if !i.Eligible || i.State != "verified" || i.SHA256 == nil || i.ByteLength == nil {
			return directoryConflict("ineligible")
		}
		if err := lifecycleCheckPreparation(tx, domain, "directory_observation", obs.ObservationID, i.RelativePath, lifecycleSourceValue(i.SourceID)); err != nil {
			return directoryConflict("revision_changed")
		}
		if _, err := directoryVerifyHead(tx, domain, registration, i.RelativePath, i.SourceID, i.ExpectedRevisionID); err != nil {
			return err
		}
		c := req.Classification
		if err := knowledgeImportClassification(tx, domain, c.GroupID, c.ComponentSide, c.Language); err != nil {
			return err
		}
		root, err := directoryOpenRoot(tx, scope.Config)
		if err != nil {
			return err
		}
		defer root.Close()
		readCtx, cancel := context.WithTimeout(tx.Statement.Context, 8*time.Second)
		defer cancel()
		content, identity, err := utility.ReadKnowledgeDirectoryFile(readCtx, root, i.RelativePath, entity.KnowledgeMaxSourceBytes)
		if err != nil || directorySHA(content) != *i.SHA256 || identity != private.Identities[i.RelativePath] || int64(len(content)) != *i.ByteLength {
			return directoryConflict("source_changed")
		}
		if directoryTextReason(i.RelativePath, content) != "" {
			return directoryConflict("source_changed")
		}
		_, current, err := directoryCurrent(tx, domain, registration, i.RelativePath)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		expiry := now.Add(10 * time.Minute)
		obsExpiry, err := time.Parse(time.RFC3339Nano, obs.ExpiresAt)
		if err != nil {
			return err
		}
		if obsExpiry.Before(expiry) {
			expiry = obsExpiry
		}
		if !now.Before(expiry) {
			return directoryError(410, "expired", "")
		}
		item := entity.KnowledgeDirectoryPreviewItem{RelativePath: i.RelativePath, Classification: c, Action: directoryAction(*i.SHA256, i.RelativePath, c, current), SourceID: i.SourceID, ExpectedRevisionID: i.ExpectedRevisionID, SHA256: *i.SHA256, ByteLength: *i.ByteLength}
		view := entity.KnowledgeDirectoryPreview{PreviewID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration, RootID: scope.Config.RootID, RootVersion: scope.Config.RootVersion, PolicyVersion: kb.KnowledgePolicyVersion, ObservationID: obs.ObservationID, ObservationSHA256: obs.ObservationSHA256, CapturedAt: directoryTime(now), ExpiresAt: directoryTime(expiry), Items: []entity.KnowledgeDirectoryPreviewItem{item}}
		view.ManifestSHA256 = directoryPreviewDigest(&view)
		if err := lifecycleCopyFence(tx, domain, "directory_observation", obs.ObservationID, i.RelativePath, "text_preview", view.PreviewID, i.RelativePath, lifecycleSourceValue(i.SourceID)); err != nil {
			return directoryConflict("revision_changed")
		}
		if err := directorySaveState(tx, user, domain, registration, "preview", view.PreviewID, expiry, view, directoryPreviewPrivate{Identity: identity}); err != nil {
			return err
		}
		native := entity.KnowledgeImportPreview{ID: view.PreviewID, RegistrationID: registration, PolicyVersion: kb.KnowledgePolicyVersion, RelativePath: i.RelativePath, GroupID: c.GroupID, ComponentSide: c.ComponentSide, Language: c.Language, Content: string(content), SHA256: item.SHA256, ByteLength: item.ByteLength, DocumentID: item.SourceID, Action: item.Action, ManifestSHA256: view.ManifestSHA256, ExpiresAt: expiry, CreatedAt: now}
		if item.ExpectedRevisionID != nil {
			native.ExpectedRevisionID = *item.ExpectedRevisionID
		}
		if err = tx.Create(&native).Error; err != nil {
			return err
		}
		result = &view
		return nil
	})
	return result, err
}
func loadDirectoryPreview(tx *gorm.DB, scope *directoryScope, user, id string, kb *entity.Knowledgebase) (*entity.KnowledgeDirectoryPreview, *entity.KnowledgeImportPreview, *entity.KnowledgeDirectoryState, error) {
	state, err := directoryLoadState(tx, scope, user, "preview", id, true)
	if err != nil {
		return nil, nil, nil, err
	}
	var view entity.KnowledgeDirectoryPreview
	if json.Unmarshal([]byte(state.Payload), &view) != nil {
		return nil, nil, nil, directoryInvalid()
	}
	native, err := importPreview(tx, scope.Native.ID, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(view.Items) != 1 {
		return nil, nil, nil, directoryConflict("observation_mismatch")
	}
	i := view.Items[0]
	nativeClass := entity.KnowledgeDirectoryClassification{GroupID: native.GroupID, ComponentSide: native.ComponentSide, Language: native.Language}
	expected := ""
	if i.ExpectedRevisionID != nil {
		expected = *i.ExpectedRevisionID
	}
	if native.RelativePath != i.RelativePath || native.SHA256 != i.SHA256 || native.ByteLength != i.ByteLength || native.Action != i.Action || native.PolicyVersion != view.PolicyVersion || native.ExpectedRevisionID != expected || directoryJSON(nativeClass) != directoryJSON(i.Classification) || directoryJSON(native.DocumentID) != directoryJSON(i.SourceID) {
		return nil, nil, nil, directoryConflict("observation_mismatch")
	}
	view.ConsumedTaskID = native.ConsumedTaskID
	if native.ConsumedTaskID == nil {
		if err = directoryCheckVersions(scope, kb, view.RootVersion, view.PolicyVersion); err != nil {
			return nil, nil, nil, err
		}
	}
	if view.ManifestSHA256 != directoryPreviewDigest(&view) || native.ManifestSHA256 != view.ManifestSHA256 {
		return nil, nil, nil, directoryConflict("observation_mismatch")
	}
	return &view, native, state, nil
}
func (d *DatasetService) GetKnowledgeDirectoryPreview(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeDirectoryPreview, error) {
	var result *entity.KnowledgeDirectoryPreview
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		result, _, _, err = loadDirectoryPreview(tx, scope, user, id, kb)
		return err
	})
	return result, err
}
func (d *DatasetService) ReadKnowledgeDirectoryPreview(ctx context.Context, user, domain, registration, id string, offset, length int64) (*entity.KnowledgeDirectoryContentWindow, error) {
	if offset < 0 || length < 1 || length > 65536 {
		return nil, directoryError(416, "range", "")
	}
	var result *entity.KnowledgeDirectoryContentWindow
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		view, native, _, err := loadDirectoryPreview(tx, scope, user, id, kb)
		if err != nil {
			return err
		}
		if native.ConsumedTaskID != nil {
			return directoryConflict("preview_consumed")
		}
		if !time.Now().Before(native.ExpiresAt) {
			return directoryError(410, "expired", "")
		}
		content := []byte(native.Content)
		if int64(len(content)) != native.ByteLength || directorySHA(content) != native.SHA256 {
			return directoryError(503, "unavailable", "result_unknown")
		}
		if offset > int64(len(content)) {
			return directoryError(416, "range", "")
		}
		end := min(offset+length, int64(len(content)))
		if !utf8.Valid(content[offset:end]) {
			return directoryError(416, "range", "")
		}
		result = &entity.KnowledgeDirectoryContentWindow{PreviewID: view.PreviewID, Offset: offset, ByteLength: end - offset, TotalBytes: native.ByteLength, SHA256: native.SHA256, Text: string(content[offset:end])}
		return nil
	})
	return result, err
}
func directoryLoadReceipt(tx *gorm.DB, registration, id, key string) (*entity.KnowledgeDirectoryReceipt, error) {
	query := tx.Where("registration_id = ?", registration)
	if key != "" {
		query = query.Where("idempotency_key = ?", key)
	} else {
		query = query.Where("id = ?", id)
	}
	var task entity.KnowledgeImport
	if err := query.Take(&task).Error; err != nil {
		return nil, err
	}
	var result entity.KnowledgeDirectoryReceipt
	if err := json.Unmarshal([]byte(task.Receipt), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (d *DatasetService) GetKnowledgeDirectoryImport(ctx context.Context, user, domain, registration, id, key string) (*entity.KnowledgeDirectoryReceipt, error) {
	if key != "" {
		if len(key) > 128 || !knowledgeImportKey.MatchString(key) {
			return nil, directoryInvalid()
		}
	} else if !isKnowledgeID(id, 32) {
		return nil, directoryMissing()
	}
	var result *entity.KnowledgeDirectoryReceipt
	err := d.directoryTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if _, err := loadDirectoryScope(tx, user, domain, registration); err != nil {
			return err
		}
		var err error
		result, err = directoryLoadReceipt(tx, registration, id, key)
		return err
	})
	return result, err
}
func (d *DatasetService) CommitKnowledgeDirectoryImport(ctx context.Context, user, domain, registration string, req entity.KnowledgeDirectoryCommitRequest) (*entity.KnowledgeDirectoryReceipt, bool, error) {
	if !isKnowledgeID(req.PreviewID, 32) || !isKnowledgeID(req.ManifestSHA256, 64) || len(req.IdempotencyKey) > 128 || !knowledgeImportKey.MatchString(req.IdempotencyKey) {
		return nil, false, directoryInvalid()
	}
	var result *entity.KnowledgeDirectoryReceipt
	created := false
	err := d.directoryTransaction(ctx, user, domain, 30*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, err := loadDirectoryScope(tx, user, domain, registration)
		if err != nil {
			return err
		}
		receipt, err := directoryLoadReceipt(tx, registration, "", req.IdempotencyKey)
		if err == nil {
			if receipt.PreviewID != req.PreviewID || receipt.ManifestSHA256 != req.ManifestSHA256 {
				return directoryConflict("idempotency_conflict")
			}
			result = receipt
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		view, native, state, err := loadDirectoryPreview(tx, scope, user, req.PreviewID, kb)
		if err != nil {
			return err
		}
		if native.ConsumedTaskID != nil {
			return directoryConflict("preview_consumed")
		}
		if !time.Now().Before(native.ExpiresAt) {
			return directoryError(410, "expired", "")
		}
		if view.ManifestSHA256 != req.ManifestSHA256 {
			return directoryConflict("observation_mismatch")
		}
		obs, _, err := loadDirectoryObservation(tx, scope, user, view.ObservationID, kb)
		if err != nil {
			return err
		}
		if !obs.Complete || obs.ObservationSHA256 != view.ObservationSHA256 {
			return directoryConflict("observation_mismatch")
		}
		i := view.Items[0]
		if !scope.contains(i.RelativePath) {
			return directoryInvalid()
		}
		c := i.Classification
		if err = knowledgeImportClassification(tx, domain, c.GroupID, c.ComponentSide, c.Language); err != nil {
			return err
		}
		binding, err := directoryVerifyHead(tx, domain, registration, i.RelativePath, i.SourceID, i.ExpectedRevisionID)
		if err != nil {
			return err
		}
		if err := lifecycleCheckPreparation(tx, domain, "text_preview", native.ID, i.RelativePath, lifecycleSourceValue(i.SourceID)); err != nil {
			return directoryConflict("revision_changed")
		}
		root, err := directoryOpenRoot(tx, scope.Config)
		if err != nil {
			return err
		}
		defer root.Close()
		var private directoryPreviewPrivate
		if json.Unmarshal([]byte(state.PrivateData), &private) != nil {
			return directoryInvalid()
		}
		readCtx, cancel := context.WithTimeout(tx.Statement.Context, 10*time.Second)
		defer cancel()
		content, identity, err := utility.ReadKnowledgeDirectoryFile(readCtx, root, i.RelativePath, entity.KnowledgeMaxSourceBytes)
		if err != nil || directorySHA(content) != i.SHA256 || identity != private.Identity || int64(len(content)) != i.ByteLength {
			return directoryConflict("source_changed")
		}
		latest, err := directoryConfig(user, domain, scope.Config.RootID)
		if err != nil {
			return err
		}
		if latest.RootVersion != view.RootVersion || latest.AbsolutePath != scope.Config.AbsolutePath {
			return directoryConflict("root_changed")
		}
		if !time.Now().Before(native.ExpiresAt) {
			return directoryError(410, "expired", "")
		}
		now := time.Now().UTC()
		result = &entity.KnowledgeDirectoryReceipt{ImportTaskID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration, PreviewID: view.PreviewID, ObservationID: view.ObservationID, ManifestSHA256: view.ManifestSHA256, Status: "succeeded", Mode: "catalogue_only", CreatedAt: directoryTime(now)}
		saved := entity.KnowledgeDirectoryResult{RelativePath: i.RelativePath, Classification: i.Classification, SaveStatus: "created", CatalogueStatus: "registered", StructureStatus: "deferred", FullTextStatus: "disabled", VectorStatus: "disabled", SHA256: i.SHA256, ByteLength: i.ByteLength, Code: "saved"}
		if err = tx.SavePoint("directory_publish").Error; err != nil {
			return err
		}
		snapshot, publishErr := publishKnowledgeImport(tx.Statement.Context, tx, kb, user, binding, native)
		if publishErr != nil {
			if err = tx.RollbackTo("directory_publish").Error; err != nil {
				return err
			}
			result.Status = "failed"
			saved.SaveStatus = "failed"
			saved.CatalogueStatus = "failed"
			saved.Code = "save_failed"
		} else {
			saved.SourceID = &snapshot.Source.DocumentID
			saved.SourceRevisionID = &snapshot.Source.ID
			saved.RootNodeID = &snapshot.Root.ID
			if i.Action == "update" {
				saved.SaveStatus = "updated"
			}
			if i.Action == "unchanged" {
				saved.SaveStatus = "unchanged"
			}
		}
		result.Items = []entity.KnowledgeDirectoryResult{saved}
		task := entity.KnowledgeImport{ID: result.ImportTaskID, RegistrationID: registration, IdempotencyKey: req.IdempotencyKey, PreviewID: view.PreviewID, ManifestSHA256: view.ManifestSHA256, Receipt: directoryJSON(result), CreatedAt: now}
		if err = tx.Create(&task).Error; err != nil {
			return err
		}
		if err = tx.Model(native).Updates(map[string]any{"consumed_task_id": task.ID, "content": ""}).Error; err != nil {
			return err
		}
		view.ConsumedTaskID = &task.ID
		if err = tx.Model(state).Updates(map[string]any{"payload": directoryJSON(view), "private_data": "{}", "expires_at": now}).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}
