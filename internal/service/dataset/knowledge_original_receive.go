package dataset

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type originalTransfer struct {
	upload *entity.KnowledgeOriginalUpload
	epoch  int64
	slot   int
	folder string
}

type originalProgressReader struct {
	ctx      context.Context
	body     io.Reader
	received *atomic.Int64
}

func (r *originalProgressReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.body.Read(p)
	r.received.Add(int64(n))
	if contextErr := r.ctx.Err(); contextErr != nil {
		return n, contextErr
	}
	return n, err
}

func originalStateWriteError(state string) error {
	switch state {
	case "expired":
		return originalError(410, "upload_expired")
	case "cancelled":
		return originalError(409, "upload_cancelled")
	case "rejected":
		return originalError(409, "upload_rejected")
	case "ready", "committed":
		return originalError(409, "upload_already_ready")
	case "receiving":
		return originalError(409, "upload_busy")
	case "interrupted":
		return originalError(409, "upload_interrupted")
	default:
		return originalError(409, "upload_state_conflict")
	}
}

func (d *DatasetService) beginOriginalTransfer(ctx context.Context, user, domain, registration, id string, contentLength int64, contentRange, encoding, contentType string) (*originalTransfer, error) {
	var transfer *originalTransfer
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadOriginalUpload(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		if view.State != "awaiting_bytes" && view.State != "interrupted" {
			return &originalCommitError{originalStateWriteError(view.State)}
		}
		if contentRange != "" {
			return originalError(400, "upload_range_unsupported")
		}
		if contentType != "application/octet-stream" {
			return originalError(415, "upload_format_unsupported")
		}
		if encoding != "" && encoding != "identity" {
			return originalError(415, "upload_encoding_unsupported")
		}
		if contentLength >= 0 && contentLength != view.DeclaredByteLength {
			status, code := 400, "upload_length_mismatch"
			if contentLength > entity.KnowledgeOriginalMaxBytes {
				status, code = 413, "upload_too_large"
			}
			view.State, view.Code = "rejected", directoryString(code)
			if err := saveOriginalUpload(tx, row, view, now); err != nil {
				return err
			}
			return &originalCommitError{originalError(status, code)}
		}
		if _, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage); !ok {
			return originalError(503, "original_storage_unavailable")
		}
		var slots []entity.KnowledgeOriginalReceiveSlot
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&slots).Error; err != nil {
			return err
		}
		if len(slots) != 2 || slots[0].ID != 1 || slots[1].ID != 2 {
			return originalError(503, "original_storage_unavailable")
		}
		available := -1
		for i, slot := range slots {
			if slot.LeaseUntil == nil || !now.Before(*slot.LeaseUntil) {
				if available < 0 {
					available = i
				}
			} else if slot.DomainID == domain && slot.OwnerID == user {
				return originalError(429, "upload_slots_busy")
			}
		}
		if available < 0 {
			return originalError(429, "upload_slots_busy")
		}
		row.Epoch++
		lease := now.Add(30 * time.Second)
		row.LeaseUntil = &lease
		view.State, view.Code, view.ReceivedBytes = "receiving", nil, 0
		if err := saveOriginalUpload(tx, row, view, now); err != nil {
			return err
		}
		slot := slots[available]
		if err := tx.Model(&entity.KnowledgeOriginalReceiveSlot{}).Where("id = ?", slot.ID).UpdateColumns(map[string]any{"upload_id": id, "domain_id": domain, "owner_id": user, "epoch": row.Epoch, "lease_until": lease}).Error; err != nil {
			return err
		}
		transfer = &originalTransfer{upload: view, epoch: row.Epoch, slot: slot.ID, folder: *kb.KnowledgeFolderID}
		return nil
	})
	return transfer, err
}

func (d *DatasetService) renewOriginalTransfer(ctx context.Context, transfer *originalTransfer, received int64) error {
	v := transfer.upload
	return d.originalTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadOriginalUpload(tx, kb, v.OwnerID, v.RegistrationID, v.UploadID, "", now)
		if err != nil {
			return err
		}
		if row.Epoch != transfer.epoch || view.State != "receiving" || row.LeaseUntil == nil || !now.Before(*row.LeaseUntil) {
			return &originalCommitError{originalError(409, "upload_interrupted")}
		}
		lease := now.Add(30 * time.Second)
		result := tx.Model(&entity.KnowledgeOriginalReceiveSlot{}).Where("id = ? AND upload_id = ? AND epoch = ? AND lease_until > ?", transfer.slot, v.UploadID, transfer.epoch, now).UpdateColumns(map[string]any{"lease_until": lease})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return originalError(409, "upload_interrupted")
		}
		row.LeaseUntil = &lease
		view.ReceivedBytes = min(received, view.DeclaredByteLength)
		return saveOriginalUpload(tx, row, view, now)
	})
}

