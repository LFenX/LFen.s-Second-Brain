package file

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"

	"gorm.io/gorm"
)

// knowledgeBrowseDB applies protection before pagination and aggregates. Native
// tenant metadata browsing remains available for sources outside registered domains.
func knowledgeBrowseDB(ctx context.Context, userID, tenantID string) (*gorm.DB, error) {
	db := dao.DB.WithContext(ctx)
	hidden := permission.KnowledgeHiddenFileIDs(ctx, dao.DB, userID)
	return db.Where("file.id NOT IN (?)", hidden), nil
}
