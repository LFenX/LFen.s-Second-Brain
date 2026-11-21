//go:build !enterprise

package permission

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"ragflow/internal/dao"
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
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}, &entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestKnowledgeFolderRestrictionsFollowRawDescendants(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy_%t", legacy), func(t *testing.T) {
			db := knowledgePolicyTestDB(t)
			valid := "1"
			folderID := "private-folder"
			for _, kb := range []entity.Knowledgebase{
				{ID: "private", TenantID: "owner", CreatedBy: "owner", Permission: "me", Status: &valid, KnowledgeEnabled: !legacy},
				{ID: "target", TenantID: "owner", CreatedBy: "owner", Permission: "team", Status: &valid},
				{ID: "ordinary", TenantID: "owner", CreatedBy: "owner", Permission: "team", Status: &valid},
			} {
				if kb.ID == "private" && !legacy {
					kb.KnowledgeFolderID = &folderID
				}
				if err := db.Create(&kb).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, membership := range []entity.UserTenant{{ID: "owner", TenantID: "owner", UserID: "owner", Role: "owner", Status: &valid}, {ID: "reader", TenantID: "owner", UserID: "reader", Role: "normal", Status: &valid}} {
				if err := db.Create(&membership).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range []entity.File{
				{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Type: "folder", Name: "/"},
				{ID: "common", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Type: "folder", Name: ".knowledgebase", SourceType: "knowledgebase"},
				{ID: folderID, ParentID: "common", TenantID: "owner", CreatedBy: "owner", Type: "folder", Name: "Private", SourceType: "knowledgebase"},
				{ID: "nested", ParentID: folderID, TenantID: "owner", CreatedBy: "owner", Type: "folder", Name: "nested"},
				{ID: "raw", ParentID: "nested", TenantID: "owner", CreatedBy: "owner", Type: "txt", Name: "private.txt"},
				{ID: "outside", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Type: "txt", Name: "ordinary.txt"},
			} {
				if err := db.Create(&f).Error; err != nil {
					t.Fatal(err)
				}
			}
			if legacy {
				if err := dao.ProtectUnclaimedKnowledgeFolders(t.Context(), db); err != nil {
					t.Fatal(err)
				}
			}
			if err := CheckKnowledgeFileTransfer(t.Context(), db, []string{"raw"}); !errors.Is(err, ErrKnowledgeIngestionDenied) {
				t.Fatalf("private descendant transfer: %v", err)
			}
			if err := CheckKnowledgeFileTransfer(t.Context(), db, []string{"outside"}); err != nil {
				t.Fatalf("ordinary transfer changed: %v", err)
			}
			for _, entry := range []struct{ doc, kb, file string }{{"copy", "target", "raw"}, {"plain", "ordinary", "outside"}} {
				doc := entity.Document{ID: entry.doc, KbID: entry.kb, ParserConfig: entity.JSONMap{}, Status: &valid}
				if err := db.Create(&doc).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&entity.File2Document{ID: entry.doc, FileID: &entry.file, DocumentID: &doc.ID}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Model(&entity.File{}).Where("id = ?", folderID).Update("name", ".knowledgebase").Error; err != nil {
				t.Fatal(err)
			}
			checker := NewDatabaseChecker(db)
			for _, user := range []string{"owner", "reader"} {
				for _, ref := range []ResourceRef{{Kind: ResourceKindFile, ID: "raw"}, {Kind: ResourceKindFolder, ID: "nested"}, {Kind: ResourceKindDataset, ID: "target"}} {
					err := checker.CheckResource(WithIdentity(t.Context(), user, ""), ref, OperationRead)
					if (err == nil) != (user == "owner") {
						t.Fatalf("user=%s ref=%+v err=%v", user, ref, err)
					}
				}
				var count int64
				if err := db.Model(&entity.File{}).Where("id = ? AND id IN (?)", "raw", KnowledgeHiddenFileIDs(t.Context(), db, user)).Count(&count).Error; err != nil || (count == 0) != (user == "owner") {
					t.Fatalf("browse user=%s count=%d err=%v", user, count, err)
				}
			}
			for _, purpose := range []string{"embedding", "rerank", "generation", "remote_parse", "wiki", "memory"} {
				if err := CheckDatasetModelUse(t.Context(), db, []string{"target"}, purpose); !errors.Is(err, ErrKnowledgeModelUseDenied) {
					t.Fatalf("purpose=%s err=%v", purpose, err)
				}
			}
			if err := CheckDatasetModelUse(t.Context(), db, []string{"ordinary"}, "embedding"); err != nil {
				t.Fatal(err)
			}
			if err := checker.CheckResource(WithIdentity(t.Context(), "reader", ""), ResourceRef{Kind: ResourceKindFile, ID: "outside"}, OperationRead); err != nil {
				t.Fatalf("ordinary team link changed: %v", err)
			}
		})
	}
}

