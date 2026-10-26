package dataset

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"
	"ragflow/internal/syncer/connector"

	"github.com/zeebo/xxh3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type webPreparedEntry struct {
	record  entity.KnowledgeWebEntryRecord
	entry   entity.KnowledgeWebEntry
	decoded []byte
}
type webConfirmation struct {
	preview     entity.KnowledgeWebPreview
	items       []entity.KnowledgeWebPreviewItem
	capture     entity.KnowledgeWebCapture
	observation entity.KnowledgeWebObservationRecord
	entries     []webPreparedEntry
	folder      string
}

func webConfirmRequest(request entity.KnowledgeWebConfirm) (entity.KnowledgeWebConfirm, string, error) {
	if !isKnowledgeID(request.PreviewID, 32) || !isKnowledgeID(request.ManifestSHA256, 64) || !originalKey(request.IdempotencyKey, 128) || !request.Confirmed || len(request.AcknowledgedEntryKeys) < 1 || len(request.AcknowledgedEntryKeys) > 32 {
		return request, "", webError(400, "confirm", "knowledge_invalid_request")
	}
	request.AcknowledgedEntryKeys = slices.Clone(request.AcknowledgedEntryKeys)
	slices.Sort(request.AcknowledgedEntryKeys)
	for i, key := range request.AcknowledgedEntryKeys {
		if !isKnowledgeID(key, 64) || i > 0 && key == request.AcknowledgedEntryKeys[i-1] {
			return request, "", webError(400, "confirm", "knowledge_invalid_request")
		}
	}
	hash, err := entity.KnowledgeWebDigest("confirmation", request)
	return request, hash, err
}
func readWebReceipt(row *entity.KnowledgeWebReceiptRecord, user, domain, registration string) (*entity.KnowledgeWebReceipt, []entity.KnowledgeWebReceiptItem, error) {
	var view entity.KnowledgeWebReceipt
	var items []entity.KnowledgeWebReceiptItem
	if json.Unmarshal([]byte(row.Payload), &view) != nil || json.Unmarshal([]byte(row.Items), &items) != nil || view.OwnerID != user || view.DomainID != domain || view.RegistrationID != registration || view.ImportTaskID != row.ID || view.PreviewID != row.PreviewID || view.IdempotencyKey != row.IdempotencyKey || view.RequestSHA256 != row.RequestSHA256 || view.SelectedCount != int64(len(items)) || view.SavedCount+view.UnchangedCount != view.SelectedCount {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	return &view, items, nil
}
func loadWebReceipt(tx *gorm.DB, user, domain, registration, id, key string) (*entity.KnowledgeWebReceiptRecord, *entity.KnowledgeWebReceipt, []entity.KnowledgeWebReceiptItem, error) {
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
	var row entity.KnowledgeWebReceiptRecord
	if err := query.Take(&row).Error; err != nil {
		return nil, nil, nil, err
	}
	view, items, err := readWebReceipt(&row, user, domain, registration)
	return &row, view, items, err
}

func (d *DatasetService) prepareWebConfirmation(ctx context.Context, user, domain, registration string, request entity.KnowledgeWebConfirm, hash string) (*webConfirmation, *entity.KnowledgeWebReceipt, error) {
	var prepared *webConfirmation
	var replay *entity.KnowledgeWebReceipt
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		regRow, reg, err := loadWebRegistration(tx, user, domain, registration, "")
		if err != nil {
			return err
		}
		prior, receipt, _, err := loadWebReceipt(tx, user, domain, registration, "", request.IdempotencyKey)
		if err == nil {
			if prior.RequestSHA256 != hash {
				return webError(409, "confirm", "idempotency_conflict")
			}
			replay = receipt
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		previewRow, preview, items, err := loadWebPreview(tx, user, domain, registration, request.PreviewID, "")
		if err != nil {
			return err
		}
		captureRow, capture, err := loadWebCapture(tx, kb, user, registration, preview.CaptureID, "", now)
		if err != nil {
			return err
		}
		observation, view, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if err := refreshWebPreview(tx, kb, previewRow, preview, capture, now); err != nil {
			return err
		}
		if preview.State != "ready" {
			return &originalCommitError{webPreviewStateError(preview)}
		}
		if err := webObservationReady(capture, view); err != nil {
			return &originalCommitError{err}
		}
		if request.ManifestSHA256 != preview.ManifestSHA256 || capture.RawSHA256 == nil || preview.CaptureSHA256 != *capture.RawSHA256 || preview.ObservationID != observation.ID || view.ObservationSHA256 == nil || preview.ObservationSHA256 != *view.ObservationSHA256 || preview.RegistrationConfigSHA256 != reg.ConfigSHA256 || preview.ScopeSHA256 != reg.ScopeSHA256 || captureRow.ObjectFolderID != regRow.FolderID || preview.PolicyVersion != kb.KnowledgePolicyVersion {
			return webError(409, "confirm", "manifest_changed")
		}
		if len(request.AcknowledgedEntryKeys) != len(items) {
			return webError(400, "confirm", "knowledge_invalid_request")
		}
		prepared = &webConfirmation{preview: *preview, items: items, capture: *capture, observation: *observation, folder: regRow.FolderID, entries: []webPreparedEntry{}}
		for i, item := range items {
			if item.EntryKey != request.AcknowledgedEntryKeys[i] {
				return webError(400, "confirm", "knowledge_invalid_request")
			}
			var row entity.KnowledgeWebEntryRecord
			if err := tx.Where("observation_id = ? AND entry_key = ?", observation.ID, item.EntryKey).Take(&row).Error; err != nil {
				return err
			}
			entry, err := readWebEntry(&row, capture.CaptureID, observation.ID, item.EntryKey)
			if err != nil {
				return err
			}
			input, err := connector.KnowledgeWebSourceInput(*entry, item.Classification)
			if err != nil || input != item.SourceInputSHA256 || !entry.Eligible || entry.Representations[0].SHA256 != item.DecodedSHA256 || entry.Representations[1].SHA256 != item.ReadingSHA256 {
				return webError(409, "confirm", "manifest_changed")
			}
			if _, err := webEntryReferences(&row, entry); err != nil {
				return err
			}
			if err := webClassification(tx, domain, item.Classification); err != nil {
				return err
			}
			prepared.entries = append(prepared.entries, webPreparedEntry{record: row, entry: *entry})
		}
		return nil
	})
	if err != nil || replay != nil {
		return nil, replay, err
	}
	if _, err := webReadObject(ctx, prepared.folder, *prepared.capture.RawSHA256, *prepared.capture.RawByteLength); err != nil {
		return nil, nil, err
	}
	for i := range prepared.entries {
		entry := &prepared.entries[i]
		decoded, err := webReadPacked(ctx, &entry.record, &prepared.observation, &entry.entry.Representations[0], 0, entry.entry.Representations[0].ByteLength)
		if err != nil {
			return nil, nil, err
		}
		if _, err := webReadPacked(ctx, &entry.record, &prepared.observation, &entry.entry.Representations[1], 0, entry.entry.Representations[1].ByteLength); err != nil {
			return nil, nil, err
		}
		nativeHash := xxh3.Hash128(decoded).Bytes()
		if hex.EncodeToString(nativeHash[:]) != entry.record.NativeHash {
			return nil, nil, webError(409, "confirm", "web_capture_integrity")
		}
		if err := webPutObject(ctx, prepared.folder, decoded, entry.entry.Representations[0].MediaType); err != nil {
			return nil, nil, err
		}
		entry.decoded = decoded
	}
	return prepared, nil, nil
}

