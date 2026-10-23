package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// CheckDatasetAccess checks a dataset operation using the active edition's permission policy.
func CheckDatasetAccess(ctx context.Context, subject permission.Subject, datasetID string, operation permission.Operation) error {
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(
		ctx,
		subject,
		permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID},
		operation,
	); err != nil {
		return err
	}
	// Catalogue-only domains cannot enter native model-backed workflows.
	// This check precedes metadata-filter/translation helpers as well as retrieval.
	if operation == permission.OperationUse || operation == permission.OperationRun {
		return permission.CheckDatasetModelUse(ctx, dao.DB, []string{datasetID}, "generation")
	}
	return nil
}
