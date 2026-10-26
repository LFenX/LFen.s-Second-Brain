package dataset

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/entity"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"
	"ragflow/internal/syncer/connector"
)

func lifecycleMoveBinding(tx *gorm.DB, data *lifecycleSourceData, relative string) error {
	registration, source, previous := data.registration.ID, data.head.DocumentID, data.head.RelativePath
	retired, err := lifecycleRetired(tx, registration, relative)
	if err != nil {
		return err
	}
	if retired != nil {
		if retired.SourceID != source {
			return lifecycleConflict("lifecycle_position_occupied")
		}
		if err := tx.Where("id = ? AND source_id = ?", retired.ID, source).Delete(&entity.KnowledgeRetiredPosition{}).Error; err != nil {
			return err
		}
	}
	old, err := lifecycleRetired(tx, registration, previous)
	if err != nil {
		return err
	}
	if old != nil {
		return lifecycleConflict("lifecycle_integrity")
	}
	if err := tx.Create(&entity.KnowledgeRetiredPosition{ID: lifecyclePositionDigest(registration, previous), DomainID: data.head.DomainID, RegistrationID: registration, RelativePath: previous, SourceID: source}).Error; err != nil {
		return err
	}
	result := tx.Model(&entity.KnowledgeSourceBinding{}).Where("id = ? AND document_id = ? AND relative_path = ?", data.binding.ID, source, previous).Update("relative_path", relative)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return lifecycleConflict("lifecycle_position_occupied")
		}
		return result.Error
	}
	if result.RowsAffected != 1 {
		return lifecycleConflict("revision_changed")
	}
	data.binding.RelativePath = relative
	return nil
}

// Metadata revisions reference a previously verified immutable native object.
// No storage reader, writer, parser or model is used on this path.
func lifecycleMetadataRevision(tx *gorm.DB, kb *entity.Knowledgebase, user string, data *lifecycleSourceData, relative string, classification entity.KnowledgeDirectoryClassification) error {
	current := data.head
	var input string
	var webData *webRevisionData
	var err error
	switch current.RepresentationKind {
	case "text":
		input = knowledgeSourceInputHash(current.SHA256, entity.SnapshotKnowledgeSourceRequest{RelativePath: relative, GroupID: classification.GroupID, ComponentSide: classification.ComponentSide, Language: classification.Language})
	case "original_only":
		input = originalInputHash(current.SHA256, current.OriginalFilename, current.OriginalMediaType, relative, classification)
	case "web_capture":
		webData, err = loadWebRevision(tx, user, kb.ID, current.ID)
		if err != nil {
			return err
		}
		input, err = connector.KnowledgeWebSourceInput(webData.entry, classification)
		if err != nil {
			return err
		}
	default:
		return lifecycleConflict("lifecycle_integrity")
	}
	var next entity.KnowledgeSourceRevision
	err = tx.Where("domain_id = ? AND document_id = ? AND input_hash = ?", kb.ID, current.DocumentID, input).Take(&next).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var item entity.FileCommitItem
		if err := tx.Where("id = ? AND commit_id = ? AND file_id = ?", current.ID, current.CommitID, current.FileID).Take(&item).Error; err != nil {
			return err
		}
		if item.NewHash == nil || *item.NewHash != current.SHA256 || item.NewLocation == nil || (*item.NewLocation != ".objects/"+current.SHA256 && *item.NewLocation != storage.KnowledgeOriginalObjectPrefix+current.SHA256) {
			return lifecycleConflict("lifecycle_integrity")
		}
		commit, nativeItem, err := file.NewFileCommitService().RecordSourceSnapshotObject(tx.Statement.Context, tx, &data.native, user, current.SHA256, *item.NewLocation, current.ByteLength)
		if err != nil {
			return err
		}
		next = *current
		next.ID, next.CommitID, next.InputHash = nativeItem.ID, commit.ID, input
		next.BaseModel = entity.BaseModel{}
		next.GroupID, next.ComponentSide, next.Language, next.RelativePath = classification.GroupID, classification.ComponentSide, classification.Language, relative
		next.IsCurrent = true
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		if next.RepresentationKind == "text" {
			root := knowledgeRoot(&next)
			if err := tx.Create(&root).Error; err != nil {
				return err
			}
		} else if next.RepresentationKind == "web_capture" {
			var old entity.KnowledgeWebRevisionRecord
			if err := tx.Where("id = ?", current.ID).Take(&old).Error; err != nil {
				return err
			}
			view := webData.view
			view.SourceRevisionID, view.SourceInputSHA256, view.Classification = next.ID, input, classification
			view.CreatedAt, view.IsCurrent = originalTime(time.Now().UTC()), true
			payload, err := json.Marshal(view)
			if err != nil {
				return err
			}
			old.ID, old.Payload, old.CreatedAt = next.ID, string(payload), time.Now().UTC()
			if err := tx.Create(&old).Error; err != nil {
				return err
			}
		}
	}
	nextClass := entity.KnowledgeDirectoryClassification{GroupID: next.GroupID, ComponentSide: next.ComponentSide, Language: next.Language}
	if next.FileID != current.FileID || next.FolderID != current.FolderID || next.RepresentationKind != current.RepresentationKind || next.SHA256 != current.SHA256 || next.ByteLength != current.ByteLength || next.RelativePath != relative || directoryJSON(nextClass) != directoryJSON(classification) {
		return lifecycleConflict("lifecycle_integrity")
	}
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("domain_id = ? AND document_id = ? AND id <> ?", kb.ID, current.DocumentID, next.ID).Update("is_current", false).Error; err != nil {
		return err
	}
	if err := tx.Model(&entity.KnowledgeSourceRevision{}).Where("id = ?", next.ID).Update("is_current", true).Error; err != nil {
		return err
	}
	next.IsCurrent = true
	data.head = &next
	return nil
}
