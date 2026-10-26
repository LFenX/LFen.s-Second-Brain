package dataset

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/syncer/connector"

	"github.com/zeebo/xxh3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func webCurrent(tx *gorm.DB, domain, registration, key string) (*entity.KnowledgeSourceBinding, *entity.KnowledgeSourceRevision, error) {
	var binding entity.KnowledgeWebBinding
	err := tx.Where("registration_id = ? AND entry_key = ?", registration, key).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var native entity.KnowledgeSourceBinding
	if err := tx.Where("id = ? AND registration_id = ?", binding.ID, registration).Take(&native).Error; err != nil {
		return nil, nil, err
	}
	var doc entity.Document
	if err := tx.Where("id = ? AND kb_id = ? AND status = ?", native.DocumentID, domain, "1").Take(&doc).Error; err != nil {
		return nil, nil, err
	}
	var sources []entity.KnowledgeSourceRevision
	if err := tx.Where("domain_id = ? AND document_id = ? AND is_current = ?", domain, native.DocumentID, true).Limit(2).Find(&sources).Error; err != nil {
		return nil, nil, err
	}
	if len(sources) != 1 || sources[0].RepresentationKind != "web_capture" || sources[0].OriginalRegistrationID != registration {
		return nil, nil, webError(409, "read", "web_capture_integrity")
	}
	return &native, &sources[0], nil
}

// GetKnowledgeWebTarget reads a live head independently of a fixed observation.
func (d *DatasetService) GetKnowledgeWebTarget(ctx context.Context, user, domain, registration, key string) (*entity.KnowledgeWebTarget, error) {
	var result *entity.KnowledgeWebTarget
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		if _, _, err := loadWebRegistration(tx, user, domain, registration, ""); err != nil {
			return err
		}
		if !isKnowledgeID(key, 64) {
			return webError(400, "read", "knowledge_invalid_request")
		}
		binding, source, err := webCurrent(tx, domain, registration, key)
		if err != nil {
			return err
		}
		result = &entity.KnowledgeWebTarget{DomainID: domain, RegistrationID: registration, EntryKey: key}
		if binding != nil {
			result.SourceID = directoryString(binding.DocumentID)
			result.CurrentRevisionID = directoryString(source.ID)
			result.CurrentInputSHA256 = directoryString(source.InputHash)
			result.CurrentClassification = &entity.KnowledgeDirectoryClassification{GroupID: source.GroupID, ComponentSide: source.ComponentSide, Language: source.Language}
		}
		return nil
	})
	return result, err
}

