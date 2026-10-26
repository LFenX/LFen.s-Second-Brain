package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/syncer/connector"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// KnowledgeWebError retains the bounded upstream failure independently of the
// local authentication status and public machine code.
type KnowledgeWebError struct {
	Public  *KnowledgeError
	Failure entity.KnowledgeWebFailure
}

func (e *KnowledgeWebError) Error() string { return e.Public.Error() }
func (e *KnowledgeWebError) Unwrap() error { return e.Public }
func webError(status int, stage, code string) error {
	return &KnowledgeWebError{Public: &KnowledgeError{Status: status, Code: code, Message: "Web operation could not be completed"}, Failure: entity.KnowledgeWebFailure{Stage: stage, MachineCode: code, OutcomeKnown: code != "web_result_unknown"}}
}

func (d *DatasetService) webTransaction(ctx context.Context, user, domain string, fn func(*gorm.DB, *entity.Knowledgebase, time.Time) error) error {
	err := d.originalTransaction(ctx, user, domain, fn)
	var public *KnowledgeError
	if errors.As(err, &public) && public.Code == "result_unknown" {
		return webError(503, "read", "web_result_unknown")
	}
	return err
}

func webClassification(tx *gorm.DB, domain string, c entity.KnowledgeDirectoryClassification) error {
	if !validKnowledgeSide(c.ComponentSide) || !utf8.ValidString(c.Language) || utf8.RuneCountInString(c.Language) > 32 {
		return webError(400, "create", "knowledge_invalid_request")
	}
	if c.GroupID != nil {
		if !isKnowledgeID(*c.GroupID, 32) {
			return webError(400, "create", "knowledge_invalid_request")
		}
		var group entity.KnowledgeGroup
		if err := tx.Where("id = ? AND domain_id = ?", *c.GroupID, domain).Take(&group).Error; err != nil {
			return err
		}
	}
	return nil
}