type webPublication struct {
	binding    *entity.KnowledgeSourceBinding
	file       entity.File
	document   entity.Document
	historical *entity.KnowledgeSourceRevision
	current    *entity.KnowledgeSourceRevision
	item       entity.KnowledgeWebPreviewItem
	entry      *webPreparedEntry
	isNew      bool
}

func prepareWebNative(tx *gorm.DB, kb *entity.Knowledgebase, user, registration, folder string, item entity.KnowledgeWebPreviewItem, entry *webPreparedEntry, binding *entity.KnowledgeSourceBinding, current *entity.KnowledgeSourceRevision) (*webPublication, error) {
	publication := &webPublication{binding: binding, current: current, item: item, entry: entry, isNew: binding == nil}
	media := entry.entry.Representations[0].MediaType
	ext, kind := ".html", "html"
	if media == "text/plain" {
		ext, kind = ".txt", "txt"
	}
	relative := "web/" + item.EntryKey + ext
	name := entry.entry.Title
	if name == "" {
		name = item.EntryKey + ext
	}
	location := storage.KnowledgeOriginalObjectPrefix + item.DecodedSHA256
	if binding == nil {
		publication.binding = &entity.KnowledgeSourceBinding{ID: common.GenerateUUID(), RegistrationID: registration, RelativePath: relative, DocumentID: common.GenerateUUID(), FileID: common.GenerateUUID()}
		b := publication.binding
		valid := "1"
		publication.document = entity.Document{ID: b.DocumentID, KbID: kb.ID, CreatedBy: user, Name: &name, Location: &location, Size: int64(len(entry.decoded)), ContentHash: &entry.record.NativeHash, Status: &valid, ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: kind, Suffix: strings.TrimPrefix(ext, ".")}
		publication.file = entity.File{ID: b.FileID, ParentID: folder, TenantID: kb.TenantID, CreatedBy: user, Name: name, Location: &location, Size: int64(len(entry.decoded)), Type: kind, SourceType: "knowledgebase"}
		for _, record := range []any{&publication.document, &publication.file, &entity.File2Document{ID: common.GenerateUUID(), FileID: &publication.file.ID, DocumentID: &publication.document.ID}, b, &entity.KnowledgeWebBinding{ID: b.ID, RegistrationID: registration, EntryKey: item.EntryKey}} {
			if err := tx.Create(record).Error; err != nil {
				return nil, err
			}
		}
	} else {
		if err := tx.Where("id = ? AND kb_id = ? AND status = ?", binding.DocumentID, kb.ID, "1").Take(&publication.document).Error; err != nil {
			return nil, err
		}
		if err := tx.Where("id = ? AND parent_id = ? AND tenant_id = ? AND source_type = ?", binding.FileID, folder, kb.TenantID, "knowledgebase").Take(&publication.file).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&publication.document).Updates(map[string]any{"name": name, "location": location, "size": len(entry.decoded), "content_hash": entry.record.NativeHash, "type": kind, "suffix": strings.TrimPrefix(ext, ".")}).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&publication.file).Updates(map[string]any{"name": name, "location": location, "size": len(entry.decoded), "type": kind}).Error; err != nil {
			return nil, err
		}
		publication.file.Name = name
		if err := tx.Model(binding).Update("relative_path", relative).Error; err != nil {
			return nil, err
		}
		binding.RelativePath = relative
		var historical entity.KnowledgeSourceRevision
		err := tx.Where("domain_id = ? AND document_id = ? AND input_hash = ?", kb.ID, binding.DocumentID, item.SourceInputSHA256).Take(&historical).Error
		if err == nil {
			if historical.RepresentationKind != "web_capture" || historical.OriginalRegistrationID != registration || historical.FolderID != folder || historical.FileID != binding.FileID {
				return nil, webError(409, "confirm", "web_capture_integrity")
			}
			var fixed entity.KnowledgeWebRevisionRecord
			if err := tx.Where("id = ? AND registration_id = ? AND entry_key = ?", historical.ID, registration, item.EntryKey).Take(&fixed).Error; err != nil {
				return nil, err
			}
			publication.historical = &historical
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return publication, nil
}

