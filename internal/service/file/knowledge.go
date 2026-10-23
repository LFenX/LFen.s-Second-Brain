package file

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

// knowledgeBrowseDB applies protection before pagination and aggregates. Native
// tenant metadata browsing remains available for sources outside registered domains.
func knowledgeBrowseDB(ctx context.Context, userID, tenantID string) (*gorm.DB, error) {
	db := dao.DB.WithContext(ctx)
	// Include the immutable folder binding plus current and historical sources.
	// Unclaimed native source folders from former name-based mappings are private
	// to their creator; their original domain cannot be inferred from a name.
	// UNION removes cycles while expanding protected subtrees.
	hidden := dao.DB.WithContext(ctx).Raw(`WITH RECURSIVE hidden(id) AS (
		SELECT file.id FROM file JOIN knowledgebase kb ON file.id = kb.knowledge_folder_id
		WHERE file.type = 'folder' AND kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT file.id FROM file JOIN file2document m ON m.file_id = file.id JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT s.file_id FROM knowledge_source_revision s JOIN knowledgebase kb ON kb.id = s.domain_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT file.parent_id FROM file JOIN file2document m ON m.file_id = file.id JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT s.folder_id FROM knowledge_source_revision s JOIN knowledgebase kb ON kb.id = s.domain_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT file.id FROM file WHERE file.tenant_id = ? AND file.created_by <> ?
		AND EXISTS (SELECT 1 FROM knowledge_source_revision s WHERE (s.folder_id = file.id OR s.file_id = file.id)
		AND NOT EXISTS (SELECT 1 FROM knowledgebase kb WHERE kb.id = s.domain_id))
		UNION SELECT file.id FROM file WHERE file.type = 'folder' AND file.source_type = 'knowledgebase' AND file.name <> '.knowledgebase'
		AND file.tenant_id = ? AND file.created_by <> ?
		AND NOT EXISTS (SELECT 1 FROM knowledgebase kb WHERE kb.tenant_id = file.tenant_id AND (kb.knowledge_folder_id = file.id OR kb.name = file.name))
		AND NOT EXISTS (SELECT 1 FROM knowledge_source_revision s WHERE s.folder_id = file.id)
		AND NOT EXISTS (SELECT 1 FROM file source JOIN file2document m ON m.file_id = source.id JOIN document d ON d.id = m.document_id WHERE source.parent_id = file.id)
		UNION SELECT file.id FROM file JOIN hidden ON file.parent_id = hidden.id
	) SELECT id FROM hidden`, true, userID, tenantID, true, userID, tenantID, true, userID, tenantID,
		true, userID, tenantID, true, userID, tenantID, tenantID, userID, tenantID, userID)
	return db.Where("file.id NOT IN (?)", hidden), nil
}

// Native moves recreate folder IDs and relocate source objects. Until a domain
// migration exists, reject moves/deletes of managed trees before any mutation.
func (s *FileService) checkKnowledgeTreeMutation(ctx context.Context, file *entity.File) error {
	ids := []string{file.ID}
	if file.Type == FileTypeFolder {
		var err error
		ids, err = s.fileDAO.GetSubtreeIDs(ctx, dao.DB, file.TenantID, file.ID)
		if err != nil {
			return err
		}
	}
	var count int64
	if err := dao.DB.WithContext(ctx).Model(&entity.File{}).
		Where("id IN ? AND id IN (?)", ids, dao.KnowledgeFolderIDs(ctx, dao.DB)).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return permission.ErrKnowledgeIngestionDenied
	}
	return permission.CheckKnowledgeFileTransfer(ctx, dao.DB, ids)
}