func loadWebRegistration(tx *gorm.DB, user, domain, id, key string) (*entity.KnowledgeWebRegistrationRecord, *entity.KnowledgeWebRegistration, error) {
	var native entity.KnowledgeSourceRegistration
	query := tx.Where("domain_id = ?", domain)
	if key != "" {
		if !originalKey(key, 128) {
			return nil, nil, webError(400, "read", "knowledge_invalid_request")
		}
		query = query.Where("registration_key = ?", key)
	} else {
		if !isKnowledgeID(id, 32) {
			return nil, nil, webError(404, "read", "knowledge_not_found")
		}
		query = query.Where("id = ?", id)
	}
	if err := query.Take(&native).Error; err != nil {
		return nil, nil, err
	}
	if !slices.Contains([]string{"web_page", "web_feed"}, native.SourceKind) || native.InputMode != "web_capture" {
		return nil, nil, webError(404, "read", "knowledge_not_found")
	}
	var row entity.KnowledgeWebRegistrationRecord
	if err := tx.Where("id = ? AND domain_id = ? AND owner_id = ?", native.ID, domain, user).Take(&row).Error; err != nil {
		return nil, nil, err
	}
	var view entity.KnowledgeWebRegistration
	if json.Unmarshal([]byte(row.Payload), &view) != nil || view.RegistrationID != row.ID || view.DomainID != domain || view.OwnerID != user || view.ConfigSHA256 != native.ConfigHash || view.RegistrationKey != native.RegistrationKey || view.SourceKind != native.SourceKind {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	if err := webClassification(tx, domain, view.DefaultClassification); err != nil {
		return nil, nil, err
	}
	return &row, &view, nil
}

// CreateKnowledgeWebRegistration binds an immutable web config and its native
// folder; it performs no network acquisition, parsing or model execution.
func (d *DatasetService) CreateKnowledgeWebRegistration(ctx context.Context, user, domain string, request entity.KnowledgeWebCreateRegistration) (*entity.KnowledgeWebRegistration, bool, error) {
	var result *entity.KnowledgeWebRegistration
	created := false
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		if !originalKey(request.RegistrationKey, 128) || !validKnowledgeText(request.Name, 128) || !slices.Contains([]string{"web_page", "web_feed"}, request.SourceKind) || !slices.Contains([]string{"direct", "rsshub", "other_feed", "browser_export"}, request.Producer) {
			return webError(400, "create", "knowledge_invalid_request")
		}
		origin, err := connector.NormalizeKnowledgeWebURL("", request.OriginURL, false)
		if err != nil {
			return webError(400, "create", "knowledge_invalid_request")
		}
		request.OriginURL = origin
		scope, err := connector.NormalizeKnowledgeWebScope(origin, request.NetworkScope)
		if err != nil {
			return webError(400, "create", "knowledge_invalid_request")
		}
		request.NetworkScope = scope
		if err := webClassification(tx, domain, request.DefaultClassification); err != nil {
			return err
		}
		config, err := entity.KnowledgeWebDigest("registration", request)
		if err != nil {
			return err
		}
		scopeHash, err := entity.KnowledgeWebDigest("network_scope", scope)
		if err != nil {
			return err
		}
		var native entity.KnowledgeSourceRegistration
		err = tx.Where("domain_id = ? AND registration_key = ?", domain, request.RegistrationKey).Take(&native).Error
		if err == nil {
			if native.ConfigHash != config || native.SourceKind != request.SourceKind || native.InputMode != "web_capture" {
				return webError(409, "create", "registration_conflict")
			}
			_, result, err = loadWebRegistration(tx, user, domain, native.ID, "")
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		id, folderID := common.GenerateUUID(), common.GenerateUUID()
		c := request.DefaultClassification
		native = entity.KnowledgeSourceRegistration{ID: id, DomainID: domain, RegistrationKey: request.RegistrationKey, Name: request.Name, SourceKind: request.SourceKind, InputMode: "web_capture", IncludePaths: []string{}, GroupID: c.GroupID, ComponentSide: c.ComponentSide, Language: c.Language, ConfigHash: config, CreatedAt: now}
		result = &entity.KnowledgeWebRegistration{RegistrationKey: request.RegistrationKey, Name: request.Name, SourceKind: request.SourceKind, OriginURL: origin, Producer: request.Producer, DefaultClassification: c, NetworkScope: scope, DomainID: domain, OwnerID: user, RegistrationID: id, ConfigSHA256: config, ScopeSHA256: scopeHash, CreatedAt: originalTime(now), PolicyVersion: kb.KnowledgePolicyVersion}
		row := entity.KnowledgeWebRegistrationRecord{ID: id, DomainID: domain, OwnerID: user, FolderID: folderID, Payload: directoryJSON(result), CreatedAt: now}
		folder := entity.File{ID: folderID, ParentID: *kb.KnowledgeFolderID, TenantID: kb.TenantID, CreatedBy: user, Name: request.Name, Type: "folder", SourceType: "knowledgebase", KnowledgePrivateOwnerID: &user}
		for _, record := range []any{&native, &row, &folder} {
			if err := tx.Create(record).Error; err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	return result, created, err
}

// GetKnowledgeWebRegistration resolves its fixed ID or exact original key.
func (d *DatasetService) GetKnowledgeWebRegistration(ctx context.Context, user, domain, id, key string) (*entity.KnowledgeWebRegistration, error) {
	var result *entity.KnowledgeWebRegistration
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		_, view, err := loadWebRegistration(tx, user, domain, id, key)
		result = view
		return err
	})
	return result, err
}

func readWebCapture(row *entity.KnowledgeWebCaptureRecord) (*entity.KnowledgeWebCapture, error) {
	var view entity.KnowledgeWebCapture
	if json.Unmarshal([]byte(row.Payload), &view) != nil || view.CaptureID != row.ID || view.DomainID != row.DomainID || view.OwnerID != row.OwnerID || view.RegistrationID != row.RegistrationID || view.IdempotencyKey != row.IdempotencyKey || view.RequestSHA256 != row.RequestSHA256 || view.State != row.State {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	if view.State == "ready" && (view.RawSHA256 == nil || !isKnowledgeID(*view.RawSHA256, 64) || view.RawByteLength == nil || *view.RawByteLength < 1 || *view.RawByteLength > 2<<20 || view.MediaType == nil || !slices.Contains([]string{"text/html", "application/rss+xml", "application/atom+xml"}, *view.MediaType)) {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	return &view, nil
}
func saveWebCapture(tx *gorm.DB, row *entity.KnowledgeWebCaptureRecord, view *entity.KnowledgeWebCapture, now time.Time) error {
	view.Revision++
	view.UpdatedAt = originalTime(now)
	row.Payload = directoryJSON(view)
	row.State = view.State
	row.UpdatedAt = now
	return tx.Model(&entity.KnowledgeWebCaptureRecord{}).Where("id = ?", row.ID).UpdateColumns(map[string]any{"payload": row.Payload, "state": row.State, "updated_at": now, "epoch": row.Epoch, "attempted": row.Attempted, "lease_until": row.LeaseUntil, "object_folder_id": row.ObjectFolderID}).Error
}

func refreshWebCapture(tx *gorm.DB, row *entity.KnowledgeWebCaptureRecord, view *entity.KnowledgeWebCapture, now time.Time) error {
	if slices.Contains([]string{"expired", "cancelled", "rejected"}, view.State) {
		return nil
	}
	code := ""
	if !now.Before(row.ExpiresAt) {
		view.State = "expired"
		code = "web_capture_expired"
	} else if slices.Contains([]string{"receiving", "fetching"}, view.State) {
		var active int64
		if row.LeaseUntil != nil && now.Before(*row.LeaseUntil) {
			if err := tx.Model(&entity.KnowledgeWebSlot{}).Where("capture_id = ? AND epoch = ? AND lease_until > ? AND operation = ?", row.ID, row.Epoch, now, view.State).Count(&active).Error; err != nil {
				return err
			}
		}
		if active == 0 {
			view.State = "interrupted"
			code = "web_capture_interrupted"
		}
	}
	if code != "" {
		row.Epoch++
		row.LeaseUntil = nil
		view.Failure = &entity.KnowledgeWebFailure{Stage: "read", MachineCode: code, OutcomeKnown: true}
		return saveWebCapture(tx, row, view, now)
	}
	return nil
}

func loadWebCapture(tx *gorm.DB, kb *entity.Knowledgebase, user, registration, id, key string, now time.Time) (*entity.KnowledgeWebCaptureRecord, *entity.KnowledgeWebCapture, error) {
	registrationRow, reg, err := loadWebRegistration(tx, user, kb.ID, registration, "")
	if err != nil {
		return nil, nil, err
	}
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("domain_id = ? AND owner_id = ? AND registration_id = ?", kb.ID, user, registration)
	if key != "" {
		if !originalKey(key, 128) {
			return nil, nil, webError(400, "read", "knowledge_invalid_request")
		}
		query = query.Where("idempotency_key = ?", key)
	} else {
		if !isKnowledgeID(id, 32) {
			return nil, nil, webError(404, "read", "knowledge_not_found")
		}
		query = query.Where("id = ?", id)
	}
	var row entity.KnowledgeWebCaptureRecord
	if err := query.Take(&row).Error; err != nil {
		return nil, nil, err
	}
	view, err := readWebCapture(&row)
	if err != nil {
		return nil, nil, err
	}
	if view.State == "ready" && row.ObjectFolderID != registrationRow.FolderID {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	if view.PolicyVersion != kb.KnowledgePolicyVersion || view.RegistrationConfigSHA256 != reg.ConfigSHA256 || view.ScopeSHA256 != reg.ScopeSHA256 {
		return nil, nil, webError(409, "read", "policy_changed")
	}
	if directoryJSON(view.ProducerVersions) != directoryJSON(connector.KnowledgeWebProducerVersions()) {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	if err := refreshWebCapture(tx, &row, view, now); err != nil {
		return nil, nil, err
	}
	return &row, view, nil
}

// CreateKnowledgeWebCapture accepts exactly one of the two frozen request
// types. Creation only reserves capacity; acquisition is a separate action.
func (d *DatasetService) CreateKnowledgeWebCapture(ctx context.Context, user, domain, registration string, request any) (*entity.KnowledgeWebCapture, bool, error) {
	var result *entity.KnowledgeWebCapture
	created := false
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, reg, err := loadWebRegistration(tx, user, domain, registration, "")
		if err != nil {
			return err
		}
		var local *entity.KnowledgeWebCreateLocalCapture
		mode, key, config := "", "", ""
		switch req := request.(type) {
		case entity.KnowledgeWebCreateLocalCapture:
			local = &req
			mode, key, config = req.AcquisitionMode, req.IdempotencyKey, req.RegistrationConfigSHA256
			if mode != "local_snapshot" {
				return webError(400, "create", "knowledge_invalid_request")
			}
		case entity.KnowledgeWebCreateFetchCapture:
			mode, key, config = req.AcquisitionMode, req.IdempotencyKey, req.RegistrationConfigSHA256
			if mode != "explicit_fetch" {
				return webError(400, "create", "knowledge_invalid_request")
			}
		default:
			return webError(400, "create", "knowledge_invalid_request")
		}
		if !originalKey(key, 128) || config != reg.ConfigSHA256 {
			return webError(400, "create", "knowledge_invalid_request")
		}
		var declaration any
		reserved := int64(2 << 20)
		if local != nil {
			if !validKnowledgeText(local.OriginalFilename, 256) || local.DeclaredByteLength < 1 || local.DeclaredByteLength > 2<<20 || !slices.Contains([]string{"text/html", "application/rss+xml", "application/atom+xml", "application/xml", "text/xml"}, local.DeclaredMediaType) || (local.ExpectedSHA256 != nil && !isKnowledgeID(*local.ExpectedSHA256, 64)) {
				return webError(400, "create", "knowledge_invalid_request")
			}
			if local.DeclaredFinalURL != nil {
				value, err := connector.NormalizeKnowledgeWebURL("", *local.DeclaredFinalURL, false)
				if err != nil {
					return webError(400, "create", "knowledge_invalid_request")
				}
				local.DeclaredFinalURL = &value
			}
			if local.ClaimedCapturedAt != nil {
				if _, err := time.Parse(time.RFC3339Nano, *local.ClaimedCapturedAt); err != nil {
					return webError(400, "create", "knowledge_invalid_request")
				}
			}
			declaration = map[string]any{"original_filename": local.OriginalFilename, "declared_byte_length": local.DeclaredByteLength, "declared_media_type": local.DeclaredMediaType, "expected_sha256": local.ExpectedSHA256, "declared_final_url": local.DeclaredFinalURL, "claimed_captured_at": local.ClaimedCapturedAt}
			reserved = local.DeclaredByteLength
		} else if !reg.NetworkScope.Enabled {
			return webError(409, "create", "web_remote_blocked")
		}
		hash, err := entity.KnowledgeWebDigest("capture_request", map[string]any{"registration_id": registration, "registration_config_sha256": config, "acquisition_mode": mode, "local_declaration": declaration})
		if err != nil {
			return err
		}
		var prior entity.KnowledgeWebCaptureRecord
		err = tx.Where("registration_id = ? AND idempotency_key = ?", registration, key).Take(&prior).Error
		if err == nil {
			if prior.RequestSHA256 != hash {
				return webError(409, "create", "idempotency_conflict")
			}
			_, result, err = loadWebCapture(tx, kb, user, registration, prior.ID, "", now)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var usage struct {
			Count int64
			Bytes int64
		}
		if err := tx.Model(&entity.KnowledgeWebCaptureRecord{}).Select("COUNT(*) AS count, COALESCE(SUM(reserved_bytes), 0) AS bytes").Where("domain_id = ? AND owner_id = ? AND state NOT IN ? AND expires_at > ?", domain, user, []string{"rejected", "cancelled", "expired"}, now).Scan(&usage).Error; err != nil {
			return err
		}
		if usage.Count >= 2 || usage.Bytes+reserved > 4<<20 {
			return webError(429, "create", "web_capture_quota_exceeded")
		}
		state, provenance := "awaiting_fetch", entity.KnowledgeWebProvenance{Kind: "server_http", RequestedURL: reg.OriginURL, RedirectChain: []entity.KnowledgeWebRedirect{}}
		id := common.GenerateUUID()
		expires := now.Add(30 * time.Minute)
		result = &entity.KnowledgeWebCapture{DomainID: domain, OwnerID: user, RegistrationID: registration, CaptureID: id, IdempotencyKey: key, RequestSHA256: hash, RegistrationConfigSHA256: config, ScopeSHA256: reg.ScopeSHA256, AcquisitionMode: mode, State: state, Revision: 1, PolicyVersion: kb.KnowledgePolicyVersion, ProducerVersions: connector.KnowledgeWebProducerVersions(), CreatedAt: originalTime(now), UpdatedAt: originalTime(now), ExpiresAt: originalTime(expires), Provenance: provenance}
		if local != nil {
			result.State = "awaiting_bytes"
			result.OriginalFilename = &local.OriginalFilename
			result.DeclaredByteLength = &local.DeclaredByteLength
			result.DeclaredMediaType = &local.DeclaredMediaType
			result.ExpectedSHA256 = local.ExpectedSHA256
			result.Provenance.Kind = "user_supplied"
			result.Provenance.FinalURL = local.DeclaredFinalURL
			result.Provenance.ClaimedCapturedAt = local.ClaimedCapturedAt
		}
		storedRequest := request
		if local != nil {
			storedRequest = *local
		}
		row := entity.KnowledgeWebCaptureRecord{ID: id, DomainID: domain, OwnerID: user, RegistrationID: registration, IdempotencyKey: key, RequestSHA256: hash, Request: directoryJSON(storedRequest), Payload: directoryJSON(result), State: result.State, ReservedBytes: reserved, CreatedAt: now, UpdatedAt: now, ExpiresAt: expires}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}

// GetKnowledgeWebCapture recovers existing state without performing an attempt.
func (d *DatasetService) GetKnowledgeWebCapture(ctx context.Context, user, domain, registration, id, key string) (*entity.KnowledgeWebCapture, error) {
	var result *entity.KnowledgeWebCapture
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, view, err := loadWebCapture(tx, kb, user, registration, id, key, now)
		result = view
		return err
	})
	return result, err
}

// CancelKnowledgeWebCapture retains all bytes and fixed receipts. It only
// fences an unconsumed acquisition/preview context with a higher epoch.
func (d *DatasetService) CancelKnowledgeWebCapture(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeWebCapture, error) {
	var result *entity.KnowledgeWebCapture
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadWebCapture(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		if !slices.Contains([]string{"cancelled", "expired", "rejected"}, view.State) {
			view.State = "cancelled"
			view.Failure = &entity.KnowledgeWebFailure{Stage: "read", MachineCode: "web_capture_cancelled", OutcomeKnown: true}
			row.Epoch++
			row.LeaseUntil = nil
			if err := saveWebCapture(tx, row, view, now); err != nil {
				return err
			}
		}
		result = view
		return nil
	})
	return result, err
}
