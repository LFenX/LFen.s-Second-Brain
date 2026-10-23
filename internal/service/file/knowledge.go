package file

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// knowledgeBrowseDB applies protection before pagination and aggregates. Native
// tenant metadata browsing remains available for sources outside registered domains.
func knowledgeBrowseDB(ctx context.Context, userID, tenantID string) (*gorm.DB, error) {
	db := dao.DB.WithContext(ctx)
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).
		Where("tenant_id = ? AND knowledge_enabled = ? AND created_by <> ?", tenantID, true, userID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return db, nil
	}
	// Include current native mappings, versioned mappings and empty native
	// dataset folders. UNION removes cycles while expanding protected subtrees.
	hidden := dao.DB.WithContext(ctx).Raw(`WITH RECURSIVE hidden(id) AS (
		SELECT file.id FROM file JOIN knowledgebase kb ON file.name = kb.name AND file.tenant_id = kb.tenant_id
		WHERE file.type = 'folder' AND file.source_type = 'knowledgebase' AND kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT file.id FROM file JOIN file2document m ON m.file_id = file.id JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT file.parent_id FROM file JOIN file2document m ON m.file_id = file.id JOIN document d ON d.id = m.document_id JOIN knowledgebase kb ON kb.id = d.kb_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT s.file_id FROM knowledge_source_revision s JOIN knowledgebase kb ON kb.id = s.domain_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT s.folder_id FROM knowledge_source_revision s JOIN knowledgebase kb ON kb.id = s.domain_id
		WHERE kb.knowledge_enabled = ? AND kb.created_by <> ? AND kb.tenant_id = ?
		UNION SELECT file.id FROM file JOIN hidden ON file.parent_id = hidden.id
	) SELECT id FROM hidden`, true, userID, tenantID, true, userID, tenantID, true, userID, tenantID, true, userID, tenantID, true, userID, tenantID)
	return db.Where("file.id NOT IN (?)", hidden), nil
}
