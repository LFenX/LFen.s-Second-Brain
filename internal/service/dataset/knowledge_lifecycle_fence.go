package dataset

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func lifecycleConflict(code string) error { return knowledgeError(409, code, code) }

func lifecycleState(tx *gorm.DB, domain, source string) (*entity.KnowledgeSourceLifecycle, error) {
	var state entity.KnowledgeSourceLifecycle
	err := tx.Where("source_id = ? AND domain_id = ?", source, domain).Take(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, lifecycleConflict("lifecycle_integrity")
		}
		return nil, err
	}
	if state.Version < 1 || state.Version > entity.KnowledgeMaxLifecycleVersion || (state.State != "active" && state.State != "archived") {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	return &state, nil
}

func lifecycleHead(tx *gorm.DB, domain, source string) (*entity.KnowledgeSourceRevision, error) {
	var rows []entity.KnowledgeSourceRevision
	if err := tx.Where("domain_id = ? AND document_id = ? AND is_current = ?", domain, source, true).Limit(2).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(rows) != 1 {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	return &rows[0], nil
}

// The domain lock serializes every publication and state command. This helper
// is called even for an unchanged input before returning an existing head.
func lifecyclePublishState(tx *gorm.DB, domain, source string, isNew, changed bool) error {
	if isNew {
		return tx.Create(&entity.KnowledgeSourceLifecycle{SourceID: source, DomainID: domain, State: "active", Version: 1}).Error
	}
	state, err := lifecycleState(tx, domain, source)
	if err != nil {
		return err
	}
	if state.State != "active" {
		return lifecycleConflict("lifecycle_source_archived")
	}
	if !changed {
		return nil
	}
	return lifecycleAdvance(tx, state, state.State)
}

func lifecycleAdvance(tx *gorm.DB, state *entity.KnowledgeSourceLifecycle, nextState string) error {
	if state.Version == entity.KnowledgeMaxLifecycleVersion {
		return lifecycleConflict("lifecycle_version_exhausted")
	}
	result := tx.Model(&entity.KnowledgeSourceLifecycle{}).Where("source_id = ? AND domain_id = ? AND version = ? AND state = ?", state.SourceID, state.DomainID, state.Version, state.State).
		Updates(map[string]any{"version": state.Version + 1, "state": nextState, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return lifecycleConflict("revision_changed")
	}
	state.Version++
	state.State = nextState
	return nil
}

func lifecycleFenceID(kind, preparation, item string) string {
	return entity.KnowledgeID("knowledge-source-fence/0.7.0", kind, preparation, item)
}

func lifecyclePositionDigest(registration, relative string) string {
	hash, _ := entity.KnowledgeLifecycleDigest(map[string]any{"kind": "knowledge-lifecycle-position/0.7.0", "registration_id": registration, "relative_path": relative})
	return hash
}

func lifecycleRetired(tx *gorm.DB, registration, relative string) (*entity.KnowledgeRetiredPosition, error) {
	var row entity.KnowledgeRetiredPosition
	err := tx.Where("id = ?", lifecyclePositionDigest(registration, relative)).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RegistrationID != registration || row.RelativePath != relative {
		return nil, lifecycleConflict("lifecycle_integrity")
	}
	return &row, nil
}

func lifecycleObserveSource(tx *gorm.DB, domain, registration, relative, source string) (*entity.KnowledgeSourceFence, error) {
	fence := &entity.KnowledgeSourceFence{DomainID: domain, RegistrationID: registration, RelativePath: relative, SourceID: source}
	if source == "" {
		retired, err := lifecycleRetired(tx, registration, relative)
		if err != nil {
			return nil, err
		}
		if retired != nil {
			return nil, lifecycleConflict("revision_changed")
		}
		return fence, nil
	}
	state, err := lifecycleState(tx, domain, source)
	if err != nil {
		return nil, err
	}
	if state.State != "active" {
		return nil, lifecycleConflict("revision_changed")
	}
	fence.Version = state.Version
	return fence, nil
}

func lifecycleSaveFence(tx *gorm.DB, kind, preparation, item string, fence *entity.KnowledgeSourceFence) error {
	copy := *fence
	copy.ID, copy.Kind, copy.PreparationID, copy.ItemKey = lifecycleFenceID(kind, preparation, item), kind, preparation, item
	copy.CreatedAt = time.Now().UTC()
	return tx.Create(&copy).Error
}

func lifecycleLoadFence(tx *gorm.DB, domain, kind, preparation, item string) (*entity.KnowledgeSourceFence, error) {
	var fence entity.KnowledgeSourceFence
	err := tx.Where("id = ? AND domain_id = ? AND kind = ? AND preparation_id = ? AND item_key = ?", lifecycleFenceID(kind, preparation, item), domain, kind, preparation, item).Take(&fence).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, lifecycleConflict("revision_changed")
	}
	if err != nil {
		return nil, err
	}
	return &fence, nil
}

func lifecycleVerifyFence(tx *gorm.DB, fence *entity.KnowledgeSourceFence, source string) error {
	if fence == nil || fence.SourceID != source {
		return lifecycleConflict("revision_changed")
	}
	if source == "" {
		if fence.Version != 0 {
			return lifecycleConflict("lifecycle_integrity")
		}
		if fence.RegistrationID != "" {
			var count int64
			if err := tx.Model(&entity.KnowledgeSourceBinding{}).Where("registration_id = ? AND relative_path = ?", fence.RegistrationID, fence.RelativePath).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return lifecycleConflict("revision_changed")
			}
			retired, err := lifecycleRetired(tx, fence.RegistrationID, fence.RelativePath)
			if err != nil {
				return err
			}
			if retired != nil {
				return lifecycleConflict("revision_changed")
			}
		}
		return nil
	}
	state, err := lifecycleState(tx, fence.DomainID, source)
	if err != nil {
		return err
	}
	if state.State != "active" || state.Version != fence.Version {
		return lifecycleConflict("revision_changed")
	}
	return nil
}

