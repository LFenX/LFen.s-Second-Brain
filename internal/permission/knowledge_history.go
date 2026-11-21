package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// CheckKnowledgeVersionAccess constrains native folder versions by every known
// source owner. Callers must also enforce the native folder/dataset permission.
// A mixed ancestor is hidden as a whole, before counts, trees or objects escape.
// Historical references are checked independently of current File existence.
func CheckKnowledgeVersionAccess(ctx context.Context, db *gorm.DB, userID, folderID string) error {
	if db == nil {
		return errors.New("knowledge version policy database unavailable")
	}
	if userID == "" {
		return ErrUnauthenticated
	}
	if folderID == "" {
		return ErrInvalidPermission
	}
	hidden := func() *gorm.DB {
		return KnowledgeHiddenFileIDs(ctx, db, userID)
	}
	var anyHidden []struct{ ResourceID string }
	if err := db.WithContext(ctx).Table("(?) AS hidden", hidden()).Select("resource_id").Limit(1).Find(&anyHidden).Error; err != nil {
		return err
	}
	if len(anyHidden) == 0 {
		return nil
	}
	checkQuery := func(refs *gorm.DB) error {
		var count int64
		if err := db.WithContext(ctx).Table("(?) AS hidden", hidden()).Where("resource_id IN (?)", refs).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrPermissionDenied
		}
		return nil
	}
	// Uncommitted changes also expose names when no commit exists yet.
	subtree := db.WithContext(ctx).Raw(`WITH RECURSIVE subtree(id) AS (
		SELECT id FROM file WHERE id = ?
		UNION SELECT f.id FROM file f JOIN subtree s ON f.parent_id = s.id WHERE f.id <> f.parent_id
	) SELECT id FROM subtree`, folderID)
	if err := checkQuery(subtree); err != nil {
		return err
	}
	items := db.WithContext(ctx).Table("file_commit_item i").Select("i.file_id").
		Joins("JOIN file_commit c ON c.id = i.commit_id").Where("c.folder_id = ?", folderID)
	if err := checkQuery(items); err != nil {
		return err
	}
	checkIDs := func(ids []string) error {
		if len(ids) == 0 {
			return nil
		}
		var count int64
		if err := db.WithContext(ctx).Table("(?) AS hidden", hidden()).Where("resource_id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrPermissionDenied
		}
		return nil
	}
	// Old files can have moved or disappeared. TreeState retains their parent
	// identities even when the current tree or a commit-item lookup cannot.
	var commits []entity.FileCommit
	return db.WithContext(ctx).Select("id", "tree_state").Where("folder_id = ?", folderID).
		FindInBatches(&commits, 50, func(_ *gorm.DB, _ int) error {
			ids := make([]string, 0, 128)
			flush := func() error {
				err := checkIDs(ids)
				ids = ids[:0]
				return err
			}
			for _, commit := range commits {
				if commit.TreeState == nil || *commit.TreeState == "" {
					continue
				}
				var tree map[string]struct {
					ParentID string `json:"parent_id"`
				}
				if err := json.Unmarshal([]byte(*commit.TreeState), &tree); err != nil {
					return fmt.Errorf("knowledge version tree unavailable: %w", err)
				}
				for fileID, entry := range tree {
					ids = append(ids, fileID)
					if entry.ParentID != "" {
						ids = append(ids, entry.ParentID)
					}
					if len(ids) >= 128 {
						if err := flush(); err != nil {
							return err
						}
					}
				}
			}
			return flush()
		}).Error
}
