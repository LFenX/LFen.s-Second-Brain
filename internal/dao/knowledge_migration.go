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
	for _, field := range []string{"KnowledgeEnabled", "KnowledgeCategory", "KnowledgePolicyVersion"} {
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
	return nil
}
