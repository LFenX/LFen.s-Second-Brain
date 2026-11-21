package service

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"ragflow/internal/server"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func accessNativeServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old; sqlDB.Close() })
	migrateNativeAccessTestDB(t, db)
	valid := "1"
	for _, id := range []string{"owner-a", "owner-b", "member-a", "invitee"} {
		if err = db.Create(&entity.User{ID: id, Nickname: "Synthetic", Email: id + "@invalid", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"tenant-a", "tenant-b"} {
		if err = db.Create(&entity.Tenant{ID: id, Status: &valid}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []entity.UserTenant{{ID: "owner-a-membership", UserID: "owner-a", TenantID: "tenant-a", Role: "owner", Status: &valid}, {ID: "owner-b-membership", UserID: "owner-b", TenantID: "tenant-b", Role: "owner", Status: &valid}, {ID: "member-a-membership", UserID: "member-a", TenantID: "tenant-a", Role: "normal", Status: &valid}} {
		row.LLMID = "original-chat"
		if err = db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func TestKnowledgeAccessNativeModelSettingsAreMembershipScoped(t *testing.T) {
	db := accessNativeServiceDB(t)
	svc := NewUserTenantService()
	owner := permission.WithIdentity(t.Context(), "owner-a", "tenant-a")
	for _, raw := range []map[string]any{
		{"tenant_id": "tenant-b", "llm_id": ""}, {"status": "0"}, {"id": "tenant-b"},
		{"arbitrary_column": "x"}, {"llm_id": false}, {"asr_id": nil},
	} {
		if code, err := svc.SetModelSettings(owner, &SetModelSettingsRequest{Raw: raw}); err == nil || code == common.CodeSuccess {
			t.Fatal("invalid model settings accepted", raw)
		}
	}
	check := func(user, tenant, model string) {
		t.Helper()
		var member entity.UserTenant
		if err := db.Where("user_id = ? AND tenant_id = ?", user, tenant).Take(&member).Error; err != nil || member.LLMID != model || member.Status == nil || *member.Status != entity.TenantMembershipStatusJoined {
			t.Fatal("unexpected membership settings", user, tenant, member.LLMID, err)
		}
	}
	req := &SetModelSettingsRequest{Raw: map[string]any{"llm_id": ""}}
	member := permission.WithIdentity(t.Context(), "member-a", "tenant-a")
	if code, err := svc.SetModelSettings(member, req); err != nil || code != common.CodeSuccess {
		t.Fatal(code, err)
	}
	check("member-a", "tenant-a", "")
	check("owner-a", "tenant-a", "original-chat")
	check("owner-b", "tenant-b", "original-chat")
	callback := "access:test:model_update_failure"
	failure := errors.New("synthetic SQL failure")
	db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_tenant" {
			tx.AddError(failure)
		}
	})
	code, err := svc.SetModelSettings(owner, req)
	db.Callback().Update().Remove(callback)
	if !errors.Is(err, failure) || code == common.CodeSuccess {
		t.Fatal("failed update accepted", code, err)
	}
	check("owner-a", "tenant-a", "original-chat")
	var state entity.KnowledgeAccessTenantStateRecord
	if err := db.Where("tenant_id = ?", "tenant-a").Take(&state).Error; err != nil || state.Epoch != 0 {
		t.Fatal("model settings changed authorization epoch", state.Epoch, err)
	}
}

func TestKnowledgeAccessNativeMemberInviteAcceptRemoveAtomic(t *testing.T) {
	db := accessNativeServiceDB(t)
	seedNativeAuditMembers(t, db, "tenant-a")
	service := NewTenantService()
	_, code, err := service.AddMember(permission.WithIdentity(t.Context(), "owner-a", "tenant-a"), "tenant-a", &AddMemberRequest{Email: "invitee@invalid"})
	if err != nil || code != common.CodeSuccess {
		t.Fatal(code, err)
	}
	check := func(role, status string, epoch int64) {
		t.Helper()
		var member entity.UserTenant
		db.Where("user_id = ? AND tenant_id = ?", "invitee", "tenant-a").Take(&member)
		if member.Role != role || member.Status == nil || *member.Status != status {
			t.Fatal(member.Role, role)
		}
		var state entity.KnowledgeAccessTenantStateRecord
		db.Where("tenant_id = ?", "tenant-a").Take(&state)
		if state.Epoch != epoch {
			t.Fatal(state.Epoch, epoch)
		}
	}
	check("", entity.TenantMembershipStatusInvited, 1)
	code, err = service.AcceptInvite(permission.WithIdentity(t.Context(), "invitee", "invitee"), "tenant-a")
	if err != nil || code != common.CodeSuccess {
		t.Fatal(code, err)
	}
	check(TenantRoleNormal, entity.TenantMembershipStatusJoined, 2)
	code, err = service.RemoveMember(permission.WithIdentity(t.Context(), "invitee", "tenant-a"), "tenant-a", "invitee")
	if err != nil || code != common.CodeSuccess {
		t.Fatal(code, err)
	}
	check("", entity.TenantMembershipStatusLeft, 3)
	code, err = service.RemoveMember(permission.WithIdentity(t.Context(), "owner-a", "tenant-a"), "tenant-a", "owner-a")
	if err == nil || code == common.CodeSuccess {
		t.Fatal("removed tenant owner")
	}
	callback := "access:test:member_audit_failure"
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			tx.AddError(errors.New("synthetic audit failure"))
		}
	})
	_, code, err = service.AddMember(permission.WithIdentity(t.Context(), "owner-a", "tenant-a"), "tenant-a", &AddMemberRequest{Email: "invitee@invalid"})
	db.Callback().Create().Remove(callback)
	if err == nil || code == common.CodeSuccess {
		t.Fatal("audit failure accepted")
	}
	check("", entity.TenantMembershipStatusLeft, 3)
	var state entity.KnowledgeAccessTenantStateRecord
	db.Where("tenant_id = ?", "tenant-a").Take(&state)
	if state.Epoch != 3 {
		t.Fatal("failed invitation advanced epoch")
	}
	assertNativeMembershipAuditDelta(t, db)
}