func TestKnowledgeExistingCopiesKeepSourceRestrictions(t *testing.T) {
	db := knowledgePolicyTestDB(t)
	valid := "1"
	for _, kb := range []entity.Knowledgebase{
		{ID: "source-domain", TenantID: "owner", CreatedBy: "owner", Permission: "me", Status: &valid, KnowledgeEnabled: true},
		{ID: "target", TenantID: "team", CreatedBy: "team-owner", Permission: "team", Status: &valid},
		{ID: "ordinary", TenantID: "team", CreatedBy: "team-owner", Permission: "team", Status: &valid},
	} {
		if err := db.Create(&kb).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []entity.UserTenant{
		{ID: "source-owner", TenantID: "owner", UserID: "owner", Role: "owner", Status: &valid},
		{ID: "team-owner", TenantID: "team", UserID: "team-owner", Role: "owner", Status: &valid},
		{ID: "source-in-team", TenantID: "team", UserID: "owner", Role: "normal", Status: &valid},
		{ID: "reader", TenantID: "team", UserID: "reader", Role: "normal", Status: &valid},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	fid := "protected-file"
	if err := db.Create(&entity.File{ID: fid, TenantID: "owner", CreatedBy: "owner", Name: "private.txt", Type: "txt"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"source-domain", "target"} {
		doc := entity.Document{ID: id, KbID: id, Status: &valid, ParserConfig: entity.JSONMap{}}
		if err := db.Create(&doc).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&entity.File2Document{ID: id, FileID: &fid, DocumentID: &doc.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&entity.KnowledgeSourceRevision{ID: "snapshot", DomainID: "source-domain", DocumentID: "source-domain", FileID: fid, FolderID: "former-folder", InputHash: "synthetic"}).Error; err != nil {
		t.Fatal(err)
	}
	for phase := 0; phase < 3; phase++ {
		if phase == 1 {
			if err := db.Where("id = ?", "source-domain").Delete(&entity.File2Document{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Where("id = ?", "source-domain").Delete(&entity.Document{}).Error; err != nil {
				t.Fatal(err)
			}
		}
		if phase == 2 {
			if err := db.Where("id = ?", "source-domain").Delete(&entity.Knowledgebase{}).Error; err != nil {
				t.Fatal(err)
			}
		}
		checker := NewDatabaseChecker(db)
		for _, user := range []string{"owner", "reader", "team-owner"} {
			for _, ref := range []ResourceRef{{Kind: ResourceKindFile, ID: fid}, {Kind: ResourceKindDataset, ID: "target"}} {
				err := checker.CheckResource(WithIdentity(t.Context(), user, ""), ref, OperationRead)
				if (user == "owner") != (err == nil) {
					t.Fatalf("phase=%d user=%s ref=%+v err=%v", phase, user, ref, err)
				}
			}
			scope, err := checker.QueryScope(WithIdentity(t.Context(), user, ""), ScopeQuery{Kind: ResourceKindDataset, Operation: OperationRead})
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			if err := scope.Apply(db.Model(&entity.Knowledgebase{})).Pluck("id", &ids).Error; err != nil {
				t.Fatal(err)
			}
			found := false
			for _, id := range ids {
				if id == "target" {
					found = true
				}
			}
			if found != (user == "owner") {
				t.Fatalf("phase=%d user=%s list=%v", phase, user, ids)
			}
		}
		for _, purpose := range []string{"embedding", "rerank", "generation", "remote_parse", "wiki", "memory"} {
			if err := CheckDatasetModelUse(t.Context(), db, []string{"target"}, purpose); !errors.Is(err, ErrKnowledgeModelUseDenied) {
				t.Fatalf("phase=%d purpose=%s err=%v", phase, purpose, err)
			}
		}
		if err := CheckDatasetModelUse(t.Context(), db, []string{"ordinary"}, "embedding"); err != nil {
			t.Fatalf("ordinary source changed: %v", err)
		}
		if err := checker.CheckResource(WithIdentity(t.Context(), "reader", ""), ResourceRef{Kind: ResourceKindDataset, ID: "ordinary"}, OperationRead); err != nil {
			t.Fatalf("ordinary team read changed: %v", err)
		}
	}
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
		actorCtx := WithIdentity(ctx, user, TenantIDFromContext(ctx))
		ref := ResourceRef{Kind: ResourceKindDataset, ID: "private"}
		err := checker.CheckResource(actorCtx, ref, OperationRead)
		if (user == "member") != (err == nil) {
			t.Fatalf("read user=%s err=%v", user, err)
		}
		scope, err := checker.QueryScope(actorCtx, ScopeQuery{Kind: ResourceKindDataset, Operation: OperationRead})
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
