package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func accessDepartmentFixture(t *testing.T, share bool) *accessTestFixture {
	t.Helper()
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("role", "owner").Error; err != nil {
		t.Fatal(err)
	}
	if share {
		f.enable(t, f.Scope.Targets[:1], "read_search")
		accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "revokeGrant", entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: "Synthetic_Department_Revoke_Direct"}, 200)
	}
	return f
}

func accessDepartmentRequest(users []string, key string) entity.KnowledgeAccessSetDepartmentRequest {
	return entity.KnowledgeAccessSetDepartmentRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner, Name: "Synthetic department A", Enabled: true, UserIDs: users, Confirmed: true, IdempotencyKey: key}
}

func accessDepartmentCreate(t *testing.T, f *accessTestFixture, users []string, key string) entity.KnowledgeAccessDepartment {
	t.Helper()
	return accessTestData[entity.KnowledgeAccessDepartment](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", accessDepartmentRequest(users, key), 200))
}

func accessDepartmentGrant(t *testing.T, f *accessTestFixture, department entity.KnowledgeAccessDepartment) entity.KnowledgeAccessGrant {
	t.Helper()
	return accessTestData[entity.KnowledgeAccessGrant](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "createGrant", entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, ExpectedModuleVersion: f.Module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "department", ID: department.DepartmentID}, Mode: "read_search", Delivery: "web_only", ExpiresAt: accessTime(time.Now().Add(time.Hour)), Confirmed: true, IdempotencyKey: "Synthetic_Department_Grant_" + department.DepartmentID}, 201))
}

func TestKnowledgeAccessDepartmentsNativeOwnerAndPrivateCatalog(t *testing.T) {
	f := accessDepartmentFixture(t, false)
	request := accessDepartmentRequest([]string{accessTestRecipient}, "Synthetic_Department_Create")
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "setDepartment", request, 404)
	department := accessDepartmentCreate(t, f, request.UserIDs, request.IdempotencyKey)
	if department.MemberCount == nil || *department.MemberCount != 1 || !slices.Equal(department.UserIDs, request.UserIDs) {
		t.Fatal(department)
	}
	request.Name, request.Enabled, request.IdempotencyKey = "Disabled department", false, "Synthetic_Department_Disabled"
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 200)
	query := entity.KnowledgeAccessListDepartmentsRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}
	owner := accessTestData[entity.KnowledgeAccessListDepartmentsData](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "listDepartments", query, 200))
	if len(owner.Items) != 2 || owner.Items[0].UserIDs == nil || owner.Items[1].MemberCount == nil {
		t.Fatal(owner)
	}
	rosterQueries := 0
	callback := "synthetic:department_catalog_no_roster"
	dao.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_department_member" {
			rosterQueries++
		}
	})
	reader := accessTestData[entity.KnowledgeAccessListDepartmentsData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "listDepartments", query, 200))
	dao.DB.Callback().Query().Remove(callback)
	if rosterQueries != 0 || len(reader.Items) != 1 || reader.Items[0].DepartmentID != department.DepartmentID || reader.Items[0].MemberCount != nil || reader.Items[0].UserIDs != nil {
		t.Fatal("catalog exposed roster", reader, rosterQueries)
	}
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: common.GenerateUUID(), Mode: "read", Delivery: "web_only"}, 404)
}

