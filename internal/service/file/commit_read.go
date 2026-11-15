package file

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
	"ragflow/internal/storage"
	"strings"
)

// ReadCommitFileContent is the immutable native commit content reader.
// Its caller owns authorization and supplies the current read transaction.
func ReadCommitFileContent(ctx context.Context, db *gorm.DB, folderID, commitID, fileID string) ([]byte, error) {
	_, err := dao.NewFileCommitDAO().GetByID(ctx, db, commitID)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %w", err)
	}

	item, err := dao.NewFileCommitItemDAO().GetByCommitIDAndFileID(ctx, db, commitID, fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found in commit: %w", err)
	}
	if (item.NewLocation != nil && strings.HasPrefix(*item.NewLocation, storage.KnowledgeOriginalObjectPrefix)) || (item.OldLocation != nil && strings.HasPrefix(*item.OldLocation, storage.KnowledgeOriginalObjectPrefix)) {
		return nil, permission.ErrKnowledgeIngestionDenied
	}

	if item.NewHash == nil && item.OldHash == nil {
		return nil, fmt.Errorf("file has no content in this commit")
	}

	hash := ""
	if item.NewHash != nil {
		hash = *item.NewHash
	} else if item.OldHash != nil {
		hash = *item.OldHash
	}

	objKey := ".objects/" + hash

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	blob, err := storageImpl.Get(ctx, folderID, objKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content from storage: %w", err)
	}

	return blob, nil
}