func readWebObservation(row *entity.KnowledgeWebObservationRecord, capture *entity.KnowledgeWebCapture) (*entity.KnowledgeWebObservation, error) {
	var view entity.KnowledgeWebObservation
	if json.Unmarshal([]byte(row.Payload), &view) != nil || view.ObservationID != row.ID || view.CaptureID != capture.CaptureID || view.DomainID != capture.DomainID || view.RegistrationID != capture.RegistrationID || capture.RawSHA256 == nil || view.CaptureSHA256 != *capture.RawSHA256 || directoryJSON(view.ProducerVersions) != directoryJSON(capture.ProducerVersions) {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	return &view, nil
}
func saveWebObservation(tx *gorm.DB, row *entity.KnowledgeWebObservationRecord, view *entity.KnowledgeWebObservation) error {
	view.Revision++
	row.Payload = directoryJSON(view)
	return tx.Model(&entity.KnowledgeWebObservationRecord{}).Where("id = ?", row.ID).UpdateColumns(map[string]any{"payload": row.Payload, "epoch": row.Epoch, "lease_until": row.LeaseUntil, "object_folder_id": row.ObjectFolderID, "object_sha256": row.ObjectSHA256, "object_byte_length": row.ObjectByteLength}).Error
}
func loadWebObservation(tx *gorm.DB, capture *entity.KnowledgeWebCapture, now time.Time) (*entity.KnowledgeWebObservationRecord, *entity.KnowledgeWebObservation, error) {
	var row entity.KnowledgeWebObservationRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("capture_id = ?", capture.CaptureID).Take(&row).Error; err != nil {
		return nil, nil, err
	}
	view, err := readWebObservation(&row, capture)
	if err != nil {
		return nil, nil, err
	}
	changed := false
	if !now.Before(row.ExpiresAt) && view.State != "expired" {
		view.State = "expired"
		view.Failure = &entity.KnowledgeWebFailure{Stage: "observe", MachineCode: "web_capture_expired", OutcomeKnown: true}
		row.Epoch++
		row.LeaseUntil = nil
		changed = true
	} else if view.State == "parsing" {
		var active int64
		if row.LeaseUntil != nil && now.Before(*row.LeaseUntil) {
			if err := tx.Model(&entity.KnowledgeWebSlot{}).Where("capture_id = ? AND epoch = ? AND operation = ? AND lease_until > ?", capture.CaptureID, row.Epoch, "parsing", now).Count(&active).Error; err != nil {
				return nil, nil, err
			}
		}
		if active == 0 {
			view.State = "interrupted"
			view.Failure = &entity.KnowledgeWebFailure{Stage: "observe", MachineCode: "web_capture_interrupted", OutcomeKnown: true}
			row.Epoch++
			row.LeaseUntil = nil
			changed = true
		}
	}
	if changed {
		if err := saveWebObservation(tx, &row, view); err != nil {
			return nil, nil, err
		}
	}
	return &row, view, nil
}

func (d *DatasetService) checkWebObservationLease(ctx context.Context, transfer *webTransfer, renew bool) error {
	v := transfer.capture
	return d.webTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		captureRow, capture, err := loadWebCapture(tx, kb, v.OwnerID, v.RegistrationID, v.CaptureID, "", now)
		if err != nil {
			return err
		}
		if capture.State != "ready" || captureRow.Epoch != transfer.epoch {
			return webStateError(capture.State)
		}
		row, view, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if row.ID != transfer.observationID || row.Epoch != transfer.epoch || view.State != "parsing" {
			return &originalCommitError{webError(409, "observe", "web_capture_interrupted")}
		}
		if !renew {
			return nil
		}
		lease := now.Add(30 * time.Second)
		updated := tx.Model(&entity.KnowledgeWebSlot{}).Where("id = ? AND capture_id = ? AND epoch = ? AND operation = ? AND lease_until > ?", transfer.slot, v.CaptureID, transfer.epoch, "parsing", now).UpdateColumn("lease_until", lease)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return webError(409, "observe", "web_capture_interrupted")
		}
		row.LeaseUntil = &lease
		return saveWebObservation(tx, row, view)
	})
}

func (d *DatasetService) beginWebObservation(ctx context.Context, user, domain, registration, id string, request entity.KnowledgeWebObserve) (*webTransfer, *entity.KnowledgeWebObservation, bool, error) {
	var transfer *webTransfer
	var result *entity.KnowledgeWebObservation
	created := false
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		captureRow, capture, err := loadWebCapture(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		if request.NormalizerVersion != capture.ProducerVersions.NormalizerVersion {
			return webError(409, "observe", "knowledge_source_contract_version")
		}
		if capture.State != "ready" {
			return &originalCommitError{webStateError(capture.State)}
		}
		regRow, reg, err := loadWebRegistration(tx, user, domain, registration, "")
		if err != nil {
			return err
		}
		row, view, err := loadWebObservation(tx, capture, now)
		if err == nil && view.State != "interrupted" {
			result = view
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row == nil {
			row = &entity.KnowledgeWebObservationRecord{ID: common.GenerateUUID(), CaptureID: id, CreatedAt: now, ExpiresAt: captureRow.ExpiresAt}
			view = &entity.KnowledgeWebObservation{DomainID: domain, RegistrationID: registration, CaptureID: id, ObservationID: row.ID, CaptureSHA256: *capture.RawSHA256, ProducerVersions: capture.ProducerVersions, State: "parsing", Revision: 1, CreatedAt: originalTime(now), ExpiresAt: capture.ExpiresAt, Warnings: []string{}}
			created = true
		}
		captureRow.Epoch++
		row.Epoch = captureRow.Epoch
		slot, lease, err := claimWebSlot(tx, user, domain, id, "parsing", row.Epoch, now)
		if err != nil {
			return err
		}
		row.LeaseUntil = lease
		view.State = "parsing"
		view.Failure = nil
		if created {
			row.Payload = directoryJSON(view)
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		} else {
			if err := saveWebObservation(tx, row, view); err != nil {
				return err
			}
		}
		capture.ObservationID = &row.ID
		if err := saveWebCapture(tx, captureRow, capture, now); err != nil {
			return err
		}
		transfer = &webTransfer{capture: capture, registration: reg, folder: regRow.FolderID, epoch: row.Epoch, slot: slot, operation: "parsing", observationID: row.ID}
		result = view
		return nil
	})
	return transfer, result, created, err
}

