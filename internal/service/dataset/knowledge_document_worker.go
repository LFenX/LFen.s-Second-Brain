package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

type documentStopError struct {
	code      string
	cancelled bool
}

func (e *documentStopError) Error() string { return e.code }

func documentFinish(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, state, code string, now time.Time) error {
	if structureTerminal(task.State) {
		return nil
	}
	task.State, task.Phase = state, "done"
	task.ResultRepresentationID = nil
	task.FailureCode = nil
	if state == "failed" {
		task.FailureCode = &code
	}
	return documentSaveTask(tx, row, task, now)
}

func documentLoadLease(tx *gorm.DB, claim *structureClaim) (*entity.KnowledgeStructureLeaseRecord, *entity.KnowledgeStructureTaskRecord, *entity.KnowledgeDocumentTask, time.Time, error) {
	lease, _, now, err := structureOwnedLease(tx, claim)
	if err != nil {
		return nil, nil, nil, now, err
	}
	row, task, err := documentLoadTask(tx, claim.OwnerID, claim.DomainID, claim.TaskID)
	if err != nil {
		return nil, nil, nil, now, err
	}
	return lease, row, task, now, nil
}

// A stop signal never makes a task terminal here. The owned child and its
// private input must first have a proven exit and cleanup.
func (d *DatasetService) documentCheckpoint(ctx context.Context, claim *structureClaim, renew bool, edit func(*gorm.DB, *entity.KnowledgeStructureTaskRecord, *entity.KnowledgeDocumentTask, *documentInputs, time.Time) error) error {
	var stopped error
	err := structureInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		lease, row, task, now, err := documentLoadLease(tx, claim)
		if err != nil {
			return err
		}
		if task.State == "cancel_requested" {
			stopped = &documentStopError{code: "cancelled", cancelled: true}
			return nil
		}
		if !now.Before(row.DeadlineAt) {
			stopped = &documentStopError{code: "native_timeout"}
			return nil
		}
		kb, err := lockKnowledgeImportDomain(ctx, tx, claim.OwnerID, claim.DomainID)
		if err != nil {
			var denied *KnowledgeError
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.As(err, &denied) && denied.Status == 404 {
				stopped = &documentStopError{code: "source_changed"}
				return nil
			}
			return err
		}
		fixed, err := documentDecode[documentInputs](row.Inputs)
		if err != nil {
			return err
		}
		digest, err := entity.KnowledgeDocumentTaskDigest(row.OwnerID, row.DomainID, task.OriginalRequest)
		if err != nil || digest != row.RequestSHA256 {
			return documentError(409, "document_integrity")
		}
		current, reason, err := documentCapture(tx, kb, task.OriginalRequest)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				stopped = &documentStopError{code: "source_changed"}
				return nil
			}
			return err
		}
		if reason != "" {
			stopped = &documentStopError{code: documentTaskFailure(reason)}
			return nil
		}
		a, _ := structureJSON(fixed)
		b, _ := structureJSON(current)
		if a != b {
			stopped = &documentStopError{code: "source_changed"}
			return nil
		}
		if renew {
			if err := structureRenewClaimLease(tx, claim, lease, now, row.DeadlineAt); err != nil {
				return err
			}
		}
		if edit != nil {
			return edit(tx, row, task, fixed, now)
		}
		return nil
	})
	if err == nil && stopped != nil {
		return stopped
	}
	return err
}

func documentTaskFailure(code string) string {
	switch code {
	case "foundation_policy_changed", "document_policy_changed", "document_policy_disabled":
		return "policy_changed"
	case "source_revision_changed", "source_archived", "unsupported_source":
		return "source_changed"
	case "lifecycle_changed", "head_changed", "producer_changed":
		return code
	}
	return "document_integrity"
}

// Child cleanup uncertainty keeps the execution row nonterminal, including
// after its processing deadline. Queued tasks cannot execute around that fence.
func (d *DatasetService) documentUnresolved(claim *structureClaim) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = structureInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		_, row, task, now, err := documentLoadLease(tx, claim)
		if err != nil {
			return err
		}
		if task.Phase != "cleaning" {
			task.Phase = "cleaning"
			return documentSaveTask(tx, row, task, now)
		}
		return nil
	})
}

func (d *DatasetService) documentFailClaim(claim *structureClaim, code string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = structureInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		_, row, task, now, err := documentLoadLease(tx, claim)
		if err != nil {
			return err
		}
		state := "failed"
		if task.State == "cancel_requested" {
			state, code = "cancelled", ""
		}
		if err := documentFinish(tx, row, task, state, code, now); err != nil {
			return err
		}
		return structureReleaseLease(tx, claim, now)
	})
}

