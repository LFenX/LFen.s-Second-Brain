package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

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
	tree := map[string]interface{}{}
	previous, err := s.commitDAO.GetLatestByFolderID(ctx, tx, source.ParentID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if previous != nil && previous.TreeState != nil && *previous.TreeState != "" {
		if err := json.Unmarshal([]byte(*previous.TreeState), &tree); err != nil {
			return nil, nil, fmt.Errorf("invalid native version tree: %w", err)
		}
	}
	operation := "add"
	if _, exists := tree[source.ID]; exists {
		operation = "modify"
	}
	tree[source.ID] = map[string]interface{}{"hash": hash, "location": location, "name": source.Name,
		"parent_id": source.ParentID, "size": len(content), "status": "1"}
	encoded, err := json.Marshal(tree)
	if err != nil {
		return nil, nil, err
	}
	treeJSON := string(encoded)
	commit := &entity.FileCommit{ID: common.GenerateUUID(), FolderID: source.ParentID, AuthorID: authorID,
		Message: "Knowledge source snapshot", FileCount: 1, TreeState: &treeJSON}
	if previous != nil {
		commit.ParentID = &previous.ID
	}
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