func (d *DatasetService) finishWebObservation(ctx context.Context, transfer *webTransfer, parsed *connector.KnowledgeWebParseResult, pack []byte, records []entity.KnowledgeWebEntryRecord, failure error) (*entity.KnowledgeWebObservation, error) {
	v := transfer.capture
	var result *entity.KnowledgeWebObservation
	err := d.webTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		captureRow, capture, err := loadWebCapture(tx, kb, v.OwnerID, v.RegistrationID, v.CaptureID, "", now)
		if err != nil {
			return err
		}
		if capture.State != "ready" || captureRow.Epoch != transfer.epoch {
			return &originalCommitError{webStateError(capture.State)}
		}
		row, view, err := loadWebObservation(tx, capture, now)
		if err != nil {
			return err
		}
		if view.State != "parsing" || row.Epoch != transfer.epoch || row.ID != transfer.observationID {
			return &originalCommitError{webError(409, "observe", "web_capture_interrupted")}
		}
		if failure != nil {
			var public *KnowledgeWebError
			if !errors.As(failure, &public) {
				failure = webTransferFailure(failure, "observe")
				public = failure.(*KnowledgeWebError)
			}
			view.State = "rejected"
			if public.Public.Code == "web_capture_interrupted" || public.Public.Code == "web_result_unknown" {
				view.State = "interrupted"
			}
			view.Failure = &public.Failure
			row.LeaseUntil = nil
			if err := saveWebObservation(tx, row, view); err != nil {
				return err
			}
			return &originalCommitError{failure}
		}
		if parsed == nil || len(records) != len(parsed.Entries) || len(pack) > 12<<20 {
			return webError(409, "observe", "web_capture_integrity")
		}
		tokens := make([]any, 0, len(records))
		eligible := int64(0)
		for i := range records {
			var entry entity.KnowledgeWebEntry
			if err := json.Unmarshal([]byte(records[i].Payload), &entry); err != nil {
				return err
			}
			binding, current, err := webCurrent(tx, v.DomainID, v.RegistrationID, entry.EntryKey)
			if err != nil {
				return err
			}
			if binding != nil {
				entry.SourceID = directoryString(binding.DocumentID)
				entry.CurrentRevisionID = directoryString(current.ID)
			}
			records[i].Payload = directoryJSON(entry)
			if entry.Eligible {
				eligible++
			}
			tokens = append(tokens, map[string]any{"entry_key": entry.EntryKey, "positions": entry.Positions, "source_input_sha256": entry.SourceInputSHA256, "eligible": entry.Eligible})
		}
		if len(records) > 0 {
			if err := tx.CreateInBatches(&records, 100).Error; err != nil {
				return err
			}
		}
		hash, err := entity.KnowledgeWebDigest("observation", map[string]any{"capture_sha256": view.CaptureSHA256, "producer_versions": view.ProducerVersions, "entries": tokens})
		if err != nil {
			return err
		}
		view.State = "ready"
		view.ObservationSHA256 = &hash
		view.InputEntryCount = directoryInt(int64(parsed.InputEntryCount))
		view.EntryCount = directoryInt(int64(len(records)))
		view.EligibleCount = &eligible
		view.DuplicateCount = directoryInt(int64(parsed.DuplicateCount))
		view.CaptureTraversalComplete = true
		view.DecodedBytes = parsed.DecodedBytes
		view.ReadingBytes = parsed.ReadingBytes
		view.MetadataBytes = parsed.MetadataBytes
		view.ReferenceBytes = parsed.ReferenceBytes
		view.ReferenceCount = parsed.ReferenceCount
		view.Warnings = parsed.Warnings
		view.Failure = nil
		row.LeaseUntil = nil
		row.ObjectFolderID = transfer.folder
		row.ObjectSHA256 = directorySHA(pack)
		row.ObjectByteLength = int64(len(pack))
		if err := saveWebObservation(tx, row, view); err != nil {
			return err
		}
		result = view
		return nil
	})
	return result, err
}

