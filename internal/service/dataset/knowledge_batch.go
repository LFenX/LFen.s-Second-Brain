package dataset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

func batchError(status int, code, reason string) error {
	e := &KnowledgeDirectoryError{Status: status, Code: "knowledge_batch_" + code}
	if reason != "" {
		e.Reason = &reason
	}
	return e
}
func batchConflict(reason string) error { return batchError(409, "conflict", reason) }
func batchInvalid() error               { return batchError(400, "invalid", "") }
func batchMissing() error               { return batchError(404, "not_found", "") }
func batchNormalize(err error) error {
	if err == nil {
		return nil
	}
	var e *KnowledgeDirectoryError
	if errors.As(err, &e) {
		copy := *e
		copy.Code = strings.Replace(copy.Code, "knowledge_directory_", "knowledge_batch_", 1)
		if copy.Status == 415 {
			return batchInvalid()
		}
		return &copy
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return batchMissing()
	}
	normalized := directoryNormalizeError(err)
	if errors.As(normalized, &e) {
		return batchNormalize(normalized)
	}
	return batchError(503, "unavailable", "result_unknown")
}
func (d *DatasetService) batchTransaction(ctx context.Context, user, domain string, timeout time.Duration, fn func(*gorm.DB, *entity.Knowledgebase) error) error {
	return batchNormalize(d.directoryTransaction(ctx, user, domain, timeout, fn))
}

