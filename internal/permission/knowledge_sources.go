package permission

import (
	"context"

	"gorm.io/gorm"
)

// knowledgeFileOrigins carries explicit current and historical source ownership.
// It is an intersection constraint, never an additional grant from a team copy.
func knowledgeFileOrigins(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Raw(`
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
		FROM knowledge_source_revision s LEFT JOIN knowledgebase kb ON kb.id = s.domain_id LEFT JOIN file f ON f.id = s.folder_id`, true, true, true)
}

func knowledgeDatasetOrigins(ctx context.Context, db *gorm.DB) *gorm.DB {
	linked := db.WithContext(ctx).Table("file2document m").Select("d.kb_id AS resource_id, origin.owner_id").
		Joins("JOIN document d ON d.id = m.document_id").
		Joins("JOIN (?) AS origin ON origin.resource_id = m.file_id", knowledgeFileOrigins(ctx, db))
	return db.WithContext(ctx).Raw(`SELECT id AS resource_id, created_by AS owner_id FROM knowledgebase WHERE knowledge_enabled = ?
		UNION SELECT resource_id, owner_id FROM (?) AS linked_origins`, true, linked)
}

func loadKnowledgeOwners(ctx context.Context, db, origins *gorm.DB, ids []string) (map[string][]string, error) {
	owners := make(map[string][]string)
	if len(ids) == 0 {
		return owners, nil
	}
	var rows []struct {
		ResourceID string
		OwnerID    string
	}
	if err := db.WithContext(ctx).Table("(?) AS origin", origins).Select("resource_id", "owner_id").
		Where("resource_id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		owners[row.ResourceID] = append(owners[row.ResourceID], row.OwnerID)
	}
	return owners, nil
}
