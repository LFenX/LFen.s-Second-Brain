package file

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestKnowledgeSnapshotBatchOneNativeTree(t *testing.T) {
	db, source, svc := setupSnapshotMetadata(t)
	objects := make([]SourceSnapshotObject, 32)
	for i := range objects {
		current := source
		current.ID = fmt.Sprintf("batch-file-%02d", i)
		current.Name = fmt.Sprintf("条目-%02d.html", i)
		if err := db.Create(&current).Error; err != nil {
			t.Fatal(err)
		}
		objects[i] = SourceSnapshotObject{File: &current, SHA256: strings.Repeat("a", 64), Location: ".objects/" + strings.Repeat("a", 64), ByteLength: int64(i)}
	}
	var commit *entity.FileCommit
	var items []*entity.FileCommitItem
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		commit, items, err = svc.RecordSourceSnapshotObjects(t.Context(), tx, "owner", objects)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&entity.FileCommit{}).Count(&count).Error; err != nil || count != 1 || commit.FileCount != 32 || len(items) != 32 {
		t.Fatal("batch did not publish one folder commit", count, err)
	}
	var tree map[string]map[string]any
	if err := json.Unmarshal([]byte(*commit.TreeState), &tree); err != nil || len(tree) != 32 {
		t.Fatal("invalid complete tree", err)
	}
	for i, item := range items {
		if item.CommitID != commit.ID || item.FileID != objects[i].File.ID || item.Operation != "add" || tree[item.FileID]["size"] != float64(i) {
			t.Fatal("batch item identity lost")
		}
	}
	firstTree := *commit.TreeState
	objects[0].ByteLength = 991
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := svc.RecordSourceSnapshotObjects(t.Context(), tx, "owner", objects[:1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var prior entity.FileCommit
	if err := db.Where("id = ?", commit.ID).Take(&prior).Error; err != nil || *prior.TreeState != firstTree {
		t.Fatal("prior native tree changed", err)
	}
	var latest entity.FileCommit
	if err := db.Where("parent_id = ?", commit.ID).Take(&latest).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*latest.TreeState), &tree); err != nil || len(tree) != 32 || tree[objects[0].File.ID]["size"] != float64(991) || tree[objects[31].File.ID]["size"] != float64(31) {
		t.Fatal("batch lost sibling", err)
	}
}

func TestKnowledgeSnapshotBatchRejectsAndRollsBack(t *testing.T) {
	db, source, svc := setupSnapshotMetadata(t)
	valid := SourceSnapshotObject{File: &source, SHA256: strings.Repeat("a", 64), Location: ".objects/" + strings.Repeat("a", 64), ByteLength: 1}
	other := source
	other.ID = "different"
	other.ParentID = "wrong-folder"
	for _, objects := range [][]SourceSnapshotObject{nil, {valid, valid}, {{File: &other, SHA256: valid.SHA256, Location: valid.Location}, {File: &source, SHA256: valid.SHA256, Location: valid.Location}}, {{File: nil, SHA256: valid.SHA256, Location: valid.Location}}} {
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, _, err := svc.RecordSourceSnapshotObjects(t.Context(), tx, "owner", objects)
			return err
		}); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	if err := db.Exec("CREATE TRIGGER fail_snapshot_item BEFORE INSERT ON file_commit_item BEGIN SELECT RAISE(ABORT, 'synthetic item failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := svc.RecordSourceSnapshotObjects(t.Context(), tx, "owner", []SourceSnapshotObject{valid})
		return err
	}); err == nil {
		t.Fatal("item fault accepted")
	}
	for _, model := range []any{&entity.FileCommit{}, &entity.FileCommitItem{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("partial native batch survived rollback", count, err)
		}
	}
}
