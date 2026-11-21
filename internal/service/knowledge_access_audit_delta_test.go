package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func seedNativeAuditMembers(t *testing.T, db *gorm.DB, tenant string) {
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
func assertNativeMembershipAuditDelta(t *testing.T, db *gorm.DB) {
	t.Helper()
	var rows []entity.KnowledgeAccessAuditRecord
	if err := db.Where("tenant_id = ? AND action = ?", "tenant-a", "native_membership").Find(&rows).Error; err != nil || len(rows) != 3 {
		t.Fatal("membership audit count", len(rows), err)
	}
	transitions := map[string]bool{}
	for _, row := range rows {
		sides := []string{}
		for _, raw := range []*string{row.BeforeDefinitionJSON, row.AfterDefinitionJSON} {
			if raw == nil || len(*raw) > 32768 || !strings.Contains(*raw, "invitee") {
				t.Fatal("membership audit lost explicit target or bound")
			}
			var definition struct {
				Members []struct {
					ID         string
					Definition *struct {
						UserID, Role string
						Status       *string
					}
				}
			}
			if err := json.Unmarshal([]byte(*raw), &definition); err != nil {
				t.Fatal(err)
			}
			if len(definition.Members) != 1 {
				t.Fatal("membership audit must preserve actual changed row")
			}
			role := "absent"
			if definition.Members[0].Definition != nil {
				member := definition.Members[0].Definition
				if member.Status == nil {
					t.Fatal("missing membership status")
				}
				role = member.Role + "/" + *member.Status
				if definition.Members[0].Definition.UserID != "invitee" {
					t.Fatal("wrong changed membership user")
				}
			}
			sides = append(sides, role)
		}
		transitions[strings.Join(sides, "->")] = true
	}
	for _, want := range []string{"absent->/" + entity.TenantMembershipStatusInvited, "/" + entity.TenantMembershipStatusInvited + "->normal/" + entity.TenantMembershipStatusJoined, "normal/" + entity.TenantMembershipStatusJoined + "->/" + entity.TenantMembershipStatusLeft} {
		if !transitions[want] {
			t.Fatal("missing durable membership transition", want)
		}
	}
}
