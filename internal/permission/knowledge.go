package permission

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

var ErrKnowledgeModelUseDenied = fmt.Errorf("%w: knowledge domain disables model calls", ErrPermissionDenied)
var ErrKnowledgeIngestionDenied = fmt.Errorf("%w: knowledge foundation only supports catalogue and source reads", ErrPermissionDenied)

func IsKnowledgeModelPurpose(purpose string) bool {
	switch purpose {
	case "embedding", "rerank", "generation", "remote_parse", "wiki", "memory":
		return true
	default:
		return false
	}
}

// CheckDatasetModelUse is an execution guard, not the dry-run policy endpoint.
// Unregistered datasets preserve their existing behavior. Database errors fail closed.
func CheckDatasetModelUse(ctx context.Context, db *gorm.DB, datasetIDs []string, purpose string) error {
	if !IsKnowledgeModelPurpose(purpose) {
		return fmt.Errorf("invalid model purpose")
	}
	if len(datasetIDs) == 0 {
		return nil
	}
	if db == nil {
		return errors.New("knowledge policy database unavailable")
	}
	var count int64
	if err := db.WithContext(ctx).Table("(?) AS origin", knowledgeDatasetOrigins(ctx, db)).
		Where("resource_id IN ?", datasetIDs).Count(&count).Error; err != nil {
		return fmt.Errorf("knowledge model policy unavailable: %w", err)
	}
	if count != 0 {
		return ErrKnowledgeModelUseDenied
	}
	return nil
}

func CheckKnowledgeIngestion(ctx context.Context, db *gorm.DB, datasetIDs []string) error {
	err := CheckDatasetModelUse(ctx, db, datasetIDs, "embedding")
	if errors.Is(err, ErrKnowledgeModelUseDenied) {
		return ErrKnowledgeIngestionDenied
	}
	return err
}

// CheckKnowledgeFileTransfer prevents native file links from republishing a
// protected source into another dataset and bypassing its fixed v1 policy.
func CheckKnowledgeFileTransfer(ctx context.Context, db *gorm.DB, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if db == nil {
		return errors.New("knowledge policy database unavailable")
	}
	var count int64
	if err := db.WithContext(ctx).Table("(?) AS origin", dao.KnowledgeFileOrigins(ctx, db)).
		Where("resource_id IN ?", fileIDs).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrKnowledgeIngestionDenied
	}
	return nil
}

// CheckKnowledgeFileTreeMutation prevents native directory consumers from
// adopting, moving or deleting managed sources and their ordinary ancestors.
func CheckKnowledgeFileTreeMutation(ctx context.Context, db *gorm.DB, file *entity.File) error {
	protected, err := dao.HasKnowledgeFileTree(ctx, db, file)
	if err != nil {
		return err
	}
	if protected {
		return ErrKnowledgeIngestionDenied
	}
	return nil
}