func TestKnowledgeAccessNativeStaleProfileAndPasswordNeverRestoreSession(t *testing.T) {
	db := accessNativeServiceDB(t)
	service := NewUserService()
	original, err := common.GenerateWerkzeugPasswordHash("synthetic-original!")
	if err != nil {
		t.Fatal(err)
	}
	token := "synthetic-current-session"
	if err = db.Model(&entity.User{}).Where("id = ?", "owner-a").Updates(map[string]any{"password": original, "access_token": token}).Error; err != nil {
		t.Fatal(err)
	}
	var stale entity.User
	if err = db.Where("id = ?", "owner-a").Take(&stale).Error; err != nil {
		t.Fatal(err)
	}
	newer, err := common.GenerateWerkzeugPasswordHash("synthetic-newer!")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&entity.User{}).Where("id = ?", stale.ID).Updates(map[string]any{"is_active": "0", "is_authenticated": "0", "is_anonymous": "1", "access_token": "synthetic-revoked", "password": newer}).Error; err != nil {
		t.Fatal(err)
	}
	writes := 0
	callback := "access:test:profile-password-writes"
	db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user" {
			writes++
		}
	})
	t.Cleanup(func() { db.Callback().Update().Remove(callback) })
	originalPlain := "synthetic-original!"
	code, err := service.ChangePassword(t.Context(), &stale, &ChangePasswordRequest{Password: &originalPlain})
	if err != nil || code != common.CodeSuccess || writes != 0 {
		t.Fatal("no-new-password wrote or changed validation", code, err, writes)
	}
	wrong := "synthetic-wrong!"
	code, err = service.ChangePassword(t.Context(), &stale, &ChangePasswordRequest{Password: &wrong})
	if err == nil || code != common.CodeBadRequest || writes != 0 {
		t.Fatal("current-password validation bypassed")
	}
	nickname := "Synthetic stale profile"
	code, err = service.UpdateUserSettings(t.Context(), &stale, &UpdateSettingsRequest{Nickname: &nickname})
	if err == nil || code == common.CodeSuccess || writes != 0 {
		t.Fatal("stale disabled profile session accepted", code, err, writes)
	}
	desired := "synthetic-desired!"
	code, err = service.ChangePassword(t.Context(), &stale, &ChangePasswordRequest{Password: &originalPlain, NewPassword: &desired})
	if err == nil || code == common.CodeSuccess || writes != 0 {
		t.Fatal("disabled password session accepted")
	}
	if err = db.Exec("UPDATE user SET is_active='1', is_authenticated='1', is_anonymous='0' WHERE id = ?", stale.ID).Error; err != nil {
		t.Fatal(err)
	}
	code, err = service.ChangePassword(t.Context(), &stale, &ChangePasswordRequest{NewPassword: &desired})
	if err == nil || code == common.CodeSuccess || writes != 0 {
		t.Fatal("revoked session restored")
	}
	var current entity.User
	if err = db.Where("id = ?", stale.ID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.AccessToken == nil || *current.AccessToken != "synthetic-revoked" || current.Password == nil || *current.Password != newer || current.Nickname == nickname {
		t.Fatal("stale pointer overwrote stored state")
	}
	fresh := current
	code, err = service.ChangePassword(t.Context(), &fresh, &ChangePasswordRequest{NewPassword: &desired})
	if err != nil || code != common.CodeSuccess || writes != 1 {
		t.Fatal("current session failed", code, err, writes)
	}
	if err = db.Where("id = ?", stale.ID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Password == nil || !service.VerifyPassword(*current.Password, desired) || current.AccessToken == nil || !strings.HasPrefix(*current.AccessToken, "INVALID_") || current.IsActive != "1" || current.IsAuthenticated != "1" || current.IsAnonymous != "0" {
		t.Fatal("fresh password update changed authority")
	}
	var epochs int64
	db.Model(&entity.KnowledgeAccessTenantStateRecord{}).Count(&epochs)
	if epochs != 0 {
		t.Fatal("profile/password initialized authorization epochs")
	}
}

func TestKnowledgeAccessNativeRegistrationFourRowsAndAuditRollback(t *testing.T) {
	db := accessNativeServiceDB(t)
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatal(err)
	}
	if err := common.InitLogger("error", common.FileOutput{}, "access-registration-test"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "synthetic-service.yaml")
	if err := os.WriteFile(file, []byte("api:\n  authentication:\n    enable_register: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := server.Init(file); err != nil {
		t.Fatal(err)
	}
	service := NewUserService()
	user, code, err := service.Register(t.Context(), &RegisterRequest{Email: "created@synthetic.invalid", Nickname: "Synthetic", Password: "synthetic-created!"})
	if err != nil || code != common.CodeSuccess || user == nil {
		t.Fatal(code, err)
	}
	for _, query := range []*gorm.DB{db.Model(&entity.User{}).Where("id = ?", user.ID), db.Model(&entity.Tenant{}).Where("id = ?", user.ID), db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ? AND role = ?", user.ID, user.ID, "owner"), db.Model(&entity.File{}).Where("tenant_id = ? AND id = parent_id", user.ID)} {
		var count int64
		if err := query.Count(&count).Error; err != nil || count != 1 {
			t.Fatal("native create incomplete", count, err)
		}
	}
	var state entity.KnowledgeAccessTenantStateRecord
	if err := db.Where("tenant_id = ?", user.ID).Take(&state).Error; err != nil || state.Epoch != 1 {
		t.Fatal("creation epoch not atomic", err, state.Epoch)
	}
	var grants int64
	db.Model(&entity.KnowledgeAccessGrantRecord{}).Count(&grants)
	if grants != 0 {
		t.Fatal("registration granted content")
	}
	failure := errors.New("synthetic registration audit failure")
	born := ""
	callback := "access:test:registration-audit-rollback"
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			if row, ok := tx.Statement.Dest.(*entity.KnowledgeAccessAuditRecord); ok {
				born = row.TenantID
			}
			tx.AddError(failure)
		}
	})
	user, code, err = service.Register(t.Context(), &RegisterRequest{Email: "failed@synthetic.invalid", Nickname: "Synthetic", Password: "synthetic-failed!"})
	db.Callback().Create().Remove(callback)
	if err == nil || code == common.CodeSuccess || user != nil || born == "" {
		t.Fatal("registration audit failure accepted", code, err)
	}
	for _, query := range []*gorm.DB{db.Model(&entity.User{}).Where("id = ?", born), db.Model(&entity.Tenant{}).Where("id = ?", born), db.Model(&entity.UserTenant{}).Where("tenant_id = ?", born), db.Model(&entity.File{}).Where("tenant_id = ?", born), db.Model(&entity.KnowledgeAccessTenantStateRecord{}).Where("tenant_id = ?", born)} {
		var count int64
		if err := query.Count(&count).Error; err != nil || count != 0 {
			t.Fatal("registration rollback left rows", count, err)
		}
	}
}

func migrateNativeAccessTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&entity.User{}, &entity.UserTenant{}, &entity.Tenant{}, &entity.Knowledgebase{}, &entity.KnowledgeAccessTenantStateRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessAuditRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"CREATE UNIQUE INDEX IF NOT EXISTS idx_ka_module_1 ON knowledge_access_module(domain_id,id)", "CREATE UNIQUE INDEX IF NOT EXISTS idx_ka_audit_command_key ON knowledge_access_audit(actor_user_id,command_operation,command_key)"} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
}
