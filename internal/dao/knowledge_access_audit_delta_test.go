package dao

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func TestKnowledgeAccessNativeAuditRetainsBothFlagStates(t *testing.T) {
	row, target := reviewNativeAuditRow(t, 420)
	var before, after knowledgeAccessNativeDefinition
	if err := json.Unmarshal([]byte(*row.BeforeDefinitionJSON), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*row.AfterDefinitionJSON), &after); err != nil {
		t.Fatal(err)
	}
	if len(before.Users) != 1 || len(after.Users) != 1 || before.Users[0].ID != target || after.Users[0].ID != target || before.Users[0].Definition == nil || after.Users[0].Definition == nil || before.Users[0].Definition.IsActive != "1" || after.Users[0].Definition.IsActive != "0" {
		t.Fatal("durable before/after changed flags missing")
	}
	if len(before.Members) != 0 || len(after.Members) != 0 || len(before.Users) > 1 || len(before.UserIDs) != 1 || before.UserIDs[0] != target {
		t.Fatal("unrelated tenant membership displaced actual delta")
	}
}

func TestKnowledgeAccessNativeAuditOversizedDeltaRollsBackAllTenants(t *testing.T) {
	db := knowledgeAccessTestDB(t)
	knowledgeAccessSeed(t, db)
	tenants := []string{"tenant-a", "tenant-b"}
	if err := KnowledgeAccessTransaction(t.Context(), db, func(tx *gorm.DB) error { _, err := LockKnowledgeAccessTenants(tx, tenants, false); return err }); err != nil {
		t.Fatal(err)
	}
	valid := "1"
	members := []entity.UserTenant{}
	for i := 0; i < 500; i++ {
		members = append(members, entity.UserTenant{ID: fmt.Sprintf("%032x", i+10000), UserID: fmt.Sprintf("%032x", i+20000), TenantID: "tenant-b", Role: "normal", Status: &valid})
	}
	if err := db.CreateInBatches(&members, 64).Error; err != nil {
		t.Fatal(err)
	}
	err := WithKnowledgeAccessNativeMutation(t.Context(), db, KnowledgeAccessNativeMutation{Action: "native_membership", TenantIDs: tenants}, func(tx *gorm.DB) error {
		return tx.Model(&entity.UserTenant{}).Where("tenant_id IN ?", tenants).Update("role", "invite").Error
	})
	if err == nil || !strings.Contains(err.Error(), "audit definition exceeds capacity") {
		t.Fatal("oversized actual delta did not fail closed", err)
	}
	var changed, audits int64
	if err = db.Model(&entity.UserTenant{}).Where("role = ?", "invite").Count(&changed).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&entity.KnowledgeAccessAuditRecord{}).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if changed != 0 || audits != 0 {
		t.Fatal("oversize left native changes or audit", changed, audits)
	}
	for _, tenant := range tenants {
		if knowledgeAccessEpoch(t, db, tenant) != 0 {
			t.Fatal("oversize advanced an epoch")
		}
	}
}
