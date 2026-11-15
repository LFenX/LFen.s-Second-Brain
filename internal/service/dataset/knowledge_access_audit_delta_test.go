package dataset

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestKnowledgeAccessNativeDomainOwnersPersistAuthorizationDelta(t *testing.T) {
	s, _ := setupKnowledgeTest(t)
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	_, code, err := s.DeleteDatasets(t.Context(), []string{knowledgeTestDomain}, false, permission.Subject{UserID: knowledgeTestOwner, TenantID: knowledgeTestOwner})
	if err != nil || code != common.CodeSuccess {
		t.Fatal("actual domain archive failed", code, err)
	}
	var rows []entity.KnowledgeAccessAuditRecord
	if err := dao.DB.Where("tenant_id = ? AND action = ?", knowledgeTestOwner, "native_domain").Find(&rows).Error; err != nil || len(rows) != 2 {
		t.Fatal("actual domain audit count", len(rows), err)
	}
	registered, archived := false, false
	for _, row := range rows {
		var before, after struct {
			Domains []struct {
				ID         string
				Definition *struct {
					ID, CreatedBy, Permission string
					Status                    *string
					KnowledgeEnabled          bool
					KnowledgePolicyVersion    int64
				}
			}
		}
		for _, raw := range []*string{row.BeforeDefinitionJSON, row.AfterDefinitionJSON} {
			if raw == nil || len(*raw) > 32768 || !strings.Contains(*raw, knowledgeTestDomain) {
				t.Fatal("domain owner audit lost exact changed domain or bound")
			}
		}
		if err := json.Unmarshal([]byte(*row.BeforeDefinitionJSON), &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(*row.AfterDefinitionJSON), &after); err != nil {
			t.Fatal(err)
		}
		if len(before.Domains) != 1 || len(after.Domains) != 1 || before.Domains[0].Definition == nil || after.Domains[0].Definition == nil {
			t.Fatal("domain before/after state missing")
		}
		old, current := before.Domains[0].Definition, after.Domains[0].Definition
		if current.ID != knowledgeTestDomain || current.CreatedBy != knowledgeTestOwner || current.Permission != "me" {
			t.Fatal("domain authorization identity lost")
		}
		if !old.KnowledgeEnabled && current.KnowledgeEnabled && current.KnowledgePolicyVersion == 1 {
			registered = true
		}
		if old.Status != nil && current.Status != nil && *old.Status == "1" && *current.Status == "0" {
			archived = true
		}
	}
	if !registered || !archived {
		t.Fatal("registration/retirement authorization transitions incomplete")
	}
}
