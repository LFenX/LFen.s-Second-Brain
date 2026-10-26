package dataset

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/syncer/connector"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type webTransfer struct {
	capture       *entity.KnowledgeWebCapture
	registration  *entity.KnowledgeWebRegistration
	folder        string
	epoch         int64
	slot          int
	operation     string
	observationID string
}

// The production constructor leaves this package-private dependency unset.
// Synthetic tests inject transport results without a private-network override.
type knowledgeWebFetchFunc func(context.Context, string, entity.KnowledgeWebNetworkScope, func(context.Context) error) (*connector.KnowledgeWebFetchResult, error)

func webStateError(state string) error {
	status, code := 409, "web_state_conflict"
	switch state {
	case "expired":
		status, code = 410, "web_capture_expired"
	case "cancelled":
		status, code = 410, "web_capture_cancelled"
	case "rejected":
		code = "web_capture_rejected"
	case "interrupted":
		code = "web_capture_interrupted"
	case "receiving", "fetching":
		code = "web_capture_busy"
	}
	return webError(status, "read", code)
}

func claimWebSlot(tx *gorm.DB, user, domain, capture, operation string, epoch int64, now time.Time) (int, *time.Time, error) {
	var slots []entity.KnowledgeWebSlot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&slots).Error; err != nil {
		return 0, nil, err
	}
	if len(slots) != 2 || slots[0].ID != 1 || slots[1].ID != 2 {
		return 0, nil, webError(503, "read", "web_storage_unavailable")
	}
	available := -1
	for i, slot := range slots {
		if slot.LeaseUntil == nil || !now.Before(*slot.LeaseUntil) {
			if available < 0 {
				available = i
			}
		} else if slot.DomainID == domain && slot.OwnerID == user {
			return 0, nil, webError(429, "read", "web_slots_busy")
		}
	}
	if available < 0 {
		return 0, nil, webError(429, "read", "web_slots_busy")
	}
	lease := now.Add(30 * time.Second)
	slot := slots[available]
	if err := tx.Model(&entity.KnowledgeWebSlot{}).Where("id = ?", slot.ID).UpdateColumns(map[string]any{"domain_id": domain, "owner_id": user, "capture_id": capture, "operation": operation, "epoch": epoch, "lease_until": lease}).Error; err != nil {
		return 0, nil, err
	}
	return slot.ID, &lease, nil
}

func (d *DatasetService) beginWebTransfer(ctx context.Context, user, domain, registration, id, operation string, acquire *entity.KnowledgeWebAcquire, length int64, contentRange, encoding, contentType string) (*webTransfer, *entity.KnowledgeWebCapture, error) {
	var transfer *webTransfer
	var replay *entity.KnowledgeWebCapture
	err := d.webTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadWebCapture(tx, kb, user, registration, id, "", now)
		if err != nil {
			return err
		}
		regRow, reg, err := loadWebRegistration(tx, user, domain, registration, "")
		if err != nil {
			return err
		}
		if operation == "fetching" {
			if acquire == nil || !acquire.Confirmed || acquire.ScopeSHA256 != view.ScopeSHA256 || view.AcquisitionMode != "explicit_fetch" || !reg.NetworkScope.Enabled {
				return webError(400, "fetch", "knowledge_invalid_request")
			}
			if row.Attempted {
				replay = view
				return nil
			}
			if view.State != "awaiting_fetch" {
				return &originalCommitError{webStateError(view.State)}
			}
			if acquire.ExpectedCaptureRevision != view.Revision {
				return webError(409, "fetch", "revision_changed")
			}
		} else {
			if view.AcquisitionMode != "local_snapshot" || !slices.Contains([]string{"awaiting_bytes", "interrupted"}, view.State) {
				return &originalCommitError{webStateError(view.State)}
			}
			if contentRange != "" {
				return webError(416, "upload", "web_range_unsupported")
			}
			if contentType != "application/octet-stream" {
				return webError(415, "upload", "web_format_unsupported")
			}
			if encoding != "" && encoding != "identity" {
				return webError(415, "upload", "web_encoding_unsupported")
			}
			if view.DeclaredByteLength == nil {
				return webError(409, "upload", "web_capture_integrity")
			}
			if length >= 0 && length != *view.DeclaredByteLength {
				code, status := "web_length_mismatch", 400
				if length > 2<<20 {
					code, status = "web_too_large", 413
				}
				view.State = "rejected"
				failure := webError(status, "upload", code)
				view.Failure = &failure.(*KnowledgeWebError).Failure
				if err := saveWebCapture(tx, row, view, now); err != nil {
					return err
				}
				return &originalCommitError{failure}
			}
		}
		if _, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage); !ok {
			return webError(503, "read", "web_storage_unavailable")
		}
		row.Epoch++
		slot, lease, err := claimWebSlot(tx, user, domain, id, operation, row.Epoch, now)
		if err != nil {
			return err
		}
		row.LeaseUntil = lease
		if operation == "fetching" {
			row.Attempted = true
		}
		deadline := now.Add(30 * time.Second)
		if operation == "fetching" {
			deadline = now.Add(15 * time.Second)
		}
		view.State = operation
		view.AttemptStartedAt = directoryString(originalTime(now))
		view.AttemptDeadlineAt = directoryString(originalTime(deadline))
		view.Failure = nil
		view.ReceivedBytes = 0
		if err := saveWebCapture(tx, row, view, now); err != nil {
			return err
		}
		transfer = &webTransfer{capture: view, registration: reg, folder: regRow.FolderID, epoch: row.Epoch, slot: slot, operation: operation}
		return nil
	})
	return transfer, replay, err
}

