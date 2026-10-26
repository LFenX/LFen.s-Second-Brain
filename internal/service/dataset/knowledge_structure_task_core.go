package dataset

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/entity"
	parser "ragflow/internal/parser/parser"
)

// Both explicitly admitted structure protocols use this claim, domain quota,
// lease and fencing path. Public payloads remain protocol-specific.
func claimKnowledgeStructure(ctx context.Context, candidate entity.KnowledgeStructureTaskRecord, runner string) (*structureClaim, error) {
	if candidate.Protocol != documentTaskProtocol && !entity.KnowledgeStructureProducerCompatible() {
		return nil, nil
	}
	var claim *structureClaim
	err := structureInternalTransaction(ctx, candidate.DomainID, func(tx *gorm.DB) error {
		var row entity.KnowledgeStructureTaskRecord
		if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ?", candidate.ID, candidate.OwnerID, candidate.DomainID).Take(&row).Error; err != nil {
			return err
		}
		if structureTerminal(row.State) {
			return nil
		}
		now, err := batchNow(tx)
		if err != nil {
			return err
		}
		var lease entity.KnowledgeStructureLeaseRecord
		err = tx.Where("domain_id = ?", row.DomainID).Take(&lease).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && now.Before(lease.ExpiresAt) {
			return nil
		}
		if row.State == "queued" {
			var prior int64
			if err := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("domain_id = ? AND id <> ? AND state IN ?", row.DomainID, row.ID, []string{"running", "cancel_requested"}).Count(&prior).Error; err != nil {
				return err
			}
			if prior > 0 {
				return nil
			}
		}
		var start func() error
		var finish func(string, string) error
		if row.Protocol == documentTaskProtocol {
			_, task, err := documentLoadTask(tx, row.OwnerID, row.DomainID, row.ID)
			if err != nil {
				return err
			}
			// A parent may have died before recording a started child. Recover the
			// deterministic attempt directory as well as any durable ownership.
			if row.AttemptCount > 0 {
				cleanup, cancel := context.WithTimeout(ctx, 2*time.Second)
				err := recoverDocumentExecution(cleanup, &row)
				cancel()
				if err != nil {
					return nil
				} // Unknown ownership keeps the nonterminal fence.
			}
			finish = func(state, code string) error { return documentFinish(tx, &row, task, state, code, now) }
			start = func() error {
				if err := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("id = ? AND revision = ?", row.ID, row.Revision).Update("execution", "").Error; err != nil {
					return err
				}
				task.AttemptCount++
				task.State, task.Phase = "running", "reading_original"
				return documentSaveTask(tx, &row, task, now)
			}
		} else {
			if row.Protocol != "" && row.Protocol != "code_structure/0.1.0" {
				return nil
			}
			_, task, err := structureLoadTask(tx, row.OwnerID, row.DomainID, row.ID)
			if err != nil {
				return err
			}
			finish = func(state, code string) error { return structureFinish(tx, &row, task, state, code, "", now) }
			start = func() error {
				task.AttemptCount++
				task.State = "running"
				for i := range task.Members {
					if task.Members[i].State == "processing" {
						task.Members[i].State = "pending"
					}
				}
				return structureSaveTask(tx, &row, task, now)
			}
		}
		retire := func(state, code string) error {
			if err := finish(state, code); err != nil {
				return err
			}
			if lease.TaskID == row.ID {
				return tx.Model(&entity.KnowledgeStructureLeaseRecord{}).Where("domain_id = ? AND task_id = ? AND epoch = ?", row.DomainID, row.ID, lease.Epoch).Updates(map[string]any{"task_id": "", "runner_id": "", "expires_at": now, "updated_at": now}).Error
			}
			return nil
		}
		if row.State == "cancel_requested" {
			return retire("cancelled", "")
		}
		if !now.Before(row.DeadlineAt) || row.AttemptCount >= 3 {
			code := "execution_expired"
			if row.Protocol == documentTaskProtocol {
				code = "native_timeout"
			}
			return retire("failed", code)
		}
		if _, err := lockKnowledgeImportDomain(ctx, tx, row.OwnerID, row.DomainID); err != nil {
			var public *KnowledgeError
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.As(err, &public) && public.Status == 404 {
				code := "permission_revoked"
				if row.Protocol == documentTaskProtocol {
					code = "source_changed"
				}
				return retire("failed", code)
			}
			return err
		}
		if lease.Epoch >= entity.KnowledgeMaxLifecycleVersion {
			code := "execution_expired"
			if row.Protocol == documentTaskProtocol {
				code = "native_timeout"
			}
			return retire("failed", code)
		}
		lease.DomainID, lease.TaskID, lease.RunnerID = row.DomainID, row.ID, runner
		lease.Epoch++
		lease.ExpiresAt, lease.UpdatedAt = now.Add(30*time.Second), now
		if err := tx.Save(&lease).Error; err != nil {
			return err
		}
		if err := start(); err != nil {
			return err
		}
		claim = &structureClaim{Protocol: row.Protocol, DomainID: row.DomainID, OwnerID: row.OwnerID, TaskID: row.ID, RunnerID: runner, Epoch: lease.Epoch, Deadline: row.DeadlineAt}
		return nil
	})
	return claim, err
}

