package dao

import (
	"context"
	"fmt"
	"sync/atomic"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

var knowledgeStructureReady atomic.Bool
var knowledgeDocumentReady atomic.Bool

func KnowledgeStructureRuntimeReady() bool { return knowledgeStructureReady.Load() }

func KnowledgeDocumentRuntimeReady() bool { return knowledgeDocumentReady.Load() }

func migrateKnowledgeStructure(ctx context.Context, db *gorm.DB) error {
	knowledgeStructureReady.Store(false)
	for _, model := range []any{
		&entity.KnowledgeStructurePolicyRecord{}, &entity.KnowledgeStructureOperationRecord{},
		&entity.KnowledgeStructureTaskRecord{}, &entity.KnowledgeStructureLeaseRecord{},
		&entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureHeadRecord{},
		&entity.KnowledgeStructureNodeRecord{}, &entity.KnowledgeStructureRelationRecord{},
		&entity.KnowledgeStructureArtifactRecord{}, &entity.KnowledgeStructureCursorRecord{},
	} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge structure: %w", err)
		}
	}
	knowledgeStructureReady.Store(true)
	knowledgeDocumentReady.Store(false)
	for _, model := range []any{
		&entity.KnowledgeDocumentPolicyRecord{}, &entity.KnowledgeDocumentOperationRow{},
		&entity.KnowledgeDocumentRepresentationRecord{}, &entity.KnowledgeDocumentHeadRecord{},
		&entity.KnowledgeDocumentNodeRecord{},
	} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge document structure: %w", err)
		}
	}
	knowledgeDocumentReady.Store(true)
	return nil
}
