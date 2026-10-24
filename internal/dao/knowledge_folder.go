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
// source folders and unclaimed legacy folders. Native name lookups must not
// adopt or delete these identities.
func KnowledgeFolderIDs(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Raw(`SELECT id FROM (
		SELECT knowledge_folder_id AS id FROM knowledgebase WHERE knowledge_enabled = ? AND knowledge_folder_id IS NOT NULL
		UNION SELECT folder_id AS id FROM knowledge_source_revision
		UNION SELECT f.parent_id AS id FROM file f JOIN file2document m ON m.file_id = f.id
		JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id WHERE kb.knowledge_enabled = ?
		UNION SELECT id FROM file WHERE knowledge_private_owner_id IS NOT NULL
		UNION SELECT id FROM (?) AS unclaimed
	) knowledge_folders`, true, true, UnclaimedKnowledgeFolderIDs(ctx, db))
}

// UnclaimedKnowledgeFolderIDs never infers ownership from a dataset name.
// Only the reserved container directly below the real tenant root is exempt.
// DISTINCT also materializes this subquery for MySQL updates of the file table.
func UnclaimedKnowledgeFolderIDs(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Raw(`SELECT DISTINCT f.id FROM file f
		WHERE f.type = 'folder' AND f.source_type = 'knowledgebase'
		AND NOT (f.name = '.knowledgebase' AND EXISTS (
			SELECT 1 FROM file root WHERE root.id = f.parent_id AND root.parent_id = root.id
			AND root.tenant_id = f.tenant_id AND root.id <> f.id))
		AND NOT EXISTS (SELECT 1 FROM knowledgebase kb WHERE kb.knowledge_folder_id = f.id)
		AND NOT EXISTS (SELECT 1 FROM knowledge_source_revision s WHERE s.folder_id = f.id)
		AND NOT EXISTS (SELECT 1 FROM file source JOIN file2document m ON m.file_id = source.id
			JOIN document d ON d.id = m.document_id WHERE source.parent_id = f.id)`)
}

// ProtectUnclaimedKnowledgeFolders freezes the classification before workers
// start. A later ordinary mapping must not silently reclassify a private tree.
func ProtectUnclaimedKnowledgeFolders(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Model(&entity.File{}).
		Where("knowledge_private_owner_id IS NULL AND id IN (SELECT id FROM (?) AS legacy_private_folders)", UnclaimedKnowledgeFolderIDs(ctx, db)).
		Update("knowledge_private_owner_id", gorm.Expr("COALESCE(NULLIF(created_by, ''), tenant_id)")).Error
}

// HasKnowledgeFileTree tests the complete tree against current and historical
// source identities. Native folder maintenance uses the same facts as permission.
func HasKnowledgeFileTree(ctx context.Context, db *gorm.DB, file *entity.File) (bool, error) {
	if db == nil || file == nil {
		return false, errors.New("knowledge policy source unavailable")
	}
	ids := []string{file.ID}
	if file.Type == "folder" {
		var err error
		ids, err = NewFileDAO().GetSubtreeIDs(ctx, db, file.TenantID, file.ID)
		if err != nil {
			return false, err
		}
	}
	var count int64
	err := db.WithContext(ctx).Table("(?) AS origin", KnowledgeFileOrigins(ctx, db)).
		Where("resource_id IN ?", ids).Count(&count).Error
	return count > 0, err
}

// KnowledgeFileOrigins carries explicit current and historical source ownership.
// It is an intersection constraint, never an additional grant from a team copy.
func KnowledgeFileOrigins(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Raw(`WITH RECURSIVE origins(resource_id, owner_id) AS (
		SELECT resource_id, owner_id FROM (
		SELECT m.file_id AS resource_id, kb.created_by AS owner_id
		FROM file2document m JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id
		WHERE kb.knowledge_enabled = ? AND m.file_id IS NOT NULL
		UNION SELECT f.parent_id AS resource_id, kb.created_by AS owner_id
		FROM file f JOIN file2document m ON m.file_id = f.id JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id
		WHERE kb.knowledge_enabled = ?
		UNION SELECT knowledge_folder_id AS resource_id, created_by AS owner_id FROM knowledgebase
		WHERE knowledge_enabled = ? AND knowledge_folder_id IS NOT NULL
		UNION SELECT s.file_id AS resource_id, COALESCE(NULLIF(kb.created_by, ''), NULLIF(f.created_by, ''), f.tenant_id) AS owner_id
		FROM knowledge_source_revision s LEFT JOIN knowledgebase kb ON kb.id = s.domain_id LEFT JOIN file f ON f.id = s.file_id
		UNION SELECT s.folder_id AS resource_id, COALESCE(NULLIF(kb.created_by, ''), NULLIF(f.created_by, ''), f.tenant_id) AS owner_id
		FROM knowledge_source_revision s LEFT JOIN knowledgebase kb ON kb.id = s.domain_id LEFT JOIN file f ON f.id = s.folder_id
		UNION SELECT id AS resource_id, knowledge_private_owner_id AS owner_id FROM file WHERE knowledge_private_owner_id IS NOT NULL
		UNION SELECT f.id AS resource_id, COALESCE(NULLIF(f.created_by, ''), f.tenant_id) AS owner_id
		FROM file f WHERE f.id IN (?)
		) AS seeds
		UNION SELECT f.id, origin.owner_id FROM file f JOIN origins origin ON f.parent_id = origin.resource_id
		WHERE f.id <> f.parent_id
	) SELECT resource_id, owner_id FROM origins`, true, true, true, UnclaimedKnowledgeFolderIDs(ctx, db))
}
