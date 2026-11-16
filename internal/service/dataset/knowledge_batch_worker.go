package dataset

import (
	"context"
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

// batchInternalTransaction may retire work after permission revocation. It never
// grants admission: every source publish separately checks the original owner.
func batchInternalTransaction(ctx context.Context, domain string, fn func(*gorm.DB) error) error {
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dao.LockKnowledgeAccessDomain(ctx, tx, domain); err != nil {
			return err
		}
		var kb entity.Knowledgebase
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", domain).Take(&kb).Error; e != nil {
			return e
		}
		return fn(tx)
	})
}
func failBatchRemaining(tx *gorm.DB, row *entity.KnowledgeBatchTaskRecord, t *entity.KnowledgeBatchTask, code string, now time.Time) error {
	items, e := batchItems(tx, row.ID)
	if e != nil {
		return e
	}
	for n := range items {
		r := &items[n]
		if r.Status != "pending" && r.Status != "running" {
			continue
		}
		v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](r.ResultJSON)
		if e != nil {
			return e
		}
		v.Status = "failed"
		v.CatalogueStatus = "failed"
		v.Code = code
		v.SaveStatus = nil
		v.SourceID = nil
		v.SourceRevisionID = nil
		v.RootNodeID = nil
		if e = saveBatchItem(tx, r, v); e != nil {
			return e
		}
	}
	row.LeaseEpoch++
	if e = tx.Model(row).Update("lease_epoch", row.LeaseEpoch).Error; e != nil {
		return e
	}
	return refreshBatchTask(tx, row, t, now)
}
func claimKnowledgeBatch(ctx context.Context, domain, id, owner string) (*entity.KnowledgeBatchTaskRecord, error) {
	var out *entity.KnowledgeBatchTaskRecord
	e := batchInternalTransaction(ctx, domain, func(tx *gorm.DB) error {
		var row entity.KnowledgeBatchTaskRecord
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND domain_id = ?", id, domain).Take(&row).Error; e != nil {
			return e
		}
		if batchTerminal(row.Status) {
			return nil
		}
		t, e := batchUnmarshal[entity.KnowledgeBatchTask](row.Payload)
		if e != nil {
			return e
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		if !now.Before(row.DeadlineAt) {
			return failBatchRemaining(tx, &row, t, "execution_expired", now)
		}
		if row.LeaseUntil != nil && now.Before(*row.LeaseUntil) {
			return nil
		}
		var active int64
		if e = tx.Model(&entity.KnowledgeBatchTaskRecord{}).Where("domain_id = ? AND id <> ? AND status IN ? AND lease_until > ?", domain, id, []string{"queued", "running", "cancelling"}, now).Count(&active).Error; e != nil {
			return e
		}
		if active > 0 {
			return nil
		}
		row.LeaseEpoch++
		row.LeaseOwner = owner
		until := now.Add(time.Minute)
		row.LeaseUntil = &until
		// Only an expired owner can leave a running item without a committed result.
		items, e := batchItems(tx, id)
		if e != nil {
			return e
		}
		for n := range items {
			r := &items[n]
			if r.Status == "running" {
				v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](r.ResultJSON)
				if e != nil {
					return e
				}
				v.Status = "pending"
				v.Code = "awaiting_execution"
				v.CatalogueStatus = "pending"
				if e = saveBatchItem(tx, r, v); e != nil {
					return e
				}
			}
		}
		if e = tx.Model(&row).Updates(map[string]any{"lease_epoch": row.LeaseEpoch, "lease_owner": owner, "lease_until": until}).Error; e != nil {
			return e
		}
		if e = refreshBatchTask(tx, &row, t, now); e != nil {
			return e
		}
		out = &row
		return nil
	})
	return out, e
}
func loadBatchLease(tx *gorm.DB, claim *entity.KnowledgeBatchTaskRecord) (*entity.KnowledgeBatchTaskRecord, *entity.KnowledgeBatchTask, time.Time, error) {
	var row entity.KnowledgeBatchTaskRecord
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND domain_id = ?", claim.ID, claim.DomainID).Take(&row).Error; e != nil {
		return nil, nil, time.Time{}, e
	}
	now, e := batchNow(tx)
	if e != nil {
		return nil, nil, now, e
	}
	if row.LeaseEpoch != claim.LeaseEpoch || row.LeaseOwner != claim.LeaseOwner || row.LeaseUntil == nil || !now.Before(*row.LeaseUntil) || batchTerminal(row.Status) {
		return nil, nil, now, errBatchLeaseLost
	}
	t, e := batchUnmarshal[entity.KnowledgeBatchTask](row.Payload)
	return &row, t, now, e
}

