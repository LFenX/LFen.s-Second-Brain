package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var originalKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

type originalCommitError struct{ err error }

func (e *originalCommitError) Error() string { return e.err.Error() }
func (e *originalCommitError) Unwrap() error { return e.err }

func originalError(status int, code string) error {
	return knowledgeError(status, code, "Original operation could not be completed")
}

func originalNormalize(err error) error {
	if err == nil {
		return nil
	}
	var public *KnowledgeError
	if errors.As(err, &public) {
		if public.Code == "knowledge_conflict" {
			return originalError(409, "policy_changed")
		}
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return originalError(404, "knowledge_not_found")
	}
	return originalError(503, "result_unknown")
}

func (d *DatasetService) originalTransaction(ctx context.Context, user, domain string, fn func(*gorm.DB, *entity.Knowledgebase, time.Time) error) error {
	if _, err := d.knowledgeBase(ctx, user, domain, true); err != nil {
		return originalNormalize(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var committedError error
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, err := lockKnowledgeImportDomain(ctx, tx, user, domain)
		if err != nil {
			return err
		}
		if kb.KnowledgePolicyVersion < 1 {
			return originalError(409, "policy_changed")
		}
		now, err := batchNow(tx)
		if err != nil {
			return err
		}
		err = fn(tx, kb, now.UTC().Truncate(time.Second))
		var stateError *originalCommitError
		if errors.As(err, &stateError) {
			committedError = stateError.err
			return nil
		}
		return err
	})
	if err != nil {
		return originalNormalize(err)
	}
	return originalNormalize(committedError)
}

func originalKey(value string, max int) bool {
	return len(value) >= 1 && len(value) <= max && originalKeyPattern.MatchString(value)
}

func originalTime(value time.Time) string { return value.UTC().Format(time.RFC3339) }

func originalDigest(tokens ...any) string { return directorySHA([]byte(directoryTokens(tokens...))) }

func originalRegistrationDigest(domain, user string, req entity.KnowledgeOriginalRegistrationRequest) string {
	tokens := []any{"knowledge-original-registration/0.5", domain, user, req.RegistrationKey, req.Name, req.SourceKind, req.InputMode, len(req.IncludePaths)}
	for _, p := range req.IncludePaths {
		tokens = append(tokens, p)
	}
	c := req.DefaultClassification
	return originalDigest(append(tokens, c.GroupID, c.ComponentSide, c.Language)...)
}

func originalClassification(reg *entity.KnowledgeSourceRegistration) entity.KnowledgeDirectoryClassification {
	return entity.KnowledgeDirectoryClassification{GroupID: reg.GroupID, ComponentSide: reg.ComponentSide, Language: reg.Language}
}

func originalRegistrationView(reg *entity.KnowledgeSourceRegistration) *entity.KnowledgeOriginalRegistration {
	return &entity.KnowledgeOriginalRegistration{RegistrationKey: reg.RegistrationKey, Name: reg.Name,
		SourceKind: reg.SourceKind, InputMode: reg.InputMode, IncludePaths: slices.Clone(reg.IncludePaths),
		DefaultClassification: originalClassification(reg), RegistrationID: reg.ID, DomainID: reg.DomainID,
		ConfigSHA256: reg.ConfigHash, CreatedAt: originalTime(reg.CreatedAt)}
}

func originalRegistration(tx *gorm.DB, domain, id string) (*entity.KnowledgeSourceRegistration, error) {
	if !isKnowledgeID(id, 32) {
		return nil, originalError(404, "knowledge_not_found")
	}
	var reg entity.KnowledgeSourceRegistration
	if err := tx.Where("id = ? AND domain_id = ? AND source_kind = ? AND input_mode = ?", id, domain, "original_document", "original_upload").Take(&reg).Error; err != nil {
		return nil, err
	}
	if err := knowledgeImportClassification(tx, domain, reg.GroupID, reg.ComponentSide, reg.Language); err != nil {
		return nil, err
	}
	return &reg, nil
}

