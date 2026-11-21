package service

import (
	"context"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// checkSkillSourceFolders rejects explicitly identified knowledge sources
// before native skill processing, including legacy index entries.
func checkSkillSourceFolders(ctx context.Context, folderIDs []string) error {
	ids := make([]string, 0, len(folderIDs))
	for _, id := range common.Deduplicate(folderIDs) {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := permission.CheckKnowledgeFileTransfer(ctx, dao.DB, ids); err != nil {
		return err
	}
	var files []entity.File
	if err := dao.DB.WithContext(ctx).Where("id IN ?", ids).Find(&files).Error; err != nil {
		return err
	}
	for i := range files {
		if err := permission.CheckKnowledgeFileTreeMutation(ctx, dao.DB, &files[i]); err != nil {
			return err
		}
	}
	return nil
}