func (d *DatasetService) checkWebTransfer(ctx context.Context, transfer *webTransfer, renew bool, received int64) error {
	if transfer.operation == "parsing" {
		return d.checkWebObservationLease(ctx, transfer, renew)
	}
	v := transfer.capture
	return d.webTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadWebCapture(tx, kb, v.OwnerID, v.RegistrationID, v.CaptureID, "", now)
		if err != nil {
			return err
		}
		if row.Epoch != transfer.epoch || view.State != transfer.operation || row.LeaseUntil == nil || !now.Before(*row.LeaseUntil) {
			return &originalCommitError{webError(409, "read", "web_capture_interrupted")}
		}
		if !renew {
			return nil
		}
		lease := now.Add(30 * time.Second)
		result := tx.Model(&entity.KnowledgeWebSlot{}).Where("id = ? AND capture_id = ? AND operation = ? AND epoch = ? AND lease_until > ?", transfer.slot, v.CaptureID, transfer.operation, transfer.epoch, now).UpdateColumn("lease_until", lease)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return webError(409, "read", "web_capture_interrupted")
		}
		row.LeaseUntil = &lease
		view.ReceivedBytes = min(received, int64(2<<20))
		return saveWebCapture(tx, row, view, now)
	})
}

func (d *DatasetService) releaseWebTransfer(ctx context.Context, transfer *webTransfer) {
	v := transfer.capture
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = d.webTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		return tx.Model(&entity.KnowledgeWebSlot{}).Where("id = ? AND capture_id = ? AND epoch = ? AND operation = ?", transfer.slot, v.CaptureID, transfer.epoch, transfer.operation).UpdateColumns(map[string]any{"capture_id": "", "domain_id": "", "owner_id": "", "operation": "", "epoch": 0, "lease_until": nil}).Error
	})
}

func webPutObject(ctx context.Context, folder string, raw []byte, media string) error {
	store, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage)
	if !ok {
		return webError(503, "read", "web_storage_unavailable")
	}
	hash := directorySHA(raw)
	if _, err := store.PutImmutableStream(ctx, folder, storage.KnowledgeOriginalObjectPrefix+hash, bytes.NewReader(raw), int64(len(raw)), media, hash); err != nil {
		return webError(503, "read", "web_storage_unavailable")
	}
	check, err := store.OpenStream(ctx, folder, storage.KnowledgeOriginalObjectPrefix+hash, int64(len(raw)), hash, 0, 0)
	if err != nil {
		return webError(503, "read", "web_storage_unavailable")
	}
	if err := check.Close(); err != nil {
		return webError(503, "read", "web_storage_unavailable")
	}
	return nil
}

func webReadObject(ctx context.Context, folder, hash string, size int64) ([]byte, error) {
	if !isKnowledgeID(hash, 64) || size < 0 || size > 2<<20 {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	store, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage)
	if !ok {
		return nil, webError(503, "read", "web_storage_unavailable")
	}
	body, err := store.OpenStream(ctx, folder, storage.KnowledgeOriginalObjectPrefix+hash, size, hash, 0, size)
	if err != nil {
		return nil, webError(503, "read", "web_storage_unavailable")
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, (2<<20)+1))
	if err != nil {
		return nil, webError(503, "read", "web_storage_unavailable")
	}
	if int64(len(raw)) != size || directorySHA(raw) != hash {
		return nil, webError(409, "read", "web_capture_integrity")
	}
	return raw, nil
}