func TestKnowledgeAccessDepartmentRosterReplayCASAndNoOp(t *testing.T) {
	f := accessDepartmentFixture(t, false)
	first := accessDepartmentRequest([]string{accessTestRecipient, knowledgeTestOwner}, "Synthetic_Department_Original")
	created := accessTestData[entity.KnowledgeAccessDepartment](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", first, 200))
	epoch := accessTestEpoch(t)
	second := first
	second.DepartmentID, second.ExpectedVersion, second.Name, second.UserIDs, second.IdempotencyKey = &created.DepartmentID, created.Version, "Renamed department", []string{accessTestRecipient}, "Synthetic_Department_Changed"
	changed := accessTestData[entity.KnowledgeAccessDepartment](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", second, 200))
	if changed.Version != 2 || accessTestEpoch(t) != epoch+1 {
		t.Fatal(changed)
	}
	replay := accessTestData[entity.KnowledgeAccessDepartment](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", first, 200))
	if replay.DepartmentID != created.DepartmentID || replay.Version != 1 || replay.Name != first.Name || *replay.MemberCount != 2 || accessTestEpoch(t) != epoch+1 {
		t.Fatal("replay was not original receipt", replay)
	}
	conflict := first
	conflict.Name = "Different body"
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", conflict, 409)
	stale := second
	stale.IdempotencyKey = "Synthetic_Department_Stale_CAS"
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", stale, 409)
	noop := second
	noop.ExpectedVersion, noop.IdempotencyKey = 2, "Synthetic_Department_NoOp"
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", noop, 200)
	if accessTestEpoch(t) != epoch+1 {
		t.Fatal("no-op bumped epoch")
	}
	var journal entity.KnowledgeAccessAuditRecord
	if err := dao.DB.Where("actor_user_id = ? AND command_operation = ? AND command_key = ?", knowledgeTestOwner, "setDepartment", first.IdempotencyKey).Take(&journal).Error; err != nil {
		t.Fatal(err)
	}
	if journal.CommandScopeKind == nil || *journal.CommandScopeKind != "tenant" || journal.CommandScopeID == nil || *journal.CommandScopeID != knowledgeTestOwner || journal.DomainID != nil || journal.CommandState == nil || *journal.CommandState != "completed" {
		t.Fatal("invalid department receipt")
	}
	if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", knowledgeTestOwner, knowledgeTestOwner).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", first, 404)
}

func TestKnowledgeAccessDepartmentGrantIsolationAndRosterRevocation(t *testing.T) {
	f := accessDepartmentFixture(t, true)
	other := accessTestSession(t, accessTestOther)
	a := accessDepartmentCreate(t, f, []string{accessTestRecipient}, "Synthetic_Department_A")
	accessDepartmentCreate(t, f, []string{accessTestOther}, "Synthetic_Department_B")
	accessDepartmentGrant(t, f, a)
	query := entity.KnowledgeAccessListModulesRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}
	visible := accessTestData[entity.KnowledgeAccessListModulesData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "listModules", query, 200))
	denied := accessTestData[entity.KnowledgeAccessListModulesData](t, accessTestCall(t, f.Service, other, accessTestOther, "listModules", query, 200))
	if len(visible.Items) != 1 || len(denied.Items) != 0 {
		t.Fatal("department scope leak")
	}
	inspect := entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: "read", Delivery: "web_only"}
	gets := accessJSON(f.Store.gets)
	accessTestCall(t, f.Service, other, accessTestOther, "inspect", inspect, 404)
	if accessJSON(f.Store.gets) != gets || f.Engine.searches != 0 {
		t.Fatal("denial read source or searched")
	}
	view := accessTestData[entity.KnowledgeAccessInspectData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", inspect, 200))
	read := accessTestReadRequest(view.ViewID, 0, 0)
	evidence := accessTestData[entity.KnowledgeAccessReadData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", read, 200))
	update := accessDepartmentRequest([]string{}, "Synthetic_Department_Remove_Recipient")
	update.DepartmentID, update.ExpectedVersion = &a.DepartmentID, a.Version
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", update, 200)
	read.Selector = json.RawMessage(accessJSON(entity.KnowledgeAccessEvidenceReadSelector{Kind: "evidence", EvidenceRef: evidence.EvidenceRef}))
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", read, 409)
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", inspect, 404)
	list := accessTestData[entity.KnowledgeAccessListModulesData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "listModules", query, 200))
	if len(list.Items) != 0 {
		t.Fatal("removed member still sees module")
	}
}