func recoverDocumentExecution(ctx context.Context, row *entity.KnowledgeStructureTaskRecord) error {
	if row.Execution != "" {
		owner, err := documentDecode[parser.DocumentChildOwnership](row.Execution)
		if err != nil {
			return err
		}
		if owner.TaskID != row.ID || int64(owner.Attempt) != row.AttemptCount {
			return documentError(409, "document_integrity")
		}
		if err := parser.RecoverDocumentStructureChild(ctx, *owner); err != nil {
			return err
		}
	}
	return parser.RecoverDocumentStructureAttempt(ctx, row.ID, int(row.AttemptCount))
}

func structureOwnedLease(tx *gorm.DB, claim *structureClaim) (*entity.KnowledgeStructureLeaseRecord, *entity.KnowledgeStructureTaskRecord, time.Time, error) {
	now, err := batchNow(tx)
	if err != nil {
		return nil, nil, now, err
	}
	var lease entity.KnowledgeStructureLeaseRecord
	if err := tx.Where("domain_id = ?", claim.DomainID).Take(&lease).Error; err != nil {
		return nil, nil, now, err
	}
	if lease.TaskID != claim.TaskID || lease.RunnerID != claim.RunnerID || lease.Epoch != claim.Epoch || !now.Before(lease.ExpiresAt) {
		return nil, nil, now, errStructureLeaseLost
	}
	var row entity.KnowledgeStructureTaskRecord
	if err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", claim.TaskID, claim.OwnerID, claim.DomainID, claim.Protocol).Take(&row).Error; err != nil {
		return nil, nil, now, err
	}
	if structureTerminal(row.State) {
		return nil, nil, now, errStructureLeaseLost
	}
	return &lease, &row, now, nil
}

func structureRenewClaimLease(tx *gorm.DB, claim *structureClaim, lease *entity.KnowledgeStructureLeaseRecord, now, deadline time.Time) error {
	until := minTime(now.Add(30*time.Second), deadline)
	result := tx.Model(lease).Where("domain_id = ? AND epoch = ? AND runner_id = ? AND task_id = ?", claim.DomainID, claim.Epoch, claim.RunnerID, claim.TaskID).Updates(map[string]any{"expires_at": until, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errStructureLeaseLost
	}
	return nil
}

func structureTaskCapacity(tx *gorm.DB, domain string) (bool, error) {
	var active, waiting int64
	if err := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("domain_id = ? AND state IN ?", domain, []string{"queued", "running", "cancel_requested"}).Count(&active).Error; err != nil {
		return false, err
	}
	if err := tx.Model(&entity.KnowledgeStructureTaskRecord{}).Where("domain_id = ? AND state = ?", domain, "queued").Count(&waiting).Error; err != nil {
		return false, err
	}
	return active >= 2 || waiting >= 1, nil
}