func (d *DatasetService) runKnowledgeDocument(ctx context.Context, claim *structureClaim) {
	deadline, stopDeadline := context.WithDeadline(ctx, minTime(time.Now().Add(30*time.Second), claim.Deadline))
	defer stopDeadline()
	work, cancel := context.WithCancelCause(deadline)
	defer cancel(nil)
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				beat, stop := context.WithTimeout(work, 5*time.Second)
				err := d.documentCheckpoint(beat, claim, true, nil)
				stop()
				if err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				var state struct{ State string }
				err := dao.DB.WithContext(work).Model(&entity.KnowledgeStructureTaskRecord{}).Select("state").Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", claim.TaskID, claim.OwnerID, claim.DomainID, documentTaskProtocol).Take(&state).Error
				if err == nil && state.State == "cancel_requested" {
					cancel(&documentStopError{code: "cancelled", cancelled: true})
					return
				}
			}
		}
	}()
	var fixed *documentInputs
	var snapshot *entity.KnowledgeDocumentTask
	err := d.documentCheckpoint(work, claim, false, func(_ *gorm.DB, _ *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, inputs *documentInputs, _ time.Time) error {
		fixed, snapshot = inputs, task
		return nil
	})
	if err != nil {
		if ctx.Err() == nil {
			d.documentFailClaim(claim, documentFailureFrom(err, "document_integrity"))
		}
		return
	}
	content, err := d.OpenKnowledgeOriginalContent(work, claim.OwnerID, claim.DomainID, fixed.Original.SourceRevisionID, "", "", false)
	if err != nil {
		d.documentFailClaim(claim, "original_unavailable")
		return
	}
	bytes, readErr := io.ReadAll(io.LimitReader(content.Body, 8<<20+1))
	closeErr := content.Body.Close()
	if readErr != nil || closeErr != nil {
		d.documentFailClaim(claim, "original_unavailable")
		return
	}
	if int64(len(bytes)) != fixed.Original.ByteLength || directorySHA(bytes) != fixed.Original.SHA256 {
		d.documentFailClaim(claim, "original_integrity")
		return
	}
	err = d.documentCheckpoint(work, claim, false, nil)
	if err != nil {
		d.documentFailClaim(claim, documentFailureFrom(err, "document_integrity"))
		return
	}
	control := parser.DocumentChildControl{TaskID: claim.TaskID, Attempt: int(snapshot.AttemptCount), Format: fixed.Profile.Format, ProfileID: fixed.Profile.ProfileID, ProducerSHA256: fixed.Profile.ProducerSHA256, InputSHA256: fixed.Original.SHA256, InputBytes: fixed.Original.ByteLength}
	raw, err := parser.RunDocumentStructureChild(work, bytes, control, func(owner parser.DocumentChildOwnership) error {
		return d.documentCheckpoint(work, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, _ *documentInputs, now time.Time) error {
			payload, err := structureJSON(owner)
			if err != nil {
				return err
			}
			if err := tx.Model(row).Where("id = ? AND revision = ?", row.ID, row.Revision).Update("execution", payload).Error; err != nil {
				return err
			}
			task.Phase = "extracting"
			return documentSaveTask(tx, row, task, now)
		})
	})
	if errors.Is(err, parser.ErrDocumentChildUnresolved) {
		d.documentUnresolved(claim)
		return
	}
	if err != nil {
		if ctx.Err() == nil {
			code := documentFailureFrom(err, "native_process_failed")
			if work.Err() != nil {
				cause := context.Cause(work)
				var stopped *documentStopError
				if errors.As(cause, &stopped) {
					code = stopped.code
				} else if errors.Is(cause, context.DeadlineExceeded) {
					code = "native_timeout"
				} else {
					return
				}
			}
			d.documentFailClaim(claim, code)
		}
		return
	}
	var parsed parser.DocumentStructureResult
	if len(raw) > 16<<20 || json.Unmarshal(raw, &parsed) != nil {
		d.documentFailClaim(claim, "document_integrity")
		return
	}
	err = d.documentCheckpoint(work, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, _ *documentInputs, now time.Time) error {
		task.Phase = "projecting"
		for _, item := range parsed.Diagnostics {
			if len(task.Diagnostics) >= 8 {
				break
			}
			task.Diagnostics = append(task.Diagnostics, entity.KnowledgeDocumentTaskDiagnostic{Code: item.Code, Message: item.Message, NativeLocator: item.NativeLocator})
		}
		return documentSaveTask(tx, row, task, now)
	})
	if err != nil {
		d.documentFailClaim(claim, documentFailureFrom(err, "document_integrity"))
		return
	}
	bound, err := documentBind(claim.OwnerID, claim.DomainID, fixed, parsed)
	if err != nil {
		d.documentFailClaim(claim, "document_integrity")
		return
	}
	err = d.documentCheckpoint(work, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, _ *documentInputs, now time.Time) error {
		task.Phase = "staged"
		return documentSaveTask(tx, row, task, now)
	})
	if err != nil {
		d.documentFailClaim(claim, documentFailureFrom(err, "document_integrity"))
		return
	}
	err = d.documentPublish(work, claim, bound)
	if err != nil && ctx.Err() == nil {
		d.documentFailClaim(claim, documentFailureFrom(err, "document_integrity"))
	}
}

