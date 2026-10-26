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
	return s.RecordSourceSnapshotObject(ctx, tx, source, authorID, hash, location, int64(len(content)))
}

// RecordSourceSnapshotObject records metadata for an object already written at its
// SHA-256 address. The caller owns byte verification, authorization and the
// transaction; this function neither reads the object nor publishes its source.
func (s *FileCommitService) RecordSourceSnapshotObject(ctx context.Context, tx *gorm.DB, source *entity.File, authorID, hash, location string, byteLength int64) (*entity.FileCommit, *entity.FileCommitItem, error) {
	commit, items, err := s.RecordSourceSnapshotObjects(ctx, tx, authorID, []SourceSnapshotObject{{File: source, SHA256: hash, Location: location, ByteLength: byteLength}})
	if err != nil {
		return nil, nil, err
	}
	return commit, items[0], nil
}

// SourceSnapshotObject describes verified, immutable bytes for one native file.
// Object verification and domain authorization belong to the calling service.
type SourceSnapshotObject struct {
	File       *entity.File
	SHA256     string
	Location   string
	ByteLength int64
}

// RecordSourceSnapshotObjects publishes one folder commit for the complete
// batch, reading and serializing its native tree once inside the caller's SQL
// transaction. Item order matches the request; duplicate files are rejected.
func (s *FileCommitService) RecordSourceSnapshotObjects(ctx context.Context, tx *gorm.DB, authorID string, objects []SourceSnapshotObject) (*entity.FileCommit, []*entity.FileCommitItem, error) {
	if len(objects) == 0 || len(objects) > 32 || objects[0].File == nil {
		return nil, nil, fmt.Errorf("invalid native snapshot batch")
	}
	folderID := objects[0].File.ParentID
	seen := make(map[string]bool, len(objects))
	for _, object := range objects {
		if object.File == nil || object.File.ID == "" || object.File.ParentID != folderID || seen[object.File.ID] || object.ByteLength < 0 || len(object.SHA256) != sha256.Size*2 || (object.Location != ".objects/"+object.SHA256 && object.Location != storage.KnowledgeOriginalObjectPrefix+object.SHA256) {
			return nil, nil, fmt.Errorf("invalid native snapshot metadata")
		}
		for _, character := range object.SHA256 {
			if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
				return nil, nil, fmt.Errorf("invalid native snapshot metadata")
			}
		}
		seen[object.File.ID] = true
	}
	tree := map[string]interface{}{}
	previous, err := s.lockFolderCommit(ctx, tx, folderID)
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
	commitID := common.GenerateUUID()
	items := make([]*entity.FileCommitItem, 0, len(objects))
	for _, object := range objects {
		operation := "add"
		if _, exists := tree[object.File.ID]; exists {
			operation = "modify"
		}
		tree[object.File.ID] = map[string]interface{}{"hash": object.SHA256, "location": object.Location, "name": object.File.Name,
			"parent_id": folderID, "size": object.ByteLength, "status": "1"}
		items = append(items, &entity.FileCommitItem{ID: common.GenerateUUID(), CommitID: commitID, FileID: object.File.ID,
			Operation: operation, NewHash: &object.SHA256, NewLocation: &object.Location, NewName: &object.File.Name})
	}
	encoded, err := json.Marshal(tree)
	if err != nil {
		return nil, nil, err
	}
	treeJSON := string(encoded)
	commit := &entity.FileCommit{ID: commitID, FolderID: folderID, AuthorID: authorID,
		Message: "Knowledge source snapshot", FileCount: len(items), TreeState: &treeJSON}
	setCommitPredecessor(commit, previous)
	if err := s.commitDAO.Create(ctx, tx, commit); err != nil {
		return nil, nil, err
	}
	for _, item := range items {
		if err := s.commitItemDAO.Create(ctx, tx, item); err != nil {
			return nil, nil, err
		}
	}
	return commit, items, nil
}
