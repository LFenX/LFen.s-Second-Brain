package dataset

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

func parsedFinish(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeParsedTask, state, code string, now time.Time) error {
	if structureTerminal(task.State) {
		return nil
	}
	task.State, task.Result, task.FailureCode = state, nil, nil
	if state == "failed" {
		task.FailureCode = &code
	}
	return parsedSaveTask(tx, row, task, now)
}

func (d *DatasetService) parsedCheckpoint(ctx context.Context, claim *structureClaim, renew bool, edit func(*gorm.DB, *entity.KnowledgeStructureTaskRecord, *entity.KnowledgeParsedTask, *parsedInputs, time.Time) error) error {
	var stopped error
	err := structureInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		lease, _, now, err := structureOwnedLease(tx, claim)
		if err != nil {
			return err
		}
		row, task, err := parsedLoadTask(tx, claim.OwnerID, claim.DomainID, claim.TaskID)
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
		var fixed parsedInputs
		if entity.KnowledgeParsedDecode([]byte(row.Inputs), &fixed) != nil {
			return documentError(409, "content_integrity")
		}
		digest, err := parsedTaskDigest(row.OwnerID, row.DomainID, task.OriginalRequest)
		if err != nil || digest != row.RequestSHA256 || fixed.Profile.ProducerSHA256 != row.ProducerSHA256 {
			return documentError(409, "content_integrity")
		}
		current, code, err := parsedCapture(tx, kb, task.OriginalRequest)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				stopped = &documentStopError{code: "source_changed"}
				return nil
			}
			return err
		}
		if code != "" {
			stopped = &documentStopError{code: code}
			return nil
		}
		a, err := structureJSON(fixed)
		b, otherErr := structureJSON(current)
		if err != nil || otherErr != nil || a != b {
			stopped = &documentStopError{code: "source_changed"}
			return nil
		}
		if renew {
			if err := structureRenewClaimLease(tx, claim, lease, now, row.DeadlineAt); err != nil {
				return err
			}
		}
		if edit != nil {
			return edit(tx, row, task, &fixed, now)
		}
		return nil
	})
	if err == nil {
		err = stopped
	}
	return err
}

// A terminal state and released domain fence require current owned-process exit
// and cleanup proof, including a parent crash before the onStarted SQL update.
func (d *DatasetService) parsedFinishClaim(claim *structureClaim, code string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = structureInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		_, _, now, err := structureOwnedLease(tx, claim)
		if err != nil {
			return err
		}
		row, task, err := parsedLoadTask(tx, claim.OwnerID, claim.DomainID, claim.TaskID)
		if err != nil {
			return err
		}
		if err := recoverDocumentExecution(ctx, row); err != nil {
			if task.Phase != "cleaning" {
				task.Phase = "cleaning"
				return parsedSaveTask(tx, row, task, now)
			}
			return nil
		}
		state := "failed"
		if task.State == "cancel_requested" {
			state, code = "cancelled", ""
		}
		if err := parsedFinish(tx, row, task, state, code, now); err != nil {
			return err
		}
		return structureReleaseLease(tx, claim, now)
	})
}

func (d *DatasetService) parsedPublish(ctx context.Context, claim *structureClaim, bound *parsedBound) error {
	return d.parsedCheckpoint(ctx, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeParsedTask, fixed *parsedInputs, now time.Time) error {
		if bound == nil || bound.Representation.Source != fixed.Source || bound.Representation.Ref.ProducerSHA256 == nil || *bound.Representation.Ref.ProducerSHA256 != fixed.Profile.ProducerSHA256 || bound.Representation.MaxUnitBytes != task.OriginalRequest.MaxUnitBytes || row.Execution == "" {
			return documentError(409, "content_integrity")
		}
		var owner parser.DocumentChildOwnership
		if entity.KnowledgeParsedDecode([]byte(row.Execution), &owner) != nil || owner.InputSHA256 != fixed.Source.SHA256 || owner.InputBytes != fixed.ByteLength {
			return documentError(409, "content_integrity")
		}
		if err := recoverDocumentExecution(ctx, row); err != nil {
			return err
		}
		if err := persistKnowledgeParsed(tx, row.OwnerID, row.DomainID, row.ID, bound, now); err != nil {
			return err
		}
		task.State, task.Phase, task.FailureCode = "succeeded", "done", nil
		task.Result = &entity.KnowledgeContentTarget{Source: bound.Representation.Source, Representation: bound.Representation.Ref}
		if err := parsedSaveTask(tx, row, task, now); err != nil {
			return err
		}
		return structureReleaseLease(tx, claim, now)
	})
}