func publishWebSources(tx *gorm.DB, kb *entity.Knowledgebase, user, registration string, prepared *webConfirmation, now time.Time) ([]entity.KnowledgeWebReceiptItem, int64, int64, error) {
	publications := make([]*webPublication, len(prepared.items))
	snapshots := []file.SourceSnapshotObject{}
	changed := []int{}
	newDocuments := int64(0)
	// Validate every live head before creating any native file or version row.
	for i, item := range prepared.items {
		binding, current, err := webCurrent(tx, kb.ID, registration, item.EntryKey)
		if err != nil {
			return nil, 0, 0, err
		}
		if item.ExpectedRevisionID == nil && current != nil || item.ExpectedRevisionID != nil && (current == nil || *item.ExpectedRevisionID != current.ID) {
			return nil, 0, 0, webError(409, "confirm", "revision_changed")
		}
		publications[i] = &webPublication{binding: binding, current: current, item: item, entry: &prepared.entries[i]}
	}
	for i, publication := range publications {
		if publication.item.Action == "unchanged" {
			if publication.current == nil || publication.current.InputHash != publication.item.SourceInputSHA256 {
				return nil, 0, 0, webError(409, "confirm", "manifest_changed")
			}
			continue
		}
		ready, err := prepareWebNative(tx, kb, user, registration, prepared.folder, publication.item, publication.entry, publication.binding, publication.current)
		if err != nil {
			return nil, 0, 0, err
		}
		publications[i] = ready
		if ready.isNew {
			newDocuments++
		}
		snapshots = append(snapshots, file.SourceSnapshotObject{File: &ready.file, SHA256: ready.item.DecodedSHA256, Location: storage.KnowledgeOriginalObjectPrefix + ready.item.DecodedSHA256, ByteLength: int64(len(ready.entry.decoded))})
		changed = append(changed, i)
	}
	if len(snapshots) > 0 {
		commit, items, err := file.NewFileCommitService().RecordSourceSnapshotObjects(tx.Statement.Context, tx, user, snapshots)
		if err != nil {
			return nil, 0, 0, err
		}
		for j, index := range changed {
			publication := publications[index]
			source := publication.historical
			if source == nil {
				item := publication.item
				c := item.Classification
				entry := publication.entry.entry
				source = &entity.KnowledgeSourceRevision{ID: items[j].ID, DomainID: kb.ID, DocumentID: publication.binding.DocumentID, FileID: publication.file.ID, CommitID: commit.ID, FolderID: prepared.folder, GroupID: c.GroupID, RelativePath: publication.binding.RelativePath, ComponentSide: c.ComponentSide, Language: c.Language, SHA256: item.DecodedSHA256, InputHash: item.SourceInputSHA256, ByteLength: int64(len(publication.entry.decoded)), LineCount: 0, IsCurrent: true, RepresentationKind: "web_capture", OriginalFilename: publication.file.Name, OriginalMediaType: entry.Representations[0].MediaType, OriginalRegistrationID: registration}
				if err := tx.Create(source).Error; err != nil {
					return nil, 0, 0, err
				}
				view := entity.KnowledgeWebRevision{DomainID: kb.ID, RegistrationID: registration, SourceID: source.DocumentID, SourceRevisionID: source.ID, EntryKey: entry.EntryKey, IdentityBasis: entry.IdentityBasis, IdentityValue: entry.IdentityValue, RelativePath: source.RelativePath, Classification: c, RepresentationKind: "web_capture", SourceInputSHA256: item.SourceInputSHA256, CaptureID: prepared.capture.CaptureID, CaptureSHA256: *prepared.capture.RawSHA256, ObservationID: prepared.observation.ID, Positions: entry.Positions, Metadata: entry.Metadata, MetadataSHA256: entry.MetadataSHA256, ReferencesSHA256: entry.ReferencesSHA256, ReferenceCount: entry.ReferenceCount, Representations: entry.Representations, ProducerVersions: entry.ProducerVersions, TextAvailable: len(publication.entry.decoded) > 0, StructureStatus: "not_built", FullTextStatus: "not_built", VectorStatus: "not_built", IsCurrent: true, CreatedAt: originalTime(now)}
				row := entity.KnowledgeWebRevisionRecord{ID: source.ID, RegistrationID: registration, CaptureID: view.CaptureID, EntryKey: entry.EntryKey, EntryRecordID: publication.entry.record.ID, Payload: directoryJSON(view), CreatedAt: now}
				if err := tx.Create(&row).Error; err != nil {
					return nil, 0, 0, err
				}
			}
			if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND id <> ?", kb.ID, source.DocumentID, source.ID).Update("is_current", false).Error; err != nil {
				return nil, 0, 0, err
			}
			if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("id = ?", source.ID).Update("is_current", true).Error; err != nil {
				return nil, 0, 0, err
			}
			source.IsCurrent = true
			publication.historical = source
		}
	}
	if newDocuments > 0 {
		if err := tx.Model(kb).Update("doc_num", gorm.Expr("doc_num + ?", newDocuments)).Error; err != nil {
			return nil, 0, 0, err
		}
	}
	receipts := make([]entity.KnowledgeWebReceiptItem, 0, len(publications))
	saved, unchanged := int64(0), int64(0)
	for _, publication := range publications {
		source := publication.historical
		if publication.item.Action == "unchanged" {
			source = publication.current
			unchanged++
		} else {
			saved++
		}
		item := publication.item
		receipts = append(receipts, entity.KnowledgeWebReceiptItem{EntryKey: item.EntryKey, SourceID: source.DocumentID, SourceRevisionID: source.ID, Action: item.Action, Classification: item.Classification, SourceInputSHA256: item.SourceInputSHA256, DecodedSHA256: item.DecodedSHA256, ReadingSHA256: item.ReadingSHA256})
	}
	return receipts, saved, unchanged, nil
}

