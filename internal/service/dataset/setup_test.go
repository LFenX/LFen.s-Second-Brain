package dataset

import (
	"net/url"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupServiceTestDB initializes an in-memory SQLite database for tests.
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	if err = db.AutoMigrate(
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.KnowledgeSourceRevision{},
		&entity.Task{},
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.File2Document{},
		&entity.File{},
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.API4Conversation{},
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserCanvas{},
		&entity.KnowledgeAccessTenantStateRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessAuditRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessViewRecord{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	for _, query := range []string{"CREATE UNIQUE INDEX IF NOT EXISTS idx_ka_module_1 ON knowledge_access_module(domain_id,id)", "CREATE UNIQUE INDEX IF NOT EXISTS idx_ka_audit_command_key ON knowledge_access_audit(actor_user_id,command_operation,command_key)"} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// pushServiceDB swaps dao.DB for the test and restores after.
func pushServiceDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	oldDB := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = oldDB })
}

func ensureDatasetTestMembership(t *testing.T, userID, tenantID, role string) {
	t.Helper()
	var userCount, tenantCount int64
	if err := dao.DB.Model(&entity.User{}).Where("id = ?", userID).Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if userCount == 0 {
		if err := dao.DB.Create(&entity.User{ID: userID, Email: userID + "@synthetic.invalid", Nickname: "Synthetic", Status: sptr("1"), IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := dao.DB.Model(&entity.Tenant{}).Where("id = ?", tenantID).Count(&tenantCount).Error; err != nil {
		t.Fatal(err)
	}
	if tenantCount == 0 {
		if err := dao.DB.Create(&entity.Tenant{ID: tenantID, Status: sptr("1")}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := dao.DB.Model(&entity.UserTenant{}).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, string(entity.StatusValid)).
		Count(&count).Error; err != nil {
		t.Fatalf("count test tenant memberships: %v", err)
	}
	if count > 0 {
		return
	}
	if err := dao.DB.Create(&entity.UserTenant{
		ID:        userID + "-" + tenantID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      role,
		InvitedBy: tenantID,
		Status:    sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("insert test tenant membership: %v", err)
	}
}

func sptr(s string) *string { return &s }
