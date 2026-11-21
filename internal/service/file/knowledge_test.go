package file

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

func TestKnowledgeTenantBrowseHidesPrivateMetadataBeforePagination(t *testing.T) {
	seedFileBrowsing(t)
	db := dao.DB
	if err := db.AutoMigrate(&entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	valid := string(entity.StatusValid)
	folderID := "private-folder"
	kb := entity.Knowledgebase{ID: "synthetic-private", TenantID: "owner", CreatedBy: "owner", Name: "Secret project", Permission: "me", Status: &valid, KnowledgeEnabled: true, KnowledgeFolderID: &folderID}
	if err := db.Create(&kb).Error; err != nil {
		t.Fatal(err)
	}
	for _, file := range []entity.File{
		{ID: "kb-root", ParentID: "root", TenantID: "owner", CreatedBy: "reader", Name: ".knowledgebase", Type: "folder"},
		{ID: "private-folder", ParentID: "kb-root", TenantID: "owner", CreatedBy: "owner", Name: kb.Name, Type: "folder", SourceType: "knowledgebase"},
		{ID: "private-file", ParentID: "private-folder", TenantID: "owner", CreatedBy: "owner", Name: "secret.txt", Size: 99, Type: "txt"},
	} {
		if err := db.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	name := "secret.txt"
	doc := entity.Document{ID: "private-doc", KbID: kb.ID, Name: &name, ParserConfig: entity.JSONMap{}, Status: &valid}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}
	fid := "private-file"
	if err := db.Create(&entity.File2Document{ID: "private-link", FileID: &fid, DocumentID: &doc.ID}).Error; err != nil {
		t.Fatal(err)
	}
	svc := testFileService()
	list, err := svc.ListFilesForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", "kb-root", 1, 1, nil, "")
	if err != nil || list.Total != 0 || len(list.Files) != 0 {
		t.Fatalf("private folder leaked: %+v err=%v", list, err)
	}
	root, err := svc.ListFilesForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", "root", 1, 100, nil, "")
	if err != nil || root.Total != 2 {
		t.Fatalf("unregistered metadata changed: %+v err=%v", root, err)
	}
	for _, f := range root.Files {
		if f["id"] == "kb-root" && (f["size"] != int64(0) || f["has_child_folder"] != false) {
			t.Fatalf("private aggregates leaked: %+v", f)
		}
	}
	search, err := svc.ListFilesForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", "root", 1, 1, nil, "secret")
	if err != nil || search.Total != 0 || len(search.Files) != 0 {
		t.Fatalf("private recursive search leaked: %+v err=%v", search, err)
	}
	if _, err := svc.ListFilesForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", "private-folder", 1, 1, nil, ""); err == nil {
		t.Fatal("guessed private folder was readable")
	}
	if _, err := svc.GetAllParentFoldersForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", fid); err == nil {
		t.Fatal("private ancestor path leaked")
	}
	// Give the owner native access to the ancestor for the owner-side read.
	if err := db.Model(&entity.File{}).Where("id = ?", "kb-root").Update("created_by", "owner").Error; err != nil {
		t.Fatal(err)
	}
	owner, err := svc.ListFilesForTenant(permission.WithIdentity(t.Context(), "owner", "owner"), "owner", "private-folder", 1, 100, nil, "")
	if err != nil || len(owner.Files) != 1 {
		t.Fatalf("owner lost source: %+v err=%v", owner, err)
	}
}