func documentFailureFrom(err error, fallback string) string {
	var stop *documentStopError
	if errors.As(err, &stop) {
		return stop.code
	}
	var controlled interface{ FailureCode() string }
	if errors.As(err, &controlled) {
		code := controlled.FailureCode()
		if slices.Contains([]string{"original_unavailable", "original_integrity", "unsupported_document", "encrypted_document", "source_structure_unavailable", "invalid_source_structure", "budget_exceeded", "native_timeout", "native_process_failed", "source_changed", "lifecycle_changed", "policy_changed", "head_changed", "producer_changed", "document_integrity", "cleanup_failed"}, code) {
			return code
		}
	}
	return fallback
}

func (d *DatasetService) documentPublish(ctx context.Context, claim *structureClaim, bound *documentBound) error {
	return d.documentCheckpoint(ctx, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeDocumentTask, _ *documentInputs, now time.Time) error {
		rep := bound.Representation
		var prior entity.KnowledgeDocumentRepresentationRecord
		err := tx.Where("id = ?", rep.RepresentationID).Take(&prior).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if prior.OwnerID != claim.OwnerID || prior.DomainID != claim.DomainID || prior.SourceID != rep.Original.SourceID {
				return documentError(409, "document_integrity")
			}
			if err := documentVerifyPublished(tx, &prior, bound); err != nil {
				return err
			}
		} else {
			rep.PublishedAt = structureTimestamp(now)
			payload, err := structureJSON(rep)
			if err != nil {
				return err
			}
			manifest, err := structureJSON(bound.Manifest)
			if err != nil {
				return err
			}
			if err := tx.Create(&entity.KnowledgeDocumentRepresentationRecord{ID: rep.RepresentationID, OwnerID: claim.OwnerID, DomainID: claim.DomainID, SourceID: rep.Original.SourceID, SourceRevisionID: rep.Original.SourceRevisionID, TaskID: claim.TaskID, Payload: payload, ReadingText: bound.ReadingText, Manifest: manifest, PublishedAt: now}).Error; err != nil {
				return err
			}
			if err := tx.CreateInBatches(&bound.Nodes, 128).Error; err != nil {
				return err
			}
		}
		_, _, _, finalNow, err := documentLoadLease(tx, claim)
		if err != nil {
			return err
		}
		if !finalNow.Before(row.DeadlineAt) {
			return errStructureLeaseLost
		}
		if err := tx.Save(&entity.KnowledgeDocumentHeadRecord{SourceID: rep.Original.SourceID, DomainID: claim.DomainID, OwnerID: claim.OwnerID, RepresentationID: rep.RepresentationID, UpdatedAt: finalNow}).Error; err != nil {
			return err
		}
		task.State, task.Phase = "succeeded", "done"
		task.ResultRepresentationID = &rep.RepresentationID
		task.FailureCode = nil
		if err := documentSaveTask(tx, row, task, finalNow); err != nil {
			return err
		}
		return structureReleaseLease(tx, claim, finalNow)
	})
}

func documentVerifyPublished(tx *gorm.DB, prior *entity.KnowledgeDocumentRepresentationRecord, bound *documentBound) error {
	invalid := func() error { return documentError(409, "document_integrity") }
	stored, err := documentDecode[entity.KnowledgeDocumentRepresentation](prior.Payload)
	if err != nil {
		return err
	}
	digest, err := entity.KnowledgeDocumentRepresentationDigest(*stored)
	if err != nil || digest != bound.Representation.RepresentationID || stored.RepresentationID != digest || stored.RootDocumentNodeID != bound.Representation.RootDocumentNodeID || stored.NodeCount != bound.Representation.NodeCount || prior.ReadingText != bound.ReadingText {
		return invalid()
	}
	manifest, err := documentDecode[entity.KnowledgeDocumentStructureManifest](prior.Manifest)
	if err != nil {
		return err
	}
	manifestSHA, err := entity.KnowledgeDocumentManifestDigest(manifest.Nodes)
	if err != nil || manifestSHA != bound.Representation.StructureManifestSHA256 {
		return invalid()
	}
	var rows []entity.KnowledgeDocumentNodeRecord
	if err := tx.Where("owner_id = ? AND domain_id = ? AND representation_id = ?", prior.OwnerID, prior.DomainID, prior.ID).Limit(8193).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(bound.Nodes) {
		return invalid()
	}
	byID := map[string]entity.KnowledgeDocumentNodeRecord{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, expected := range bound.Nodes {
		got, ok := byID[expected.ID]
		if !ok || got.ParentID != expected.ParentID || got.Ordinal != expected.Ordinal || got.Payload != expected.Payload {
			return invalid()
		}
	}
	return nil
}
