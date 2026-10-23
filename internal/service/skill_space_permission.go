//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

func resolveSkillSpaceTenantID(ctx context.Context, userID, spaceID string, operation permission.Operation) (string, error) {
	spaceID = normalizeSpaceID(spaceID)
	if spaceID == defaultSkillSpaceID {
		return requireOwnedTenantID(ctx, userID)
	}

	checker := permission.NewDatabaseChecker(dao.DB)
	subject := permission.Subject{UserID: userID}
	ref := permission.ResourceRef{Kind: permission.ResourceKindSkillSpace, ID: spaceID}
	if err := checker.CheckResource(ctx, subject, ref, operation); err != nil {
		return "", err
	}
	access, err := checker.ResolveAccess(ctx, subject, ref)
	if err != nil {
		return "", err
	}
	return access.TenantID, nil
}

func accessibleSkillSpaceIDs(ctx context.Context, userID string, operation permission.Operation) ([]string, error) {
	scope, err := permission.NewDatabaseChecker(dao.DB).Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindSkillSpace,
		Operation: operation,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode != permission.ScopeIDs {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

func normalizeSkillSpacePermissionError(err error) (common.ErrorCode, error) {
	return permissionresponse.NormalizeHidden(err)
}

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
