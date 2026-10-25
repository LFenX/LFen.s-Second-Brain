package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

func setupSnapshotMetadata(t *testing.T) (*gorm.DB, entity.File, *FileCommitService) {
	t.Helper()
	db := setupFolderTestDB(t)
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatal(err)
	}
	folder := entity.File{ID: "synthetic-folder", ParentID: "synthetic-folder", TenantID: "owner", CreatedBy: "owner", Name: "Synthetic folder", Type: "folder"}
	source := entity.File{ID: "synthetic-file", ParentID: folder.ID, TenantID: "owner", CreatedBy: "owner", Name: "Synthetic 中文.pdf", Type: "pdf", Size: 17}
	for _, row := range []entity.File{folder, source} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(nil)
	t.Cleanup(func() { factory.SetStorage(previous) })
	return db, source, NewFileCommitService()
}

// These rows model metadata for an object the caller has already written. No
// object is fabricated or read here; existence and byte checks are not proven.
func TestKnowledgeSnapshotMetadataWithoutStorage(t *testing.T) {
	db, source, svc := setupSnapshotMetadata(t)
	var before entity.File
	if err := db.Where("id = ?", source.ID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	var first, previous *entity.FileCommit
	for i, byteLength := range []int64{0, 3 * 1024 * 1024, 64 * 1024 * 1024} {
		current := source
		if i == 1 {
			current.ID, current.Name = "synthetic-second", "Second.pdf"
			if err := db.Create(&current).Error; err != nil {
				t.Fatal(err)
			}
		}
		var commit *entity.FileCommit
		var item *entity.FileCommitItem
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			commit, item, err = svc.recordSourceSnapshot(t.Context(), tx, &current, "owner", hash, byteLength)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && (commit.ParentID == nil || *commit.ParentID != previous.ID || *commit.CreateTime <= *previous.CreateTime) {
			t.Fatal("native predecessor/time did not advance")
		}
		var tree map[string]map[string]any
		if err := json.Unmarshal([]byte(*commit.TreeState), &tree); err != nil {
			t.Fatal(err)
		}
		entry := tree[current.ID]
		if entry["size"] != float64(byteLength) || entry["hash"] != hash || entry["location"] != ".objects/"+hash || entry["name"] != current.Name || entry["parent_id"] != current.ParentID || entry["status"] != "1" {
			t.Fatalf("metadata changed: %+v", entry)
		}
		operation := "add"
		if i == 2 {
			operation = "modify"
		}
		if item.FileID != current.ID || item.CommitID != commit.ID || item.Operation != operation || item.NewHash == nil || *item.NewHash != hash || item.NewLocation == nil || *item.NewLocation != ".objects/"+hash || item.NewName == nil || *item.NewName != current.Name {
			t.Fatalf("native item changed: %+v", item)
		}
		if i == 0 {
			first = commit
			future := time.Now().UnixMilli() + 10000
			if err := db.Model(commit).Update("create_time", future).Error; err != nil {
				t.Fatal(err)
			}
			commit.CreateTime = &future
		} else if len(tree) != 2 || tree["synthetic-second"]["size"] != float64(3*1024*1024) {
			t.Fatal("same digest collapsed file identities or lost sibling")
		}
		previous = commit
	}
	var after entity.File
	if err := db.Where("id = ?", source.ID).Take(&after).Error; err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("source row changed: %v", err)
	}
	var stored entity.FileCommit
	if err := db.Where("id = ?", first.ID).Take(&stored).Error; err != nil || *stored.TreeState != *first.TreeState {
		t.Fatalf("fixed history changed: %v", err)
	}
}