var errBatchLeaseLost = errors.New("batch lease no longer owned")

func beginKnowledgeBatchItem(ctx context.Context, d *DatasetService, claim *entity.KnowledgeBatchTaskRecord) (string, error) {
	id := ""
	e := batchInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		row, t, now, e := loadBatchLease(tx, claim)
		if e != nil {
			return e
		}
		if !now.Before(row.DeadlineAt) {
			return failBatchRemaining(tx, row, t, "execution_expired", now)
		}
		if _, e = lockKnowledgeImportDomain(tx.Statement.Context, tx, row.UserID, row.DomainID); e != nil {
			return failBatchRemaining(tx, row, t, "permission_revoked", now)
		}
		if _, e = loadDirectoryScope(tx, row.UserID, row.DomainID, row.RegistrationID); e != nil {
			return failBatchRemaining(tx, row, t, "permission_revoked", now)
		}
		var item entity.KnowledgeBatchItemRecord
		e = tx.Where("task_id = ? AND status = ?", row.ID, "pending").Order("ordinal ASC").Take(&item).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](item.ResultJSON)
		if e != nil {
			return e
		}
		v.Status = "running"
		v.Code = "running"
		if e = saveBatchItem(tx, &item, v); e != nil {
			return e
		}
		if e = refreshBatchTask(tx, row, t, now); e != nil {
			return e
		}
		id = item.ID
		return nil
	})
	return id, e
}
func batchFailureCode(err error) string {
	var public *KnowledgeDirectoryError
	if errors.As(err, &public) {
		if public.Reason != nil {
			switch *public.Reason {
			case "root_changed", "policy_changed", "revision_changed", "source_changed", "root_unready":
				return *public.Reason
			}
		}
		if public.Status == 404 {
			return "permission_revoked"
		}
	}
	return "save_failed"
}
func publishBatchItem(ctx context.Context, tx *gorm.DB, d *DatasetService, row *entity.KnowledgeBatchTaskRecord, t *entity.KnowledgeBatchTask, record *entity.KnowledgeBatchItemRecord, v *entity.KnowledgeBatchTaskItem) error {
	kb, e := lockKnowledgeImportDomain(ctx, tx, row.UserID, row.DomainID)
	if e != nil {
		return batchError(404, "not_found", "")
	}
	scope, e := loadDirectoryScope(tx, row.UserID, row.DomainID, row.RegistrationID)
	if e != nil {
		return e
	}
	_, p, e := loadBatchPreview(tx, row.UserID, row.DomainID, row.RegistrationID, row.PreviewID)
	if e != nil {
		return e
	}
	if e = directoryCheckVersions(scope, kb, p.RootVersion, p.PolicyVersion); e != nil {
		return e
	}
	i, e := batchUnmarshal[entity.KnowledgeBatchPreviewItem](record.PreviewJSON)
	if e != nil {
		return e
	}
	if !scope.contains(i.RelativePath) {
		return batchConflict("policy_changed")
	}
	if e := lifecycleCheckPreparation(tx, row.DomainID, "text_preview", i.ItemID, i.RelativePath, lifecycleSourceValue(i.SourceID)); e != nil {
		return batchConflict("revision_changed")
	}
	if e = knowledgeImportClassification(tx, row.DomainID, i.Classification.GroupID, i.Classification.ComponentSide, i.Classification.Language); e != nil {
		return batchConflict("policy_changed")
	}
	binding, e := directoryVerifyHead(tx, row.DomainID, row.RegistrationID, i.RelativePath, i.SourceID, i.ExpectedRevisionID)
	if e != nil {
		return e
	}
	root, e := directoryOpenRoot(tx, scope.Config)
	if e != nil {
		return e
	}
	defer root.Close()
	body, identity, e := utility.ReadKnowledgeDirectoryFile(ctx, root, i.RelativePath, entity.KnowledgeMaxSourceBytes)
	if e != nil || identity != record.Identity || directorySHA(body) != i.SHA256 || int64(len(body)) != i.ByteLength || directorySHA([]byte(record.Content)) != i.SHA256 || int64(len(record.Content)) != i.ByteLength || directoryTextReason(i.RelativePath, body) != "" {
		return batchConflict("source_changed")
	}
	latest, e := directoryConfig(row.UserID, row.DomainID, scope.Config.RootID)
	if e != nil {
		return e
	}
	if latest.RootVersion != p.RootVersion {
		return batchConflict("root_changed")
	}
	native := entity.KnowledgeImportPreview{ID: i.ItemID, RegistrationID: row.RegistrationID, PolicyVersion: p.PolicyVersion, RelativePath: i.RelativePath, GroupID: i.Classification.GroupID, ComponentSide: i.Classification.ComponentSide, Language: i.Classification.Language, Content: record.Content, SHA256: i.SHA256, ByteLength: i.ByteLength, DocumentID: i.SourceID, Action: i.Action, ManifestSHA256: p.ManifestSHA256, ExpiresAt: row.DeadlineAt}
	if i.ExpectedRevisionID != nil {
		native.ExpectedRevisionID = *i.ExpectedRevisionID
	}
	snapshot, e := publishKnowledgeImport(ctx, tx, kb, row.UserID, binding, &native)
	if e != nil {
		return e
	}
	v.Status = "succeeded"
	v.CatalogueStatus = "registered"
	v.Code = "saved"
	saved := "created"
	if i.Action == "update" {
		saved = "updated"
	} else if i.Action == "unchanged" {
		saved = "unchanged"
	}
	v.SaveStatus = &saved
	v.SourceID = &snapshot.Source.DocumentID
	v.SourceRevisionID = &snapshot.Source.ID
	v.RootNodeID = &snapshot.Root.ID
	return nil
}
func executeKnowledgeBatchItem(ctx context.Context, d *DatasetService, claim *entity.KnowledgeBatchTaskRecord, id string) error {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return batchInternalTransaction(work, claim.DomainID, func(tx *gorm.DB) error {
		row, t, now, e := loadBatchLease(tx, claim)
		if e != nil {
			return e
		}
		if !now.Before(row.DeadlineAt) {
			return failBatchRemaining(tx, row, t, "execution_expired", now)
		}
		var record entity.KnowledgeBatchItemRecord
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND task_id = ?", id, row.ID).Take(&record).Error; e != nil {
			return e
		}
		if record.Status != "running" {
			return errBatchLeaseLost
		}
		v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](record.ResultJSON)
		if e != nil {
			return e
		}
		if e = tx.SavePoint("batch_publish").Error; e != nil {
			return e
		}
		failure := publishBatchItem(work, tx, d, row, t, &record, v)
		if failure != nil {
			if e = tx.RollbackTo("batch_publish").Error; e != nil {
				return e
			}
			v = pendingBatchResultValue(v)
			v.Status = "failed"
			v.CatalogueStatus = "failed"
			v.Code = batchFailureCode(failure)
		}
		// A delayed storage call cannot commit with an expired epoch or deadline.
		row, t, now, e = loadBatchLease(tx, claim)
		if e != nil {
			return e
		}
		if !now.Before(row.DeadlineAt) {
			return errBatchLeaseLost
		}
		if e = saveBatchItem(tx, &record, v); e != nil {
			return e
		}
		return refreshBatchTask(tx, row, t, now)
	})
}
func pendingBatchResultValue(v *entity.KnowledgeBatchTaskItem) *entity.KnowledgeBatchTaskItem {
	copy := *v
	copy.SaveStatus = nil
	copy.SourceID = nil
	copy.SourceRevisionID = nil
	copy.RootNodeID = nil
	return &copy
}
func renewKnowledgeBatch(ctx context.Context, claim *entity.KnowledgeBatchTaskRecord) error {
	return batchInternalTransaction(ctx, claim.DomainID, func(tx *gorm.DB) error {
		row, _, now, e := loadBatchLease(tx, claim)
		if e != nil {
			return e
		}
		if !now.Before(row.DeadlineAt) {
			return errBatchLeaseLost
		}
		until := minTime(now.Add(time.Minute), row.DeadlineAt)
		return tx.Model(row).Update("lease_until", until).Error
	})
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func runKnowledgeBatch(ctx context.Context, d *DatasetService, claim *entity.KnowledgeBatchTaskRecord) {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-ticker.C:
				renewCtx, done := context.WithTimeout(work, 5*time.Second)
				e := renewKnowledgeBatch(renewCtx, claim)
				done()
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); heartbeat.Wait() }()
	for work.Err() == nil {
		id, e := beginKnowledgeBatchItem(work, d, claim)
		if e != nil || id == "" {
			return
		}
		if e = executeKnowledgeBatchItem(work, d, claim, id); e != nil {
			return
		}
	}
}

