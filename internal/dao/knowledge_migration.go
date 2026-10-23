package dao

import (
	"context"
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// migrateKnowledgeFoundation adds only this extension's columns and tables.
// It never rewrites existing dataset contents, permissions or model settings.
func migrateKnowledgeFoundation(ctx context.Context, db *gorm.DB) error {
	m := db.WithContext(ctx).Migrator()
	for _, field := range []string{"KnowledgeEnabled", "KnowledgeCategory", "KnowledgePolicyVersion", "KnowledgeFolderID"} {
		if !m.HasColumn(&entity.Knowledgebase{}, field) {
			if err := m.AddColumn(&entity.Knowledgebase{}, field); err != nil {
				return fmt.Errorf("add knowledge foundation column %s: %w", field, err)
			}
		}
	}
	for _, model := range []interface{}{&entity.KnowledgeGroup{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeNode{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge foundation: %w", err)
		}
	}
	if !m.HasColumn(&entity.File{}, "KnowledgePrivateOwnerID") {
		if err := m.AddColumn(&entity.File{}, "KnowledgePrivateOwnerID"); err != nil {
			return fmt.Errorf("add private folder ownership: %w", err)
		}
	}
	if !m.HasIndex(&entity.Knowledgebase{}, "idx_knowledge_domain_folder") {
		if err := m.CreateIndex(&entity.Knowledgebase{}, "idx_knowledge_domain_folder"); err != nil {
			return err
		}
	}
	var ids []string
	if err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("knowledge_enabled = ? AND (knowledge_folder_id IS NULL OR knowledge_folder_id = '')", true).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := NewFileDAO().EnsureKnowledgeFolder(ctx, db, id); err != nil {
			return fmt.Errorf("bind knowledge folder: %w", err)
		}
	}
	return ProtectUnclaimedKnowledgeFolders(ctx, db)
}
