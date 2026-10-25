package dataset

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func knowledgeProject(tx *gorm.DB, domain, id string) error {
	if !isKnowledgeID(id, 32) {
		return batchMissing()
	}
	var g entity.KnowledgeGroup
	return tx.Where("id = ? AND domain_id = ? AND kind = ?", id, domain, "project").Take(&g).Error
}
func projectManifestHead(tx *gorm.DB, domain, project string) (*string, error) {
	var row entity.KnowledgeProjectManifestHeadRecord
	e := tx.Where("project_id = ? AND domain_id = ?", project, domain).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return &row.ManifestID, nil
}
func sameOptionalID(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func projectManifestDigest(p *entity.KnowledgeProjectManifestPreviewPage) string {
	parts := []any{"lfen.project.manifest.preview.v0.4", p.PreviewID, p.DomainID, p.ProjectID, p.TaskID, p.TaskManifestSHA256, p.ExpectedManifestID, p.Coverage, p.CapturedAtomically, p.CreatedAt, p.ExpiresAt, len(p.Items), p.Added, p.Replaced, p.Carried}
	for _, i := range p.Items {
		parts = append(parts, i.RegistrationID, i.RelativePath, i.SourceID, i.SourceRevisionID, i.RootNodeID, i.SHA256, i.ByteLength, i.Classification.GroupID, i.Classification.ComponentSide, i.Classification.Language, i.OriginTaskID)
	}
	return directorySHA([]byte(directoryTokens(parts...)))
}
func authorizedManifestTask(tx *gorm.DB, user, domain, id string) (*entity.KnowledgeBatchTaskRecord, *entity.KnowledgeBatchTask, error) {
	if !isKnowledgeID(id, 32) {
		return nil, nil, batchMissing()
	}
	var row entity.KnowledgeBatchTaskRecord
	if e := tx.Where("id = ? AND domain_id = ? AND user_id = ?", id, domain, user).Take(&row).Error; e != nil {
		return nil, nil, e
	}
	if _, e := loadDirectoryScope(tx, user, domain, row.RegistrationID); e != nil {
		return nil, nil, e
	}
	v, e := batchUnmarshal[entity.KnowledgeBatchTask](row.Payload)
	if e != nil {
		return nil, nil, e
	}
	if v.Status != "succeeded" {
		return nil, nil, batchConflict("task_not_succeeded")
	}
	return &row, v, nil
}
func validateProjectEntries(tx *gorm.DB, domain, project string, entries []entity.KnowledgeProjectManifestEntry) error {
	groups := map[string]entity.KnowledgeGroup{}
	validGroups := map[string]bool{}
	member := func(start *string) bool {
		if start == nil {
			return false
		}
		if yes, ok := validGroups[*start]; ok {
			return yes
		}
		id := *start
		for depth := 0; depth < 16; depth++ {
			g, ok := groups[id]
			if !ok {
				if tx.Where("id = ? AND domain_id = ?", id, domain).Take(&g).Error != nil {
					return false
				}
				groups[id] = g
			}
			if depth == 0 && g.ID != project && g.Kind != "module" {
				return false
			}
			if g.Kind == "project" {
				yes := g.ID == project
				validGroups[*start] = yes
				return yes
			}
			if g.ParentID == nil {
				return false
			}
			id = *g.ParentID
		}
		return false
	}
	ids := make([]string, 0, len(entries))
	for _, i := range entries {
		ids = append(ids, i.SourceRevisionID)
	}
	var revisions []entity.KnowledgeSourceRevision
	if e := tx.Where("domain_id = ? AND id IN ?", domain, ids).Find(&revisions).Error; e != nil {
		return e
	}
	byID := map[string]entity.KnowledgeSourceRevision{}
	for _, r := range revisions {
		byID[r.ID] = r
	}
	sourceIDs := map[string]bool{}
	paths := map[string]bool{}
	for _, i := range entries {
		if sourceIDs[i.SourceID] || paths[i.RegistrationID+"/"+i.RelativePath] || !member(i.Classification.GroupID) {
			return batchConflict("project_membership_changed")
		}
		sourceIDs[i.SourceID] = true
		paths[i.RegistrationID+"/"+i.RelativePath] = true
		r, ok := byID[i.SourceRevisionID]
		if !ok || r.DocumentID != i.SourceID || r.RelativePath != i.RelativePath || r.SHA256 != i.SHA256 || r.ByteLength != i.ByteLength || knowledgeRoot(&r).ID != i.RootNodeID || directoryJSON(entity.KnowledgeDirectoryClassification{GroupID: r.GroupID, ComponentSide: r.ComponentSide, Language: r.Language}) != directoryJSON(i.Classification) {
			return batchConflict("project_membership_changed")
		}
		var n int64
		if e := tx.Model(&entity.KnowledgeSourceBinding{}).Where("registration_id = ? AND relative_path = ? AND document_id = ?", i.RegistrationID, i.RelativePath, i.SourceID).Count(&n).Error; e != nil {
			return e
		}
		if n != 1 {
			return batchConflict("project_membership_changed")
		}
	}
	return nil
}
func loadProjectManifest(tx *gorm.DB, domain, project, id string) (*entity.KnowledgeProjectManifestRecord, *entity.KnowledgeProjectManifestPreviewPage, *entity.KnowledgeProjectManifestReceipt, error) {
	if !isKnowledgeID(id, 32) {
		return nil, nil, nil, batchMissing()
	}
	var row entity.KnowledgeProjectManifestRecord
	if e := tx.Where("id = ? AND domain_id = ? AND project_id = ?", id, domain, project).Take(&row).Error; e != nil {
		return nil, nil, nil, e
	}
	p, e := batchUnmarshal[entity.KnowledgeProjectManifestPreviewPage](row.Payload)
	if e != nil {
		return nil, nil, nil, e
	}
	receipt, e := batchUnmarshal[entity.KnowledgeProjectManifestReceipt](row.Receipt)
	if e != nil {
		return nil, nil, nil, e
	}
	if projectManifestDigest(p) != receipt.ManifestSHA256 {
		return nil, nil, nil, batchConflict("observation_mismatch")
	}
	return &row, p, receipt, nil
}
func manifestPreviewPage(p *entity.KnowledgeProjectManifestPreviewPage, cursor string) (*entity.KnowledgeProjectManifestPreviewPage, error) {
	pos := 0
	if cursor != "" {
		c, e := decodeBatchCursor(cursor)
		if e != nil {
			return nil, e
		}
		if c.View != p.PreviewID || c.Revision != 0 {
			return nil, batchConflict("cursor_invalid")
		}
		pos = c.Position
	}
	if pos >= len(p.Items) {
		return nil, batchConflict("cursor_invalid")
	}
	end := min(pos+50, len(p.Items))
	copy := *p
	copy.Items = slices.Clone(p.Items[pos:end])
	copy.NextCursor = nil
	if end < len(p.Items) {
		v := encodeBatchCursor(p.PreviewID, end, 0)
		copy.NextCursor = &v
	}
	return &copy, nil
}
func (d *DatasetService) PrepareKnowledgeProjectManifest(ctx context.Context, user, domain string, req entity.KnowledgeProjectManifestPreviewRequest) (*entity.KnowledgeProjectManifestPreviewPage, error) {
	if !isKnowledgeID(req.ProjectID, 32) || !isKnowledgeID(req.TaskID, 32) || (req.ExpectedManifestID != nil && !isKnowledgeID(*req.ExpectedManifestID, 32)) {
		return nil, batchInvalid()
	}
	var out *entity.KnowledgeProjectManifestPreviewPage
	err := d.batchTransaction(ctx, user, domain, 30*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if e := knowledgeProject(tx, domain, req.ProjectID); e != nil {
			return e
		}
		row, t, e := authorizedManifestTask(tx, user, domain, req.TaskID)
		if e != nil {
			return e
		}
		head, e := projectManifestHead(tx, domain, req.ProjectID)
		if e != nil {
			return e
		}
		if !sameOptionalID(head, req.ExpectedManifestID) {
			return batchConflict("project_head_changed")
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		var n int64
		if e = tx.Model(&entity.KnowledgeBatchViewRecord{}).Where("domain_id = ? AND user_id = ? AND kind = ? AND expires_at > ? AND consumed_id IS NULL", domain, user, "manifest_preview", now).Count(&n).Error; e != nil {
			return e
		}
		if n >= 2 {
			return batchError(429, "capacity", "")
		}
		merged := map[string]entity.KnowledgeProjectManifestEntry{}
		if head != nil {
			_, base, _, e := loadProjectManifest(tx, domain, req.ProjectID, *head)
			if e != nil {
				return e
			}
			for _, i := range base.Items {
				merged[i.SourceID] = i
			}
		}
		p := &entity.KnowledgeProjectManifestPreviewPage{PreviewID: common.GenerateUUID(), DomainID: domain, ProjectID: req.ProjectID, TaskID: t.TaskID, TaskManifestSHA256: t.ManifestSHA256, ExpectedManifestID: req.ExpectedManifestID, Coverage: "explicit_selection", CapturedAtomically: false, CreatedAt: directoryTime(now), ExpiresAt: directoryTime(now.Add(10 * time.Minute)), Items: []entity.KnowledgeProjectManifestEntry{}}
		items, e := batchItems(tx, row.ID)
		if e != nil {
			return e
		}
		if len(items) != int(t.Counts.Total) {
			return batchConflict("task_not_succeeded")
		}
		for _, r := range items {
			v, e := batchUnmarshal[entity.KnowledgeBatchTaskItem](r.ResultJSON)
			if e != nil {
				return e
			}
			if r.Status != "succeeded" || v.SourceID == nil || v.SourceRevisionID == nil || v.RootNodeID == nil {
				return batchConflict("task_not_succeeded")
			}
			if _, exists := merged[*v.SourceID]; exists {
				p.Replaced++
			} else {
				p.Added++
			}
			merged[*v.SourceID] = entity.KnowledgeProjectManifestEntry{RegistrationID: row.RegistrationID, RelativePath: v.RelativePath, SourceID: *v.SourceID, SourceRevisionID: *v.SourceRevisionID, RootNodeID: *v.RootNodeID, SHA256: v.SHA256, ByteLength: v.ByteLength, Classification: v.Classification, OriginTaskID: t.TaskID}
		}
		if len(merged) > 1024 {
			return batchError(413, "too_large", "")
		}
		for _, i := range merged {
			p.Items = append(p.Items, i)
		}
		slices.SortFunc(p.Items, func(a, b entity.KnowledgeProjectManifestEntry) int {
			return strings.Compare(a.RegistrationID+"/"+a.RelativePath, b.RegistrationID+"/"+b.RelativePath)
		})
		p.TotalEntries = int64(len(p.Items))
		p.Carried = p.TotalEntries - p.Added - p.Replaced
		if e = validateProjectEntries(tx, domain, req.ProjectID, p.Items); e != nil {
			return e
		}
		p.ManifestSHA256 = projectManifestDigest(p)
		view := entity.KnowledgeBatchViewRecord{ID: p.PreviewID, DomainID: domain, UserID: user, ScopeID: req.ProjectID, Kind: "manifest_preview", Payload: directoryJSON(p), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
		if e = tx.Create(&view).Error; e != nil {
			return e
		}
		out, e = manifestPreviewPage(p, "")
		return e
	})
	return out, err
}
func loadManifestPreview(tx *gorm.DB, user, domain, id string) (*entity.KnowledgeBatchViewRecord, *entity.KnowledgeProjectManifestPreviewPage, error) {
	if !isKnowledgeID(id, 32) {
		return nil, nil, batchMissing()
	}
	var row entity.KnowledgeBatchViewRecord
	if e := tx.Where("id = ? AND user_id = ? AND domain_id = ? AND kind = ?", id, user, domain, "manifest_preview").Take(&row).Error; e != nil {
		return nil, nil, e
	}
	p, e := batchUnmarshal[entity.KnowledgeProjectManifestPreviewPage](row.Payload)
	if e != nil {
		return nil, nil, e
	}
	if projectManifestDigest(p) != p.ManifestSHA256 {
		return nil, nil, batchConflict("observation_mismatch")
	}
	p.ConsumedManifestID = row.ConsumedID
	return &row, p, nil
}
func (d *DatasetService) GetKnowledgeProjectManifestPreview(ctx context.Context, user, domain, id, cursor string) (*entity.KnowledgeProjectManifestPreviewPage, error) {
	var out *entity.KnowledgeProjectManifestPreviewPage
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		row, p, e := loadManifestPreview(tx, user, domain, id)
		if e != nil {
			return e
		}
		if e = knowledgeProject(tx, domain, p.ProjectID); e != nil {
			return e
		}
		if _, _, e = authorizedManifestTask(tx, user, domain, p.TaskID); e != nil {
			return e
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		if row.ConsumedID == nil && !now.Before(row.ExpiresAt) {
			return batchError(410, "expired", "")
		}
		out, e = manifestPreviewPage(p, cursor)
		return e
	})
	return out, e
}
func (d *DatasetService) PublishKnowledgeProjectManifest(ctx context.Context, user, domain string, req entity.KnowledgeProjectManifestPublicationRequest) (*entity.KnowledgeProjectManifestReceipt, bool, error) {
	if !isKnowledgeID(req.ProjectID, 32) || !isKnowledgeID(req.PreviewID, 32) || !isKnowledgeID(req.ManifestSHA256, 64) || !batchKey(req.IdempotencyKey) {
		return nil, false, batchInvalid()
	}
	var out *entity.KnowledgeProjectManifestReceipt
	created := false
	e := d.batchTransaction(ctx, user, domain, 30*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if e := knowledgeProject(tx, domain, req.ProjectID); e != nil {
			return e
		}
		var old entity.KnowledgeProjectManifestRecord
		e := tx.Where("domain_id = ? AND user_id = ? AND project_id = ? AND idempotency_key = ?", domain, user, req.ProjectID, req.IdempotencyKey).Take(&old).Error
		if e == nil {
			if old.RequestJSON != directoryJSON(req) {
				return batchConflict("idempotency_conflict")
			}
			receipt, e := batchUnmarshal[entity.KnowledgeProjectManifestReceipt](old.Receipt)
			if e != nil {
				return e
			}
			if _, _, e = authorizedManifestTask(tx, user, domain, receipt.TaskID); e != nil {
				return e
			}
			out = receipt
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		view, p, e := loadManifestPreview(tx, user, domain, req.PreviewID)
		if e != nil {
			return e
		}
		if p.ProjectID != req.ProjectID {
			return batchMissing()
		}
		if view.ConsumedID != nil {
			return batchConflict("preview_consumed")
		}
		if p.ManifestSHA256 != req.ManifestSHA256 {
			return batchConflict("observation_mismatch")
		}
		now, e := batchNow(tx)
		if e != nil {
			return e
		}
		if !now.Before(view.ExpiresAt) {
			return batchError(410, "expired", "")
		}
		_, t, e := authorizedManifestTask(tx, user, domain, p.TaskID)
		if e != nil {
			return e
		}
		if t.ManifestSHA256 != p.TaskManifestSHA256 {
			return batchConflict("observation_mismatch")
		}
		head, e := projectManifestHead(tx, domain, req.ProjectID)
		if e != nil {
			return e
		}
		if !sameOptionalID(head, p.ExpectedManifestID) {
			return batchConflict("project_head_changed")
		}
		if e = validateProjectEntries(tx, domain, req.ProjectID, p.Items); e != nil {
			return e
		}
		receipt := &entity.KnowledgeProjectManifestReceipt{DomainID: domain, ProjectID: p.ProjectID, ManifestID: common.GenerateUUID(), PreviewID: p.PreviewID, TaskID: p.TaskID, ManifestSHA256: p.ManifestSHA256, IdempotencyKey: req.IdempotencyKey, ExpectedManifestID: p.ExpectedManifestID, Coverage: p.Coverage, CapturedAtomically: false, TotalEntries: p.TotalEntries, PublishedAt: directoryTime(now)}
		record := entity.KnowledgeProjectManifestRecord{ID: receipt.ManifestID, DomainID: domain, UserID: user, ProjectID: p.ProjectID, PreviewID: p.PreviewID, IdempotencyKey: req.IdempotencyKey, RequestJSON: directoryJSON(req), Payload: directoryJSON(p), Receipt: directoryJSON(receipt), CreatedAt: now}
		if e = tx.Create(&record).Error; e != nil {
			return e
		}
		if head == nil {
			e = tx.Create(&entity.KnowledgeProjectManifestHeadRecord{DomainID: domain, ProjectID: p.ProjectID, ManifestID: receipt.ManifestID}).Error
		} else {
			update := tx.Model(&entity.KnowledgeProjectManifestHeadRecord{}).Where("project_id = ? AND domain_id = ? AND manifest_id = ?", p.ProjectID, domain, *head).Update("manifest_id", receipt.ManifestID)
			e = update.Error
			if e == nil && update.RowsAffected != 1 {
				return batchConflict("project_head_changed")
			}
		}
		if e != nil {
			return e
		}
		if e = tx.Model(view).Update("consumed_id", receipt.ManifestID).Error; e != nil {
			return e
		}
		out = receipt
		created = true
		return nil
	})
	return out, created, e
}
func (d *DatasetService) GetKnowledgeProjectManifestPublicationByKey(ctx context.Context, user, domain, project, key string) (*entity.KnowledgeProjectManifestReceipt, error) {
	if !batchKey(key) {
		return nil, batchInvalid()
	}
	var out *entity.KnowledgeProjectManifestReceipt
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if e := knowledgeProject(tx, domain, project); e != nil {
			return e
		}
		var row entity.KnowledgeProjectManifestRecord
		if e := tx.Where("domain_id = ? AND user_id = ? AND project_id = ? AND idempotency_key = ?", domain, user, project, key).Take(&row).Error; e != nil {
			return e
		}
		r, e := batchUnmarshal[entity.KnowledgeProjectManifestReceipt](row.Receipt)
		if e != nil {
			return e
		}
		if _, _, e = authorizedManifestTask(tx, user, domain, r.TaskID); e != nil {
			return e
		}
		out = r
		return nil
	})
	return out, e
}
func (d *DatasetService) GetKnowledgeProjectManifestHead(ctx context.Context, user, domain, project string) (*entity.KnowledgeProjectManifestHead, error) {
	var out *entity.KnowledgeProjectManifestHead
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if e := knowledgeProject(tx, domain, project); e != nil {
			return e
		}
		id, e := projectManifestHead(tx, domain, project)
		out = &entity.KnowledgeProjectManifestHead{DomainID: domain, ProjectID: project, ManifestID: id}
		return e
	})
	return out, e
}
func (d *DatasetService) GetKnowledgeProjectManifest(ctx context.Context, user, domain, project, id, cursor string) (*entity.KnowledgeProjectManifestVersionPage, error) {
	var out *entity.KnowledgeProjectManifestVersionPage
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if e := knowledgeProject(tx, domain, project); e != nil {
			return e
		}
		_, p, r, e := loadProjectManifest(tx, domain, project, id)
		if e != nil {
			return e
		}
		pos := 0
		if cursor != "" {
			c, e := decodeBatchCursor(cursor)
			if e != nil {
				return e
			}
			if c.View != id || c.Revision != 0 {
				return batchConflict("cursor_invalid")
			}
			pos = c.Position
		}
		if pos >= len(p.Items) {
			return batchConflict("cursor_invalid")
		}
		end := min(pos+50, len(p.Items))
		out = &entity.KnowledgeProjectManifestVersionPage{DomainID: domain, ProjectID: project, ManifestID: id, PreviewID: r.PreviewID, TaskID: r.TaskID, ManifestSHA256: r.ManifestSHA256, ExpectedManifestID: r.ExpectedManifestID, Coverage: r.Coverage, CapturedAtomically: false, TotalEntries: r.TotalEntries, PublishedAt: r.PublishedAt, Items: slices.Clone(p.Items[pos:end])}
		if end < len(p.Items) {
			v := encodeBatchCursor(id, end, 0)
			out.NextCursor = &v
		}
		return nil
	})
	return out, e
}
func (d *DatasetService) ListKnowledgeProjectManifests(ctx context.Context, user, domain, project, cursor string) (*entity.KnowledgeProjectManifestHistoryPage, error) {
	var out *entity.KnowledgeProjectManifestHistoryPage
	e := d.batchTransaction(ctx, user, domain, 10*time.Second, func(tx *gorm.DB, _ *entity.Knowledgebase) error {
		if e := knowledgeProject(tx, domain, project); e != nil {
			return e
		}
		view, ids, pos, e := batchListSnapshot(tx, user, domain, project, "history", cursor)
		if e != nil {
			return e
		}
		out = &entity.KnowledgeProjectManifestHistoryPage{DomainID: domain, ProjectID: project, ListSnapshotID: view.ID, TotalManifests: int64(len(ids)), CapturedAt: directoryTime(view.CreatedAt), ExpiresAt: directoryTime(view.ExpiresAt), Items: []entity.KnowledgeProjectManifestReceipt{}}
		end := min(pos+20, len(ids))
		for _, id := range ids[pos:end] {
			_, _, r, e := loadProjectManifest(tx, domain, project, id)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, *r)
		}
		if end < len(ids) {
			v := encodeBatchCursor(view.ID, end, 0)
			out.NextCursor = &v
		}
		return nil
	})
	return out, e
}
