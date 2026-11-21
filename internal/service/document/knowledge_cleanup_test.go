package document

import (
	"errors"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// This synthetic test uses the production deletion service and an in-memory DB.
func TestKnowledgeDocumentCleanupPreservesExplicitOrigins(t *testing.T) {
	for _, tc := range []struct {
		name, sourceType string
		bound, history   bool
	}{
		{"legacy_external_local", "local", false, false},
		{"legacy_external_upload", "knowledgebase", false, false},
		{"bound_local_control", "local", true, false},
		{"historical_external_control", "local", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			valid, primary, fid := "1", "primary-private", "legacy-file"
			parent := "ordinary-parent"
			if tc.bound {
				parent = primary
			}
			for _, row := range []any{
				&entity.UserTenant{ID: "owner-member", UserID: "user-1", TenantID: "tenant-1", Role: "owner", Status: &valid},
				&entity.UserTenant{ID: "reader-member", UserID: "reader", TenantID: "tenant-1", Role: "normal", Status: &valid},
				&entity.Knowledgebase{ID: "registered", TenantID: "tenant-1", CreatedBy: "user-1", Name: "Private", Permission: "me", Status: &valid, KnowledgeEnabled: true, KnowledgePolicyVersion: 1, KnowledgeFolderID: &primary, DocNum: 1},
				&entity.Knowledgebase{ID: "target", TenantID: "tenant-1", CreatedBy: "user-1", Name: "Shared", Permission: "team", Status: &valid},
				&entity.File{ID: "root", ParentID: "root", TenantID: "tenant-1", CreatedBy: "user-1", Name: "/", Type: "folder"},
				&entity.File{ID: primary, ParentID: "root", TenantID: "tenant-1", CreatedBy: "user-1", Name: "Private", Type: "folder", SourceType: "knowledgebase"},
				&entity.File{ID: "ordinary-parent", ParentID: "root", TenantID: "tenant-1", CreatedBy: "user-1", Name: "Ordinary", Type: "folder", SourceType: "local"},
				&entity.File{ID: fid, ParentID: parent, TenantID: "tenant-1", CreatedBy: "user-1", Name: "private.txt", Type: "txt", SourceType: tc.sourceType},
				&entity.Document{ID: "source-doc", KbID: "registered", CreatedBy: "user-1", Name: sptr("private.txt"), Status: &valid, ParserConfig: entity.JSONMap{}},
				&entity.Document{ID: "copy-doc", KbID: "target", CreatedBy: "user-1", Name: sptr("private-copy.txt"), Status: &valid, ParserConfig: entity.JSONMap{}},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			insertTestFile2Document(t, "source-map", fid, "source-doc")
			insertTestFile2Document(t, "copy-map", fid, "copy-doc")
			if tc.history {
				if err := db.Create(&entity.KnowledgeSourceRevision{ID: "old-revision", DomainID: "registered", DocumentID: "source-doc", FileID: fid, FolderID: parent, InputHash: "old-input"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			refs := []permission.ResourceRef{{Kind: permission.ResourceKindFile, ID: fid}, {Kind: permission.ResourceKindDataset, ID: "target"}}
			for _, ref := range refs {
				if err := permission.NewDatabaseChecker(db).CheckResource(permission.WithIdentity(t.Context(), "reader", ""), ref, permission.OperationRead); err == nil {
					t.Fatalf("fixture was public before deletion: %+v", ref)
				}
			}
			if err := permission.CheckDatasetModelUse(t.Context(), db, []string{"target"}, "generation"); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
				t.Fatalf("fixture model use before deletion=%v", err)
			}
			insertTestIngestionTask(t, "protected-task", "user-1", "source-doc", "registered")
			n, err := testDocumentService(t).DeleteDocuments(permission.WithIdentity(t.Context(), "user-1", "tenant-1"), []string{"source-doc"}, false, "registered")
			if !errors.Is(err, permission.ErrKnowledgeIngestionDenied) || n != 0 {
				t.Fatalf("registered cleanup was not rejected: n=%d err=%v", n, err)
			}
			svc := testDocumentService(t)
			if err := svc.DeleteDocument(t.Context(), "source-doc"); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
				t.Fatalf("single cleanup guard=%v", err)
			}
			if err := svc.RemoveDocumentKeepFile(t.Context(), "source-doc"); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
				t.Fatalf("keep-file cleanup guard=%v", err)
			}
			if n, err := svc.DeleteDocuments(permission.WithIdentity(t.Context(), "user-1", "tenant-1"), nil, true, "registered"); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) || n != 0 {
				t.Fatalf("delete-all cleanup guard n=%d err=%v", n, err)
			}
			var sourceRows, mappingRows, fileRows int64
			dao.DB.Model(&entity.Document{}).Where("id = ?", "source-doc").Count(&sourceRows)
			dao.DB.Model(&entity.File2Document{}).Where("document_id = ?", "copy-doc").Count(&mappingRows)
			dao.DB.Model(&entity.File{}).Where("id = ?", fid).Count(&fileRows)
			if sourceRows != 1 || mappingRows != 1 || fileRows != 1 {
				t.Fatalf("denied cleanup removed source facts: %d/%d/%d", sourceRows, mappingRows, fileRows)
			}
			var taskRows int64
			if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "protected-task").Count(&taskRows).Error; err != nil || taskRows != 1 {
				t.Fatalf("denied cleanup touched task: count=%d err=%v", taskRows, err)
			}
			for _, ref := range refs {
				if err := permission.NewDatabaseChecker(db).CheckResource(permission.WithIdentity(t.Context(), "reader", ""), ref, permission.OperationRead); err == nil {
					t.Errorf("deleting origin removed known private-source restriction: reader can read %+v", ref)
				}
			}
			for _, purpose := range []string{"embedding", "rerank", "generation", "remote_parse", "wiki", "memory"} {
				if err := permission.CheckDatasetModelUse(t.Context(), db, []string{"target"}, purpose); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
					t.Errorf("deleting origin opened purpose %s: %v", purpose, err)
				}
			}
			// Removing an ordinary copy is permitted without deleting its private origin.
			if n, err := svc.DeleteDocuments(permission.WithIdentity(t.Context(), "user-1", "tenant-1"), []string{"copy-doc"}, false, "target"); err != nil || n != 1 {
				t.Fatalf("ordinary copy cleanup n=%d err=%v", n, err)
			}
			if err := permission.CheckKnowledgeFileTransfer(t.Context(), db, []string{fid}); !errors.Is(err, permission.ErrKnowledgeIngestionDenied) {
				t.Fatalf("copy cleanup lost private origin: %v", err)
			}
		})
	}
}
