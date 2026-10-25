package dataset

import (
	"context"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func loadBatchTask(tx *gorm.DB, user, domain, registration, id, key string) (*entity.KnowledgeBatchTaskRecord, *entity.KnowledgeBatchTask, error) {
	q := tx.Where("domain_id = ? AND user_id = ? AND registration_id = ?", domain, user, registration)
	if key != "" {
		q = q.Where("idempotency_key = ?", key)
	} else {
		if !isKnowledgeID(id, 32) {
			return nil, nil, batchMissing()
		}
		q = q.Where("id = ?", id)
	}
	var row entity.KnowledgeBatchTaskRecord
	if e := q.Take(&row).Error; e != nil {
		return nil, nil, e
	}
	task, e := batchUnmarshal[entity.KnowledgeBatchTask](row.Payload)
	return &row, task, e
}
func batchItems(tx *gorm.DB, task string) ([]entity.KnowledgeBatchItemRecord, error) {
	var items []entity.KnowledgeBatchItemRecord
	e := tx.Where("task_id = ?", task).Order("ordinal ASC").Find(&items).Error
	return items, e
}
func pendingBatchResult(i entity.KnowledgeBatchPreviewItem) entity.KnowledgeBatchTaskItem {
	return entity.KnowledgeBatchTaskItem{ItemID: i.ItemID, Ordinal: i.Ordinal, RelativePath: i.RelativePath, Classification: i.Classification, SHA256: i.SHA256, ByteLength: i.ByteLength, Status: "pending", CatalogueStatus: "pending", StructureStatus: "deferred", FullTextStatus: "disabled", VectorStatus: "disabled", Code: "awaiting_execution"}
}
func saveBatchItem(tx *gorm.DB, row *entity.KnowledgeBatchItemRecord, item *entity.KnowledgeBatchTaskItem) error {
	values := map[string]any{"status": item.Status, "result_json": directoryJSON(item)}
	if item.Status != "pending" && item.Status != "running" {
		values["content"] = ""
		values["identity"] = ""
	}
	return tx.Model(row).Updates(values).Error
}
func refreshBatchTask(tx *gorm.DB, row *entity.KnowledgeBatchTaskRecord, task *entity.KnowledgeBatchTask, now time.Time) error {
	items, e := batchItems(tx, row.ID)
	if e != nil {
		return e
	}
	c := entity.KnowledgeBatchCounts{Total: int64(len(items))}
	for _, i := range items {
		switch i.Status {
		case "pending":
			c.Pending++
		case "running":
			c.Running++
		case "succeeded":
			c.Succeeded++
		case "failed":
			c.Failed++
		case "cancelled":
			c.Cancelled++
		default:
			return errors.New("invalid batch item state")
		}
	}
	task.Counts = c
	task.Revision++
	task.UpdatedAt = directoryTime(now)
	if c.Pending+c.Running == 0 {
		switch {
		case task.CancelRequested:
			task.Status = "cancelled"
		case c.Succeeded == c.Total:
			task.Status = "succeeded"
		case c.Succeeded > 0:
			task.Status = "partial_failed"
		default:
			task.Status = "failed"
		}
		value := directoryTime(now)
		task.TerminalAt = &value
		row.LeaseUntil = nil
		row.LeaseOwner = ""
	} else if task.CancelRequested {
		task.Status = "cancelling"
	} else if row.LeaseOwner != "" {
		task.Status = "running"
	}
	row.Status = task.Status
	row.Payload = directoryJSON(task)
	return tx.Model(row).Updates(map[string]any{"payload": row.Payload, "status": row.Status, "lease_until": row.LeaseUntil, "lease_owner": row.LeaseOwner}).Error
}
func validateBatchRetry(tx *gorm.DB, user, domain, reg string, req entity.KnowledgeBatchAcceptRequest, p *entity.KnowledgeBatchPreview) error {
	if req.RetryOfTaskID == nil {
		return nil
	}
	if !isKnowledgeID(*req.RetryOfTaskID, 32) {
		return batchConflict("retry_parent_invalid")
	}
	_, parent, e := loadBatchTask(tx, user, domain, reg, *req.RetryOfTaskID, "")
	if e != nil {
		return e
	}
	if !slices.Contains([]string{"partial_failed", "failed", "cancelled"}, parent.Status) {
		return batchConflict("retry_parent_invalid")
	}
	items, e := batchItems(tx, parent.TaskID)
	if e != nil {
		return e
	}
	all := make([]string, 0, len(items))
	unfinished := map[string]bool{}
	for _, row := range items {
		v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](row.ResultJSON)
		if e != nil {
			return e
		}
		all = append(all, v.RelativePath)
		if row.Status != "succeeded" {
			unfinished[v.RelativePath] = true
		}
	}
	selected := make([]string, 0, len(p.Items))
	for _, i := range p.Items {
		selected = append(selected, i.RelativePath)
	}
	if slices.Equal(all, selected) {
		return nil
	}
	for _, path := range selected {
		if !unfinished[path] {
			return batchConflict("retry_parent_invalid")
		}
	}
	return nil
}
func (d *DatasetService) AcceptKnowledgeBatchTask(ctx context.Context, user, domain, registration string, req entity.KnowledgeBatchAcceptRequest) (*entity.KnowledgeBatchTask, bool, error) {
	if !isKnowledgeID(req.PreviewID, 32) || !isKnowledgeID(req.ManifestSHA256, 64) || !batchKey(req.IdempotencyKey) || len(req.ConfirmedItemIDs) < 1 || len(req.ConfirmedItemIDs) > 32 {
		return nil, false, batchInvalid()
	}
	var out *entity.KnowledgeBatchTask
	created := false
	err := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		scope, e := loadDirectoryScope(tx, user, domain, registration)
		if e != nil {
			return e
		}
		old, task, e := loadBatchTask(tx, user, domain, registration, "", req.IdempotencyKey)
		if e == nil {
			if old.RequestJSON != directoryJSON(req) {
				return batchConflict("idempotency_conflict")
			}
			out = task
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		row, p, e := loadBatchPreview(tx, user, domain, registration, req.PreviewID)
		if e != nil {
			return e
		}
		if row.ConsumedTaskID != nil {
			return batchConflict("preview_consumed")
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		if !now.Before(row.ExpiresAt) {
			return batchError(410, "expired", "")
		}
		if e = directoryCheckVersions(scope, kb, p.RootVersion, p.PolicyVersion); e != nil {
			return e
		}
		if req.ManifestSHA256 != p.ManifestSHA256 {
			return batchConflict("observation_mismatch")
		}
		ids := make([]string, 0, len(p.Items))
		for _, i := range p.Items {
			ids = append(ids, i.ItemID)
			if !scope.contains(i.RelativePath) {
				return batchInvalid()
			}
			if e = knowledgeImportClassification(tx, domain, i.Classification.GroupID, i.Classification.ComponentSide, i.Classification.Language); e != nil {
				return e
			}
			if _, e = directoryVerifyHead(tx, domain, registration, i.RelativePath, i.SourceID, i.ExpectedRevisionID); e != nil {
				return e
			}
		}
		if !slices.Equal(ids, req.ConfirmedItemIDs) {
			return batchConflict("confirmation_mismatch")
		}
		if e = validateBatchRetry(tx, user, domain, registration, req, p); e != nil {
			return e
		}
		var count int64
		if e = tx.Model(&entity.KnowledgeBatchTaskRecord{}).Where("domain_id = ? AND user_id = ? AND status IN ?", domain, user, []string{"queued", "running", "cancelling"}).Count(&count).Error; e != nil {
			return e
		}
		if count >= 2 {
			return batchError(429, "capacity", "")
		}
		task = &entity.KnowledgeBatchTask{TaskID: common.GenerateUUID(), DomainID: domain, RegistrationID: registration, PreviewID: p.PreviewID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: req.IdempotencyKey, ConfirmedItemIDs: ids, RetryOfTaskID: req.RetryOfTaskID, Status: "queued", Revision: 1, Counts: entity.KnowledgeBatchCounts{Total: int64(len(ids)), Pending: int64(len(ids))}, Mode: "catalogue_only", CreatedAt: directoryTime(now), UpdatedAt: directoryTime(now), ExecutionDeadline: directoryTime(now.Add(30 * time.Minute))}
		record := entity.KnowledgeBatchTaskRecord{ID: task.TaskID, DomainID: domain, UserID: user, RegistrationID: registration, PreviewID: p.PreviewID, IdempotencyKey: req.IdempotencyKey, RequestJSON: directoryJSON(req), Payload: directoryJSON(task), Status: task.Status, CreatedAt: now, DeadlineAt: now.Add(30 * time.Minute)}
		if e = tx.Create(&record).Error; e != nil {
			return e
		}
		for _, i := range p.Items {
			v := pendingBatchResult(i)
			res := tx.Model(&entity.KnowledgeBatchItemRecord{}).Where("id = ? AND preview_id = ? AND task_id IS NULL", i.ItemID, p.PreviewID).Updates(map[string]any{"task_id": task.TaskID, "result_json": directoryJSON(v)})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return batchConflict("preview_consumed")
			}
		}
		if e = tx.Model(row).Update("consumed_task_id", task.TaskID).Error; e != nil {
			return e
		}
		out = task
		created = true
		return nil
	})
	return out, created, err
}
func (d *DatasetService) GetKnowledgeBatchTask(ctx context.Context, user, domain, reg, id, key string) (*entity.KnowledgeBatchTask, error) {
	if key != "" && !batchKey(key) {
		return nil, batchInvalid()
	}
	var out *entity.KnowledgeBatchTask
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if _, e := loadDirectoryScope(tx, user, domain, reg); e != nil {
			return e
		}
		_, v, e := loadBatchTask(tx, user, domain, reg, id, key)
		out = v
		return e
	})
	return out, e
}
func (d *DatasetService) ListKnowledgeBatchTaskItems(ctx context.Context, user, domain, reg, id, cursor string) (*entity.KnowledgeBatchTaskItemsPage, error) {
	var out *entity.KnowledgeBatchTaskItemsPage
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if _, e := loadDirectoryScope(tx, user, domain, reg); e != nil {
			return e
		}
		_, t, e := loadBatchTask(tx, user, domain, reg, id, "")
		if e != nil {
			return e
		}
		position := 0
		if cursor != "" {
			c, e := decodeBatchCursor(cursor)
			if e != nil {
				return e
			}
			if c.View != id {
				return batchConflict("cursor_invalid")
			}
			if c.Revision != t.Revision {
				return batchConflict("result_changed")
			}
			position = c.Position
		}
		rows, e := batchItems(tx, id)
		if e != nil {
			return e
		}
		if position >= len(rows) {
			return batchConflict("cursor_invalid")
		}
		end := min(position+10, len(rows))
		out = &entity.KnowledgeBatchTaskItemsPage{TaskID: id, RegistrationID: reg, ResultRevision: t.Revision, Status: t.Status, Counts: t.Counts, Items: []entity.KnowledgeBatchTaskItem{}}
		for _, r := range rows[position:end] {
			v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](r.ResultJSON)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, *v)
		}
		if end < len(rows) {
			v := encodeBatchCursor(id, end, t.Revision)
			out.NextCursor = &v
		}
		return nil
	})
	return out, e
}

