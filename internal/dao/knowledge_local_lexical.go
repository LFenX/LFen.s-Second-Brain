package dao

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func migrateKnowledgeLocalLexical(ctx context.Context, db *gorm.DB) error {
	for _, model := range []any{&entity.KnowledgeLexicalPolicyRecord{}, &entity.KnowledgeLexicalOperationRecord{}, &entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate local lexical: %w", err)
		}
	}
	return nil
}