func TestKnowledgeUnclaimedRenamedFolderRemainsPrivate(t *testing.T) {
	seedFileBrowsing(t)
	db := dao.DB
	if err := db.AutoMigrate(&entity.KnowledgeSourceRevision{}, &entity.User{}, &entity.Tenant{}, &entity.KnowledgeAccessModuleRecord{}, &entity.KnowledgeAccessTenantStateRecord{}, &entity.KnowledgeAccessDomainPolicyRecord{}, &entity.KnowledgeAccessGrantRecord{}, &entity.KnowledgeAccessAuditRecord{}, &entity.KnowledgeAccessViewRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: "old-domain", TenantID: "owner", CreatedBy: "owner", Name: "Current name", KnowledgeEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.File{ID: "old-empty-folder", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "Former private name", Type: "folder", SourceType: "knowledgebase"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := dao.NewFileDAO().EnsureKnowledgeFolder(t.Context(), db, "old-domain"); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"owner", "reader"} {
		view, err := knowledgeBrowseDB(t.Context(), user, "owner")
		if err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := view.Model(&entity.File{}).Where("id = ?", "old-empty-folder").Count(&count).Error; err != nil || (count == 1) != (user == "owner") {
			t.Fatalf("user=%s orphan count=%d err=%v", user, count, err)
		}
	}
	if err := db.Create(&entity.Knowledgebase{ID: "ordinary-domain", TenantID: "owner", Name: "Former private name"}).Error; err != nil {
		t.Fatal(err)
	}
	unclaimedView, err := knowledgeBrowseDB(t.Context(), "reader", "owner")
	if err != nil {
		t.Fatal(err)
	}
	var unclaimedCount int64
	if err := unclaimedView.Model(&entity.File{}).Where("id = ?", "old-empty-folder").Count(&unclaimedCount).Error; err != nil || unclaimedCount != 0 {
		t.Fatalf("never-snapshotted folder leaked through ordinary name: count=%d err=%v", unclaimedCount, err)
	}
	if err := db.Create(&entity.KnowledgeSourceRevision{ID: "archived", DomainID: "deleted-domain", FolderID: "old-empty-folder", DocumentID: "former-doc", InputHash: "synthetic"}).Error; err != nil {
		t.Fatal(err)
	}
	view, err := knowledgeBrowseDB(t.Context(), "reader", "owner")
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := view.Model(&entity.File{}).Where("id = ?", "old-empty-folder").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("archived folder leaked through a new ordinary name: count=%d err=%v", count, err)
	}
}

