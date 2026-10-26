package dao

import (
	"context"
	"fmt"
	"sync/atomic"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/entity"
)

var knowledgeLifecycleReady atomic.Bool

// KnowledgeLifecycleRuntimeReady is true only after this binary's migration
// has checked every existing valid head and initialized its private generation.
func KnowledgeLifecycleRuntimeReady() bool { return knowledgeLifecycleReady.Load() }

func migrateKnowledgeLifecycle(ctx context.Context, db *gorm.DB) error {
	knowledgeLifecycleReady.Store(false)
	for _, model := range []any{&entity.KnowledgeSourceLifecycle{}, &entity.KnowledgeRetiredPosition{}, &entity.KnowledgeSourceFence{}, &entity.KnowledgeLifecycleCommandRecord{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge lifecycle: %w", err)
		}
	}
	// Stop on ambiguous heads. Initial generation is not a reconstructed operation count.
	query := db.WithContext(ctx).Model(&entity.KnowledgeSourceRevision{}).
		Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").
		Where("knowledge_source_revision.is_current = ? AND document.status = ?", true, "1")
	var duplicates int64
	groups := db.WithContext(ctx).Model(&entity.KnowledgeSourceRevision{}).
		Joins("JOIN document ON document.id = knowledge_source_revision.document_id AND document.kb_id = knowledge_source_revision.domain_id").
		Where("document.status = ?", "1").Select("knowledge_source_revision.document_id").Group("knowledge_source_revision.document_id").Having("SUM(CASE WHEN knowledge_source_revision.is_current THEN 1 ELSE 0 END) <> 1")
	if err := db.Table("(?) AS lifecycle_invalid_heads", groups).Count(&duplicates).Error; err != nil {
		return err
	}
	if duplicates != 0 {
		return fmt.Errorf("knowledge lifecycle has %d ambiguous sources", duplicates)
	}
	var rows []entity.KnowledgeSourceRevision
	err := query.Select("knowledge_source_revision.*").Order("knowledge_source_revision.id").FindInBatches(&rows, 200, func(_ *gorm.DB, _ int) error {
		tx := db.WithContext(ctx)
		for _, source := range rows {
			state := entity.KnowledgeSourceLifecycle{SourceID: source.DocumentID, DomainID: source.DomainID, State: "active", Version: 1}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
				return err
			}
			var existing entity.KnowledgeSourceLifecycle
			if err := tx.Where("source_id = ?", source.DocumentID).Take(&existing).Error; err != nil {
				return err
			}
			if existing.DomainID != source.DomainID || (existing.State != "active" && existing.State != "archived") || existing.Version < 1 || existing.Version > entity.KnowledgeMaxLifecycleVersion {
				return fmt.Errorf("invalid knowledge lifecycle state")
			}
		}
		return nil
	}).Error
	if err == nil {
		knowledgeLifecycleReady.Store(true)
	}
	return err
}
