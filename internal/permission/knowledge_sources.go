package permission

import (
	"context"
	"ragflow/internal/dao"

	"gorm.io/gorm"
)

// KnowledgeHiddenFileIDs is the same ownership intersection used for exact
// resource reads, applied before native browse pagination and aggregates.
func KnowledgeHiddenFileIDs(ctx context.Context, db *gorm.DB, userID string) *gorm.DB {
	return db.WithContext(ctx).Table("(?) AS origin", dao.KnowledgeFileOrigins(ctx, db)).
		Distinct("resource_id").Where("owner_id <> ? OR owner_id IS NULL", userID)
}

func knowledgeDatasetOrigins(ctx context.Context, db *gorm.DB) *gorm.DB {
	linked := db.WithContext(ctx).Table("file2document m").Select("d.kb_id AS resource_id, origin.owner_id").
		Joins("JOIN document d ON d.id = m.document_id").
		Joins("JOIN (?) AS origin ON origin.resource_id = m.file_id", dao.KnowledgeFileOrigins(ctx, db))
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