// ConfirmKnowledgeWebImport verifies all immutable products before a single
// short, all-or-nothing native publication transaction.
func (d *DatasetService) ConfirmKnowledgeWebImport(ctx context.Context, user, domain, registration string, request entity.KnowledgeWebConfirm) (*entity.KnowledgeWebReceipt, bool, error) {
	var requestHash string
	var err error
	request, requestHash, err = webConfirmRequest(request)
	if err != nil {
		return nil, false, err
	}
	prepared, replay, err := d.prepareWebConfirmation(ctx, user, domain, registration, request, requestHash)
	if err != nil || replay != nil {
		return replay, false, err
	}
	var result *entity.KnowledgeWebReceipt
	created := false
	err = d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, reg, err := loadWebRegistration(tx, user, domain, registration, "")
		if err != nil {
			return err
		}
		_, capture, err := loadWebCapture(tx, kb, user, registration, prepared.capture.CaptureID, "", now)
		if err != nil {
			return err
		}
		observation, obsView, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		row, preview, items, err := loadWebPreview(tx.Clauses(clause.Locking{Strength: "UPDATE"}), user, domain, registration, request.PreviewID, "")
		if err != nil {
			return err
		}
		prior, receipt, _, err := loadWebReceipt(tx, user, domain, registration, "", request.IdempotencyKey)
		if err == nil {
			if prior.RequestSHA256 != requestHash {
				return webError(409, "confirm", "idempotency_conflict")
			}
			result = receipt
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := refreshWebPreview(tx, kb, row, preview, capture, now); err != nil {
			return err
		}
		if preview.State != "ready" {
			return &originalCommitError{webPreviewStateError(preview)}
		}
		if err := webObservationReady(capture, obsView); err != nil {
			return &originalCommitError{err}
		}
		if preview.ManifestSHA256 != request.ManifestSHA256 || preview.ManifestSHA256 != prepared.preview.ManifestSHA256 || directoryJSON(items) != directoryJSON(prepared.items) || capture.RawSHA256 == nil || *capture.RawSHA256 != preview.CaptureSHA256 || observation.ID != preview.ObservationID || obsView.ObservationSHA256 == nil || *obsView.ObservationSHA256 != preview.ObservationSHA256 || reg.ConfigSHA256 != preview.RegistrationConfigSHA256 || reg.ScopeSHA256 != preview.ScopeSHA256 || kb.KnowledgePolicyVersion != preview.PolicyVersion {
			return webError(409, "confirm", "manifest_changed")
		}
		for _, item := range items {
			if err := webClassification(tx, domain, item.Classification); err != nil {
				return err
			}
		}
		results, saved, unchanged, err := publishWebSources(tx, kb, user, registration, prepared, now)
		if err != nil {
			return err
		}
		id := common.GenerateUUID()
		result = &entity.KnowledgeWebReceipt{DomainID: domain, OwnerID: user, RegistrationID: registration, CaptureID: capture.CaptureID, CaptureSHA256: *capture.RawSHA256, ObservationID: observation.ID, ObservationSHA256: preview.ObservationSHA256, PreviewID: preview.PreviewID, PreviewRequestSHA256: preview.RequestSHA256, ProducerVersions: preview.ProducerVersions, PolicyVersion: preview.PolicyVersion, ImportTaskID: id, IdempotencyKey: request.IdempotencyKey, RequestSHA256: requestHash, ManifestSHA256: preview.ManifestSHA256, SelectionSHA256: preview.SelectionSHA256, SelectedCount: preview.SelectedCount, Status: "completed", SavedCount: saved, UnchangedCount: unchanged, IndexStatus: "not_built", ModelCalls: 0, CreatedAt: originalTime(now)}
		record := entity.KnowledgeWebReceiptRecord{ID: id, RegistrationID: registration, IdempotencyKey: request.IdempotencyKey, PreviewID: preview.PreviewID, RequestSHA256: requestHash, Payload: directoryJSON(result), Items: directoryJSON(results), CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		preview.State = "committed"
		preview.ImportTaskID = &id
		if err := saveWebPreview(tx, row, preview); err != nil {
			return err
		}
		created = true
		return nil
	})
	return result, created, err
}

