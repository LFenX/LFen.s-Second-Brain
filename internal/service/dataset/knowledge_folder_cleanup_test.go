package dataset

import (
	"errors"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/service/file"
	"testing"
)

func TestKnowledgeNativeDatasetCleanupAncestor(t *testing.T) {
	for _, mode := range []string{"registered_child", "ordinary_control"} {
		t.Run(mode, func(t *testing.T) {
			svc, store := setupKnowledgeTest(t)
			if mode == "registered_child" {
				registerKnowledgeTest(t, svc, knowledgeTestDomain)
			}
			var kb entity.Knowledgebase
			if err := dao.DB.Where("id = ?", knowledgeTestOther).Take(&kb).Error; err != nil {
				t.Fatal(err)
			}
			location := "ordinary.txt"
			if err := store.Put(t.Context(), kb.ID, location, []byte("ordinary control bytes")); err != nil {
				t.Fatal(err)
			}
			ancestor := "ordinary-ancestor"
			file, doc := "ordinary-file", "ordinary-doc"
			for _, row := range []any{&entity.File{ID: ancestor, ParentID: "parent", TenantID: kb.TenantID, CreatedBy: kb.CreatedBy, Name: kb.Name, Type: "folder", SourceType: "knowledgebase"}, &entity.File{ID: file, ParentID: ancestor, TenantID: kb.TenantID, CreatedBy: kb.CreatedBy, Name: "ordinary.txt", Type: "txt", SourceType: "knowledgebase"}, &entity.Document{ID: doc, KbID: kb.ID, Location: &location, ParserConfig: entity.JSONMap{}}, &entity.File2Document{ID: "ordinary-link", FileID: &file, DocumentID: &doc}} {
				if err := dao.DB.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			bound := ""
			if mode == "registered_child" {
				var source entity.Knowledgebase
				if err := dao.DB.Where("id = ?", knowledgeTestDomain).Take(&source).Error; err != nil {
					t.Fatal(err)
				}
				bound = *source.KnowledgeFolderID
				if err := dao.DB.Model(&entity.File{}).Where("id = ?", bound).Update("parent_id", ancestor).Error; err != nil {
					t.Fatal(err)
				}
				guard := permission.CheckKnowledgeFileTreeMutation(t.Context(), dao.DB, &entity.File{ID: ancestor, TenantID: kb.TenantID, Type: "folder"})
				if !errors.Is(guard, permission.ErrKnowledgeIngestionDenied) {
					t.Fatalf("ancestor fixture not protected: %v", guard)
				}
			}
			got, code, err := svc.DeleteDatasets(t.Context(), []string{kb.ID}, false, permission.Subject{UserID: knowledgeTestOwner})
			var n int64
			if e := dao.DB.Model(&entity.File{}).Where("id = ?", ancestor).Count(&n).Error; e != nil {
				t.Fatal(e)
			}
			t.Logf("mode=%s result=%v code=%d err=%v protected_ancestor_rows=%d", mode, got, code, err, n)
			if mode == "registered_child" {
				if n != 1 || code != common.CodeForbidden || err == nil {
					t.Fatalf("protected deletion was not denied: rows=%d code=%v err=%v", n, code, err)
				}
				if ok, e := store.ObjectExists(t.Context(), kb.ID, location); e != nil || !ok {
					t.Fatalf("denial removed storage before validation: exists=%t err=%v", ok, e)
				}
				var kept int64
				if e := dao.DB.Model(&entity.Document{}).Where("id = ?", doc).Count(&kept).Error; e != nil || kept != 1 {
					t.Fatalf("denial changed ordinary document: %d %v", kept, e)
				}
			}
			if mode == "ordinary_control" && (err != nil || n != 0) {
				t.Error("ordinary dataset cleanup control did not execute")
			}
		})
	}
}

func TestKnowledgeRegistrationCommonContainerDedup(t *testing.T) {
	svc, _ := setupKnowledgeTest(t)
	one, two := int64(1), int64(2)
	protected := "private-folder"
	valid := "1"
	for _, row := range []any{
		&entity.File{ID: "root", ParentID: "root", TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "/", Type: "folder"},
		&entity.File{ID: "common-first", ParentID: "root", TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase", BaseModel: entity.BaseModel{CreateTime: &one}},
		&entity.File{ID: "common-second", ParentID: "root", TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase", BaseModel: entity.BaseModel{CreateTime: &two}},
		&entity.File{ID: protected, ParentID: "common-second", TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "Prior private", Type: "folder", SourceType: "knowledgebase"},
		&entity.File{ID: "raw", ParentID: protected, TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "raw.txt", Type: "txt"},
		&entity.Knowledgebase{ID: "prior-domain", TenantID: knowledgeTestOwner, CreatedBy: knowledgeTestOwner, Name: "Prior private", Permission: "me", Status: &valid, KnowledgeEnabled: true, KnowledgeFolderID: &protected},
	} {
		if err := dao.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := permission.CheckKnowledgeFileTreeMutation(t.Context(), dao.DB, &entity.File{ID: "common-second", TenantID: knowledgeTestOwner, Type: "folder"}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("fixture guard: %v", err)
	}
	got, err := svc.RegisterKnowledgeDomain(t.Context(), knowledgeTestOwner, knowledgeTestOther, entity.RegisterKnowledgeDomainRequest{Category: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	var f entity.File
	if err := dao.DB.Where("id = ?", protected).Take(&f).Error; err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := dao.DB.Model(&entity.File{}).Where("id = ?", "common-second").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("registration succeeded=%t protected_source_parent=%s protected_ancestor_rows=%d", got != nil, f.ParentID, n)
	if f.ParentID != "common-second" || n != 1 {
		t.Error("registering another domain reparented a known source and deleted its protected ancestor")
	}
}

func TestKnowledgeOrdinaryDatasetReservedNameDeletesCommonAncestor(t *testing.T) {
	svc, store := setupKnowledgeTest(t)
	registerKnowledgeTest(t, svc, knowledgeTestDomain)
	doc := "22222222222222222222222222222222"
	uploadKnowledgeTest(t, store, doc, "synthetic.txt", []byte("protected source\n"))
	snapshot, err := svc.SnapshotKnowledgeSource(t.Context(), knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: doc})
	if err != nil {
		t.Fatal(err)
	}
	var bound entity.File
	if err := dao.DB.Where("id = ?", snapshot.Source.FolderID).Take(&bound).Error; err != nil {
		t.Fatal(err)
	}
	commonID := bound.ParentID
	var parent entity.File
	if err := dao.DB.Where("id = ?", commonID).Take(&parent).Error; err != nil || parent.Name != ".knowledgebase" {
		t.Fatalf("common container: %+v %v", parent, err)
	}
	if err := permission.CheckKnowledgeFileTreeMutation(t.Context(), dao.DB, &parent); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
		t.Fatalf("common container not protected: %v", err)
	}
	name := ".knowledgebase"
	_, code, err := svc.UpdateDataset(t.Context(), knowledgeTestOther, knowledgeTestOwner, service.UpdateDatasetRequest{Name: &name})
	if err != nil || code != 0 {
		t.Fatalf("ordinary rename: code=%d err=%v", code, err)
	}
	before, err := file.NewFileService(nil).ListFiles(t.Context(), knowledgeTestOwner, bound.ID, 1, 20, nil, "")
	if err != nil || before == nil {
		t.Fatalf("before native list: %v", err)
	}
	got, code, err := svc.DeleteDatasets(t.Context(), []string{knowledgeTestOther}, false, permission.Subject{UserID: knowledgeTestOwner})
	var n int64
	if e := dao.DB.Model(&entity.File{}).Where("id = ?", commonID).Count(&n).Error; e != nil {
		t.Fatal(e)
	}
	_, browseErr := file.NewFileService(nil).ListFiles(t.Context(), knowledgeTestOwner, bound.ID, 1, 20, nil, "")
	after, readErr := svc.ReadKnowledgeNode(t.Context(), knowledgeTestOwner, knowledgeTestDomain, snapshot.Root.ID, nil, nil)
	t.Logf("ordinary reserved-name delete result=%v code=%d err=%v common_ancestor_rows=%d native_source_browse_error=%v C1_fixed_revision_read_ok=%t", got, code, err, n, browseErr, readErr == nil && after.Text == "protected source\n")
	if n != 1 || browseErr != nil || err == nil || code != common.CodeForbidden || readErr != nil || after.Text != "protected source\n" {
		t.Error("ordinary reserved dataset name deleted true public ancestor and broke native protected-source browsing")
	}
}