func TestKnowledgeAccessDepartmentFinalPermitAndLiveChecks(t *testing.T) {
	f := accessDepartmentFixture(t, true)
	a := accessDepartmentCreate(t, f, []string{accessTestRecipient}, "Synthetic_Department_A")
	accessDepartmentGrant(t, f, a)
	inspect := entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: "read", Delivery: "web_only"}
	view := accessTestData[entity.KnowledgeAccessInspectData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", inspect, 200))
	changed := false
	f.Store.onGet = func() {
		if changed {
			return
		}
		changed = true
		update := accessDepartmentRequest([]string{}, "Synthetic_Department_MidRead")
		update.DepartmentID, update.ExpectedVersion = &a.DepartmentID, a.Version
		accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", update, 200)
	}
	read := accessTestReadRequest(view.ViewID, 0, 0)
	reply := accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", read, 409)
	if !changed || string(reply.Data) != "null" {
		t.Fatal("late roster change released data")
	}
	f.Store.onGet = nil
	update := accessDepartmentRequest([]string{accessTestRecipient}, "Synthetic_Department_Restore")
	update.DepartmentID, update.ExpectedVersion = &a.DepartmentID, 2
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", update, 200)
	view = accessTestData[entity.KnowledgeAccessInspectData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", inspect, 200))
	epoch := accessTestEpoch(t)
	if err := dao.DB.Where("department_id = ? AND user_id = ?", a.DepartmentID, accessTestRecipient).Delete(&entity.KnowledgeAccessDepartmentMemberRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	read = accessTestReadRequest(view.ViewID, 0, 0)
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", read, 409)
	if accessTestEpoch(t) != epoch {
		t.Fatal("test changed epoch instead of exercising live join")
	}
}

func TestKnowledgeAccessDepartmentRosterValidationAndNativeOwnership(t *testing.T) {
	f := accessDepartmentFixture(t, false)
	other := accessTestSession(t, accessTestOther)
	request := accessDepartmentRequest([]string{accessTestRecipient, accessTestRecipient}, "Synthetic_Department_Duplicate")
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 400)
	request.UserIDs = []string{common.GenerateUUID()}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 404)
	request.UserIDs = []string{}
	request.ExpectedVersion = 1
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 400)
	valid := "1"
	if err := dao.DB.Create(&entity.Tenant{ID: accessTestOther, Status: &valid}).Error; err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct{ user, role string }{{knowledgeTestOwner, "owner"}, {accessTestOther, "normal"}} {
		if err := dao.DB.Create(&entity.UserTenant{ID: common.GenerateUUID(), TenantID: accessTestOther, UserID: pair.user, Role: pair.role, Status: &valid}).Error; err != nil {
			t.Fatal(err)
		}
	}
	request.TenantID, request.ExpectedVersion, request.IdempotencyKey = accessTestOther, 0, "Synthetic_Different_Tenant_Owner"
	accessTestCall(t, f.Service, other, accessTestOther, "setDepartment", request, 404)
	created := accessTestData[entity.KnowledgeAccessDepartment](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 200))
	if created.TenantID == knowledgeTestOwner {
		t.Fatal("owner inferred from tenant ID")
	}
	query := entity.KnowledgeAccessListDepartmentsRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}
	if err := dao.DB.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", accessTestOther, knowledgeTestOwner).Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	accessTestCall(t, f.Service, other, accessTestOther, "listDepartments", query, 404)
}

func TestKnowledgeAccessDepartmentCompleteSetBounds(t *testing.T) {
	f := accessDepartmentFixture(t, false)
	for i := 0; i < 64; i++ {
		row := entity.KnowledgeAccessDepartmentRecord{ID: common.GenerateUUID(), TenantID: knowledgeTestOwner, Name: fmt.Sprintf("Synthetic department %d", i), Version: 1, Enabled: true, CreatedBy: knowledgeTestOwner}
		if err := dao.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	query := entity.KnowledgeAccessListDepartmentsRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}
	list := accessTestData[entity.KnowledgeAccessListDepartmentsData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "listDepartments", query, 200))
	if len(list.Items) != 64 {
		t.Fatal("partial list")
	}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", accessDepartmentRequest([]string{}, "Synthetic_Over_Department_Cap"), 413)
	row := entity.KnowledgeAccessDepartmentRecord{ID: common.GenerateUUID(), TenantID: knowledgeTestOwner, Name: "Disabled department", Version: 1, CreatedBy: knowledgeTestOwner}
	if err := dao.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "listDepartments", query, 413)
	list = accessTestData[entity.KnowledgeAccessListDepartmentsData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "listDepartments", query, 200))
	if len(list.Items) != 64 {
		t.Fatal("hidden department affected permitted count")
	}
}

func TestKnowledgeAccessDepartmentAuditFailureAtomicRollback(t *testing.T) {
	f := accessDepartmentFixture(t, false)
	department := accessDepartmentCreate(t, f, []string{accessTestRecipient}, "Synthetic_Department_Before_Rollback")
	epoch := accessTestEpoch(t)
	request := accessDepartmentRequest([]string{}, "Synthetic_Department_Failed_Update")
	request.DepartmentID, request.ExpectedVersion, request.Name = &department.DepartmentID, department.Version, "Changed department"
	callback := "synthetic:department_audit_failure"
	dao.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			tx.AddError(errors.New("synthetic audit unavailable"))
		}
	})
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 503)
	dao.DB.Callback().Update().Remove(callback)
	row, err := accessDepartment(dao.DB, knowledgeTestOwner, department.DepartmentID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := accessDepartmentDTO(dao.DB, row, true)
	if err != nil || current.Version != 1 || current.Name != department.Name || *current.MemberCount != 1 || accessTestEpoch(t) != epoch {
		t.Fatal("partial department/roster/epoch survived", current, err)
	}
	receipt, err := accessReceipt(f.Owner, knowledgeTestOwner, "setDepartment", request.IdempotencyKey)
	if err != nil || receipt != nil {
		t.Fatal("failed command consumed key")
	}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setDepartment", request, 200)
	if accessTestEpoch(t) != epoch+1 {
		t.Fatal("retry was not one mutation")
	}
}