func webTransferFailure(err error, stage string) error {
	if err == nil {
		return nil
	}
	var existing *KnowledgeWebError
	if errors.As(err, &existing) {
		copy := *existing
		copy.Failure.Stage = stage
		return &copy
	}
	var fetch *connector.KnowledgeWebFetchError
	var parse *connector.KnowledgeWebParseError
	if errors.As(err, &parse) {
		status := 415
		if parse.Code == "web_budget_exceeded" || parse.Code == "web_too_large" {
			status = 413
		}
		return webError(status, stage, parse.Code)
	}
	if errors.As(err, &fetch) {
		status := 502
		switch fetch.Code {
		case "web_fetch_timeout":
			status = 504
		case "web_too_large", "web_budget_exceeded":
			status = 413
		case "web_encoding_unsupported", "web_format_unsupported", "web_compression_rejected":
			status = 415
		case "web_result_unknown":
			status = 503
		}
		public := webError(status, stage, fetch.Code).(*KnowledgeWebError)
		public.Failure.UpstreamHTTPStatus = fetch.UpstreamHTTPStatus
		return public
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code := "web_upload_timeout"
		if stage == "fetch" {
			code = "web_fetch_timeout"
		}
		return webError(504, stage, code)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.ErrUnexpectedEOF) {
		return webError(409, stage, "web_capture_interrupted")
	}
	return webError(503, stage, "web_result_unknown")
}

func (d *DatasetService) finishWebTransfer(ctx context.Context, transfer *webTransfer, raw []byte, declaredMedia string, provenance *entity.KnowledgeWebProvenance, failure error, received int64) (*entity.KnowledgeWebCapture, error) {
	v := transfer.capture
	var result *entity.KnowledgeWebCapture
	err := d.webTransaction(ctx, v.OwnerID, v.DomainID, func(tx *gorm.DB, kb *entity.Knowledgebase, now time.Time) error {
		row, view, err := loadWebCapture(tx, kb, v.OwnerID, v.RegistrationID, v.CaptureID, "", now)
		if err != nil {
			return err
		}
		if row.Epoch != transfer.epoch || view.State != transfer.operation {
			return &originalCommitError{webStateError(view.State)}
		}
		if failure != nil {
			stage := "upload"
			if transfer.operation == "fetching" {
				stage = "fetch"
			}
			failure = webTransferFailure(failure, stage)
			public := failure.(*KnowledgeWebError)
			view.State = "rejected"
			if public.Failure.MachineCode == "web_capture_interrupted" || public.Failure.MachineCode == "web_result_unknown" || (stage == "upload" && (public.Failure.MachineCode == "web_upload_timeout" || public.Failure.MachineCode == "web_storage_unavailable")) {
				view.State = "interrupted"
			}
			view.Failure = &public.Failure
			view.ReceivedBytes = min(received, int64(2<<20))
			row.LeaseUntil = nil
			if err := saveWebCapture(tx, row, view, now); err != nil {
				return err
			}
			return &originalCommitError{failure}
		}
		view.State = "ready"
		view.RawSHA256 = directoryString(directorySHA(raw))
		view.RawByteLength = directoryInt(int64(len(raw)))
		view.MediaType = directoryString(declaredMedia)
		view.ReceivedBytes = int64(len(raw))
		view.Failure = nil
		if provenance != nil {
			view.Provenance = *provenance
		} else {
			view.Provenance.ServerReceivedAt = directoryString(originalTime(now))
		}
		row.ObjectFolderID = transfer.folder
		row.LeaseUntil = nil
		if err := saveWebCapture(tx, row, view, now); err != nil {
			return err
		}
		result = view
		return nil
	})
	return result, err
}