func TestKnowledgeReservedNameDoesNotUnhideUnclaimedNestedFolder(t *testing.T) {
	db := setupFolderTestDB(t)
	for _, file := range []entity.File{
		{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: "folder"},
		{ID: "common", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase"},
		{ID: "orphan", ParentID: "common", TenantID: "owner", CreatedBy: "owner", Name: "Former", Type: "folder", SourceType: "knowledgebase"},
		{ID: "raw", ParentID: "orphan", TenantID: "owner", CreatedBy: "owner", Name: "private.txt", Type: "txt"},
	} {
		if err := db.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Former", ".knowledgebase"} {
		if err := db.Model(&entity.File{}).Where("id = ?", "orphan").Update("name", name).Error; err != nil {
			t.Fatal(err)
		}
		view, err := knowledgeBrowseDB(t.Context(), "reader", "owner")
		if err != nil {
			t.Fatal(err)
		}
		var hidden, common int64
		if err := view.Model(&entity.File{}).Where("id IN ?", []string{"orphan", "raw"}).Count(&hidden).Error; err != nil || hidden != 0 {
			t.Fatalf("name=%s visible=%d err=%v", name, hidden, err)
		}
		view, err = knowledgeBrowseDB(t.Context(), "reader", "owner")
		if err != nil {
			t.Fatal(err)
		}
		if err := view.Model(&entity.File{}).Where("id = ?", "common").Count(&common).Error; err != nil || common != 1 {
			t.Fatalf("common root hidden=%d err=%v", common, err)
		}
	}
}

func TestKnowledgeNativeMoveAndDeletePreserveManagedTree(t *testing.T) {
	db := setupFolderTestDB(t)
	folderID := "managed"
	if err := db.Create(&entity.Knowledgebase{ID: "domain", TenantID: "owner", CreatedBy: "owner", Name: "Private", KnowledgeEnabled: true, KnowledgeFolderID: &folderID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, f := range []entity.File{
		{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: "folder"},
		{ID: "ancestor", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: ".knowledgebase", Type: "folder"},
		{ID: folderID, ParentID: "ancestor", TenantID: "owner", CreatedBy: "owner", Name: "Private", Type: "folder", SourceType: "knowledgebase"},
		{ID: "destination", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "Destination", Type: "folder"},
	} {
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := testFileService()
	for _, id := range []string{folderID, "ancestor"} {
		if err := svc.MoveFiles(permission.WithIdentity(t.Context(), "owner", "owner"), []string{id}, "destination", ""); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
			t.Fatalf("move %s: %v", id, err)
		}
		if err := svc.DeleteFiles(permission.WithIdentity(t.Context(), "owner", "owner"), []string{id}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
			t.Fatalf("delete %s: %v", id, err)
		}
	}
	var unchanged int64
	if err := db.Model(&entity.File{}).Where("id IN ?", []string{"root", "ancestor", folderID, "destination"}).Count(&unchanged).Error; err != nil || unchanged != 4 {
		t.Fatalf("tree mutated: count=%d err=%v", unchanged, err)
	}
}

func TestKnowledgeEmptyFolderBindingSurvivesRenames(t *testing.T) {
	seedFileBrowsing(t)
	db := dao.DB
	if err := db.AutoMigrate(&entity.KnowledgeSourceRevision{}); err != nil {
		t.Fatal(err)
	}
	folderID := "private-empty-folder"
	if err := db.Create(&entity.Knowledgebase{ID: "empty-domain", TenantID: "owner", CreatedBy: "owner", Name: "Current domain name", Permission: "me", KnowledgeEnabled: true, KnowledgeFolderID: &folderID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.File{ID: folderID, ParentID: "root", TenantID: "owner", Name: "Former private name", Type: "folder", SourceType: "knowledgebase"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := testFileService()
	list, err := svc.ListFilesForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", "root", 1, 100, nil, "")
	if err != nil || list.Total != 1 {
		t.Fatalf("renamed empty folder leaked: %+v err=%v", list, err)
	}
	if _, err := svc.GetAllParentFoldersForTenant(permission.WithIdentity(t.Context(), "reader", "owner"), "owner", folderID); err == nil {
		t.Fatal("renamed empty folder path leaked")
	}
}

func TestKnowledgeAndNativeCommitsShareFolderHead(t *testing.T) {
	db := setupFolderTestDB(t)
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatal(err)
	}
	for _, file := range []entity.File{{ID: "folder", ParentID: "folder", TenantID: "owner", Name: "Synthetic", Type: "folder"},
		{ID: "first", ParentID: "folder", TenantID: "owner", Name: "first.txt", Type: "txt"},
		{ID: "second", ParentID: "folder", TenantID: "owner", Name: "second.txt", Type: "txt"}} {
		if err := db.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	factory := storage.GetStorageFactory()
	old := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(old) })
	svc := NewFileCommitService()
	first, err := svc.CreateCommit(t.Context(), "folder", "owner", "Synthetic native", []entity.FileChange{{FileID: "first", FileName: "first.txt", Operation: "add", Content: "first"}})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UnixMilli() + 1000
	if err := db.Model(first).Update("create_time", future).Error; err != nil {
		t.Fatal(err)
	}
	var second *entity.FileCommit
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		second, _, err = svc.RecordSourceSnapshot(t.Context(), tx, &entity.File{ID: "second", ParentID: "folder", Name: "second.txt"}, "owner", []byte("second"))
		return err
	})
	if err != nil || second.ParentID == nil || *second.ParentID != first.ID || *second.CreateTime <= future {
		t.Fatalf("snapshot head=%+v err=%v", second, err)
	}
	third, err := svc.CreateCommit(t.Context(), "folder", "owner", "Synthetic append", []entity.FileChange{{FileID: "first", FileName: "first.txt", Operation: "modify", Content: "changed"}})
	if err != nil || third.ParentID == nil || *third.ParentID != second.ID {
		t.Fatalf("native head=%+v err=%v", third, err)
	}
	var tree map[string]any
	if err := json.Unmarshal([]byte(*third.TreeState), &tree); err != nil || len(tree) != 2 {
		t.Fatalf("lost previous tree entry: %v err=%v", tree, err)
	}
}

func TestKnowledgeCommitRejectsForeignIDsBeforeBatchWrites(t *testing.T) {
	db := setupFolderTestDB(t)
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []entity.File{
		{ID: "own", ParentID: "own", TenantID: "attacker", CreatedBy: "attacker", Name: "Own", Type: "folder"},
		{ID: "nested", ParentID: "own", TenantID: "attacker", CreatedBy: "attacker", Name: "Nested", Type: "folder"},
		{ID: "valid", ParentID: "nested", TenantID: "attacker", CreatedBy: "attacker", Name: "own.txt", Type: "txt"},
		{ID: "sibling", ParentID: "sibling", TenantID: "attacker", CreatedBy: "attacker", Name: "Sibling", Type: "folder"},
		{ID: "outside", ParentID: "sibling", TenantID: "attacker", CreatedBy: "attacker", Name: "outside.txt", Type: "txt"},
		{ID: "foreign", ParentID: "foreign", TenantID: "owner", CreatedBy: "owner", Name: "Private", Type: "folder"},
		{ID: "source", ParentID: "foreign", TenantID: "owner", CreatedBy: "owner", Name: "private.txt", Type: "txt"},
	} {
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	old := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(old) })
	svc := NewFileCommitService()
	for _, id := range []string{"source", "foreign", "outside"} {
		for _, operation := range []string{"add", "modify", "delete", "rename"} {
			changes := []entity.FileChange{{FileID: "valid", FileName: "own.txt", Operation: "modify", Content: "first must not write"},
				{FileID: id, FileName: "private.txt", Operation: operation, Content: "forged foreign data", NewName: "forged.txt"}}
			if _, err := svc.CreateCommit(t.Context(), "own", "attacker", "Synthetic denied batch", changes); !errors.Is(err, permission.ErrResourceNotFound) && !errors.Is(err, permission.ErrPermissionDenied) {
				t.Fatalf("id=%s op=%s err=%v", id, operation, err)
			}
		}
	}
	var count int64
	if err := db.Model(&entity.FileCommit{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("published rejected batch: %d err=%v", count, err)
	}
	var before entity.File
	if err := db.Where("id = ?", "valid").Take(&before).Error; err != nil || before.Size != 0 || before.Location != nil {
		t.Fatalf("earlier valid change leaked: %+v err=%v", before, err)
	}
	if exists, err := store.BucketExistsWithError(t.Context(), "own"); err != nil || exists {
		t.Fatalf("object written before complete authorization: exists=%v err=%v", exists, err)
	}
	if _, err := svc.CreateCommit(t.Context(), "own", "attacker", "Synthetic valid nested", []entity.FileChange{{FileID: "valid", FileName: "own.txt", Operation: "add", Content: "valid"}}); err != nil {
		t.Fatalf("valid nested file rejected: %v", err)
	}
}

func TestKnowledgeCommitCannotDeleteManagedSourceOrAncestor(t *testing.T) {
	db := setupFolderTestDB(t)
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatal(err)
	}
	fid := "domain-folder"
	if err := db.Create(&entity.Knowledgebase{ID: "domain", TenantID: "owner", CreatedBy: "owner", KnowledgeEnabled: true, KnowledgeFolderID: &fid}).Error; err != nil {
		t.Fatal(err)
	}
	for _, f := range []entity.File{
		{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: "folder"},
		{ID: "ancestor", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "Sources", Type: "folder"},
		{ID: fid, ParentID: "ancestor", TenantID: "owner", CreatedBy: "owner", Name: "Private", Type: "folder", SourceType: "knowledgebase"},
		{ID: "source", ParentID: fid, TenantID: "owner", CreatedBy: "owner", Name: "private.txt", Type: "txt"},
	} {
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&entity.KnowledgeSourceRevision{ID: "snapshot", DomainID: "domain", DocumentID: "doc", FileID: "source", FolderID: fid, InputHash: "synthetic"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ancestor", fid, "source"} {
		if _, err := NewFileCommitService().CreateCommit(t.Context(), "root", "owner", "Synthetic delete", []entity.FileChange{{FileID: id, Operation: "delete"}}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
			t.Fatalf("id=%s err=%v", id, err)
		}
	}
}
