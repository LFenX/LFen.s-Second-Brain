package dao

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ragflow/internal/entity"
)

func knowledgeAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err = db.AutoMigrate(&entity.User{}, &entity.UserTenant{}, &entity.Tenant{}, &entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	if err = migrateKnowledgeAccess(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestKnowledgeAccessDepartmentNamedIndexesAndUniqueness(t *testing.T) {
	db := knowledgeAccessTestDB(t)
	if err := migrateKnowledgeAccess(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for _, index := range []struct{ table, name string }{{"knowledge_access_department", "idx_ka_department_1"}, {"knowledge_access_department_member", "idx_ka_department_member_1"}} {
		if !db.Migrator().HasIndex(index.table, index.name) {
			t.Fatal("missing named department index", index.name)
		}
	}
	department := entity.KnowledgeAccessDepartmentRecord{ID: strings.Repeat("a", 32), TenantID: strings.Repeat("b", 32), Name: "Synthetic department", Version: 1, Enabled: true, CreatedBy: strings.Repeat("c", 32)}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	member := entity.KnowledgeAccessDepartmentMemberRecord{ID: strings.Repeat("d", 32), DepartmentID: department.ID, UserID: strings.Repeat("e", 32)}
	if err := db.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	member.ID = strings.Repeat("f", 32)
	if db.Create(&member).Error == nil {
		t.Fatal("department membership duplicate accepted")
	}
	probe := db.Session(&gorm.Session{DryRun: true}).Where("department_id = ? AND user_id = ?", department.ID, member.UserID).Take(&entity.KnowledgeAccessDepartmentMemberRecord{})
	if len(probe.Statement.Vars) < 2 || strings.Contains(probe.Statement.SQL.String(), department.ID) || !strings.Contains(probe.Statement.SQL.String(), "department_id = ?") {
		t.Fatal("unbound department query")
	}
}
func knowledgeAccessSeed(t *testing.T, db *gorm.DB) {
	t.Helper()
	valid := "1"
	if err := db.Create(&entity.User{ID: "user", Nickname: "Synthetic", Email: "synthetic@invalid", Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"tenant-a", "tenant-b"} {
		if err := db.Create(&entity.Tenant{ID: id, Status: &valid}).Error; err != nil {
			t.Fatal(err)
		}
		status := valid
		if i == 1 {
			status = "0"
		}
		if err := db.Create(&entity.UserTenant{ID: "membership-" + id, UserID: "user", TenantID: id, Role: "normal", Status: &status}).Error; err != nil {
			t.Fatal(err)
		}
	}
}
func knowledgeAccessEpoch(t *testing.T, db *gorm.DB, tenant string) int64 {
	t.Helper()
	var row entity.KnowledgeAccessTenantStateRecord
	if err := db.Where("tenant_id = ?", tenant).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.Epoch
}

func TestKnowledgeAccessMigrationNamedNullAndCaseSensitiveKeys(t *testing.T) {
	db := knowledgeAccessTestDB(t)
	if err := migrateKnowledgeAccess(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	actor, op, key := "actor", "setPolicy", "Synthetic_Command_ABC"
	newAudit := func(id string) *entity.KnowledgeAccessAuditRecord {
		return &entity.KnowledgeAccessAuditRecord{ID: id, TenantID: "tenant", ActorUserID: &actor, Action: "setPolicy", Outcome: "changed", RequestID: "request", OccurredAt: time.Now(), CommandOperation: &op, CommandKey: &key}
	}
	if !db.Migrator().HasIndex(&entity.KnowledgeAccessAuditRecord{}, "idx_ka_audit_command_key") {
		t.Fatal("missing named command unique index")
	}
	if err := db.Create(newAudit("first")).Error; err != nil {
		t.Fatal(err)
	}
	if db.Create(newAudit("duplicate")).Error == nil {
		t.Fatal("same actor/operation/key duplicated")
	}
	key = "synthetic_command_abc"
	if err := db.Create(newAudit("different-case")).Error; err != nil {
		t.Fatal("case-sensitive key collapsed", err)
	}
	for _, id := range []string{"read-a", "read-b"} {
		row := newAudit(id)
		row.CommandOperation = nil
		row.CommandKey = nil
		if err := db.Create(row).Error; err != nil {
			t.Fatal("NULL command fields constrained ordinary events", err)
		}
	}
	indexes, err := db.Migrator().GetIndexes(&entity.KnowledgeAccessTenantStateRecord{})
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range indexes {
		if index.Name() == "idx_knowledge_access_tenant_state_tenant_id" {
			t.Fatal("redundant tenant PK index")
		}
	}
}

func TestKnowledgeAccessNativeAtomicEpochAuditAndAllStatusMemberships(t *testing.T) {
	db := knowledgeAccessTestDB(t)
	knowledgeAccessSeed(t, db)
	failure := errors.New("synthetic SQL failure")
	mutation := KnowledgeAccessNativeMutation{Action: "native_user_flags", UserIDs: []string{"user"}}
	err := WithKnowledgeAccessNativeMutation(t.Context(), db, mutation, func(tx *gorm.DB) error {
		if err := tx.Model(&entity.User{}).Where("id = ?", "user").Update("is_active", "0").Error; err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var user entity.User
	db.Where("id = ?", "user").Take(&user)
	if user.IsActive != "1" {
		t.Fatal("native rollback failed")
	}
	var count int64
	db.Model(&entity.KnowledgeAccessAuditRecord{}).Count(&count)
	if count != 0 {
		t.Fatal("rolled-back audit committed")
	}
	if err = WithKnowledgeAccessNativeMutation(t.Context(), db, mutation, func(tx *gorm.DB) error {
		return tx.Model(&entity.User{}).Where("id = ?", "user").Update("is_active", "0").Error
	}); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		if knowledgeAccessEpoch(t, db, tenant) != 1 {
			t.Fatal("inactive membership tenant omitted")
		}
	}
	if err = WithKnowledgeAccessNativeMutation(t.Context(), db, mutation, func(tx *gorm.DB) error {
		return tx.Model(&entity.User{}).Where("id = ?", "user").Update("access_token", "Synthetic_Secret_Must_Not_Be_Audited").Error
	}); err != nil {
		t.Fatal(err)
	}
	if knowledgeAccessEpoch(t, db, "tenant-a") != 1 {
		t.Fatal("session rotation changed epoch")
	}
	var rows []entity.KnowledgeAccessAuditRecord
	db.Find(&rows)
	if len(rows) != 2 {
		t.Fatal("unexpected native mutation audit count", len(rows))
	}
	for _, row := range rows {
		if row.CommandOperation != nil || row.BeforeDefinitionJSON == nil || row.AfterDefinitionJSON == nil || strings.Contains(*row.AfterDefinitionJSON, "Synthetic_Secret") {
			t.Fatal("unsafe native audit")
		}
	}
	callback := "access:test:native_audit_failure"
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			tx.AddError(failure)
		}
	})
	err = WithKnowledgeAccessNativeMutation(t.Context(), db, mutation, func(tx *gorm.DB) error {
		return tx.Model(&entity.User{}).Where("id = ?", "user").Update("is_active", "1").Error
	})
	db.Callback().Create().Remove(callback)
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	db.Where("id = ?", "user").Take(&user)
	if user.IsActive != "0" || knowledgeAccessEpoch(t, db, "tenant-a") != 1 {
		t.Fatal("audit failure did not roll back native auth and epoch")
	}
}

func TestKnowledgeAccessNativeDeleteRetainsEpochAndReceipt(t *testing.T) {
	db := knowledgeAccessTestDB(t)
	knowledgeAccessSeed(t, db)
	actor, op, key, state, result := "user", "setPolicy", "Synthetic_Tombstone_1", "completed", `{"safe":true}`
	now := time.Now()
	receipt := entity.KnowledgeAccessAuditRecord{ID: "command", TenantID: "tenant-a", ActorUserID: &actor, Action: "setPolicy", Outcome: "changed", RequestID: "request", OccurredAt: now, CommandOperation: &op, CommandKey: &key, CommandState: &state, CommandResultJSON: &result}
	if err := db.Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.KnowledgeAccessViewRecord{ID: "view", TenantID: "tenant-a", SubjectUserID: "user", GrantID: "grant", DomainID: "domain", ModuleID: "module", ModuleVersion: 1, TenantEpoch: 0, PrivateManifestJSON: "{}", CaptureJSON: "{}", IssuedAt: now, ExpiresAt: now.Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := WithKnowledgeAccessNativeMutation(t.Context(), db, KnowledgeAccessNativeMutation{Action: "native_user_delete", UserIDs: []string{"user"}}, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", "user").Delete(&entity.UserTenant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", []string{"tenant-a", "tenant-b"}).Delete(&entity.Tenant{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", "user").Delete(&entity.User{}).Error
	}); err != nil {
		t.Fatal(err)
	}
	if knowledgeAccessEpoch(t, db, "tenant-a") != 1 {
		t.Fatal("epoch was deleted/reset")
	}
	var commands, views int64
	db.Model(&entity.KnowledgeAccessAuditRecord{}).Where("id = ?", "command").Count(&commands)
	db.Model(&entity.KnowledgeAccessViewRecord{}).Where("id = ?", "view").Count(&views)
	if commands != 1 || views != 1 {
		t.Fatal("native delete cascaded access evidence")
	}
	if err := migrateKnowledgeAccess(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if knowledgeAccessEpoch(t, db, "tenant-a") != 1 {
		t.Fatal("migration reset retained generation")
	}
}