func originalAdmissionLimits() knowledgeOriginalLimits {
	return knowledgeOriginalLimits{fileBytes: entity.KnowledgeOriginalMaxBytes, officeEntries: 2048, officeExpandedBytes: 256 << 20,
		officeRatio: 100, officeXMLBytes: 2 << 20, officeAllXMLBytes: 8 << 20, officeDirectoryBytes: 2 << 20, officeXMLDepth: 256}
}

func originalPreparationError(err error, received int64) error {
	switch {
	case received > entity.KnowledgeOriginalMaxBytes:
		return originalError(413, "upload_too_large")
	case errors.Is(err, errOriginalLength):
		return originalError(400, "upload_length_mismatch")
	case errors.Is(err, errOriginalDigest):
		return originalError(400, "upload_sha256_mismatch")
	case errors.Is(err, errOriginalType), errors.Is(err, errOriginalText), errors.Is(err, errOriginalSecret):
		return originalError(415, "upload_format_unsupported")
	case errors.Is(err, context.DeadlineExceeded):
		return originalError(409, "upload_timeout")
	case errors.Is(err, context.Canceled), errors.Is(err, io.ErrUnexpectedEOF):
		return originalError(409, "upload_interrupted")
	default:
		return originalError(503, "original_storage_unavailable")
	}
}

// The HTTP read deadline can cancel the request just before its context timer
// runs. The shared elapsed deadline still identifies a timeout, not a disconnect.
func originalReceiveContextError(ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return ctx.Err()
}

func (d *DatasetService) finishOriginalTransfer(ctx context.Context, transfer *originalTransfer, stage *knowledgeOriginalStage, failure error, received int64) (*entity.KnowledgeOriginalUpload, error) {
	v := transfer.upload
	var result *entity.KnowledgeOriginalUpload
	err := d.originalTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadOriginalUpload(tx, kb, v.OwnerID, v.RegistrationID, v.UploadID, "", now)
		if err != nil {
			return err
		}
		if row.Epoch != transfer.epoch || view.State != "receiving" {
			return &originalCommitError{originalStateWriteError(view.State)}
		}
		if failure != nil {
			var public *KnowledgeError
			if !errors.As(failure, &public) {
				return failure
			}
			view.State, view.Code, view.ReceivedBytes = "interrupted", directoryString(public.Code), min(received, view.DeclaredByteLength)
			if public.Status == 400 || public.Status == 413 || public.Status == 415 {
				view.State = "rejected"
			}
			if err := saveOriginalUpload(tx, row, view, now); err != nil {
				return err
			}
			return &originalCommitError{failure}
		}
		if stage == nil || transfer.folder != *kb.KnowledgeFolderID {
			return originalError(409, "policy_changed")
		}
		action := "created"
		if view.ExpectedRevisionID != nil {
			var previous entity.KnowledgeSourceRevision
			if view.SourceID == nil {
				return errors.New("missing expected original identity")
			}
			if err := tx.Where("id = ? AND domain_id = ? AND document_id = ? AND representation_kind = ? AND original_registration_id = ?", *view.ExpectedRevisionID, v.DomainID, *view.SourceID, "original_only", v.RegistrationID).Take(&previous).Error; err != nil {
				return err
			}
			action = "updated"
			if previous.InputHash == originalInputHash(stage.sha256, view.OriginalFilename, stage.mediaType, view.RelativePath, view.Classification) {
				action = "unchanged"
			}
		}
		view.State, view.Code, view.ReceivedBytes = "ready", nil, stage.byteLength
		view.ByteLength, view.SHA256, view.MediaType, view.Action = directoryInt(stage.byteLength), directoryString(stage.sha256), directoryString(stage.mediaType), directoryString(action)
		view.ManifestSHA256 = directoryString(originalPreviewDigest(view))
		row.ObjectFolderID, row.ObjectLocation, row.NativeHash = transfer.folder, storage.KnowledgeOriginalObjectPrefix+stage.sha256, stage.nativeHash
		row.LeaseUntil = nil
		if err := saveOriginalUpload(tx, row, view, now); err != nil {
			return err
		}
		result = view
		return nil
	})
	return result, err
}