// CreateKnowledgeOriginalRegistration binds exact original paths to a domain.
func (d *DatasetService) CreateKnowledgeOriginalRegistration(ctx context.Context, user, domain string, req entity.KnowledgeOriginalRegistrationRequest) (*entity.KnowledgeOriginalRegistration, bool, error) {
	if _, err := d.knowledgeBase(ctx, user, domain, true); err != nil {
		return nil, false, originalNormalize(err)
	}
	if !originalKey(req.RegistrationKey, 64) || !validKnowledgeText(req.Name, 128) || req.SourceKind != "original_document" || req.InputMode != "original_upload" || len(req.IncludePaths) < 1 || len(req.IncludePaths) > 32 {
		return nil, false, originalError(400, "knowledge_invalid_request")
	}
	req.IncludePaths = slices.Clone(req.IncludePaths)
	slices.Sort(req.IncludePaths)
	for i, p := range req.IncludePaths {
		if !isKnowledgeImportPath(p) || strings.ContainsAny(p, "*?[]{}") || (i > 0 && p == req.IncludePaths[i-1]) {
			return nil, false, originalError(400, "knowledge_invalid_request")
		}
	}
	var result *entity.KnowledgeOriginalRegistration
	created := false
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		c := req.DefaultClassification
		if err := knowledgeImportClassification(tx, domain, c.GroupID, c.ComponentSide, c.Language); err != nil {
			return err
		}
		hash := originalRegistrationDigest(domain, user, req)
		var reg entity.KnowledgeSourceRegistration
		err := tx.Where("domain_id = ? AND registration_key = ?", domain, req.RegistrationKey).Take(&reg).Error
		if err == nil {
			if reg.SourceKind != req.SourceKind || reg.InputMode != req.InputMode || reg.ConfigHash != hash {
				return originalError(409, "registration_conflict")
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			reg = entity.KnowledgeSourceRegistration{ID: common.GenerateUUID(), DomainID: domain, RegistrationKey: req.RegistrationKey, Name: req.Name,
				SourceKind: req.SourceKind, InputMode: req.InputMode, IncludePaths: req.IncludePaths, GroupID: c.GroupID,
				ComponentSide: c.ComponentSide, Language: c.Language, ConfigHash: hash, CreatedAt: now}
			if encoded, err := json.Marshal(originalRegistrationView(&reg)); err != nil || len(encoded) > 31744 {
				return originalError(413, "upload_too_large")
			}
			if err := tx.Create(&reg).Error; err != nil {
				return err
			}
			created = true
		} else {
			return err
		}
		result = originalRegistrationView(&reg)
		return nil
	})
	return result, created, err
}

// GetKnowledgeOriginalRegistration resolves one ID or an original config key.
func (d *DatasetService) GetKnowledgeOriginalRegistration(ctx context.Context, user, domain, id, key string) (*entity.KnowledgeOriginalRegistration, error) {
	var result *entity.KnowledgeOriginalRegistration
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		if key != "" {
			if !originalKey(key, 64) {
				return originalError(400, "knowledge_invalid_request")
			}
			var reg entity.KnowledgeSourceRegistration
			if err := tx.Where("domain_id = ? AND registration_key = ? AND source_kind = ? AND input_mode = ?", domain, key, "original_document", "original_upload").Take(&reg).Error; err != nil {
				return err
			}
			id = reg.ID
		}
		reg, err := originalRegistration(tx, domain, id)
		if err == nil {
			result = originalRegistrationView(reg)
		}
		return err
	})
	return result, err
}

func originalCurrent(tx *gorm.DB, domain, registration, relative string) (*entity.KnowledgeSourceBinding, *entity.KnowledgeSourceRevision, error) {
	var binding entity.KnowledgeSourceBinding
	err := tx.Where("registration_id = ? AND relative_path = ?", registration, relative).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	current, err := knowledgeImportCurrent(tx, domain, binding.DocumentID)
	if err != nil {
		return nil, nil, err
	}
	if current.RepresentationKind != "original_only" || current.OriginalRegistrationID != registration || current.RelativePath != relative || current.FileID != binding.FileID {
		return nil, nil, originalError(409, "revision_changed")
	}
	return &binding, current, nil
}

