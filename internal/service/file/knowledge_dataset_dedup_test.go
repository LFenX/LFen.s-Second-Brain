package file

import (
	"errors"
	"go.uber.org/zap"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"testing"
)

func TestKnowledgeDatasetContainerDedupAncestor(t *testing.T) {
	old := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = old })
	for _, mode := range []string{"registered", "orphan", "ordinary_control"} {
		t.Run(mode, func(t *testing.T) {
			db := setupFolderTestDB(t)
			valid := "1"
			one, two := int64(1), int64(2)
			protected := "private-folder"
			rows := []any{
				&entity.UserTenant{ID: "owner-member", UserID: "owner", TenantID: "owner", Role: "owner", Status: &valid},
				&entity.UserTenant{ID: "reader-member", UserID: "reader", TenantID: "owner", Role: "normal", Status: &valid},
				&entity.File{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: "folder"},
				&entity.File{ID: "common-first", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: DatasetFolderName, Type: "folder", SourceType: "knowledgebase", BaseModel: entity.BaseModel{CreateTime: &one}},
				&entity.File{ID: "common-second", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: DatasetFolderName, Type: "folder", SourceType: "knowledgebase", BaseModel: entity.BaseModel{CreateTime: &two}},
				&entity.File{ID: protected, ParentID: "common-second", TenantID: "owner", CreatedBy: "owner", Name: "Private", Type: "folder", SourceType: "knowledgebase"},
				&entity.File{ID: "raw", ParentID: protected, TenantID: "owner", CreatedBy: "owner", Name: "sentinel.txt", Type: "txt"},
			}
			if mode == "registered" {
				rows = append(rows, &entity.Knowledgebase{ID: "private-domain", TenantID: "owner", CreatedBy: "owner", Name: "Private", Permission: "me", Status: &valid, KnowledgeEnabled: true, KnowledgeFolderID: &protected})
			}
			if mode == "ordinary_control" {
				doc, fid := "ordinary-doc", "raw"
				rows = append(rows, &entity.Knowledgebase{ID: "ordinary-domain", TenantID: "owner", CreatedBy: "owner", Name: "Private", Permission: "team", Status: &valid}, &entity.Document{ID: doc, KbID: "ordinary-domain", ParserConfig: entity.JSONMap{}, Status: &valid}, &entity.File2Document{ID: "ordinary-link", DocumentID: &doc, FileID: &fid})
			}
			for _, row := range rows {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			guard := permission.CheckKnowledgeFileTreeMutation(t.Context(), db, &entity.File{ID: "common-second", TenantID: "owner", Type: "folder"})
			if mode != "ordinary_control" && !errors.Is(guard, permission.ErrKnowledgeIngestionDenied) {
				t.Fatalf("fixture ancestor not protected: %v", guard)
			}
			if mode != "ordinary_control" {
				if err := permission.NewDatabaseChecker(db).CheckResource(permission.WithIdentity(t.Context(), "reader", ""), permission.ResourceRef{Kind: permission.ResourceKindFolder, ID: protected}, permission.OperationRead); err == nil {
					t.Fatal("fixture reader already authorized")
				}
			}
			result, err := testFileService().ListFilesForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", "", 1, 20, nil, "")
			if err != nil {
				t.Fatalf("listing failed: %v", err)
			}
			var before entity.File
			if err := db.Where("id = ?", protected).Take(&before).Error; err != nil || before.ParentID != "common-second" {
				t.Fatalf("member listing mutated ancestry: %+v %v", before, err)
			}
			result, err = testFileService().ListFilesForTenant(permission.WithIdentity(t.Context(), "owner", "owner"), "owner", "", 1, 20, nil, "")
			if err != nil {
				t.Fatalf("owner listing failed: %v", err)
			}
			var f entity.File
			if err := db.Where("id = ?", protected).Take(&f).Error; err != nil {
				t.Fatal(err)
			}
			var n int64
			if err := db.Model(&entity.File{}).Where("id = ?", "common-second").Count(&n).Error; err != nil {
				t.Fatal(err)
			}
			t.Logf("mode=%s member_listing_unchanged=true explicit_ancestor_guard=%v owner_listing_total=%d source_parent=%s old_ancestor_rows=%d", mode, guard, result.Total, f.ParentID, n)
			if mode != "ordinary_control" && (f.ParentID != "common-second" || n != 1) {
				t.Error("self-owned native browse reparented protected source and deleted its protected ancestor")
			}
			if mode == "ordinary_control" && (f.ParentID != "common-first" || n != 0) {
				t.Error("ordinary deduplication control did not execute")
			}
		})
	}
}