type batchList struct {
	IDs []string `json:"ids"`
}

func batchListSnapshot(tx *gorm.DB, user, domain, scope, kind, cursor string) (*entity.KnowledgeBatchViewRecord, []string, int, error) {
	pos := 0
	var row entity.KnowledgeBatchViewRecord
	if cursor != "" {
		c, e := decodeBatchCursor(cursor)
		if e != nil {
			return nil, nil, 0, e
		}
		if c.Revision != 0 {
			return nil, nil, 0, batchConflict("cursor_invalid")
		}
		pos = c.Position
		if e = tx.Where("id = ? AND user_id = ? AND domain_id = ? AND scope_id = ? AND kind = ?", c.View, user, domain, scope, kind).Take(&row).Error; e != nil {
			return nil, nil, 0, e
		}
		now, e := batchNow(tx)
		if e != nil {
			return nil, nil, 0, e
		}
		if !now.Before(row.ExpiresAt) {
			return nil, nil, 0, batchError(410, "expired", "")
		}
	} else {
		ids := []string{}
		var e error
		if kind == "tasks" {
			e = tx.Model(&entity.KnowledgeBatchTaskRecord{}).Where("domain_id = ? AND user_id = ? AND registration_id = ?", domain, user, scope).Order("created_at DESC, id DESC").Pluck("id", &ids).Error
		} else {
			e = tx.Model(&entity.KnowledgeProjectManifestRecord{}).Where("domain_id = ? AND project_id = ?", domain, scope).Order("created_at DESC, id DESC").Pluck("id", &ids).Error
		}
		if e != nil {
			return nil, nil, 0, e
		}
		now, e := batchNow(tx)
		if e != nil {
			return nil, nil, 0, e
		}
		row = entity.KnowledgeBatchViewRecord{ID: common.GenerateUUID(), DomainID: domain, UserID: user, ScopeID: scope, Kind: kind, Payload: directoryJSON(batchList{ids}), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
		if e = tx.Create(&row).Error; e != nil {
			return nil, nil, 0, e
		}
	}
	v, e := batchUnmarshal[batchList](row.Payload)
	if e != nil {
		return nil, nil, 0, e
	}
	if pos > len(v.IDs) || (pos > 0 && pos == len(v.IDs)) {
		return nil, nil, 0, batchConflict("cursor_invalid")
	}
	return &row, v.IDs, pos, nil
}
func (d *DatasetService) ListKnowledgeBatchTasks(ctx context.Context, user, domain, reg, cursor string) (*entity.KnowledgeBatchTaskPage, error) {
	var out *entity.KnowledgeBatchTaskPage
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if _, e := loadDirectoryScope(tx, user, domain, reg); e != nil {
			return e
		}
		view, ids, pos, e := batchListSnapshot(tx, user, domain, reg, "tasks", cursor)
		if e != nil {
			return e
		}
		out = &entity.KnowledgeBatchTaskPage{DomainID: domain, RegistrationID: reg, ListSnapshotID: view.ID, TotalTasks: int64(len(ids)), CapturedAt: directoryTime(view.CreatedAt), ExpiresAt: directoryTime(view.ExpiresAt), Items: []entity.KnowledgeBatchTask{}}
		end := min(pos+20, len(ids))
		for _, id := range ids[pos:end] {
			_, t, e := loadBatchTask(tx, user, domain, reg, id, "")
			if e != nil {
				return e
			}
			out.Items = append(out.Items, *t)
		}
		if end < len(ids) {
			c := encodeBatchCursor(view.ID, end, 0)
			out.NextCursor = &c
		}
		return nil
	})
	return out, e
}
func (d *DatasetService) CancelKnowledgeBatchTask(ctx context.Context, user, domain, reg, id string, req entity.KnowledgeBatchCancellationRequest) (*entity.KnowledgeBatchTask, error) {
	if !batchKey(req.IdempotencyKey) {
		return nil, batchInvalid()
	}
	var out *entity.KnowledgeBatchTask
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if _, e := loadDirectoryScope(tx, user, domain, reg); e != nil {
			return e
		}
		row, t, e := loadBatchTask(tx, user, domain, reg, id, "")
		if e != nil {
			return e
		}
		request := directoryJSON([]string{id, req.IdempotencyKey})
		var command entity.KnowledgeBatchCommandRecord
		e = tx.Where("domain_id = ? AND user_id = ? AND scope_id = ? AND kind = ? AND idempotency_key = ?", domain, user, reg, "cancel", req.IdempotencyKey).Take(&command).Error
		if e == nil {
			if command.RequestJSON != request {
				return batchConflict("idempotency_conflict")
			}
			out = t
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		command = entity.KnowledgeBatchCommandRecord{ID: common.GenerateUUID(), DomainID: domain, UserID: user, ScopeID: reg, Kind: "cancel", IdempotencyKey: req.IdempotencyKey, RequestJSON: request, CreatedAt: now}
		if e = tx.Create(&command).Error; e != nil {
			return e
		}
		if batchTerminal(t.Status) {
			out = t
			return nil
		}
		t.CancelRequested = true
		items, e := batchItems(tx, id)
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
			v.Status = "cancelled"
			v.CatalogueStatus = "cancelled"
			v.Code = "cancelled"
			if e = saveBatchItem(tx, r, v); e != nil {
				return e
			}
		}
		row.LeaseEpoch++
		if e = tx.Model(row).Update("lease_epoch", row.LeaseEpoch).Error; e != nil {
			return e
		}
		if e = refreshBatchTask(tx, row, t, now); e != nil {
			return e
		}
		out = t
		return nil
	})
	return out, e
}