func originalUploadDigest(domain, owner, registration string, req entity.KnowledgeOriginalUploadRequest) string {
	c := req.Classification
	return originalDigest("knowledge-original-upload-request/0.5", domain, owner, registration, req.OriginalFilename, req.RelativePath,
		c.GroupID, c.ComponentSide, c.Language, req.DeclaredByteLength, req.DeclaredMediaType, req.ExpectedSHA256, req.ExpectedRevisionID)
}

func originalPreviewDigest(v *entity.KnowledgeOriginalUpload) string {
	c := v.Classification
	return originalDigest("knowledge-original-preview/0.5", v.DomainID, v.OwnerID, v.RegistrationID, v.UploadID, v.RequestSHA256,
		v.PolicyVersion, v.RelativePath, c.GroupID, c.ComponentSide, c.Language, v.ExpectedRevisionID,
		v.SHA256, v.ByteLength, v.MediaType, v.Action, v.ExpiresAt)
}

func originalInputHash(sha, name, media, relative string, c entity.KnowledgeDirectoryClassification) string {
	return originalDigest("knowledge-original-source/0.5", sha, name, media, relative, c.GroupID, c.ComponentSide, c.Language)
}

func originalTerminal(state string) bool {
	return slices.Contains([]string{"rejected", "cancelled", "expired", "committed"}, state)
}

func originalUploadView(row *entity.KnowledgeOriginalUploadRecord) (*entity.KnowledgeOriginalUpload, error) {
	var view entity.KnowledgeOriginalUpload
	if err := json.Unmarshal([]byte(row.Payload), &view); err != nil {
		return nil, err
	}
	if view.UploadID != row.ID || view.DomainID != row.DomainID || view.OwnerID != row.OwnerID || view.RegistrationID != row.RegistrationID || view.IdempotencyKey != row.IdempotencyKey || view.RequestSHA256 != row.RequestSHA256 || view.State != row.State || view.DeclaredByteLength != row.DeclaredBytes {
		return nil, errors.New("original metadata mismatch")
	}
	request := entity.KnowledgeOriginalUploadRequest{OriginalFilename: view.OriginalFilename, RelativePath: view.RelativePath,
		Classification: view.Classification, DeclaredByteLength: view.DeclaredByteLength, DeclaredMediaType: view.DeclaredMediaType,
		ExpectedSHA256: view.ExpectedSHA256, ExpectedRevisionID: view.ExpectedRevisionID, IdempotencyKey: view.IdempotencyKey}
	if originalUploadDigest(view.DomainID, view.OwnerID, view.RegistrationID, request) != view.RequestSHA256 {
		return nil, errors.New("original request identity mismatch")
	}
	return &view, nil
}

func saveOriginalUpload(tx *gorm.DB, row *entity.KnowledgeOriginalUploadRecord, view *entity.KnowledgeOriginalUpload, now time.Time) error {
	view.Revision++
	view.UpdatedAt = originalTime(now)
	row.Payload, row.State, row.UpdatedAt = directoryJSON(view), view.State, now
	return tx.Model(&entity.KnowledgeOriginalUploadRecord{}).Where("id = ?", row.ID).UpdateColumns(map[string]any{
		"payload": row.Payload, "state": row.State, "updated_at": now, "epoch": row.Epoch, "lease_until": row.LeaseUntil,
		"object_folder_id": row.ObjectFolderID, "object_location": row.ObjectLocation, "native_hash": row.NativeHash,
	}).Error
}

