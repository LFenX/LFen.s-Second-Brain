package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

type webPage[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
type webCursor struct {
	Scope  string `json:"scope"`
	Offset int    `json:"offset"`
	LastID string `json:"last_id"`
}

func webCursorPosition(tx *gorm.DB, user, domain, scope, id string, now time.Time) (webCursor, error) {
	position := webCursor{Scope: scope}
	if id == "" {
		return position, nil
	}
	if !isKnowledgeID(id, 32) {
		return position, webError(400, "read", "knowledge_invalid_request")
	}
	var row entity.KnowledgeDirectoryState
	if err := tx.Where("id = ? AND domain_id = ? AND user_id = ? AND kind = ? AND expires_at > ?", id, domain, user, "web_cursor", now).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return position, webError(400, "read", "knowledge_invalid_request")
		}
		return position, err
	}
	if json.Unmarshal([]byte(row.Payload), &position) != nil || position.Scope != scope || position.Offset < 0 {
		return position, webError(400, "read", "knowledge_invalid_request")
	}
	return position, nil
}
func webSaveCursor(tx *gorm.DB, user, domain, registration string, position webCursor, expires, now time.Time) (*string, error) {
	id := common.GenerateUUID()
	row := entity.KnowledgeDirectoryState{ID: id, DomainID: domain, UserID: user, RegistrationID: registration, Kind: "web_cursor", Payload: directoryJSON(position), PrivateData: "null", ExpiresAt: expires, CreatedAt: now}
	if err := tx.Create(&row).Error; err != nil {
		return nil, err
	}
	return &id, nil
}
func webArrayPage[T any](tx *gorm.DB, user, domain, registration, scope, cursor string, all []T, expires, now time.Time) (*webPage[T], error) {
	position, err := webCursorPosition(tx, user, domain, scope, cursor, now)
	if err != nil {
		return nil, err
	}
	if position.Offset > len(all) {
		return nil, webError(400, "read", "knowledge_invalid_request")
	}
	page := &webPage[T]{Items: []T{}}
	next := position.Offset
	for next < len(all) && len(page.Items) < 20 {
		candidate := append(page.Items, all[next])
		if len(directoryJSON(webPage[T]{Items: candidate, NextCursor: directoryString("00000000000000000000000000000000")})) > 31744 {
			break
		}
		page.Items = candidate
		next++
	}
	if next == position.Offset && next < len(all) {
		return nil, webError(413, "read", "web_budget_exceeded")
	}
	if next < len(all) {
		page.NextCursor, err = webSaveCursor(tx, user, domain, registration, webCursor{Scope: scope, Offset: next}, expires, now)
		if err != nil {
			return nil, err
		}
	}
	return page, nil
}