// StartKnowledgeBatchRuntime performs catalogue-only work. It never dispatches
// ingestion, parsing, indexing or model tasks, even if their workers are enabled.
func StartKnowledgeBatchRuntime(ctx context.Context) {
	if dao.DB == nil {
		return
	}
	d := NewDatasetService()
	slots := make(chan struct{}, 2)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if e := cleanupKnowledgeBatchTemporary(ctx); e != nil && ctx.Err() == nil {
					common.Warn("Knowledge batch temporary cleanup failed")
				}
				var candidates []entity.KnowledgeBatchTaskRecord
				now, e := batchNow(dao.DB.WithContext(ctx))
				if e != nil {
					continue
				}
				e = dao.DB.WithContext(ctx).Where("status IN ? AND (lease_until IS NULL OR lease_until <= ? OR deadline_at <= ?)", []string{"queued", "running", "cancelling"}, now, now).Order("created_at ASC, id ASC").Limit(16).Find(&candidates).Error
				if e != nil {
					continue
				}
				for _, candidate := range candidates {
					select {
					case slots <- struct{}{}:
						go func(record entity.KnowledgeBatchTaskRecord) {
							defer func() { <-slots }()
							claimCtx, done := context.WithTimeout(ctx, 10*time.Second)
							claim, e := claimKnowledgeBatch(claimCtx, record.DomainID, record.ID, common.GenerateUUID())
							done()
							if e == nil && claim != nil {
								runKnowledgeBatch(ctx, d, claim)
							}
						}(candidate)
					default:
					}
				}
			}
		}
	}()
}
func cleanupKnowledgeBatchTemporary(ctx context.Context) error {
	tx := dao.DB.WithContext(ctx)
	now, e := batchNow(tx)
	if e != nil {
		return e
	}
	ids := tx.Model(&entity.KnowledgeBatchPreviewRecord{}).Select("id").Where("consumed_task_id IS NULL AND expires_at <= ?", now)
	if e = tx.Model(&entity.KnowledgeBatchItemRecord{}).Where("preview_id IN (?) AND task_id IS NULL", ids).Updates(map[string]any{"content": "", "identity": ""}).Error; e != nil {
		return e
	}
	return tx.Where("kind IN ? AND expires_at <= ?", []string{"tasks", "history"}, now).Delete(&entity.KnowledgeBatchViewRecord{}).Error
}
