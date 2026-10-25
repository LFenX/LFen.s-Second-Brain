package dataset

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strconv"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

// KnowledgeOriginalContent carries an authorized fixed stream to the HTTP adapter.
type KnowledgeOriginalContent struct {
	Source *entity.KnowledgeOriginalRevision
	Status int
	Offset int64
	Length int64
	Body   io.ReadCloser
}

var originalRangePattern = regexp.MustCompile(`^bytes=([0-9]*)-([0-9]*)$`)

func originalRange(size int64, value, ifRange, etag string, head bool) (int, int64, int64, error) {
	if head || value == "" || (ifRange != "" && ifRange != etag) {
		return 200, 0, size, nil
	}
	parts := originalRangePattern.FindStringSubmatch(value)
	bad := func() (int, int64, int64, error) { return 416, 0, 0, originalError(416, "range_not_satisfiable") }
	if parts == nil || (parts[1] == "" && parts[2] == "") {
		return bad()
	}
	if parts[1] == "" {
		length, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || length <= 0 {
			return bad()
		}
		length = min(size, length)
		return 206, size - length, length, nil
	}
	start, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || start >= size {
		return bad()
	}
	end := size - 1
	if parts[2] != "" {
		end, err = strconv.ParseInt(parts[2], 10, 64)
		if err != nil || end < start {
			return bad()
		}
		end = min(end, size-1)
	}
	return 206, start, end - start + 1, nil
}

// OpenKnowledgeOriginalContent checks authorization and identity before headers.
func (d *DatasetService) OpenKnowledgeOriginalContent(ctx context.Context, user, domain, id, rangeValue, ifRange string, head bool) (*KnowledgeOriginalContent, error) {
	var source *entity.KnowledgeSourceRevision
	var view *entity.KnowledgeOriginalRevision
	err := d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		var err error
		source, err = loadOriginalRevision(tx, domain, id)
		if err != nil {
			return err
		}
		view, err = originalRevisionView(source)
		return err
	})
	if err != nil {
		return nil, err
	}
	if source.ByteLength < 1 || source.ByteLength > entity.KnowledgeOriginalMaxBytes || !isKnowledgeID(source.SHA256, 64) {
		return nil, originalError(409, "original_integrity")
	}
	store, ok := storage.GetStorageFactory().GetStorage().(storage.StreamingStorage)
	if !ok {
		return nil, originalError(503, "original_storage_unavailable")
	}
	etag := `"sha256-` + view.SHA256 + `"`
	status, offset, length, rangeErr := originalRange(source.ByteLength, rangeValue, ifRange, etag, head)
	readLength := length
	if head || rangeErr != nil {
		readLength = 0
	}
	body, err := store.OpenStream(ctx, source.FolderID, storage.KnowledgeOriginalObjectPrefix+source.SHA256, source.ByteLength, source.SHA256, offset, readLength)
	if err != nil {
		if errors.Is(err, storage.ErrStreamObjectIntegrity) {
			return nil, originalError(409, "original_integrity")
		}
		return nil, originalError(503, "original_storage_unavailable")
	}
	// A slow object-header request must not extend a revoked caller's grant.
	err = d.originalTransaction(ctx, user, domain, func(tx *gorm.DB, _ *entity.Knowledgebase, _ time.Time) error {
		current, err := loadOriginalRevision(tx, domain, id)
		if err != nil {
			return err
		}
		view, err = originalRevisionView(current)
		return err
	})
	if err != nil {
		_ = body.Close()
		return nil, err
	}
	result := &KnowledgeOriginalContent{Source: view, Status: status, Offset: offset, Length: length}
	if head || rangeErr != nil {
		if err := body.Close(); err != nil {
			return nil, originalError(503, "original_storage_unavailable")
		}
		return result, rangeErr
	}
	result.Body = body
	return result, nil
}