func webObservationReady(capture *entity.KnowledgeWebCapture, view *entity.KnowledgeWebObservation) error {
	if capture.State != "ready" {
		return webStateError(capture.State)
	}
	if view.State == "expired" {
		return webError(410, "read", "web_capture_expired")
	}
	if view.State != "ready" || view.ObservationSHA256 == nil {
		return webError(409, "read", "web_observation_not_ready")
	}
	return nil
}
func readWebEntry(row *entity.KnowledgeWebEntryRecord, capture, observation, key string) (*entity.KnowledgeWebEntry, error) {
	var entry entity.KnowledgeWebEntry
	if json.Unmarshal([]byte(row.Payload), &entry) != nil || entry.CaptureID != capture || entry.ObservationID != observation || row.ObservationID != observation || row.EntryKey != key || entry.EntryKey != key || len(entry.Representations) != 2 || entry.Representations[0].Kind != "decoded_content" || entry.Representations[1].Kind != "safe_reading" {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	metadata, err := entity.KnowledgeWebDigest("metadata", entry.Metadata)
	if err != nil || metadata != entry.MetadataSHA256 {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	identity, err := entity.KnowledgeWebDigest("entry_key", entry)
	if err != nil || identity != entry.EntryKey {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	return &entry, nil
}

func (d *DatasetService) webEntryData(ctx context.Context, user, domain, registration, captureID, key string) (*entity.KnowledgeWebEntryRecord, *entity.KnowledgeWebEntry, *entity.KnowledgeWebObservationRecord, error) {
	var record *entity.KnowledgeWebEntryRecord
	var entry *entity.KnowledgeWebEntry
	var observation *entity.KnowledgeWebObservationRecord
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, capture, err := loadWebCapture(tx, kb, user, registration, captureID, "", now)
		if err != nil {
			return err
		}
		obs, view, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if err := webObservationReady(capture, view); err != nil {
			return &originalCommitError{err}
		}
		if !isKnowledgeID(key, 64) {
			return webError(404, "read", "knowledge_not_found")
		}
		var row entity.KnowledgeWebEntryRecord
		if err := tx.Where("observation_id = ? AND entry_key = ?", obs.ID, key).Take(&row).Error; err != nil {
			return err
		}
		entry, err = readWebEntry(&row, captureID, obs.ID, key)
		if err != nil {
			return err
		}
		record = &row
		observation = obs
		return nil
	})
	return record, entry, observation, err
}

// GetKnowledgeWebEntry returns a fixed observation product, not its live head.
func (d *DatasetService) GetKnowledgeWebEntry(ctx context.Context, user, domain, registration, capture, key string) (*entity.KnowledgeWebEntry, error) {
	_, entry, _, err := d.webEntryData(ctx, user, domain, registration, capture, key)
	return entry, err
}

func webEntrySummary(entry entity.KnowledgeWebEntry) entity.KnowledgeWebEntrySummary {
	return entity.KnowledgeWebEntrySummary{EntryKey: entry.EntryKey, IdentityBasis: entry.IdentityBasis, IdentityStrength: entry.IdentityStrength, Title: entry.Title, BodyCompleteness: entry.BodyCompleteness, Eligible: entry.Eligible, IneligibleReason: entry.IneligibleReason, SourceID: entry.SourceID, CurrentRevisionID: entry.CurrentRevisionID, DecodedByteLength: entry.DecodedByteLength, ReadingByteLength: entry.ReadingByteLength, SourceInputSHA256: entry.SourceInputSHA256, FirstPosition: entry.FirstPosition}
}

// ListKnowledgeWebEntries returns at most 20 bounded summaries per cursor.
func (d *DatasetService) ListKnowledgeWebEntries(ctx context.Context, user, domain, registration, captureID, cursor string) (*entity.KnowledgeWebEntryPage, error) {
	var result *entity.KnowledgeWebEntryPage
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, capture, err := loadWebCapture(tx, kb, user, registration, captureID, "", now)
		if err != nil {
			return err
		}
		obs, view, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if err := webObservationReady(capture, view); err != nil {
			return &originalCommitError{err}
		}
		scope := entity.KnowledgeID("web_entries", registration, captureID, obs.ID, *view.ObservationSHA256)
		position, err := webCursorPosition(tx, user, domain, scope, cursor, now)
		if err != nil {
			return err
		}
		var rows []entity.KnowledgeWebEntryRecord
		if err := tx.Select("id", "observation_id", "entry_key", "payload").Where("observation_id = ?", obs.ID).Order("entry_key ASC").Offset(position.Offset).Limit(21).Find(&rows).Error; err != nil {
			return err
		}
		items := []entity.KnowledgeWebEntrySummary{}
		for _, row := range rows {
			entry, err := readWebEntry(&row, captureID, obs.ID, row.EntryKey)
			if err != nil {
				return err
			}
			items = append(items, webEntrySummary(*entry))
		}
		page, err := webArrayPage(tx, user, domain, registration, scope, "", items, obs.ExpiresAt, now)
		if err != nil {
			return err
		}
		// The page helper sees a limited SQL window; persist its absolute offset.
		if page.NextCursor != nil {
			if err := tx.Model(&entity.KnowledgeDirectoryState{}).Where("id = ?", *page.NextCursor).UpdateColumn("payload", directoryJSON(webCursor{Scope: scope, Offset: position.Offset + len(page.Items)})).Error; err != nil {
				return err
			}
		}
		result = &entity.KnowledgeWebEntryPage{Items: page.Items, NextCursor: page.NextCursor}
		return nil
	})
	return result, err
}

