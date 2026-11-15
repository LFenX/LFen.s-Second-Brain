package dao

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func reviewNativeAuditRow(t *testing.T, members int) (entity.KnowledgeAccessAuditRecord, string) {
	t.Helper()
	db := knowledgeAccessTestDB(t)
	tenant, target, actor, valid := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("9", 32), "1"
	if err := db.Create(&entity.Tenant{ID: tenant, Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	users := []entity.User{{ID: actor, Email: actor + "@synthetic.invalid", Nickname: "Synthetic administrator", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}}
	relations := []entity.UserTenant{{ID: strings.Repeat("8", 32), UserID: actor, TenantID: tenant, Role: "owner", Status: &valid}}
	for i := 0; i < members; i++ {
		userID := fmt.Sprintf("%032x", i+1000)
		if i == 0 {
			userID = target
		}
		users = append(users, entity.User{ID: userID, Email: userID + "@synthetic.invalid", Nickname: "Synthetic member", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"})
		relations = append(relations, entity.UserTenant{ID: fmt.Sprintf("%032x", i+100000), UserID: userID, TenantID: tenant, Role: "normal", Status: &valid})
	}
	if err := db.CreateInBatches(&users, 64).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(&relations, 64).Error; err != nil {
		t.Fatal(err)
	}
	if err := WithKnowledgeAccessNativeMutation(t.Context(), db, KnowledgeAccessNativeMutation{ActorUserID: &actor, Action: "native_user_flags", UserIDs: []string{target, actor}, TenantIDs: []string{tenant}}, func(tx *gorm.DB) error {
		return tx.Model(&entity.User{}).Where("id = ?", target).Update("is_active", "0").Error
	}); err != nil {
		t.Fatal(err)
	}
	var audits []entity.KnowledgeAccessAuditRecord
	if err := db.Where("action = ?", "native_user_flags").Find(&audits).Error; err != nil || len(audits) != 1 {
		t.Fatal("actual native audit fixture", err, len(audits))
	}
	if audits[0].ActorUserID == nil || *audits[0].ActorUserID != actor || audits[0].BeforeDefinitionJSON == nil || audits[0].AfterDefinitionJSON == nil {
		t.Fatal("actual actor or before/after definitions missing")
	}
	return audits[0], target
}

// Review-only improved oracle: inspect the actual durable native mutation audit,
// including all subject/resource fields and both definitions, not just its helper.
func TestReviewAccessNativeAuditDefinitionKeepsApprovedByteBound(t *testing.T) {
	row, _ := reviewNativeAuditRow(t, 210)
	t.Logf("actual persisted native audit: before_bytes=%d after_bytes=%d", len(*row.BeforeDefinitionJSON), len(*row.AfterDefinitionJSON))
	if len(*row.BeforeDefinitionJSON) > 32768 || len(*row.AfterDefinitionJSON) > 32768 {
		t.Error("actual persisted native authorization definition exceeds approved 32768-byte bound")
	}
}

func TestReviewAccessNativeAuditRetainsChangedPrincipalAtLargeTenant(t *testing.T) {
	row, target := reviewNativeAuditRow(t, 420)
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("actual persisted audit: actor_present=%t domain_present=%t module_present=%t command_resource_present=%t before_bytes=%d after_bytes=%d changed_principal_anywhere=%t", row.ActorUserID != nil, row.DomainID != nil, row.ModuleID != nil, row.CommandResourceID != nil, len(*row.BeforeDefinitionJSON), len(*row.AfterDefinitionJSON), strings.Contains(string(raw), target))
	if !strings.Contains(string(raw), target) {
		t.Error("entire persisted native_user_flags audit omits changed principal, including subject/resource fields and before/after definitions")
	}
}