// GetKnowledgeWebReceipt reads a fixed result without temporary TTL checks.
func (d *DatasetService) GetKnowledgeWebReceipt(ctx context.Context, user, domain, registration, id, key string) (*entity.KnowledgeWebReceipt, error) {
	var result *entity.KnowledgeWebReceipt
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
			return err
		}
		_, view, _, err := loadWebReceipt(tx, user, domain, registration, id, key)
		result = view
		return err
	})
	return result, err
}

// ListKnowledgeWebReceiptItems pages the immutable, complete batch result.
func (d *DatasetService) ListKnowledgeWebReceiptItems(ctx context.Context, user, domain, registration, id, cursor string) (*entity.KnowledgeWebReceiptItemPage, error) {
	var result *entity.KnowledgeWebReceiptItemPage
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, now time.Time) error {
		if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
			return err
		}
		_, view, items, err := loadWebReceipt(tx, user, domain, registration, id, "")
		if err != nil {
			return err
		}
		page, err := webArrayPage(tx, user, domain, registration, entity.KnowledgeID("web_receipt_items", id, view.RequestSHA256), cursor, items, now.Add(30*time.Minute), now)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeWebReceiptItemPage{Items: page.Items, NextCursor: page.NextCursor}
		return nil
	})
	return result, err
}