func webEntryReferences(row *entity.KnowledgeWebEntryRecord, entry *entity.KnowledgeWebEntry) ([]entity.KnowledgeWebReference, error) {
	var refs []entity.KnowledgeWebReference
	if json.Unmarshal([]byte(row.References), &refs) != nil || refs == nil || int64(len(refs)) != entry.ReferenceCount {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	hash, err := entity.KnowledgeWebDigest("references", map[string]any{"references": refs})
	if err != nil || hash != entry.ReferencesSHA256 {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	return refs, nil
}

// ListKnowledgeWebEntryReferences reads no resources referenced by the source.
func (d *DatasetService) ListKnowledgeWebEntryReferences(ctx context.Context, user, domain, registration, captureID, key, cursor string) (*entity.KnowledgeWebReferencePage, error) {
	var result *entity.KnowledgeWebReferencePage
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, capture, err := loadWebCapture(tx, kb, user, registration, captureID, "", now)
		if err != nil {
			return err
		}
		obs, view, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if err := webObservationReady(capture, view); err != nil {
			return &originalCommitError{err}
		}
		if !isKnowledgeID(key, 64) {
			return webError(404, "read", "knowledge_not_found")
		}
		var row entity.KnowledgeWebEntryRecord
		if err := tx.Where("observation_id = ? AND entry_key = ?", obs.ID, key).Take(&row).Error; err != nil {
			return err
		}
		entry, err := readWebEntry(&row, captureID, obs.ID, key)
		if err != nil {
			return err
		}
		refs, err := webEntryReferences(&row, entry)
		if err != nil {
			return err
		}
		page, err := webArrayPage(tx, user, domain, registration, entity.KnowledgeID("web_entry_refs", obs.ID, key, entry.ReferencesSHA256), cursor, refs, obs.ExpiresAt, now)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeWebReferencePage{Items: page.Items, NextCursor: page.NextCursor}
		return nil
	})
	return result, err
}

func webRepresentation(entry *entity.KnowledgeWebEntry, kind string) (*entity.KnowledgeWebRepresentation, error) {
	for i := range entry.Representations {
		if entry.Representations[i].Kind == kind && isKnowledgeID(entry.Representations[i].SHA256, 64) && entry.Representations[i].ByteLength >= 0 && entry.Representations[i].ByteLength <= 2<<20 {
			return &entry.Representations[i], nil
		}
	}
	return nil, webError(400, "read", "knowledge_invalid_request")
}

