package admin

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func assertNativeAuditDelta(t *testing.T, row entity.KnowledgeAccessAuditRecord, target string) {
	t.Helper()
	for _, raw := range []*string{row.BeforeDefinitionJSON, row.AfterDefinitionJSON} {
		if raw == nil || len(*raw) > 32768 || !json.Valid([]byte(*raw)) || !strings.Contains(*raw, target) {
			t.Fatal("native owner audit missing bounded explicit target")
		}
		for _, secret := range []string{"access_token", "password", "synthetic-session-", "synthetic-created!"} {
			if strings.Contains(*raw, secret) {
				t.Fatal("native audit contains session/password data")
			}
		}
	}
}

func TestKnowledgeAccessNativeAdminDeleteRecordsExplicitAbsence(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.AutoMigrate(&entity.KnowledgeAccessTenantStateRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessAuditRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{}); err != nil {
		t.Fatal(err)
	}
	valid, token, super := "1", "synthetic-admin-delete-session", true
	actor := entity.User{ID: "audit-admin", Email: "audit-admin@invalid", Nickname: "Synthetic", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0", IsSuperuser: &super, AccessToken: &token}
	target := entity.User{ID: "audit-target", Email: "audit-target@invalid", Nickname: "Synthetic", Status: &valid, IsActive: "0", IsAuthenticated: "1", IsAnonymous: "0"}
	for _, user := range []entity.User{actor, target} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&entity.Tenant{ID: "audit-tenant", Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	for _, member := range []entity.UserTenant{{ID: "audit-owner", UserID: actor.ID, TenantID: "audit-tenant", Role: "owner", Status: &valid}, {ID: "audit-member", UserID: target.ID, TenantID: "audit-tenant", Role: "normal", Status: &valid}} {
		if err := db.Create(&member).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := common.InitLogger("error", common.FileOutput{}, "native-audit-delete"); err != nil {
		t.Fatal(err)
	}
	s := &Service{userDAO: dao.NewUserDAO(), deleteEngine: &deletionEngine{}, deleteStorage: &deletionStorage{}}
	if _, err := s.DeleteUser(nativeAdminContext(t.Context(), &actor), target.Email); err != nil {
		t.Fatal(err)
	}
	var row entity.KnowledgeAccessAuditRecord
	if err := db.Where("tenant_id = ? AND action = ?", "audit-tenant", "native_user_delete").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	assertNativeAuditDelta(t, row, target.ID)
	var before, after struct {
		Users []struct {
			ID         string
			Definition json.RawMessage
		}
		Members []struct {
			ID         string
			Definition json.RawMessage
		}
	}
	if err := json.Unmarshal([]byte(*row.BeforeDefinitionJSON), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*row.AfterDefinitionJSON), &after); err != nil {
		t.Fatal(err)
	}
	if len(before.Users) != 1 || len(after.Users) != 1 || before.Users[0].ID != target.ID || string(before.Users[0].Definition) == "null" || string(after.Users[0].Definition) != "null" || len(after.Members) != 1 || string(after.Members[0].Definition) != "null" {
		t.Fatal("native delete did not retain prior authorization and explicit absence")
	}
	var count int64
	if err := db.Model(&entity.User{}).Where("id = ?", target.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("native deletion did not commit", err)
	}
}

func seedAdminAuditMembers(t *testing.T, db *gorm.DB, tenant string) {
	t.Helper()
	valid := "1"
	rows := []entity.UserTenant{}
	for i := 0; i < 420; i++ {
		rows = append(rows, entity.UserTenant{ID: fmt.Sprintf("%032x", i+100000), UserID: fmt.Sprintf("%032x", i+200000), TenantID: tenant, Role: "normal", Status: &valid})
	}
	if err := db.CreateInBatches(&rows, 64).Error; err != nil {
		t.Fatal(err)
	}
}
func assertNativeCreateAbsence(t *testing.T, row entity.KnowledgeAccessAuditRecord, id string) {
	t.Helper()
	var before, after struct {
		Users []struct {
			ID         string
			Definition json.RawMessage
		}
	}
	if err := json.Unmarshal([]byte(*row.BeforeDefinitionJSON), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*row.AfterDefinitionJSON), &after); err != nil {
		t.Fatal(err)
	}
	if len(before.Users) != 1 || len(after.Users) != 1 || before.Users[0].ID != id || after.Users[0].ID != id || string(before.Users[0].Definition) != "null" || string(after.Users[0].Definition) == "null" {
		t.Fatal("native creation lost prior absence or created user authorization")
	}
}