// ObserveKnowledgeWebCapture creates or explicitly resumes its one observation.
// All products are measured outside SQL, packed once, then published atomically.
func (d *DatasetService) ObserveKnowledgeWebCapture(ctx context.Context, user, domain, registration, id string, request entity.KnowledgeWebObserve) (*entity.KnowledgeWebObservation, bool, error) {
	transfer, replay, created, err := d.beginWebObservation(ctx, user, domain, registration, id, request)
	if err != nil || transfer == nil {
		return replay, created, err
	}
	defer d.releaseWebTransfer(ctx, transfer)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var received atomic.Int64
	stopRenew, renewed := d.runWebRenewal(ctx, transfer, &received, cancel)
	v := transfer.capture
	raw, workErr := webReadObject(ctx, transfer.folder, *v.RawSHA256, *v.RawByteLength)
	base := v.Provenance.RequestedURL
	if v.Provenance.FinalURL != nil {
		base = *v.Provenance.FinalURL
	}
	var parsed *connector.KnowledgeWebParseResult
	var pack bytes.Buffer
	records := []entity.KnowledgeWebEntryRecord{}
	if workErr == nil {
		parsed, workErr = connector.NormalizeKnowledgeWebCapture(ctx, transfer.registration.SourceKind, base, *v.MediaType, raw, transfer.registration.DefaultClassification)
	}
	if workErr == nil {
		pack.Grow(int(parsed.DecodedBytes + parsed.ReadingBytes))
		for _, product := range parsed.Entries {
			if err := ctx.Err(); err != nil {
				workErr = err
				break
			}
			entry := product.Entry
			entry.CaptureID = id
			entry.ObservationID = transfer.observationID
			decodedOffset := int64(pack.Len())
			pack.Write(product.Decoded)
			readingOffset := int64(pack.Len())
			pack.Write(product.Reading)
			nativeHash := xxh3.Hash128(product.Decoded).Bytes()
			records = append(records, entity.KnowledgeWebEntryRecord{ID: common.GenerateUUID(), ObservationID: transfer.observationID, EntryKey: entry.EntryKey, Payload: directoryJSON(entry), References: directoryJSON(product.References), ObjectFolderID: transfer.folder, NativeHash: hex.EncodeToString(nativeHash[:]), DecodedOffset: decodedOffset, ReadingOffset: readingOffset})
		}
		if workErr == nil {
			workErr = webPutObject(ctx, transfer.folder, pack.Bytes(), "application/octet-stream")
		}
	}
	stopRenew()
	if err := <-renewed; err != nil {
		workErr = err
	}
	if err := originalReceiveContextError(ctx); err != nil {
		workErr = err
	}
	if workErr != nil {
		var parse *connector.KnowledgeWebParseError
		if errors.As(workErr, &parse) {
			status := 422
			if parse.Code == "web_entry_conflict" {
				status = 409
			} else if parse.Code == "web_budget_exceeded" {
				status = 413
			} else if parse.Code == "web_format_unsupported" || parse.Code == "web_encoding_unsupported" {
				status = 415
			}
			workErr = webError(status, "observe", parse.Code)
		} else if errors.Is(workErr, context.DeadlineExceeded) {
			workErr = webError(504, "observe", "web_parse_timeout")
		} else {
			workErr = webTransferFailure(workErr, "observe")
		}
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	view, err := d.finishWebObservation(finishCtx, transfer, parsed, pack.Bytes(), records, workErr)
	return view, created, err
}

// GetKnowledgeWebObservation discovers the unique observation without parsing.
func (d *DatasetService) GetKnowledgeWebObservation(ctx context.Context, user, domain, registration, id string) (*entity.KnowledgeWebObservation, error) {
	var result *entity.KnowledgeWebObservation
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		_, capture, err := loadWebCapture(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		_, result, err = loadWebObservation(tx, capture, now)
		return err
	})
	return result, err
}
