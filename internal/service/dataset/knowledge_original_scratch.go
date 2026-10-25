package dataset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// An empty owned marker is complete at O_EXCL creation, including process exit.
const originalStagingMarker = ""

var originalScratchName = regexp.MustCompile(`^transfer-([0-9a-f]{32})-([1-9][0-9]{0,18})$`)

func openOriginalScratchRoot() (*os.Root, string, error) {
	name := os.Getenv("LFEN_KNOWLEDGE_ORIGINAL_STAGING_ROOT")
	if name == "" {
		name = filepath.Join(os.TempDir(), "lfen-knowledge-originals")
	}
	if !filepath.IsAbs(name) || !utility.KnowledgeDirectoryPlatformVerified() {
		return nil, "", errors.New("unverified staging root")
	}
	if err := os.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, "", err
	}
	info, err := os.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || !originalStagingOwned(info) {
		return nil, "", errors.New("staging root is not private and owned")
	}
	root, _, err := utility.OpenKnowledgeDirectoryRoot(name)
	if err != nil {
		return nil, "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = root.Close()
		}
	}()
	marker, err := root.Open(".staging-v1")
	if errors.Is(err, os.ErrNotExist) {
		dir, openErr := root.Open(".")
		if openErr != nil {
			return nil, "", openErr
		}
		entries, listErr := dir.ReadDir(1)
		_ = dir.Close()
		if listErr != nil && !errors.Is(listErr, io.EOF) {
			return nil, "", listErr
		}
		if len(entries) != 0 {
			return nil, "", errors.New("unclaimed nonempty staging root")
		}
		marker, err = root.OpenFile(".staging-v1", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, "", err
		}
		closeErr := marker.Close()
		if closeErr != nil {
			return nil, "", closeErr
		}
		marker, err = root.Open(".staging-v1")
	}
	if err != nil {
		return nil, "", err
	}
	defer marker.Close()
	markerInfo, err := marker.Stat()
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode().Perm() != 0600 || !originalStagingOwned(markerInfo) {
		return nil, "", errors.New("invalid staging marker")
	}
	content, err := io.ReadAll(io.LimitReader(marker, int64(len(originalStagingMarker)+1)))
	if err != nil || string(content) != originalStagingMarker {
		return nil, "", errors.New("invalid staging marker content")
	}
	ok = true
	return root, name, nil
}

// cleanOriginalScratch holds the same global slot locks as admission. A newly
// admitted transfer cannot create its directory before its slot becomes visible.
// Only generated directories inside the pinned, owned staging root are removed.
func cleanOriginalScratch(ctx context.Context, root *os.Root) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slots []entity.KnowledgeOriginalReceiveSlot
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&slots).Error; err != nil {
			return err
		}
		now, err := batchNow(tx)
		if err != nil {
			return err
		}
		active := map[string]bool{}
		for _, slot := range slots {
			if slot.LeaseUntil != nil && now.Before(*slot.LeaseUntil) {
				active[fmt.Sprintf("transfer-%s-%d", slot.UploadID, slot.Epoch)] = true
			}
		}
		dir, err := root.Open(".")
		if err != nil {
			return err
		}
		defer dir.Close()
		entries, err := dir.ReadDir(129)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries[:min(len(entries), 128)] {
			name := entry.Name()
			if name == ".staging-v1" || active[name] {
				continue
			}
			if !originalScratchName.MatchString(name) {
				return errors.New("unknown staging entry")
			}
			info, err := root.Lstat(name)
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode().Perm() != 0700 || !originalStagingOwned(info) {
				return errors.New("unsafe staging entry")
			}
			if err := root.RemoveAll(name); err != nil {
				return err
			}
		}
		if len(entries) > 128 {
			return errors.New("staging cleanup requires another attempt")
		}
		return nil
	})
}

func prepareOriginalScratch(ctx context.Context, transfer *originalTransfer) (string, func() error, error) {
	root, absolute, err := openOriginalScratchRoot()
	if err != nil {
		return "", nil, err
	}
	if err := cleanOriginalScratch(ctx, root); err != nil {
		_ = root.Close()
		return "", nil, err
	}
	name := fmt.Sprintf("transfer-%s-%d", transfer.upload.UploadID, transfer.epoch)
	if !originalScratchName.MatchString(name) {
		_ = root.Close()
		return "", nil, errors.New("invalid transfer staging name")
	}
	if err := root.Mkdir(name, 0700); err != nil {
		_ = root.Close()
		return "", nil, err
	}
	cleanup := func() error { return errors.Join(root.RemoveAll(name), root.Close()) }
	return filepath.Join(absolute, name), cleanup, nil
}