func TestKnowledgeSnapshotMetadataRejectsInvalid(t *testing.T) {
	db, source, svc := setupSnapshotMetadata(t)
	for _, test := range []struct {
		name string
		hash string
		size int64
	}{
		{"negative_length", strings.Repeat("a", 64), -1},
		{"short_digest", "a", 1},
		{"upper_digest", strings.Repeat("A", 64), 1},
		{"non_hex_digest", strings.Repeat("g", 64), 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := db.Transaction(func(tx *gorm.DB) error {
				_, _, err := svc.recordSourceSnapshot(t.Context(), tx, &source, "owner", test.hash, test.size)
				return err
			})
			if err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	for _, model := range []any{&entity.FileCommit{}, &entity.FileCommitItem{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("invalid metadata published rows: %d %v", count, err)
		}
	}
}

func TestKnowledgeSnapshotMetadataFailureRollback(t *testing.T) {
	for _, failure := range []string{"file_commit", "file_commit_item", "invalid_tree", "null_tree", "missing_folder", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			db, source, svc := setupSnapshotMetadata(t)
			hash := strings.Repeat("a", 64)
			var first *entity.FileCommit
			if err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				first, _, err = svc.recordSourceSnapshot(t.Context(), tx, &source, "owner", hash, 1)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			sentinel := errors.New("synthetic metadata write failure")
			switch failure {
			case "file_commit", "file_commit_item":
				if err := db.Callback().Create().Before("gorm:create").Register("synthetic_snapshot_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == failure {
						tx.AddError(sentinel)
					}
				}); err != nil {
					t.Fatal(err)
				}
			case "invalid_tree", "null_tree":
				badTree := "{"
				if failure == "null_tree" {
					badTree = "null"
				}
				if err := db.Model(first).Update("tree_state", badTree).Error; err != nil {
					t.Fatal(err)
				}
			case "missing_folder":
				source.ParentID = "not-a-folder"
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := db.Transaction(func(tx *gorm.DB) error {
				_, _, err := svc.recordSourceSnapshot(ctx, tx, &source, "owner", strings.Repeat("b", 64), 2)
				return err
			})
			if err == nil || (strings.HasPrefix(failure, "file_commit") && !errors.Is(err, sentinel)) || (failure == "canceled" && !errors.Is(err, context.Canceled)) {
				t.Fatalf("failure not propagated: %v", err)
			}
			head, err := dao.NewFileCommitDAO().GetLatestByFolderID(t.Context(), db, first.FolderID)
			if err != nil || head.ID != first.ID {
				t.Fatalf("failure advanced head: %v", err)
			}
			for _, model := range []any{&entity.FileCommit{}, &entity.FileCommitItem{}} {
				var count int64
				if err := db.Model(model).Count(&count).Error; err != nil || count != 1 {
					t.Fatalf("transaction leaked rows: %d %v", count, err)
				}
			}
		})
	}
}

func TestKnowledgeSnapshotMetadataBytePath(t *testing.T) {
	for _, test := range []struct {
		name string
		body []byte
	}{{"empty", []byte{}}, {"unicode_bom_crlf", []byte("\xef\xbb\xbf中文\r\nemoji😀\r\n")}} {
		t.Run(test.name, func(t *testing.T) {
			body := test.body
			db, source, svc := setupSnapshotMetadata(t)
			store := storage.NewMemoryStorage()
			storage.GetStorageFactory().SetStorage(store)
			var commit *entity.FileCommit
			if err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				commit, _, err = svc.RecordSourceSnapshot(t.Context(), tx, &source, "owner", body)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			actual, err := svc.GetCommitFileContent(t.Context(), source.ParentID, commit.ID, source.ID)
			if err != nil || !bytes.Equal(actual, body) {
				t.Fatalf("original byte path changed: %v", err)
			}
			digest := sha256.Sum256(body)
			var tree map[string]struct {
				Hash string `json:"hash"`
				Size int64  `json:"size"`
			}
			if err := json.Unmarshal([]byte(*commit.TreeState), &tree); err != nil || tree[source.ID].Hash != hex.EncodeToString(digest[:]) || tree[source.ID].Size != int64(len(body)) {
				t.Fatalf("byte-derived metadata changed: %v", err)
			}
		})
	}
}