// ReceiveKnowledgeOriginal accepts one complete, bounded binary attempt.
// Body.Close must interrupt a blocked read; the HTTP adapter sets its deadline.
func (d *DatasetService) ReceiveKnowledgeOriginal(ctx context.Context, user, domain, registration, id string, body io.ReadCloser, contentLength int64, contentRange, encoding, contentType string) (*entity.KnowledgeOriginalUpload, error) {
	if body == nil {
		if _, err := d.knowledgeBase(ctx, user, domain, true); err != nil {
			return nil, originalNormalize(err)
		}
		return nil, originalError(400, "knowledge_invalid_request")
	}
	transfer, err := d.beginOriginalTransfer(ctx, user, domain, registration, id, contentLength, contentRange, encoding, contentType)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = d.originalTransaction(cleanupCtx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
			return tx.Model(&entity.KnowledgeOriginalReceiveSlot{}).Where("id = ? AND upload_id = ? AND epoch = ?", transfer.slot, id, transfer.epoch).UpdateColumns(map[string]any{"upload_id": "", "domain_id": "", "owner_id": "", "epoch": int64(0), "lease_until": nil}).Error
		})
	}()
	ctx, stop := context.WithTimeout(ctx, 120*time.Second)
	defer stop()
	var received atomic.Int64
	renewed := make(chan error, 1)
	renewCtx, stopRenew := context.WithCancel(ctx)
	go func() {
		defer close(renewed)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				renewed <- nil
				return
			case <-ticker.C:
				if err := d.renewOriginalTransfer(renewCtx, transfer, received.Load()); err != nil {
					stop()
					renewed <- err
					return
				}
			}
		}
	}()
	stopBody := watchKnowledgeUploadBody(ctx, body)
	defer func() {
		stopRenew()
		for range renewed {
		}
		stopBody()
	}()
	scratch, cleanupScratch, prepareErr := prepareOriginalScratch(ctx, transfer)
	var stage *knowledgeOriginalStage
	if prepareErr == nil {
		defer cleanupScratch()
		expectedSHA := ""
		if transfer.upload.ExpectedSHA256 != nil {
			expectedSHA = *transfer.upload.ExpectedSHA256
		}
		stage, prepareErr = stageKnowledgeOriginal(ctx, scratch, transfer.upload.OriginalFilename, transfer.upload.RelativePath,
			transfer.upload.DeclaredMediaType, &originalProgressReader{ctx, body, &received}, transfer.upload.DeclaredByteLength, expectedSHA, originalAdmissionLimits())
	}
	var failure error
	if prepareErr != nil {
		failure = originalPreparationError(prepareErr, received.Load())
	} else {
		defer stage.close()
		store, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage)
		if !ok {
			failure = originalError(503, "original_storage_unavailable")
		} else if _, err := store.PutImmutableStream(ctx, transfer.folder, storage.KnowledgeOriginalObjectPrefix+stage.sha256, stage.file, stage.byteLength, stage.mediaType, stage.sha256); err != nil {
			failure = originalError(503, "original_storage_unavailable")
		} else if check, err := store.OpenStream(ctx, transfer.folder, storage.KnowledgeOriginalObjectPrefix+stage.sha256, stage.byteLength, stage.sha256, 0, 0); err != nil {
			failure = originalError(503, "original_storage_unavailable")
		} else if err := check.Close(); err != nil {
			failure = originalError(503, "original_storage_unavailable")
		}
	}
	stopRenew()
	if renewalErr := <-renewed; renewalErr != nil {
		failure = renewalErr
	}
	if err := originalReceiveContextError(ctx); err != nil {
		failure = originalPreparationError(err, received.Load())
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	result, err := d.finishOriginalTransfer(finishCtx, transfer, stage, failure, received.Load())
	var public *KnowledgeError
	if errors.As(err, &public) && public.Code == "result_unknown" {
		return nil, originalError(503, "original_storage_unavailable")
	}
	return result, err
}