func (d *DatasetService) runKnowledgeParsed(ctx context.Context, claim *structureClaim) {
	deadline, stop := context.WithDeadline(ctx, minTime(time.Now().Add(30*time.Second), claim.Deadline))
	defer stop()
	work, cancel := context.WithCancelCause(deadline)
	defer cancel(nil)
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		ticks := 0
		for {
			select {
			case <-done:
				return
			case <-work.Done():
				return
			case <-ticker.C:
				var row struct{ State string }
				if err := dao.DB.WithContext(work).Model(&entity.KnowledgeStructureTaskRecord{}).Select("state").Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", claim.TaskID, claim.OwnerID, claim.DomainID, entity.KnowledgeParsedProtocol).Take(&row).Error; err != nil {
					cancel(err)
					return
				}
				if row.State == "cancel_requested" {
					cancel(&documentStopError{code: "cancelled", cancelled: true})
					return
				}
				ticks++
				if ticks%40 == 0 {
					beat, beatStop := context.WithTimeout(work, 5*time.Second)
					err := d.parsedCheckpoint(beat, claim, true, nil)
					beatStop()
					if err != nil {
						cancel(err)
						return
					}
				}
			}
		}
	}()
	fail := func(err error, fallback string) {
		if ctx.Err() != nil {
			return
		}
		code := documentFailureFrom(err, fallback)
		var cause *documentStopError
		if errors.As(context.Cause(work), &cause) {
			code = cause.code
		} else if errors.Is(context.Cause(work), context.DeadlineExceeded) {
			code = "native_timeout"
		}
		d.parsedFinishClaim(claim, code)
	}
	var fixed *parsedInputs
	var snapshot *entity.KnowledgeParsedTask
	err := d.parsedCheckpoint(work, claim, false, func(_ *gorm.DB, _ *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeParsedTask, inputs *parsedInputs, _ time.Time) error {
		fixed, snapshot = inputs, task
		return nil
	})
	if err != nil {
		fail(err, "content_integrity")
		return
	}
	source, err := d.parsedAuthorizedSource(work, claim.OwnerID, claim.DomainID, fixed.Source)
	if err != nil {
		fail(err, "original_unavailable")
		return
	}
	input, err := parsedOriginalBytes(work, source)
	if err != nil {
		fail(err, "original_integrity")
		return
	}
	if err := d.parsedCheckpoint(work, claim, false, nil); err != nil {
		fail(err, "content_integrity")
		return
	}
	control := parser.DocumentChildControl{TaskID: claim.TaskID, Attempt: int(snapshot.AttemptCount), Format: fixed.Profile.Format, Filename: fixed.Filename, ProfileID: fixed.Profile.ProfileID, ProducerSHA256: fixed.Profile.ProducerSHA256, InputSHA256: fixed.Source.SHA256, InputBytes: fixed.ByteLength}
	raw, err := parser.RunDocumentStructureChild(work, input, control, func(owner parser.DocumentChildOwnership) error {
		return d.parsedCheckpoint(work, claim, false, func(tx *gorm.DB, row *entity.KnowledgeStructureTaskRecord, task *entity.KnowledgeParsedTask, _ *parsedInputs, now time.Time) error {
			payload, err := structureJSON(owner)
			if err != nil {
				return err
			}
			result := tx.Model(row).Where("id = ? AND protocol = ? AND revision = ?", row.ID, entity.KnowledgeParsedProtocol, row.Revision).Update("execution", payload)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return documentError(409, "content_integrity")
			}
			task.Phase = "extracting"
			return parsedSaveTask(tx, row, task, now)
		})
	})
	if err != nil {
		fail(err, "native_process_failed")
		return
	}
	projection, err := parser.DecodeKnowledgeParsedChild(raw)
	if err != nil {
		fail(err, "invalid_source_structure")
		return
	}
	bound, err := bindKnowledgeParsedProjection(fixed.Source, input, fixed.Profile.ProducerSHA256, projection, snapshot.OriginalRequest.MaxUnitBytes)
	if err != nil {
		fail(err, "invalid_source_structure")
		return
	}
	if err := d.parsedPublish(work, claim, bound); err != nil {
		fail(err, "content_integrity")
	}
}
