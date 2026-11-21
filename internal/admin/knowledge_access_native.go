package admin

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

type nativeAdminActorKey struct{}
type nativeAdminActor struct {
	UserID        string
	SessionSHA256 [32]byte
}

func nativeAdminContext(ctx context.Context, user *entity.User) context.Context {
	value := nativeAdminActor{UserID: user.ID}
	if user.AccessToken != nil {
		value.SessionSHA256 = sha256.Sum256([]byte(*user.AccessToken))
	}
	return context.WithValue(ctx, nativeAdminActorKey{}, value)
}
func nativeAdminMutation(ctx context.Context, action string, users, tenants, domains []string, fn func(*gorm.DB) error) error {
	actor, ok := ctx.Value(nativeAdminActorKey{}).(nativeAdminActor)
	if !ok || actor.UserID == "" {
		return fmt.Errorf("current native administrator required")
	}
	users = append(append([]string(nil), users...), actor.UserID)
	return dao.WithKnowledgeAccessNativeMutation(ctx, dao.DB, dao.KnowledgeAccessNativeMutation{ActorUserID: &actor.UserID, Action: action, UserIDs: users, TenantIDs: tenants, DomainIDs: domains}, func(tx *gorm.DB) error {
		var current entity.User
		if err := tx.Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ? AND is_superuser = ?", actor.UserID, "1", "1", "1", "0", true).Take(&current).Error; err != nil {
			return fmt.Errorf("current native administrator required")
		}
		if current.AccessToken == nil || sha256.Sum256([]byte(*current.AccessToken)) != actor.SessionSHA256 {
			return fmt.Errorf("native administrator session changed")
		}
		return fn(tx)
	})
}

func (s *Service) mutateNativeUserAuthority(ctx context.Context, username string, fn func(*gorm.DB, *entity.User) error) error {
	users, err := s.userDAO.ListByEmail(ctx, dao.DB, username)
	if err != nil || len(users) == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}
	if len(users) > 1 {
		return fmt.Errorf("exist more than 1 user: %s", username)
	}
	id := users[0].ID
	return nativeAdminMutation(ctx, "native_user_flags", []string{id}, nil, nil, func(tx *gorm.DB) error {
		var current entity.User
		if err := tx.Where("id = ?", id).Take(&current).Error; err != nil {
			return err
		}
		return fn(tx, &current)
	})
}

func (s *Service) addTenantForAdminTx(tx *gorm.DB, userID, nickname string) error {
	if err := dao.CheckKnowledgeAccessNativeUser(tx, userID); err != nil {
		return err
	}
	var tenant entity.Tenant
	err := tx.Where("id = ?", userID).Take(&tenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		name := utility.TruncateRunes(nickname+"'s Kingdom", common.TenantNameLimit)
		status := "1"
		tenant = entity.Tenant{ID: userID, Name: &name, Status: &status}
		if err = dao.ApplyDefaultTenantInviteEnabled(context.Background(), tx, &tenant); err != nil {
			return fmt.Errorf("failed to initialize tenant invitation setting: %w", err)
		}
		if err = dao.NewTenantDAO().Create(context.Background(), tx, &tenant); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if tenant.Status == nil || *tenant.Status != "1" {
		return fmt.Errorf("admin tenant is inactive")
	}
	var member entity.UserTenant
	err = tx.Where("user_id = ? AND tenant_id = ?", userID, tenant.ID).Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status := "1"
		return tx.Create(&entity.UserTenant{ID: common.GenerateUUID(), UserID: userID, TenantID: tenant.ID, Role: "owner", Status: &status, InvitedBy: userID}).Error
	}
	if err != nil {
		return err
	}
	if member.Role != "owner" {
		return fmt.Errorf("admin tenant relation is not an owner")
	}
	if member.Status != nil && *member.Status == "1" {
		return nil
	}
	result := tx.Model(&entity.UserTenant{}).Where("id = ? AND role = ?", member.ID, "owner").Update("status", "1")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("admin tenant repair changed")
	}
	return nil
}
