package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/syncer/connector"

	"gorm.io/gorm"
)

func readWebPreview(row *entity.KnowledgeWebPreviewRecord, user, domain, registration string) (*entity.KnowledgeWebPreview, []entity.KnowledgeWebPreviewItem, error) {
	var view entity.KnowledgeWebPreview
	var items []entity.KnowledgeWebPreviewItem
	if json.Unmarshal([]byte(row.Payload), &view) != nil || json.Unmarshal([]byte(row.Items), &items) != nil || view.PreviewID != row.ID || view.OwnerID != user || view.DomainID != domain || view.RegistrationID != registration || row.RegistrationID != registration || view.IdempotencyKey != row.IdempotencyKey || view.RequestSHA256 != row.RequestSHA256 || view.CaptureID != row.CaptureID || len(items) != int(view.SelectedCount) || len(items) < 1 || len(items) > 32 {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	hash, err := webPreviewManifest(&view, items)
	if err != nil || hash != view.ManifestSHA256 {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	return &view, items, nil
}
func webPreviewManifest(view *entity.KnowledgeWebPreview, items []entity.KnowledgeWebPreviewItem) (string, error) {
	var projection map[string]any
	if err := json.Unmarshal([]byte(directoryJSON(view)), &projection); err != nil {
		return "", err
	}
	projection["items"] = items
	return entity.KnowledgeWebDigest("preview_manifest", projection)
}
func loadWebPreview(tx *gorm.DB, user, domain, registration, id, key string) (*entity.KnowledgeWebPreviewRecord, *entity.KnowledgeWebPreview, []entity.KnowledgeWebPreviewItem, error) {
	query := tx.Where("registration_id = ?", registration)
	if key != "" {
		if !originalKey(key, 128) {
			return nil, nil, nil, webError(400, "read", "knowledge_invalid_request")
		}
		query = query.Where("idempotency_key = ?", key)
	} else {
		if !isKnowledgeID(id, 32) {
			return nil, nil, nil, webError(404, "read", "knowledge_not_found")
		}
		query = query.Where("id = ?", id)
	}
	var row entity.KnowledgeWebPreviewRecord
	if err := query.Take(&row).Error; err != nil {
		return nil, nil, nil, err
	}
	view, items, err := readWebPreview(&row, user, domain, registration)
	return &row, view, items, err
}
func saveWebPreview(tx *gorm.DB, row *entity.KnowledgeWebPreviewRecord, view *entity.KnowledgeWebPreview) error {
	view.Revision++
	row.Payload = directoryJSON(view)
	return tx.Model(&entity.KnowledgeWebPreviewRecord{}).Where("id = ?", row.ID).UpdateColumn("payload", row.Payload).Error
}
func refreshWebPreview(tx *gorm.DB, kb *entity.Knowledgebase, row *entity.KnowledgeWebPreviewRecord, view *entity.KnowledgeWebPreview, capture *entity.KnowledgeWebCapture, now time.Time) error {
	if view.State == "committed" {
		return nil
	}
	if view.PolicyVersion != kb.KnowledgePolicyVersion {
		return webError(409, "preview", "policy_changed")
	}
	if view.State == "ready" {
		if !now.Before(row.ExpiresAt) || capture.State == "expired" {
			view.State = "expired"
		} else if capture.State == "cancelled" {
			view.State = "cancelled"
		} else {
			return nil
		}
		return saveWebPreview(tx, row, view)
	}
	return nil
}
func webPreviewStateError(view *entity.KnowledgeWebPreview) error {
	code, status := "web_state_conflict", 409
	switch view.State {
	case "expired":
		code, status = "web_preview_expired", 410
	case "cancelled":
		code = "web_preview_cancelled"
	case "committed":
		code = "web_preview_consumed"
	}
	return webError(status, "confirm", code)
}

// CreateKnowledgeWebPreview fixes a complete, sorted selection and its current
// expected heads. It never refreshes the observation or acquires a website.
func (d *DatasetService) CreateKnowledgeWebPreview(ctx context.Context, user, domain, registration string, request entity.KnowledgeWebCreatePreview) (*entity.KnowledgeWebPreview, bool, error) {
	var result *entity.KnowledgeWebPreview
	created := false
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, reg, err := loadWebRegistration(tx, user, domain, registration, "")
		if err != nil {
			return err
		}
		if !originalKey(request.IdempotencyKey, 128) || !isKnowledgeID(request.CaptureID, 32) || !isKnowledgeID(request.ObservationID, 32) || !isKnowledgeID(request.ObservationSHA256, 64) || len(request.Selection) < 1 || len(request.Selection) > 32 {
			return webError(400, "preview", "knowledge_invalid_request")
		}
		request.Selection = slices.Clone(request.Selection)
		slices.SortFunc(request.Selection, func(a, b entity.KnowledgeWebSelection) int { return strings.Compare(a.EntryKey, b.EntryKey) })
		for i, item := range request.Selection {
			if !isKnowledgeID(item.EntryKey, 64) || (item.ExpectedRevisionID != nil && !isKnowledgeID(*item.ExpectedRevisionID, 32)) || (i > 0 && item.EntryKey == request.Selection[i-1].EntryKey) {
				return webError(400, "preview", "knowledge_invalid_request")
			}
			if err := webClassification(tx, domain, item.Classification); err != nil {
				return err
			}
		}
		requestHash, err := entity.KnowledgeWebDigest("selection_request", request)
		if err != nil {
			return err
		}
		prior, view, _, err := loadWebPreview(tx, user, domain, registration, "", request.IdempotencyKey)
		if err == nil {
			if prior.RequestSHA256 != requestHash {
				return webError(409, "preview", "idempotency_conflict")
			}
			if view.State != "committed" {
				_, cap, err := loadWebCapture(tx, kb, user, registration, view.CaptureID, "", now)
				if err != nil {
					return err
				}
				if err := refreshWebPreview(tx, kb, prior, view, cap, now); err != nil {
					return err
				}
			}
			result = view
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		captureRow, capture, err := loadWebCapture(tx, kb, user, registration, request.CaptureID, "", now)
		if err != nil {
			return err
		}
		obs, observation, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if err := webObservationReady(capture, observation); err != nil {
			return &originalCommitError{err}
		}
		if obs.ID != request.ObservationID || observation.ObservationSHA256 == nil || *observation.ObservationSHA256 != request.ObservationSHA256 {
			return webError(409, "preview", "manifest_changed")
		}
		items := make([]entity.KnowledgeWebPreviewItem, 0, len(request.Selection))
		total := int64(0)
		for _, selected := range request.Selection {
			var row entity.KnowledgeWebEntryRecord
			if err := tx.Where("observation_id = ? AND entry_key = ?", obs.ID, selected.EntryKey).Take(&row).Error; err != nil {
				return err
			}
			entry, err := readWebEntry(&row, capture.CaptureID, obs.ID, selected.EntryKey)
			if err != nil {
				return err
			}
			if !entry.Eligible {
				return webError(422, "preview", "web_parse_rejected")
			}
			binding, current, err := webCurrent(tx, domain, registration, selected.EntryKey)
			if err != nil {
				return err
			}
			if selected.ExpectedRevisionID == nil && current != nil || selected.ExpectedRevisionID != nil && (current == nil || *selected.ExpectedRevisionID != current.ID) {
				return webError(409, "preview", "revision_changed")
			}
			input, err := connector.KnowledgeWebSourceInput(*entry, selected.Classification)
			if err != nil {
				return err
			}
			item := entity.KnowledgeWebPreviewItem{EntryKey: entry.EntryKey, Title: entry.Title, ExpectedRevisionID: selected.ExpectedRevisionID, Classification: selected.Classification, SourceInputSHA256: input, Action: "created", DecodedSHA256: entry.Representations[0].SHA256, ReadingSHA256: entry.Representations[1].SHA256, BodyCompleteness: entry.BodyCompleteness, Acknowledgement: "unknown_completeness"}
			if binding != nil {
				item.SourceID = directoryString(binding.DocumentID)
				item.Action = "updated"
				if current.InputHash == input {
					item.Action = "unchanged"
				}
			}
			switch entry.BodyCompleteness {
			case "summary":
				item.Acknowledgement = "summary_only"
			case "external_reference":
				item.Acknowledgement = "external_reference_only"
			}
			total += entry.DecodedByteLength + entry.ReadingByteLength + int64(len(directoryJSON(entry.Metadata))+len(row.References))
			if total > 8<<20 {
				return webError(413, "preview", "web_budget_exceeded")
			}
			items = append(items, item)
		}
		selectionHash, err := entity.KnowledgeWebDigest("selection", map[string]any{"selection": request.Selection})
		if err != nil {
			return err
		}
		expires := now.Add(10 * time.Minute)
		if captureRow.ExpiresAt.Before(expires) {
			expires = captureRow.ExpiresAt
		}
		id := common.GenerateUUID()
		result = &entity.KnowledgeWebPreview{DomainID: domain, OwnerID: user, RegistrationID: registration, CaptureID: capture.CaptureID, CaptureSHA256: *capture.RawSHA256, ObservationID: obs.ID, ObservationSHA256: *observation.ObservationSHA256, PreviewID: id, IdempotencyKey: request.IdempotencyKey, RequestSHA256: requestHash, RegistrationConfigSHA256: reg.ConfigSHA256, ScopeSHA256: reg.ScopeSHA256, ProducerVersions: capture.ProducerVersions, PolicyVersion: kb.KnowledgePolicyVersion, SelectionSHA256: selectionHash, SelectedCount: int64(len(items)), SelectedByteLength: total, State: "ready", Revision: 1, CreatedAt: originalTime(now), ExpiresAt: originalTime(expires)}
		manifest, err := webPreviewManifest(result, items)
		if err != nil {
			return err
		}
		result.ManifestSHA256 = manifest
		row := entity.KnowledgeWebPreviewRecord{ID: id, RegistrationID: registration, IdempotencyKey: request.IdempotencyKey, CaptureID: capture.CaptureID, RequestSHA256: requestHash, Request: directoryJSON(request), Payload: directoryJSON(result), Items: directoryJSON(items), CreatedAt: now, ExpiresAt: expires}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}

// GetKnowledgeWebPreview recovers a saved selection, including committed state
// after the temporary Capture has expired or been cancelled.
func (d *DatasetService) GetKnowledgeWebPreview(ctx context.Context, user, domain, registration, id, key string) (*entity.KnowledgeWebPreview, error) {
	var result *entity.KnowledgeWebPreview
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
			return err
		}
		row, view, _, err := loadWebPreview(tx, user, domain, registration, id, key)
		if err != nil {
			return err
		}
		if view.State != "committed" {
			_, capture, err := loadWebCapture(tx, kb, user, registration, view.CaptureID, "", now)
			if err != nil {
				return err
			}
			if err := refreshWebPreview(tx, kb, row, view, capture, now); err != nil {
				return err
			}
		}
		result = view
		return nil
	})
	return result, err
}

