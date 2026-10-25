package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// All native folder writers lock the same persisted folder before reading its
// head. The locking head read also avoids an older MySQL transaction snapshot.
func (s *FileCommitService) lockFolderCommit(ctx context.Context, tx *gorm.DB, folderID string) (*entity.FileCommit, error) {
	var folder entity.File
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND type = ?", folderID, "folder").Take(&folder).Error; err != nil {
		return nil, err
	}
	previous, err := s.commitDAO.GetLatestByFolderID(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), folderID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return previous, err
}

func setCommitPredecessor(commit, previous *entity.FileCommit) {
	timestamp := time.Now().UnixMilli()
	if previous != nil {
		commit.ParentID = &previous.ID
		if previous.CreateTime != nil && *previous.CreateTime >= timestamp {
			timestamp = *previous.CreateTime + 1
		}
	}
	commit.CreateTime = &timestamp
}

// RecordSourceSnapshot writes native version records in the caller's catalogue
// transaction. It archives observed bytes without editing the current File row.
// The caller must authorize the document, file and folder before calling it.
func (s *FileCommitService) RecordSourceSnapshot(ctx context.Context, tx *gorm.DB, source *entity.File, authorID string, content []byte) (*entity.FileCommit, *entity.FileCommitItem, error) {
	store := storage.GetStorageFactory().GetStorage()
	if store == nil {
		return nil, nil, fmt.Errorf("storage not initialized")
	}
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	location := ".objects/" + hash
	if err := store.Put(ctx, source.ParentID, location, content); err != nil {
		return nil, nil, err
	}
	return s.recordSourceSnapshot(ctx, tx, source, authorID, hash, int64(len(content)))
}

// recordSourceSnapshot records metadata for an object already written at its
// SHA-256 address. The caller owns byte verification, authorization and the
// transaction; this function neither reads the object nor publishes its source.
func (s *FileCommitService) recordSourceSnapshot(ctx context.Context, tx *gorm.DB, source *entity.File, authorID, hash string, byteLength int64) (*entity.FileCommit, *entity.FileCommitItem, error) {
	if byteLength < 0 || len(hash) != sha256.Size*2 {
		return nil, nil, fmt.Errorf("invalid native snapshot metadata")
	}
	for _, character := range hash {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return nil, nil, fmt.Errorf("invalid native snapshot metadata")
		}
	}
	location := ".objects/" + hash
	tree := map[string]interface{}{}
	previous, err := s.lockFolderCommit(ctx, tx, source.ParentID)
	if err != nil {
		return nil, nil, err
	}
	if previous != nil && previous.TreeState != nil && *previous.TreeState != "" {
		if err := json.Unmarshal([]byte(*previous.TreeState), &tree); err != nil {
			return nil, nil, fmt.Errorf("invalid native version tree: %w", err)
		}
		if tree == nil {
			return nil, nil, fmt.Errorf("invalid native version tree: null")
		}
	}
	operation := "add"
	if _, exists := tree[source.ID]; exists {
		operation = "modify"
	}
	tree[source.ID] = map[string]interface{}{"hash": hash, "location": location, "name": source.Name,
		"parent_id": source.ParentID, "size": byteLength, "status": "1"}
	encoded, err := json.Marshal(tree)
	if err != nil {
		return nil, nil, err
	}
	treeJSON := string(encoded)
	commit := &entity.FileCommit{ID: common.GenerateUUID(), FolderID: source.ParentID, AuthorID: authorID,
		Message: "Knowledge source snapshot", FileCount: 1, TreeState: &treeJSON}
	setCommitPredecessor(commit, previous)
	if err := s.commitDAO.Create(ctx, tx, commit); err != nil {
		return nil, nil, err
	}
	item := &entity.FileCommitItem{ID: common.GenerateUUID(), CommitID: commit.ID, FileID: source.ID,
		Operation: operation, NewHash: &hash, NewLocation: &location, NewName: &source.Name}
	if err := s.commitItemDAO.Create(ctx, tx, item); err != nil {
		return nil, nil, err
	}
	return commit, item, nil
}
