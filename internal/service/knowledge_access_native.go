package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

var errNativeUserSessionChanged = fmt.Errorf("current native user session changed")

// updateCurrentNativeUser narrows profile/password writes and rechecks the
// actual native session while holding the account row. It changes no flags.
func updateCurrentNativeUser(ctx context.Context, snapshot *entity.User, updates map[string]any) error {
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current entity.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", snapshot.ID, "1", "1", "1", "0").Take(&current).Error
		if err == gorm.ErrRecordNotFound {
			return errNativeUserSessionChanged
		}
		if err != nil {
			return err
		}
		if snapshot.AccessToken == nil || current.AccessToken == nil || *snapshot.AccessToken == "" || *snapshot.AccessToken != *current.AccessToken {
			return errNativeUserSessionChanged
		}
		if _, changing := updates["password"]; changing {
			if (snapshot.Password == nil) != (current.Password == nil) || snapshot.Password != nil && *snapshot.Password != *current.Password {
				return errNativeUserSessionChanged
			}
		}
		return tx.Model(&entity.User{}).Where("id = ?", current.ID).Updates(updates).Error
	})
}

func setNativeTenantInfo(ctx context.Context, userID string, req *SetTenantInfoRequest) (common.ErrorCode, error) {
	if req == nil || req.TenantID == nil || *req.TenantID == "" {
		return common.CodeArgumentError, fmt.Errorf("tenant_id is required")
	}
	tenantID := *req.TenantID
	updates := map[string]any{}
	allowed := map[string]bool{"asr_id": true, "embd_id": true, "img2txt_id": true, "llm_id": true, "rerank_id": true, "tts_id": true}
	for key, value := range req.Raw {
		if key == "tenant_id" {
			actual, ok := value.(string)
			if !ok || actual != tenantID {
				return common.CodeArgumentError, fmt.Errorf("invalid tenant_id")
			}
			continue
		}
		if !allowed[key] {
			return common.CodeArgumentError, fmt.Errorf("unknown tenant model setting")
		}
		if value == nil && key == "tts_id" {
			updates[key] = nil
			continue
		}
		text, ok := value.(string)
		if !ok || len(text) > 256 {
			return common.CodeArgumentError, fmt.Errorf("invalid tenant model setting")
		}
		updates[key] = text
	}
	for _, key := range []string{"tenant_id", "asr_id", "embd_id", "img2txt_id", "llm_id"} {
		if _, ok := req.Raw[key]; !ok {
			return common.CodeArgumentError, fmt.Errorf("required tenant model setting missing")
		}
	}
	code := common.CodeSuccess
	err := dao.WithKnowledgeAccessNativeMutation(ctx, dao.DB, dao.KnowledgeAccessNativeMutation{ActorUserID: &userID, Action: "native_tenant_settings", UserIDs: []string{userID}, TenantIDs: []string{tenantID}}, func(tx *gorm.DB) error {
		if err := dao.CheckKnowledgeAccessNativeUser(tx, userID); err != nil {
			code = common.CodeForbidden
			return fmt.Errorf("tenant model settings denied")
		}
		if err := dao.CheckKnowledgeAccessNativeTenant(tx, tenantID); err != nil {
			code = common.CodeForbidden
			return fmt.Errorf("tenant model settings denied")
		}
		if err := permission.NewLockingDatabaseChecker(tx).CheckTenant(ctx, permission.Subject{UserID: userID}, tenantID, permission.TenantOwner); err != nil {
			code, err = permissionresponse.Normalize(err)
			return err
		}
		return tx.Model(&entity.Tenant{}).Where("id = ?", tenantID).Updates(updates).Error
	})
	if err != nil && code == common.CodeSuccess {
		code = common.CodeExceptionError
	}
	return code, err
}

func updateAcceptedNativeInvite(ctx context.Context, tx *gorm.DB, id, user, tenant string) error {
	result := tx.WithContext(ctx).Model(&entity.UserTenant{}).Where("id = ? AND user_id = ? AND tenant_id = ? AND role = ? AND status = ?", id, user, tenant, TenantRoleInvite, "1").Update("role", TenantRoleNormal)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("pending invitation changed")
	}
	return nil
}
