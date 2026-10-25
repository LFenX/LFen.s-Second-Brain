package dao

import (
	"context"
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	for _, model := range []interface{}{&entity.KnowledgeGroup{}, &entity.KnowledgeSourceRevision{}, &entity.KnowledgeNode{}, &entity.KnowledgeSourceRegistration{}, &entity.KnowledgeSourceBinding{}, &entity.KnowledgeImportPreview{}, &entity.KnowledgeImport{}, &entity.KnowledgeDirectoryRegistrationBinding{}, &entity.KnowledgeDirectoryRootPin{}, &entity.KnowledgeDirectoryState{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge foundation: %w", err)
		}
	}
	for _, model := range []any{&entity.KnowledgeBatchPreviewRecord{}, &entity.KnowledgeBatchItemRecord{}, &entity.KnowledgeBatchTaskRecord{}, &entity.KnowledgeBatchViewRecord{}, &entity.KnowledgeProjectManifestRecord{}, &entity.KnowledgeProjectManifestHeadRecord{}, &entity.KnowledgeBatchCommandRecord{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge batch: %w", err)
		}
	}
	for _, model := range []any{&entity.KnowledgeOriginalUploadRecord{}, &entity.KnowledgeOriginalReceiveSlot{}, &entity.KnowledgeOriginalImportRecord{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			return fmt.Errorf("migrate knowledge original: %w", err)
		}
	}
	for _, id := range []int{1, 2} {
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&entity.KnowledgeOriginalReceiveSlot{ID: id}).Error; err != nil {
			return fmt.Errorf("initialize original receive slots: %w", err)
		}
	}
	if err := migrateKnowledgeOriginalExactKeys(ctx, db); err != nil {
		return err
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

// These keys identify exact requests and paths, not human-language labels.
// MySQL's inherited accent/case-insensitive collation can merge distinct inputs.
func migrateKnowledgeOriginalExactKeys(ctx context.Context, db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return nil
	}
	for _, column := range []struct {
		table, name string
		size        int
	}{
		{"knowledge_source_registration", "registration_key", 64},
		{"knowledge_source_binding", "relative_path", 512},
		{"knowledge_original_upload", "idempotency_key", 128},
		{"knowledge_original_import", "idempotency_key", 128},
	} {
		var collation string
		if err := db.WithContext(ctx).Raw("SELECT COLLATION_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", column.table, column.name).Scan(&collation).Error; err != nil {
			return err
		}
		if collation == "utf8mb4_bin" {
			continue
		}
		query := fmt.Sprintf("ALTER TABLE `%s` MODIFY `%s` VARCHAR(%d) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL", column.table, column.name, column.size)
		if err := db.WithContext(ctx).Exec(query).Error; err != nil {
			return fmt.Errorf("preserve exact knowledge key %s.%s: %w", column.table, column.name, err)
		}
	}
	return nil
}
