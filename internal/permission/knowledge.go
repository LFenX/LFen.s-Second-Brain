package permission

import (
	"context"
	"errors"
	"fmt"

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
	if err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id IN ? AND knowledge_enabled = ?", datasetIDs, true).Count(&count).Error; err != nil {
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
	if err := db.WithContext(ctx).Table("file2document AS mapping").
		Joins("JOIN document ON document.id = mapping.document_id").
		Joins("JOIN knowledgebase ON knowledgebase.id = document.kb_id").
		Where("mapping.file_id IN ? AND knowledgebase.knowledge_enabled = ?", fileIDs, true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrKnowledgeIngestionDenied
	}
	if err := db.WithContext(ctx).Model(&entity.KnowledgeSourceRevision{}).
		Where("file_id IN ?", fileIDs).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrKnowledgeIngestionDenied
	}
	return nil
}
