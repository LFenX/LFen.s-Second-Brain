//go:build linux

package dataset

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestKnowledgeAccessLiveJoinsRejectMissingEpochHooks(t *testing.T) {
	contentLexicalTokenizer(t)
	for _, test := range []struct {
		name  string
		model any
		where string
		value map[string]any
	}{{"recipient disabled", &entity.User{}, "id = '" + accessTestRecipient + "'", map[string]any{"is_active": "0"}}, {"recipient membership", &entity.UserTenant{}, "user_id = '" + accessTestRecipient + "'", map[string]any{"role": "invite"}}, {"issuer disabled", &entity.User{}, "id = '" + knowledgeTestOwner + "'", map[string]any{"is_active": "0"}}, {"issuer membership", &entity.UserTenant{}, "user_id = '" + knowledgeTestOwner + "'", map[string]any{"status": "0"}}, {"tenant disabled", &entity.Tenant{}, "id = '" + knowledgeTestOwner + "'", map[string]any{"status": "0"}}, {"domain disabled", &entity.Knowledgebase{}, "id = '" + knowledgeTestDomain + "'", map[string]any{"status": "0"}}, {"domain permission", &entity.Knowledgebase{}, "id = '" + knowledgeTestDomain + "'", map[string]any{"permission": "team"}}} {
		t.Run(test.name, func(t *testing.T) {
			f := accessTestSetup(t, false)
			f.enable(t, f.Scope.Targets[:1], "read")
			view := f.inspect(t, "read", nil)
			epoch := accessTestEpoch(t)
			if err := dao.DB.Model(test.model).Where(test.where).Updates(test.value).Error; err != nil {
				t.Fatal(err)
			}
			f.Store.gets = map[string]int{}
			status, raw := f.Service.KnowledgeAccess(f.Recipient, accessTestRecipient, "read", common.GenerateUUID(), []byte(accessJSON(accessTestReadRequest(view.ViewID, 0, 0))))
			if status == 200 || !strings.Contains(string(raw), `"data":null`) || len(f.Store.gets) > 0 || accessTestEpoch(t) != epoch {
				t.Fatal("missed epoch allowed data", status, string(raw), f.Store.gets)
			}
		})
	}
}

func TestKnowledgeAccessSourceRestoreCannotReviveAndFinalAuditFailureDenies(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read")
	view := f.inspect(t, "read", nil)
	callback := "access:test:final_audit_failure"
	dao.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "knowledge_access_audit" {
			return
		}
		updates, _ := tx.Statement.Dest.(map[string]any)
		if updates["outcome"] == "allowed" {
			tx.AddError(errors.New("synthetic final audit failure"))
		}
	})
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 503)
	dao.DB.Callback().Update().Remove(callback)
	var allowed int64
	dao.DB.Model(&entity.KnowledgeAccessAuditRecord{}).Where("action = ? AND outcome = ?", "read", "allowed").Count(&allowed)
	if allowed != 0 {
		t.Fatal("failed permit left allowed audit")
	}
	source := f.Module.Scope.Targets[0].Source.SourceID
	for i, state := range []string{"archived", "active"} {
		if err := dao.DB.Model(&entity.KnowledgeSourceLifecycle{}).Where("domain_id = ? AND source_id = ?", knowledgeTestDomain, source).Updates(map[string]any{"state": state, "version": int64(i + 2)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.Store.gets = map[string]int{}
	reply := accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 409)
	if string(reply.Data) != "null" || len(f.Store.gets) > 0 {
		t.Fatal("restored source revived old view")
	}
	row := f.Manifest.Rows[0]
	evidence := contentLexicalEvidence(entity.KnowledgeLexicalIndexRef{ID: f.Scope.Publication.ID, ManifestSHA256: f.Scope.Publication.ManifestSHA256}, row)
	if _, err := f.Service.ReadKnowledgeContentLexical(f.Owner, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: evidence}); err != nil {
		t.Fatal("new sharing source fence changed old owner history", err)
	}
}

func TestKnowledgeAccessExpiryCapacityAndStaleViews(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read")
	view := f.inspect(t, "read", nil)
	if err := dao.DB.Model(&entity.KnowledgeAccessViewRecord{}).Where("id = ?", view.ViewID).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 409)
	// Retained stale generations do not consume the current live-view cap.
	for i := 0; i < 20; i++ {
		row := entity.KnowledgeAccessViewRecord{ID: common.GenerateUUID(), TenantID: knowledgeTestOwner, SubjectUserID: accessTestRecipient, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, GrantID: f.Grant.GrantID, ModuleVersion: 1, TenantEpoch: accessTestEpoch(t) - 1, PrivateManifestJSON: "{}", CaptureJSON: "{}", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
		if err := dao.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	current := f.inspect(t, "read", nil)
	if current.ViewID == view.ViewID {
		t.Fatal("expired view reused")
	}
	for i := 0; i < 60; i++ {
		actor, domain := accessTestRecipient, knowledgeTestDomain
		if err := dao.DB.Create(&entity.KnowledgeAccessAuditRecord{ID: common.GenerateUUID(), TenantID: knowledgeTestOwner, ActorUserID: &actor, DomainID: &domain, Action: "read", Outcome: "denied", RequestID: common.GenerateUUID(), OccurredAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.Store.gets = map[string]int{}
	if accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(current.ViewID, 0, 0), 429).Code != "rate_limited" || len(f.Store.gets) > 0 {
		t.Fatal("rate limit materialized body")
	}
	oversized := strings.Repeat(" ", 32769)
	status, raw := f.Service.KnowledgeAccess(f.Recipient, accessTestRecipient, "read", common.GenerateUUID(), []byte(oversized))
	if status != 413 || !strings.Contains(string(raw), "capacity_exceeded") {
		t.Fatal(status, string(raw))
	}
}

func TestKnowledgeAccessModuleVersionAndGrantDefinitionPins(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read")
	view := f.inspect(t, "read", nil)
	change := entity.KnowledgeAccessSetModuleRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: &f.Module.ModuleID, ExpectedVersion: 1, Name: f.Module.Name, Enabled: false, Scope: f.Module.Scope, Confirmed: true, IdempotencyKey: "Synthetic_Module_Disable"}
	updated := accessTestData[entity.KnowledgeAccessModuleAdmin](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setModule", change, 200))
	if updated.Version != 2 || len(updated.Grants) != 0 {
		t.Fatal("old-version grants remained current", updated)
	}
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 409)
	ownerList := accessTestData[entity.KnowledgeAccessListModulesData](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "listModules", entity.KnowledgeAccessListModulesRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}, 200))
	if len(ownerList.Items) != 1 || ownerList.Items[0].Enabled || !ownerList.Items[0].AdministrativeOwner || len(ownerList.Items[0].Modes) != 0 {
		t.Fatal("owner lost disabled administration", ownerList)
	}
	memberList := accessTestData[entity.KnowledgeAccessListModulesData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "listModules", entity.KnowledgeAccessListModulesRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}, 200))
	if len(memberList.Items) != 0 || len(memberList.OwnedDomainPolicies) != 0 {
		t.Fatal("recipient got admin catalog", memberList)
	}
}