func webReadPacked(ctx context.Context, record *entity.KnowledgeWebEntryRecord, observation *entity.KnowledgeWebObservationRecord, representation *entity.KnowledgeWebRepresentation, start, end int64) ([]byte, error) {
	if start < 0 || end < start || end > representation.ByteLength || !isKnowledgeID(observation.ObjectSHA256, 64) || observation.ObjectByteLength < 0 || observation.ObjectByteLength > 12<<20 || observation.ObjectFolderID != record.ObjectFolderID {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	offset := record.DecodedOffset
	if representation.Kind == "safe_reading" {
		offset = record.ReadingOffset
	}
	if offset < 0 || offset > observation.ObjectByteLength || representation.ByteLength > observation.ObjectByteLength-offset {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	store, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage)
	if !ok {
		return nil, webError(503, "read", "web_storage_unavailable")
	}
	length := end - start
	peek := length == 0 && start > 0 && start < representation.ByteLength
	if peek {
		length = 1
	}
	body, err := store.OpenStream(ctx, record.ObjectFolderID, storage.KnowledgeOriginalObjectPrefix+observation.ObjectSHA256, observation.ObjectByteLength, observation.ObjectSHA256, offset+start, length)
	if err != nil {
		return nil, webError(503, "read", "web_storage_unavailable")
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, length+1))
	if err != nil {
		return nil, webError(503, "read", "web_storage_unavailable")
	}
	if int64(len(raw)) != length || (peek && !utf8.RuneStart(raw[0])) || (!peek && !utf8.Valid(raw)) {
		return nil, webError(416, "read", "web_window_invalid")
	}
	if peek {
		return []byte{}, nil
	}
	if start == 0 && end == representation.ByteLength && directorySHA(raw) != representation.SHA256 {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	return raw, nil
}

// KnowledgeWebContent is an authorized binary result; no internal object locator
// or executable content type crosses this service boundary.
type KnowledgeWebContent struct {
	Raw      []byte
	SHA256   string
	Filename string
}

// ReadKnowledgeWebEntryContent verifies the complete bounded representation.
func (d *DatasetService) ReadKnowledgeWebEntryContent(ctx context.Context, user, domain, registration, captureID, key, kind string) (*KnowledgeWebContent, error) {
	row, entry, obs, err := d.webEntryData(ctx, user, domain, registration, captureID, key)
	if err != nil {
		return nil, err
	}
	rep, err := webRepresentation(entry, kind)
	if err != nil {
		return nil, err
	}
	raw, err := webReadPacked(ctx, row, obs, rep, 0, rep.ByteLength)
	if err != nil {
		return nil, err
	}
	if _, _, _, err := d.webEntryData(ctx, user, domain, registration, captureID, key); err != nil {
		return nil, err
	}
	filename := "decoded.txt"
	if kind == "safe_reading" {
		filename = "safe-reading.html"
	}
	return &KnowledgeWebContent{Raw: raw, SHA256: rep.SHA256, Filename: filename}, nil
}

// ReadKnowledgeWebEntryWindow reads only the requested UTF-8 byte interval.
func (d *DatasetService) ReadKnowledgeWebEntryWindow(ctx context.Context, user, domain, registration, captureID, key, kind string, start, end int64) (*entity.KnowledgeWebWindow, error) {
	row, entry, obs, err := d.webEntryData(ctx, user, domain, registration, captureID, key)
	if err != nil {
		return nil, err
	}
	rep, err := webRepresentation(entry, kind)
	if err != nil {
		return nil, err
	}
	if start < 0 || end < start || end > rep.ByteLength || end-start > 4096 {
		return nil, webError(416, "read", "web_window_invalid")
	}
	raw, err := webReadPacked(ctx, row, obs, rep, start, end)
	if err != nil {
		return nil, err
	}
	if _, _, _, err := d.webEntryData(ctx, user, domain, registration, captureID, key); err != nil {
		return nil, err
	}
	view := &entity.KnowledgeWebWindow{DomainID: domain, CaptureID: captureID, ObservationID: obs.ID, EntryKey: key, Representation: *rep, StartByte: start, EndByte: end, Text: string(raw)}
	if end < rep.ByteLength {
		view.NextStartByte = &end
	}
	return view, nil
}

// DownloadKnowledgeWebCapture returns raw evidence only while its temporary
// context is usable. Fixed revisions use their independently authorized path.
func (d *DatasetService) DownloadKnowledgeWebCapture(ctx context.Context, user, domain, registration, id string) (*KnowledgeWebContent, error) {
	var record *entity.KnowledgeWebCaptureRecord
	var capture *entity.KnowledgeWebCapture
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		var err error
		record, capture, err = loadWebCapture(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		if capture.State != "ready" || capture.RawSHA256 == nil || capture.RawByteLength == nil {
			return &originalCommitError{webStateError(capture.State)}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	raw, err := webReadObject(ctx, record.ObjectFolderID, *capture.RawSHA256, *capture.RawByteLength)
	if err != nil {
		return nil, err
	}
	current, err := d.GetKnowledgeWebCapture(ctx, user, domain, registration, id, "")
	if err != nil {
		return nil, err
	}
	if current.State != "ready" {
		return nil, webStateError(current.State)
	}
	return &KnowledgeWebContent{Raw: raw, SHA256: *capture.RawSHA256, Filename: "capture-snapshot.bin"}, nil
}
