package admin

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func nativeAccessAdminDB(t *testing.T) (*gorm.DB, *Service, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old; sqlDB.Close() })
	if err = db.AutoMigrate(&entity.User{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.Knowledgebase{}, &entity.KnowledgeAccessTenantStateRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessAuditRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{}); err != nil {
		t.Fatal(err)
	}
	valid := "1"
	var actor entity.User
	for i, id := range []string{"native-admin", "native-target"} {
		token := "synthetic-session-" + id
		super := i == 0
		user := entity.User{ID: id, Email: id + "@invalid", Nickname: "Synthetic", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0", IsSuperuser: &super, AccessToken: &token}
		if err = db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			actor = user
		}
		tenant := id + "-tenant"
		if err = db.Create(&entity.Tenant{ID: tenant, Status: &valid}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&entity.UserTenant{ID: id + "-membership", UserID: id, TenantID: tenant, Role: "owner", Status: &valid}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, &Service{userDAO: dao.NewUserDAO()}, nativeAdminContext(t.Context(), &actor)
}
func TestKnowledgeAccessNativeAdminFlagsCurrentActorAndEpoch(t *testing.T) {
	db, s, ctx := nativeAccessAdminDB(t)
	seedAdminAuditMembers(t, db, "native-target-tenant")
	for _, active := range []bool{false, true} {
		if err := s.UpdateUserActivateStatus(ctx, "native-target@invalid", active); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.GrantAdmin(ctx, "native-target@invalid"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeAdmin(ctx, "native-target@invalid"); err != nil {
		t.Fatal(err)
	}
	var target, actor entity.KnowledgeAccessTenantStateRecord
	db.Where("tenant_id = ?", "native-target-tenant").Take(&target)
	db.Where("tenant_id = ?", "native-admin-tenant").Take(&actor)
	if target.Epoch != 4 || actor.Epoch != 0 {
		t.Fatal("wrong affected tenant generations", target.Epoch, actor.Epoch)
	}
	var audits []entity.KnowledgeAccessAuditRecord
	db.Where("action = ?", "native_user_flags").Find(&audits)
	if len(audits) != 4 {
		t.Fatal(len(audits))
	}
	for _, row := range audits {
		assertNativeAuditDelta(t, row, "native-target")
		if row.ActorUserID == nil || *row.ActorUserID != "native-admin" || row.CommandOperation != nil {
			t.Fatal("native actor audit missing")
		}
	}
	if err := db.Model(&entity.User{}).Where("id = ?", "native-admin").Update("is_superuser", false).Error; err != nil {
		t.Fatal(err)
	}
	if s.UpdateUserActivateStatus(ctx, "native-target@invalid", false) == nil {
		t.Fatal("revoked administrator changed native authority")
	}
	var user entity.User
	db.Where("id = ?", "native-target").Take(&user)
	if user.IsActive != "1" {
		t.Fatal("denied admin mutation wrote user")
	}
}

func TestKnowledgeAccessNativeBootstrapRepairIsAtomicAndRepeatable(t *testing.T) {
	db, s, _ := nativeAccessAdminDB(t)
	valid := "1"
	super := true
	newUser := func(id string) {
		t.Helper()
		if err := db.Create(&entity.User{ID: id, Email: id + "@invalid", Nickname: "Synthetic", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0", IsSuperuser: &super}).Error; err != nil {
			t.Fatal(err)
		}
	}
	newUser("repair")
	for i := 0; i < 2; i++ {
		if err := s.addTenantForAdmin("repair", "Synthetic"); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ?", "repair", "repair").Count(&count)
	if count != 1 {
		t.Fatal("duplicate repair relation", count)
	}
	var state entity.KnowledgeAccessTenantStateRecord
	db.Where("tenant_id = ?", "repair").Take(&state)
	if state.Epoch != 1 {
		t.Fatal("repair reset/incremented epoch twice")
	}
	newUser("repair-failure")
	callback := "access:test:bootstrap_audit_failure"
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			tx.AddError(errors.New("synthetic audit failure"))
		}
	})
	err := s.addTenantForAdmin("repair-failure", "Synthetic")
	db.Callback().Create().Remove(callback)
	if err == nil {
		t.Fatal("bootstrap audit failure accepted")
	}
	db.Model(&entity.Tenant{}).Where("id = ?", "repair-failure").Count(&count)
	if count != 0 {
		t.Fatal("failed repair committed tenant")
	}
	db.Model(&entity.UserTenant{}).Where("user_id = ?", "repair-failure").Count(&count)
	if count != 0 {
		t.Fatal("failed repair committed membership")
	}
}

func TestKnowledgeAccessNativeAdminPasswordRechecksActualSession(t *testing.T) {
	db, s, ctx := nativeAccessAdminDB(t)
	if err := s.ChangePassword(ctx, "native-target@invalid", "synthetic-admin-password!"); err != nil {
		t.Fatal(err)
	}
	var target entity.User
	if err := db.Where("id = ?", "native-target").Take(&target).Error; err != nil {
		t.Fatal(err)
	}
	if target.Password == nil || !common.CheckWerkzeugPassword("synthetic-admin-password!", *target.Password) {
		t.Fatal("password mutation failed")
	}
	original := *target.Password
	if err := db.Model(&entity.User{}).Where("id = ?", "native-admin").Update("access_token", "synthetic-new-admin-session").Error; err != nil {
		t.Fatal(err)
	}
	if s.ChangePassword(ctx, "native-target@invalid", "synthetic-denied-password!") == nil {
		t.Fatal("revoked admin session changed password")
	}
	if err := db.Where("id = ?", "native-target").Take(&target).Error; err != nil {
		t.Fatal(err)
	}
	if target.Password == nil || *target.Password != original {
		t.Fatal("denied password changed")
	}
	var states []entity.KnowledgeAccessTenantStateRecord
	db.Find(&states)
	for _, state := range states {
		if state.Epoch != 0 {
			t.Fatal("password-only update changed epoch")
		}
	}
}

func TestKnowledgeAccessNativeAdminCreateFourRowsAndAuditRollback(t *testing.T) {
	db, service, ctx := nativeAccessAdminDB(t)
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatal(err)
	}
	if err := common.InitLogger("error", common.FileOutput{}, "access-admin-create-test"); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateUser(ctx, "created@synthetic.invalid", "synthetic-created!", "user")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	for _, query := range []*gorm.DB{db.Model(&entity.User{}).Where("id = ?", id), db.Model(&entity.Tenant{}).Where("id = ?", id), db.Model(&entity.UserTenant{}).Where("user_id = ? AND tenant_id = ? AND role = ?", id, id, "owner"), db.Model(&entity.File{}).Where("tenant_id = ? AND id = parent_id", id)} {
		var count int64
		if err := query.Count(&count).Error; err != nil || count != 1 {
			t.Fatal("admin native create incomplete", count, err)
		}
	}
	var receipt entity.KnowledgeAccessAuditRecord
	if err := db.Where("tenant_id = ? AND action = ?", id, "native_user_create").Take(&receipt).Error; err != nil || receipt.ActorUserID == nil || *receipt.ActorUserID != "native-admin" || receipt.CommandOperation != nil {
		t.Fatal("admin creation actual actor missing", err)
	}
	assertNativeAuditDelta(t, receipt, id)
	assertNativeCreateAbsence(t, receipt, id)
	var state entity.KnowledgeAccessTenantStateRecord
	if err := db.Where("tenant_id = ?", id).Take(&state).Error; err != nil || state.Epoch != 1 {
		t.Fatal("admin create epoch missing", err)
	}
	failure := errors.New("synthetic admin-create audit failure")
	born := ""
	callback := "access:test:admin-create-audit-rollback"
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			if row, ok := tx.Statement.Dest.(*entity.KnowledgeAccessAuditRecord); ok {
				born = row.TenantID
			}
			tx.AddError(failure)
		}
	})
	_, err = service.CreateUser(ctx, "failed@synthetic.invalid", "synthetic-failed!", "user")
	db.Callback().Create().Remove(callback)
	if err == nil || born == "" {
		t.Fatal("admin creation audit failure accepted")
	}
	for _, query := range []*gorm.DB{db.Model(&entity.User{}).Where("id = ?", born), db.Model(&entity.Tenant{}).Where("id = ?", born), db.Model(&entity.UserTenant{}).Where("tenant_id = ?", born), db.Model(&entity.File{}).Where("tenant_id = ?", born), db.Model(&entity.KnowledgeAccessTenantStateRecord{}).Where("tenant_id = ?", born)} {
		var count int64
		if err := query.Count(&count).Error; err != nil || count != 0 {
			t.Fatal("admin creation rollback left rows", count, err)
		}
	}
}
