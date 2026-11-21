package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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
