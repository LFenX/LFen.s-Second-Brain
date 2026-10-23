//go:build !enterprise

package permission

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func knowledgePolicyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestKnowledgePolicyDeniesBeforeModelUseAndFailsClosed(t *testing.T) {
	db := knowledgePolicyTestDB(t)
	ctx := context.Background()
	if err := db.Create(&entity.Knowledgebase{ID: "private", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{"embedding", "rerank", "generation", "remote_parse", "wiki", "memory"} {
		if err := CheckDatasetModelUse(ctx, db, []string{"native", "private"}, purpose); !errors.Is(err, ErrKnowledgeModelUseDenied) {
			t.Fatalf("purpose %s: %v", purpose, err)
		}
	}
	if err := CheckKnowledgeIngestion(ctx, db, []string{"private"}); !errors.Is(err, ErrKnowledgeIngestionDenied) {
		t.Fatal(err)
	}
	if err := CheckDatasetModelUse(ctx, db, []string{"native"}, "embedding"); err != nil {
		t.Fatalf("unregistered dataset changed: %v", err)
	}
	if err := CheckDatasetModelUse(ctx, nil, []string{"private"}, "embedding"); err == nil {
		t.Fatal("nil DB failed open")
	}
	if err := CheckDatasetModelUse(ctx, db, []string{"private"}, "unknown"); err == nil {
		t.Fatal("unknown model purpose accepted")
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if err := CheckDatasetModelUse(ctx, db, []string{"private"}, "generation"); err == nil {
		t.Fatal("DB failure failed open")
	}
}

func TestKnowledgePrivateDomainResourceAndListAgree(t *testing.T) {
	db := knowledgePolicyTestDB(t)
	ctx := context.Background()
	valid := "1"
	for _, row := range []entity.UserTenant{{ID: "owner", UserID: "tenant-owner", TenantID: "tenant", Role: "owner", Status: &valid}, {ID: "member", UserID: "member", TenantID: "tenant", Role: "normal", Status: &valid}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, kb := range []entity.Knowledgebase{{ID: "private", TenantID: "tenant", CreatedBy: "member", Permission: "team", Status: &valid, KnowledgeEnabled: true}, {ID: "native", TenantID: "tenant", CreatedBy: "tenant-owner", Permission: "team", Status: &valid}} {
		if err := db.Create(&kb).Error; err != nil {
			t.Fatal(err)
		}
	}
	checker := NewDatabaseChecker(db)
	for _, user := range []string{"member", "tenant-owner", "outsider"} {
		subject := Subject{UserID: user}
		ref := ResourceRef{Kind: ResourceKindDataset, ID: "private"}
		err := checker.CheckResource(ctx, subject, ref, OperationRead)
		if (user == "member") != (err == nil) {
			t.Fatalf("read user=%s err=%v", user, err)
		}
		scope, err := checker.QueryScope(ctx, subject, ScopeQuery{Kind: ResourceKindDataset, Operation: OperationRead})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		if err := scope.Apply(db.Model(&entity.Knowledgebase{})).Pluck("id", &ids).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, id := range ids {
			if id == "private" {
				found = true
			}
		}
		if found != (user == "member") {
			t.Fatalf("list user=%s ids=%v", user, ids)
		}
	}
}