func refreshOriginalUpload(tx *gorm.DB, row *entity.KnowledgeOriginalUploadRecord, view *entity.KnowledgeOriginalUpload, now time.Time) error {
	if originalTerminal(view.State) {
		return nil
	}
	if !now.Before(row.ExpiresAt) {
		view.State, view.Code = "expired", directoryString("upload_expired")
		row.Epoch++
		return saveOriginalUpload(tx, row, view, now)
	}
	if view.State == "receiving" {
		active := row.LeaseUntil != nil && now.Before(*row.LeaseUntil)
		if active {
			var slot entity.KnowledgeOriginalReceiveSlot
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("upload_id = ? AND epoch = ? AND lease_until > ?", row.ID, row.Epoch, now).Take(&slot).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			active = err == nil
		}
		if !active {
			view.State, view.Code = "interrupted", directoryString("upload_interrupted")
			row.Epoch++
			row.LeaseUntil = nil
			return saveOriginalUpload(tx, row, view, now)
		}
	}
	return nil
}

func loadOriginalUpload(tx *gorm.DB, kb *entity.Knowledgebase, user, registration, id, key string, now time.Time) (*entity.KnowledgeOriginalUploadRecord, *entity.KnowledgeOriginalUpload, error) {
	if _, err := originalRegistration(tx, kb.ID, registration); err != nil {
		return nil, nil, err
	}
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("registration_id = ? AND domain_id = ? AND owner_id = ?", registration, kb.ID, user)
	if key != "" {
		if !originalKey(key, 128) {
			return nil, nil, originalError(400, "knowledge_invalid_request")
		}
		query = query.Where("idempotency_key = ?", key)
	} else {
		if !isKnowledgeID(id, 32) {
			return nil, nil, originalError(404, "knowledge_not_found")
		}
		query = query.Where("id = ?", id)
	}
	var row entity.KnowledgeOriginalUploadRecord
	if err := query.Take(&row).Error; err != nil {
		return nil, nil, err
	}
	view, err := originalUploadView(&row)
	if err != nil {
		return nil, nil, err
	}
	if err := refreshOriginalUpload(tx, &row, view, now); err != nil {
		return nil, nil, err
	}
	if view.State != "committed" && view.PolicyVersion != kb.KnowledgePolicyVersion {
		if !originalTerminal(view.State) {
			view.State, view.Code = "rejected", directoryString("policy_changed")
			row.Epoch++
			if err := saveOriginalUpload(tx, &row, view, now); err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, &originalCommitError{originalError(409, "policy_changed")}
	}
	if err := knowledgeImportClassification(tx, kb.ID, view.Classification.GroupID, view.Classification.ComponentSide, view.Classification.Language); err != nil {
		return nil, nil, err
	}
	return &row, view, nil
}

// CreateKnowledgeOriginalUpload reserves bounded capacity without creating a source.
func (d *DatasetService) CreateKnowledgeOriginalUpload(ctx context.Context, user, domain, registration string, req entity.KnowledgeOriginalUploadRequest) (*entity.KnowledgeOriginalUpload, bool, error) {
	if _, err := d.knowledgeBase(ctx, user, domain, true); err != nil {
		return nil, false, originalNormalize(err)
	}
	if !originalKey(req.IdempotencyKey, 128) || req.DeclaredByteLength < 1 || req.DeclaredByteLength > entity.KnowledgeOriginalMaxBytes || (req.ExpectedSHA256 != nil && !isKnowledgeID(*req.ExpectedSHA256, 64)) || (req.ExpectedRevisionID != nil && !isKnowledgeID(*req.ExpectedRevisionID, 32)) {
		return nil, false, originalError(400, "knowledge_invalid_request")
	}
	if _, err := inspectKnowledgeOriginalNames(req.OriginalFilename, req.RelativePath, req.DeclaredMediaType); err != nil {
		return nil, false, originalError(415, "upload_format_unsupported")
	}
	var result *entity.KnowledgeOriginalUpload
	created := false
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		reg, err := originalRegistration(tx, domain, registration)
		if err != nil {
			return err
		}
		hash := originalUploadDigest(domain, user, registration, req)
		var replay entity.KnowledgeOriginalUploadRecord
		err = tx.Where("registration_id = ? AND idempotency_key = ?", registration, req.IdempotencyKey).Take(&replay).Error
		if err == nil {
			if replay.RequestSHA256 != hash {
				return originalError(409, "idempotency_conflict")
			}
			_, result, err = loadOriginalUpload(tx, kb, user, registration, replay.ID, "", now)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !slices.Contains(reg.IncludePaths, req.RelativePath) {
			return originalError(400, "knowledge_invalid_request")
		}
		c := req.Classification
		if err := knowledgeImportClassification(tx, domain, c.GroupID, c.ComponentSide, c.Language); err != nil {
			return err
		}
		_, current, err := originalCurrent(tx, domain, registration, req.RelativePath)
		if err != nil {
			return err
		}
		expected := ""
		if req.ExpectedRevisionID != nil {
			expected = *req.ExpectedRevisionID
		}
		if (current == nil && expected != "") || (current != nil && current.ID != expected) {
			return originalError(409, "revision_changed")
		}
		var quota struct {
			Count    int64
			Reserved int64
		}
		if err := tx.Model(&entity.KnowledgeOriginalUploadRecord{}).Select("COUNT(*) AS count, COALESCE(SUM(declared_bytes), 0) AS reserved").Where("domain_id = ? AND owner_id = ? AND expires_at > ? AND state NOT IN ?", domain, user, now, []string{"rejected", "cancelled", "expired", "committed"}).Scan(&quota).Error; err != nil {
			return err
		}
		if quota.Count >= 2 || quota.Reserved+req.DeclaredByteLength > 2*entity.KnowledgeOriginalMaxBytes {
			return originalError(413, "upload_quota_exceeded")
		}
		result = &entity.KnowledgeOriginalUpload{UploadID: common.GenerateUUID(), DomainID: domain, OwnerID: user,
			RegistrationID: registration, IdempotencyKey: req.IdempotencyKey, RequestSHA256: hash,
			OriginalFilename: req.OriginalFilename, RelativePath: req.RelativePath, Classification: c,
			DeclaredByteLength: req.DeclaredByteLength, DeclaredMediaType: req.DeclaredMediaType, ExpectedSHA256: req.ExpectedSHA256,
			ExpectedRevisionID: req.ExpectedRevisionID, PolicyVersion: kb.KnowledgePolicyVersion, State: "awaiting_bytes",
			CreatedAt: originalTime(now), UpdatedAt: originalTime(now), ExpiresAt: originalTime(now.Add(30 * time.Minute)), Revision: 1}
		if current != nil {
			result.SourceID = directoryString(current.DocumentID)
		}
		row := entity.KnowledgeOriginalUploadRecord{ID: result.UploadID, DomainID: domain, OwnerID: user, RegistrationID: registration,
			IdempotencyKey: req.IdempotencyKey, RequestSHA256: hash, Payload: directoryJSON(result), State: result.State,
			DeclaredBytes: req.DeclaredByteLength, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}

// GetKnowledgeOriginalUpload observes the state without transferring or confirming.
func (d *DatasetService) GetKnowledgeOriginalUpload(ctx context.Context, user, domain, registration, id, key string) (*entity.KnowledgeOriginalUpload, error) {
	var result *entity.KnowledgeOriginalUpload
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, view, err := loadOriginalUpload(tx, kb, user, registration, id, key, now)
		result = view
		return err
	})
	return result, err
}

// CancelKnowledgeOriginalUpload fences a pending epoch, retaining fixed history.
func (d *DatasetService) CancelKnowledgeOriginalUpload(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeOriginalUpload, error) {
	var result *entity.KnowledgeOriginalUpload
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadOriginalUpload(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		if !originalTerminal(view.State) {
			view.State, view.Code = "cancelled", directoryString("upload_cancelled")
			row.Epoch++
			if err := saveOriginalUpload(tx, row, view, now); err != nil {
				return err
			}
		}
		result = view
		return nil
	})
	return result, err
}
