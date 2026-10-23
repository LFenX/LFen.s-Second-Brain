package dao

import (
	"context"
	"errors"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnsureKnowledgeFolder binds a native folder once under the dataset lock.
// Names are display values; subsequent renames do not change the binding.
func (d *FileDAO) EnsureKnowledgeFolder(ctx context.Context, db *gorm.DB, domainID string) (*entity.File, error) {
	var result entity.File
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", domainID).Take(&kb).Error; err != nil {
			return err
		}
		if kb.KnowledgeFolderID != nil && *kb.KnowledgeFolderID != "" {
			return tx.Where("id = ? AND tenant_id = ? AND type = ?", *kb.KnowledgeFolderID, kb.TenantID, "folder").Take(&result).Error
		}
		// Older registered domains may already have source or document mappings.
		// Reuse the actual folder identity rather than guessing from its name.
		var candidates []entity.File
		if err := tx.Raw(`SELECT candidates.* FROM (SELECT DISTINCT f.* FROM file f JOIN file source ON source.parent_id = f.id
			JOIN file2document m ON m.file_id = source.id JOIN document doc ON doc.id = m.document_id
			WHERE doc.kb_id = ? AND f.tenant_id = ? AND f.type = 'folder' AND f.source_type = 'knowledgebase'
			UNION SELECT DISTINCT f.* FROM file f JOIN knowledge_source_revision s ON s.folder_id = f.id
			WHERE s.domain_id = ? AND f.tenant_id = ? AND f.type = 'folder' AND f.source_type = 'knowledgebase') candidates
			WHERE candidates.id NOT IN (SELECT knowledge_folder_id FROM knowledgebase WHERE id <> ? AND knowledge_folder_id IS NOT NULL)`, kb.ID, kb.TenantID, kb.ID, kb.TenantID, kb.ID).Scan(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) > 0 {
			// Earlier name-based folders can coexist after a dataset rename.
			// Choose one for new uploads; existing revision folder IDs stay intact.
			result = candidates[0]
			for _, candidate := range candidates[1:] {
				if candidate.Name == kb.Name && result.Name != kb.Name || (candidate.Name == kb.Name) == (result.Name == kb.Name) && candidate.ID < result.ID {
					result = candidate
				}
			}
		} else {
			root, err := d.GetRootFolder(ctx, tx, kb.TenantID)
			if err != nil {
				return err
			}
			parent, err := d.newAFileFromDataset(ctx, tx, kb.TenantID, DatasetFolderName, root.ID)
			if err != nil {
				return err
			}
			children := tx.Model(&entity.File{}).Select("parent_id").Where("id <> parent_id")
			err = tx.Where("tenant_id = ? AND parent_id = ? AND name = ? AND type = ? AND source_type = ?", kb.TenantID, parent.ID, kb.Name, "folder", "knowledgebase").
				Where("id NOT IN (?) AND id NOT IN (?)", KnowledgeFolderIDs(ctx, tx), children).Order("id ASC").Take(&result).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				result = entity.File{ID: common.GenerateUUID(), ParentID: parent.ID, TenantID: kb.TenantID, CreatedBy: kb.CreatedBy, Name: kb.Name, Type: "folder", SourceType: "knowledgebase"}
				if err := tx.Create(&result).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&entity.Knowledgebase{}).Where("id = ?", kb.ID).Update("knowledge_folder_id", result.ID).Error
	})
	return &result, err
}

// KnowledgeFolderIDs includes the upload binding and actual current/historical
// source folders. Native name lookups must not adopt or delete these identities.
func KnowledgeFolderIDs(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Raw(`SELECT id FROM (
		SELECT knowledge_folder_id AS id FROM knowledgebase WHERE knowledge_enabled = ? AND knowledge_folder_id IS NOT NULL
		UNION SELECT folder_id AS id FROM knowledge_source_revision
		UNION SELECT f.parent_id AS id FROM file f JOIN file2document m ON m.file_id = f.id
		JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id WHERE kb.knowledge_enabled = ?
	) knowledge_folders`, true, true)
}
