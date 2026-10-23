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
	hidden := permission.KnowledgeHiddenFileIDs(ctx, dao.DB, userID)
	return db.Where("file.id NOT IN (?)", hidden), nil
}

// Native moves recreate folder IDs and relocate source objects. Until a domain
// migration exists, reject moves/deletes of managed trees before any mutation.
func checkKnowledgeTreeMutation(ctx context.Context, db *gorm.DB, file *entity.File) error {
	ids := []string{file.ID}
	if file.Type == FileTypeFolder {
		var err error
		ids, err = dao.NewFileDAO().GetSubtreeIDs(ctx, db, file.TenantID, file.ID)
		if err != nil {
			return err
		}
	}
	var count int64
	if err := db.WithContext(ctx).Model(&entity.File{}).
		Where("id IN ? AND id IN (?)", ids, dao.KnowledgeFolderIDs(ctx, db)).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return permission.ErrKnowledgeIngestionDenied
	}
	return permission.CheckKnowledgeFileTransfer(ctx, db, ids)
}
