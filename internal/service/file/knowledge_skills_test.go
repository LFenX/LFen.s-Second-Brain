package file

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"go.uber.org/zap"
)

func TestKnowledgeSkillsInitializationPreservesProtectedTrees(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	for _, mode := range []string{"ordinary_first", "managed_first", "only_managed", "orphan", "ancestor"} {
		t.Run(mode, func(t *testing.T) {
			db := setupFolderTestDB(t)
			one, two := int64(1), int64(2)
			ordinaryTime, managedTime := &one, &two
			if mode == "managed_first" {
				ordinaryTime, managedTime = &two, &one
			}
			folderID, parentID := "managed-skills-source", "root"
			rows := []any{
				&entity.File{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: FileTypeFolder},
			}
			if mode != "only_managed" {
				rows = append(rows, &entity.File{ID: "ordinary-skills", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: SkillsFolderName, Type: FileTypeFolder, BaseModel: entity.BaseModel{CreateTime: ordinaryTime}})
			}
			name := SkillsFolderName
			if mode == "ancestor" {
				parentID, name = "ordinary-ancestor-skills", "private-source"
				rows = append(rows, &entity.File{ID: parentID, ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: SkillsFolderName, Type: FileTypeFolder, BaseModel: entity.BaseModel{CreateTime: managedTime}})
			}
			if mode != "orphan" {
				rows = append(rows, &entity.Knowledgebase{ID: "registered-skills-source", TenantID: "owner", CreatedBy: "owner", Name: "Private domain", KnowledgeEnabled: true, KnowledgeFolderID: &folderID})
			}
			rows = append(rows,
				&entity.File{ID: folderID, ParentID: parentID, TenantID: "owner", CreatedBy: "owner", Name: name, Type: FileTypeFolder, SourceType: "knowledgebase", BaseModel: entity.BaseModel{CreateTime: managedTime}},
				&entity.File{ID: "protected-raw", ParentID: folderID, TenantID: "owner", CreatedBy: "owner", Name: "private-raw.txt", Type: "txt"})
			for _, row := range rows {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			assertProtected := func() {
				t.Helper()
				var folder, raw entity.File
				if err := db.Where("id = ?", folderID).Take(&folder).Error; err != nil || folder.ParentID != parentID {
					t.Fatalf("protected directory changed: %+v err=%v", folder, err)
				}
				if err := db.Where("id = ?", "protected-raw").Take(&raw).Error; err != nil || raw.ParentID != folderID {
					t.Fatalf("raw source reparented: %+v err=%v", raw, err)
				}
				if err := permission.CheckKnowledgeFileTransfer(t.Context(), db, []string{raw.ID}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
					t.Fatalf("raw restriction lost: %v", err)
				}
				if mode != "orphan" {
					var kb entity.Knowledgebase
					if err := db.Where("id = ?", "registered-skills-source").Take(&kb).Error; err != nil || kb.KnowledgeFolderID == nil || *kb.KnowledgeFolderID != folderID {
						t.Fatalf("domain binding changed: %+v err=%v", kb, err)
					}
				}
			}
			assertProtected()
			for range 2 {
				if err := testFileService().initSkillsFolder(t.Context(), "root", "owner"); err != nil {
					t.Fatal(err)
				}
				assertProtected()
			}
			var ordinary int64
			if err := db.Model(&entity.File{}).Where("parent_id = ? AND name = ? AND type = ? AND source_type = ?", "root", SkillsFolderName, FileTypeFolder, "").Count(&ordinary).Error; err != nil || ordinary == 0 {
				t.Fatalf("no ordinary system folder remains: count=%d err=%v", ordinary, err)
			}
		})
	}
}

func TestKnowledgeSkillsInitializationRetainsOrdinaryDeduplication(t *testing.T) {
	previous := common.Logger
	common.Logger = zap.NewNop()
	t.Cleanup(func() { common.Logger = previous })
	db := setupFolderTestDB(t)
	one, two := int64(1), int64(2)
	for _, row := range []entity.File{
		{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: FileTypeFolder},
		{ID: "first", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: SkillsFolderName, Type: FileTypeFolder, BaseModel: entity.BaseModel{CreateTime: &one}},
		{ID: "second", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: SkillsFolderName, Type: FileTypeFolder, BaseModel: entity.BaseModel{CreateTime: &two}},
		{ID: "ordinary-child", ParentID: "second", TenantID: "owner", CreatedBy: "owner", Name: "ordinary.txt", Type: "txt"},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testFileService().initSkillsFolder(t.Context(), "root", "owner"); err != nil {
		t.Fatal(err)
	}
	var child entity.File
	if err := db.Where("id = ?", "ordinary-child").Take(&child).Error; err != nil || child.ParentID != "first" {
		t.Fatalf("ordinary deduplication changed: %+v err=%v", child, err)
	}
	var remaining int64
	if err := db.Model(&entity.File{}).Where("id = ?", "second").Count(&remaining).Error; err != nil || remaining != 0 {
		t.Fatalf("ordinary duplicate retained: %d err=%v", remaining, err)
	}
}