func lifecycleCheckPreparation(tx *gorm.DB, domain, kind, preparation, item, source string) error {
	fence, err := lifecycleLoadFence(tx, domain, kind, preparation, item)
	if err != nil {
		return err
	}
	return lifecycleVerifyFence(tx, fence, source)
}

func lifecycleCopyFence(tx *gorm.DB, domain, oldKind, oldPreparation, oldItem, kind, preparation, item, source string) error {
	fence, err := lifecycleLoadFence(tx, domain, oldKind, oldPreparation, oldItem)
	if err != nil {
		return err
	}
	if err := lifecycleVerifyFence(tx, fence, source); err != nil {
		return err
	}
	return lifecycleSaveFence(tx, kind, preparation, item, fence)
}

func lifecycleSourceValue(source *string) string {
	if source == nil {
		return ""
	}
	return *source
}

func lifecycleActiveQuery(tx *gorm.DB) *gorm.DB {
	return tx.Where("document_id IN (?)", tx.Session(&gorm.Session{NewDB: true}).Model(&entity.KnowledgeSourceLifecycle{}).Select("source_id").Where("state = ?", "active"))
}

func lifecycleC1Organization(tx *gorm.DB, domain, source string, current *entity.KnowledgeSourceRevision, request entity.SnapshotKnowledgeSourceRequest) error {
	var binding entity.KnowledgeSourceBinding
	err := tx.Where("document_id = ?", source).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if current == nil || binding.RelativePath != request.RelativePath || current.RelativePath != request.RelativePath || !sameOptionalID(current.GroupID, request.GroupID) || current.ComponentSide != request.ComponentSide || current.Language != request.Language {
		return knowledgeConflict("Registered source organization must be changed through lifecycle commands")
	}
	return nil
}