// CancelKnowledgeWebPreview fences only its unconsumed selection.
func (d *DatasetService) CancelKnowledgeWebPreview(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeWebPreview, error) {
	var result *entity.KnowledgeWebPreview
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
			return err
		}
		row, view, _, err := loadWebPreview(tx, user, domain, registration, id, "")
		if err != nil {
			return err
		}
		if view.State != "committed" {
			_, capture, err := loadWebCapture(tx, kb, user, registration, view.CaptureID, "", now)
			if err != nil {
				return err
			}
			if err := refreshWebPreview(tx, kb, row, view, capture, now); err != nil {
				return err
			}
			if view.State == "ready" {
				view.State = "cancelled"
				if err := saveWebPreview(tx, row, view); err != nil {
					return err
				}
			}
		}
		result = view
		return nil
	})
	return result, err
}

// ListKnowledgeWebPreviewItems returns the fixed complete selection by pages.
func (d *DatasetService) ListKnowledgeWebPreviewItems(ctx context.Context, user, domain, registration, id, cursor string) (*entity.KnowledgeWebPreviewItemPage, error) {
	if _, err := d.GetKnowledgeWebPreview(ctx, user, domain, registration, id, ""); err != nil {
		return nil, err
	}
	var result *entity.KnowledgeWebPreviewItemPage
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
			return err
		}
		row, view, items, err := loadWebPreview(tx, user, domain, registration, id, "")
		if err != nil {
			return err
		}
		expires := row.ExpiresAt
		if view.State == "committed" {
			expires = now.Add(30 * time.Minute)
		}
		page, err := webArrayPage(tx, user, domain, registration, entity.KnowledgeID("web_preview_items", id, view.ManifestSHA256), cursor, items, expires, now)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeWebPreviewItemPage{Items: page.Items, NextCursor: page.NextCursor}
		return nil
	})
	return result, err
}