// ReceiveKnowledgeWebCapture accepts a complete local snapshot. Body.Close must
// interrupt reads; the HTTP adapter shares the same 30-second read deadline.
func (d *DatasetService) ReceiveKnowledgeWebCapture(ctx context.Context, user, domain, registration, id string, body io.ReadCloser, length int64, contentRange, encoding, contentType string) (*entity.KnowledgeWebCapture, error) {
	if body == nil {
		return nil, webError(400, "upload", "knowledge_invalid_request")
	}
	transfer, _, err := d.beginWebTransfer(ctx, user, domain, registration, id, "receiving", nil, length, contentRange, encoding, contentType)
	if err != nil {
		return nil, err
	}
	defer d.releaseWebTransfer(ctx, transfer)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer body.Close()
	var received atomic.Int64
	stopRenew, renewed := d.runWebRenewal(ctx, transfer, &received, func() { cancel(); _ = body.Close() })
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = body.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	raw, readErr := io.ReadAll(io.LimitReader(&originalProgressReader{ctx: ctx, body: body, received: &received}, (2<<20)+1))
	var failure error
	if readErr != nil {
		failure = webTransferFailure(readErr, "upload")
	} else if len(raw) > 2<<20 {
		failure = webError(413, "upload", "web_too_large")
	} else if transfer.capture.DeclaredByteLength == nil || int64(len(raw)) != *transfer.capture.DeclaredByteLength {
		failure = webError(400, "upload", "web_length_mismatch")
	} else if transfer.capture.ExpectedSHA256 != nil && directorySHA(raw) != *transfer.capture.ExpectedSHA256 {
		failure = webError(422, "upload", "web_sha256_mismatch")
	} else if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		failure = webError(415, "upload", "web_encoding_unsupported")
	}
	media := ""
	if transfer.capture.DeclaredMediaType != nil {
		media = *transfer.capture.DeclaredMediaType
	}
	if failure == nil {
		detected, err := connector.InspectKnowledgeWebMedia(ctx, transfer.registration.SourceKind, media, raw)
		if err != nil {
			failure = webTransferFailure(err, "upload")
		} else {
			media = detected
		}
	}
	if failure == nil {
		failure = webPutObject(ctx, transfer.folder, raw, media)
	}
	stopRenew()
	if err := <-renewed; err != nil {
		failure = err
	}
	if err := originalReceiveContextError(ctx); err != nil {
		failure = webTransferFailure(err, "upload")
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	return d.finishWebTransfer(finishCtx, transfer, raw, media, nil, failure, received.Load())
}

func (d *DatasetService) runWebRenewal(ctx context.Context, transfer *webTransfer, received *atomic.Int64, interrupt func()) (context.CancelFunc, <-chan error) {
	ctx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := d.checkWebTransfer(ctx, transfer, true, received.Load()); err != nil {
					interrupt()
					done <- err
					return
				}
			}
		}
	}()
	return stop, done
}

// AcquireKnowledgeWebCapture is the only online acquisition entry. An attempted
// Capture is replayed, never fetched again, irrespective of its terminal state.
func (d *DatasetService) AcquireKnowledgeWebCapture(ctx context.Context, user, domain, registration, id string, request entity.KnowledgeWebAcquire) (*entity.KnowledgeWebCapture, error) {
	transfer, replay, err := d.beginWebTransfer(ctx, user, domain, registration, id, "fetching", &request, 0, "", "", "")
	if err != nil || replay != nil {
		return replay, err
	}
	defer d.releaseWebTransfer(ctx, transfer)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var received atomic.Int64
	stopRenew, renewed := d.runWebRenewal(ctx, transfer, &received, cancel)
	fetch := d.webFetch
	if fetch == nil {
		fetch = connector.FetchKnowledgeWebCapture
	}
	captured, fetchErr := fetch(ctx, transfer.registration.OriginURL, transfer.registration.NetworkScope, func(ctx context.Context) error { return d.checkWebTransfer(ctx, transfer, false, received.Load()) })
	var raw []byte
	var provenance *entity.KnowledgeWebProvenance
	media := ""
	if captured != nil {
		raw = captured.Raw
		received.Store(int64(len(raw)))
		provenance = &captured.Provenance
		media = captured.DeclaredMediaType
		if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
			fetchErr = webError(415, "fetch", "web_encoding_unsupported")
		} else {
			media, fetchErr = connector.InspectKnowledgeWebMedia(ctx, transfer.registration.SourceKind, media, raw)
			if fetchErr == nil {
				fetchErr = webPutObject(ctx, transfer.folder, raw, media)
			}
		}
	}
	stopRenew()
	if err := <-renewed; err != nil {
		fetchErr = err
	}
	if err := originalReceiveContextError(ctx); err != nil {
		fetchErr = webTransferFailure(err, "fetch")
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	return d.finishWebTransfer(finishCtx, transfer, raw, media, provenance, fetchErr, received.Load())
}
