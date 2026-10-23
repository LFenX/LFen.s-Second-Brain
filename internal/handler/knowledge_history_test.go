package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	filesvc "ragflow/internal/service/file"
	"ragflow/internal/storage"
)

func TestKnowledgeAncestorHistoryRequiresSourceIntersection(t *testing.T) {
	_, _, _ = setupFileCommitTestWithHandler("copy-owner")
	if err := dao.DB.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatal(err)
	}
	valid := "1"
	for _, row := range []any{
		&entity.UserTenant{ID: "source-owner", TenantID: "source-owner", UserID: "source-owner", Role: "owner", Status: &valid},
		&entity.Knowledgebase{ID: "private-domain", TenantID: "source-owner", CreatedBy: "source-owner", Name: "Private", Permission: "me", KnowledgeEnabled: true, Status: &valid},
		&entity.File{ID: "nested", ParentID: "folder-1", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "Nested", Type: "folder"},
		&entity.File{ID: "protected-file", ParentID: "nested", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "private-source.txt", Type: "txt"},
		&entity.File{ID: "ordinary-file", ParentID: "folder-1", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "ordinary.txt", Type: "txt"},
		&entity.File{ID: "ordinary-history-root", ParentID: "ordinary-history-root", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "Ordinary", Type: "folder"},
		&entity.File{ID: "ordinary-deleted-file", ParentID: "ordinary-history-root", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "deleted.txt", Type: "txt"},
		&entity.Document{ID: "private-document", KbID: "private-domain", CreatedBy: "source-owner", ParserConfig: entity.JSONMap{}, Status: &valid},
	} {
		if err := dao.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	fid, did := "protected-file", "private-document"
	factory := storage.GetStorageFactory()
	old := factory.GetStorage()
	store := storage.NewMemoryStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(old) })
	body := []byte("synthetic private revision")
	commitService := filesvc.NewFileCommitService()
	// The real native ancestor commit succeeds before this old file is linked
	// to a private domain. This is an existing version, not a fabricated blob.
	commit, err := commitService.CreateCommit(t.Context(), "folder-1", "copy-owner", "Existing ancestor revision", []entity.FileChange{{FileID: fid, FileName: "private-source.txt", Operation: "add", Content: string(body)}, {FileID: "ordinary-file", FileName: "ordinary.txt", Operation: "add", Content: "ordinary historical bytes"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := commitService.CreateCommit(t.Context(), "folder-1", "copy-owner", "Existing ancestor change", []entity.FileChange{{FileID: fid, FileName: "private-source.txt", Operation: "modify", Content: "second synthetic private revision"}})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryCommit, err := commitService.CreateCommit(t.Context(), "ordinary-history-root", "copy-owner", "Ordinary history", []entity.FileChange{{FileID: "ordinary-deleted-file", FileName: "deleted.txt", Operation: "add", Content: "ordinary historical bytes"}})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the explicitly supported old File2Document relationship that
	// predates the current transfer rejection. The IDs and versions persist.
	if err := dao.DB.Create(&entity.File2Document{ID: "legacy-link", FileID: &fid, DocumentID: &did}).Error; err != nil {
		t.Fatal(err)
	}
	checker := permission.NewDatabaseChecker(dao.DB)
	for _, ref := range []permission.ResourceRef{{Kind: permission.ResourceKindFile, ID: fid}, {Kind: permission.ResourceKindFolder, ID: "nested"}} {
		if err := checker.CheckResource(t.Context(), permission.Subject{UserID: "copy-owner"}, ref, permission.OperationRead); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatalf("exact source guard: %v", err)
		}
	}
	if err := checker.CheckResource(t.Context(), permission.Subject{UserID: "copy-owner"}, permission.ResourceRef{Kind: permission.ResourceKindFolder, ID: "folder-1"}, permission.OperationRead); err != nil {
		t.Fatal(err)
	}
	r := fileCommitRouter(NewFileCommitHandler(commitService), "copy-owner")
	assertHidden := func(w *httptest.ResponseRecorder) {
		t.Helper()
		var response struct{ Code int }
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Code != 404 {
			t.Fatalf("native version did not hide restricted ancestor: HTTP=%d body=%s err=%v", w.Code, w.Body.String(), err)
		}
	}
	if err := dao.DB.Where("id = ?", "ordinary-deleted-file").Delete(&entity.File{}).Error; err != nil {
		t.Fatal(err)
	}
	positive := httptest.NewRecorder()
	r.ServeHTTP(positive, httptest.NewRequest(http.MethodGet, "/api/v1/folders/ordinary-history-root/commits/"+ordinaryCommit.ID+"/files/ordinary-deleted-file/content", nil))
	if positive.Code != 200 || !strings.Contains(positive.Body.String(), "ordinary historical bytes") {
		t.Fatalf("ordinary deleted-file history regressed: %d %s", positive.Code, positive.Body.String())
	}
	t.Logf("ordinary current File removed; historical payload remains readable: %d", positive.Code)
	leaked := []string{}
	for _, suffix := range []string{"/tree", "/files/" + fid + "/content", "/files", ""} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/folders/folder-1/commits/"+commit.ID+suffix, nil))
		assertHidden(w)
		if strings.Contains(w.Body.String(), "private-source.txt") || strings.Contains(w.Body.String(), string(body)) || strings.Contains(w.Body.String(), fid) {
			leaked = append(leaked, suffix)
			t.Logf("guarded exact read denied, ancestor history %s returned %d %s", suffix, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/folders/folder-1/commits/diff?from="+commit.ID+"&to="+second.ID, nil))
	assertHidden(w)
	if strings.Contains(w.Body.String(), fid) {
		leaked = append(leaked, "diff")
		t.Logf("ancestor diff returned %d %s", w.Code, w.Body.String())
	}
	if err := dao.DB.Create(&entity.File{ID: "private-raw-new", ParentID: "nested", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "new-private-raw.txt", Type: "txt"}).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/folders/folder-1/changes", nil))
	assertHidden(w)
	if strings.Contains(w.Body.String(), "new-private-raw.txt") {
		leaked = append(leaked, "changes")
		t.Logf("ancestor changes returned %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/folders/folder-1/commits", nil))
	assertHidden(w)
	if strings.Contains(w.Body.String(), `"file_count":2`) {
		leaked = append(leaked, "list total/FileCount")
		t.Logf("ancestor list includes counts of protected file: %d %s", w.Code, w.Body.String())
	}
	postBody := `{"message":"ordinary-only edit","files":[{"file_id":"ordinary-file","file_name":"ordinary.txt","operation":"modify","content":"ordinary edit"}]}`
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/workspace-files/ordinary-file/versions", nil))
	assertHidden(w)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/folders/folder-1/commits", strings.NewReader(postBody)))
	assertHidden(w)
	if strings.Contains(w.Body.String(), fid) {
		leaked = append(leaked, "CreateCommit inherited TreeState")
		t.Logf("ordinary-only write returned protected inherited tree: %d %s", w.Code, w.Body.String())
	}
	if _, err := commitService.CreateCommit(t.Context(), "folder-1", "copy-owner", "Denied inherited tree", []entity.FileChange{{FileID: "ordinary-file", FileName: "ordinary.txt", Operation: "modify", Content: "must not store"}}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("service inherited-tree guard: %v", err)
	}
	digest := sha256.Sum256([]byte("must not store"))
	exists, err := store.ObjectExists(t.Context(), "folder-1", ".objects/"+hex.EncodeToString(digest[:]))
	if err != nil || exists {
		t.Fatalf("denied service wrote object: %v %v", exists, err)
	}
	privateOwner := "source-owner"
	for _, row := range []entity.File{{ID: "no-history-root", ParentID: "", TenantID: "copy-owner", CreatedBy: "copy-owner", Name: "No history", Type: "folder"}, {ID: "no-history-private-folder", ParentID: "no-history-root", TenantID: "copy-owner", CreatedBy: privateOwner, KnowledgePrivateOwnerID: &privateOwner, Name: "Private marked folder", Type: "folder", SourceType: "knowledgebase"}, {ID: "no-history-private-file", ParentID: "no-history-private-folder", TenantID: "copy-owner", CreatedBy: privateOwner, Name: "private-without-commit.txt", Type: "txt"}} {
		if err := dao.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/folders/no-history-root/changes", nil))
	assertHidden(w)
	if strings.Contains(w.Body.String(), "private-without-commit.txt") {
		leaked = append(leaked, "changes without history")
		t.Logf("no-history ancestor changes returned %d %s", w.Code, w.Body.String())
	}
	// Historical parent IDs remain authoritative after the current File is gone
	// and the private directory no longer lies in the URL subtree.
	if err := dao.DB.Where("id = ?", "legacy-link").Delete(&entity.File2Document{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Where("id = ?", fid).Delete(&entity.File{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.File{}).Where("id = ?", "nested").Update("parent_id", "outside-current-tree").Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "private-domain").Update("knowledge_folder_id", "nested").Error; err != nil {
		t.Fatal(err)
	}
	if err := permission.CheckKnowledgeVersionAccess(t.Context(), dao.DB, "copy-owner", "folder-1"); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("lost historical parent restriction: %v", err)
	}
	if err := permission.CheckKnowledgeVersionAccess(t.Context(), dao.DB, "source-owner", "folder-1"); err != nil {
		t.Fatalf("source owner constraint became a denial: %v", err)
	}
	// Commit items also protect historical sources absent from TreeState/File.
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "private-domain").Update("knowledge_folder_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Create(&entity.File2Document{ID: "restored-legacy-link", FileID: &fid, DocumentID: &did}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Model(&entity.FileCommit{}).Where("folder_id = ?", "folder-1").Update("tree_state", "{}").Error; err != nil {
		t.Fatal(err)
	}
	if err := permission.CheckKnowledgeVersionAccess(t.Context(), dao.DB, "copy-owner", "folder-1"); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("lost historical item restriction: %v", err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", "private-domain").Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := permission.CheckKnowledgeVersionAccess(t.Context(), dao.DB, "copy-owner", "folder-1"); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("archiving reopened old versions: %v", err)
	}
	if len(leaked) > 0 {
		t.Fatalf("private source data escaped through ancestor commit surfaces: %v", leaked)
	}
}