// batchNow uses database UTC time for leases across API processes.
func batchNow(tx *gorm.DB) (time.Time, error) {
	query := "SELECT CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6)) * 1000000 AS SIGNED)"
	switch tx.Dialector.Name() {
	case "sqlite":
		query = "SELECT CAST((julianday('now') - 2440587.5) * 86400000000 AS INTEGER)"
	case "postgres":
		query = "SELECT CAST(EXTRACT(EPOCH FROM clock_timestamp()) * 1000000 AS BIGINT)"
	}
	var value int64
	if err := tx.Raw(query).Scan(&value).Error; err != nil {
		return time.Time{}, err
	}
	if value <= 0 {
		return time.Time{}, errors.New("database UTC clock unavailable")
	}
	return time.UnixMicro(value).UTC(), nil
}
func batchKey(value string) bool {
	return len(value) >= 1 && len(value) <= 128 && knowledgeImportKey.MatchString(value)
}
func batchTerminal(status string) bool {
	return slices.Contains([]string{"succeeded", "partial_failed", "failed", "cancelled"}, status)
}
func batchPreviewDigest(p *entity.KnowledgeBatchPreview, user string) string {
	parts := []any{"lfen.batch.preview.v0.4", p.PreviewID, p.DomainID, user, p.RegistrationID, p.ObservationID, p.ObservationSHA256, p.RootID, p.RootVersion, p.PolicyVersion, p.CreatedAt, p.ExpiresAt, len(p.Items)}
	for _, i := range p.Items {
		parts = append(parts, i.ItemID, i.Ordinal, i.RelativePath, i.SourceID, i.ExpectedRevisionID, i.SHA256, i.ByteLength, i.Classification.GroupID, i.Classification.ComponentSide, i.Classification.Language, i.Action)
	}
	return directorySHA([]byte(directoryTokens(parts...)))
}
func loadBatchPreview(tx *gorm.DB, user, domain, registration, id string) (*entity.KnowledgeBatchPreviewRecord, *entity.KnowledgeBatchPreview, error) {
	if !isKnowledgeID(id, 32) {
		return nil, nil, batchMissing()
	}
	var row entity.KnowledgeBatchPreviewRecord
	if e := tx.Where("id = ? AND domain_id = ? AND user_id = ? AND registration_id = ?", id, domain, user, registration).Take(&row).Error; e != nil {
		return nil, nil, e
	}
	var p entity.KnowledgeBatchPreview
	if json.Unmarshal([]byte(row.Payload), &p) != nil || batchPreviewDigest(&p, user) != p.ManifestSHA256 {
		return nil, nil, batchConflict("observation_mismatch")
	}
	p.ConsumedTaskID = row.ConsumedTaskID
	return &row, &p, nil
}
func (d *DatasetService) PrepareKnowledgeBatch(ctx context.Context, user, domain, registration string, req entity.KnowledgeBatchPreviewRequest) (*entity.KnowledgeBatchPreview, error) {
	if !isKnowledgeID(req.ObservationID, 32) || !isKnowledgeID(req.ObservationSHA256, 64) || len(req.SelectedItems) < 1 || len(req.SelectedItems) > 32 {
		return nil, batchInvalid()
	}
	for n, i := range req.SelectedItems {
		if !utility.IsKnowledgeDirectoryPath(i.RelativePath, false) || (n > 0 && strings.Compare(req.SelectedItems[n-1].RelativePath, i.RelativePath) >= 0) {
			return nil, batchInvalid()
		}
	}
	var result *entity.KnowledgeBatchPreview
	err := d.batchTransaction(ctx, user, domain, 30*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, e := loadDirectoryScope(tx, user, domain, registration)
		if e != nil {
			return e
		}
		obs, private, e := loadDirectoryObservation(tx, scope, user, req.ObservationID, kb)
		if e != nil {
			return e
		}
		if !obs.Complete {
			return batchConflict("observation_incomplete")
		}
		if obs.ObservationSHA256 != req.ObservationSHA256 {
			return batchConflict("observation_mismatch")
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		expiry, e := time.Parse(time.RFC3339Nano, obs.ExpiresAt)
		if e != nil {
			return e
		}
		if expiry.After(now.Add(10 * time.Minute)) {
			expiry = now.Add(10 * time.Minute)
		}
		if !now.Before(expiry) {
			return batchError(410, "expired", "")
		}
		root, e := directoryOpenRoot(tx, scope.Config)
		if e != nil {
			return e
		}
		defer root.Close()
		p := &entity.KnowledgeBatchPreview{PreviewID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration, ObservationID: obs.ObservationID, ObservationSHA256: obs.ObservationSHA256, RootID: scope.Config.RootID, RootVersion: scope.Config.RootVersion, PolicyVersion: kb.KnowledgePolicyVersion, Mode: "catalogue_only", CreatedAt: directoryTime(now), ExpiresAt: directoryTime(expiry), Items: []entity.KnowledgeBatchPreviewItem{}}
		records := make([]entity.KnowledgeBatchItemRecord, 0, len(req.SelectedItems))
		for n, selection := range req.SelectedItems {
			if !scope.contains(selection.RelativePath) {
				return batchInvalid()
			}
			idx := slices.IndexFunc(obs.Items, func(i entity.KnowledgeDirectoryObservationItem) bool { return i.RelativePath == selection.RelativePath })
			if idx < 0 {
				return batchConflict("observation_mismatch")
			}
			observed := obs.Items[idx]
			if !observed.Eligible || observed.State != "verified" || observed.SHA256 == nil || observed.ByteLength == nil {
				return batchConflict("ineligible")
			}
			c := selection.Classification
			if e = knowledgeImportClassification(tx, domain, c.GroupID, c.ComponentSide, c.Language); e != nil {
				return e
			}
			if _, e = directoryVerifyHead(tx, domain, registration, selection.RelativePath, observed.SourceID, observed.ExpectedRevisionID); e != nil {
				return e
			}
			readCtx, cancel := context.WithTimeout(tx.Statement.Context, 10*time.Second)
			body, identity, readErr := utility.ReadKnowledgeDirectoryFile(readCtx, root, selection.RelativePath, entity.KnowledgeMaxSourceBytes)
			cancel()
			if readErr != nil || identity != private.Identities[selection.RelativePath] || int64(len(body)) != *observed.ByteLength || directorySHA(body) != *observed.SHA256 || directoryTextReason(selection.RelativePath, body) != "" {
				return batchConflict("source_changed")
			}
			_, current, e := directoryCurrent(tx, domain, registration, selection.RelativePath)
			if e != nil {
				return e
			}
			item := entity.KnowledgeBatchPreviewItem{ItemID: common.GenerateUUID(), Ordinal: int64(n), RelativePath: selection.RelativePath, Classification: c, SourceID: observed.SourceID, ExpectedRevisionID: observed.ExpectedRevisionID, SHA256: *observed.SHA256, ByteLength: *observed.ByteLength, Action: directoryAction(*observed.SHA256, selection.RelativePath, c, current)}
			if e := lifecycleCopyFence(tx, domain, "directory_observation", obs.ObservationID, selection.RelativePath, "text_preview", item.ItemID, item.RelativePath, lifecycleSourceValue(item.SourceID)); e != nil {
				return batchConflict("revision_changed")
			}
			p.Items = append(p.Items, item)
			p.ByteLength += item.ByteLength
			if p.ByteLength > 64<<20 {
				return batchError(413, "too_large", "")
			}
			records = append(records, entity.KnowledgeBatchItemRecord{ID: item.ItemID, PreviewID: p.PreviewID, Ordinal: item.Ordinal, Status: "pending", PreviewJSON: directoryJSON(item), ResultJSON: "{}", Content: string(body), Identity: identity})
		}
		latest, e := directoryConfig(user, domain, scope.Config.RootID)
		if e != nil {
			return e
		}
		if latest.RootVersion != p.RootVersion {
			return batchConflict("root_changed")
		}
		end, e := batchNow(tx)
		if e != nil {
			return e
		}
		if !end.Before(expiry) {
			return batchError(410, "expired", "")
		}
		// Only a complete explicit preparation replaces unconsumed previews.
		// Both replacement and byte cleanup roll back if the new insert fails.
		previous := tx.Model(&entity.KnowledgeBatchPreviewRecord{}).Select("id").Where("domain_id = ? AND user_id = ? AND consumed_task_id IS NULL", domain, user)
		if e = tx.Model(&entity.KnowledgeBatchItemRecord{}).Where("preview_id IN (?) AND task_id IS NULL", previous).Updates(map[string]any{"content": "", "identity": ""}).Error; e != nil {
			return e
		}
		if e = tx.Model(&entity.KnowledgeBatchPreviewRecord{}).Where("domain_id = ? AND user_id = ? AND consumed_task_id IS NULL AND expires_at > ?", domain, user, end).Update("expires_at", end).Error; e != nil {
			return e
		}
		p.ManifestSHA256 = batchPreviewDigest(p, user)
		row := entity.KnowledgeBatchPreviewRecord{ID: p.PreviewID, DomainID: domain, UserID: user, RegistrationID: registration, Payload: directoryJSON(p), CreatedAt: now, ExpiresAt: expiry}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		for n := range records {
			if e = tx.Create(&records[n]).Error; e != nil {
				return e
			}
		}
		result = p
		return nil
	})
	return result, err
}
func (d *DatasetService) GetKnowledgeBatchPreview(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeBatchPreview, error) {
	var out *entity.KnowledgeBatchPreview
	err := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, e := loadDirectoryScope(tx, user, domain, registration)
		if e != nil {
			return e
		}
		row, p, e := loadBatchPreview(tx, user, domain, registration, id)
		if e != nil {
			return e
		}
		if row.ConsumedTaskID == nil {
			now, e := batchNow(tx)
			if e != nil {
				return e
			}
			if !now.Before(row.ExpiresAt) {
				return batchError(410, "expired", "")
			}
			if e = directoryCheckVersions(scope, kb, p.RootVersion, p.PolicyVersion); e != nil {
				return e
			}
		}
		out = p
		return nil
	})
	return out, err
}
func (d *DatasetService) ReadKnowledgeBatchPreviewItem(ctx context.Context, user, domain, registration, preview, item string, offset, length int64) (*entity.KnowledgeBatchContentWindow, error) {
	if offset < 0 || length < 1 || length > 65536 {
		return nil, batchError(416, "range", "")
	}
	var out *entity.KnowledgeBatchContentWindow
	err := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, e := loadDirectoryScope(tx, user, domain, registration)
		if e != nil {
			return e
		}
		row, p, e := loadBatchPreview(tx, user, domain, registration, preview)
		if e != nil {
			return e
		}
		if row.ConsumedTaskID != nil {
			return batchConflict("preview_consumed")
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		if !now.Before(row.ExpiresAt) {
			return batchError(410, "expired", "")
		}
		if e = directoryCheckVersions(scope, kb, p.RootVersion, p.PolicyVersion); e != nil {
			return e
		}
		var record entity.KnowledgeBatchItemRecord
		if !isKnowledgeID(item, 32) {
			return batchMissing()
		}
		if e = tx.Where("id = ? AND preview_id = ?", item, preview).Take(&record).Error; e != nil {
			return e
		}
		var i entity.KnowledgeBatchPreviewItem
		if json.Unmarshal([]byte(record.PreviewJSON), &i) != nil || directorySHA([]byte(record.Content)) != i.SHA256 || int64(len(record.Content)) != i.ByteLength {
			return batchConflict("observation_mismatch")
		}
		if offset > i.ByteLength {
			return batchError(416, "range", "")
		}
		end := min(offset+length, i.ByteLength)
		text := record.Content[offset:end]
		if !utf8.ValidString(text) {
			return batchError(416, "range", "")
		}
		out = &entity.KnowledgeBatchContentWindow{PreviewID: preview, ItemID: item, RelativePath: i.RelativePath, SHA256: i.SHA256, TotalBytes: i.ByteLength, Offset: offset, ByteLength: end - offset, Text: text}
		if end < i.ByteLength {
			out.NextOffset = &end
		}
		return nil
	})
	return out, err
}

type batchCursor struct {
	View     string `json:"v"`
	Position int    `json:"p"`
	Revision int64  `json:"r"`
}

func encodeBatchCursor(id string, pos int, revision int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(directoryJSON(batchCursor{id, pos, revision})))
}
func decodeBatchCursor(value string) (batchCursor, error) {
	var c batchCursor
	if len(value) > 1024 {
		return c, batchConflict("cursor_invalid")
	}
	raw, e := base64.RawURLEncoding.DecodeString(value)
	if e != nil || json.Unmarshal(raw, &c) != nil || !isKnowledgeID(c.View, 32) || c.Position < 1 || c.Revision < 0 || encodeBatchCursor(c.View, c.Position, c.Revision) != value {
		return c, batchConflict("cursor_invalid")
	}
	return c, nil
}

func batchUnmarshal[T any](value string) (*T, error) {
	var out T
	if e := json.Unmarshal([]byte(value), &out); e != nil {
		return nil, fmt.Errorf("invalid persisted batch record: %w", e)
	}
	return &out, nil
}
