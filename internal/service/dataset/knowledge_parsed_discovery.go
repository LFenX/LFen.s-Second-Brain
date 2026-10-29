package dataset

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ListKnowledgeParsedRepresentations discovers only published parsed targets
// for one exact authorized source revision. It is not R1/R2 history discovery.
func (d *DatasetService) ListKnowledgeParsedRepresentations(ctx context.Context, owner, domain string, query entity.KnowledgeParsedRepresentationQuery) (value *entity.KnowledgeParsedRepresentationPage, err error) {
	defer func() { err = contentNormalize(err) }()
	if _, err = d.knowledgeBase(ctx, owner, domain, true); err != nil {
		return nil, err
	}
	if query.Source.Validate(domain) != nil || query.Limit < 1 || query.Limit > 20 {
		return nil, contentError(400, "invalid_request")
	}
	source, err := d.parsedAuthorizedSource(ctx, owner, domain, query.Source)
	if err != nil {
		return nil, err
	}
	if !contentSameSource(query.Source, source) {
		return nil, contentError(409, "content_integrity")
	}
	value = &entity.KnowledgeParsedRepresentationPage{Version: entity.KnowledgeContentVersion, Source: query.Source, Items: []entity.KnowledgeContentTarget{}}
	scope := structureReadScope(owner, domain, "parsed_representations", query.Source, query.Limit)
	cursor, err := structureLoadReadCursor(ctx, owner, domain, scope, query.Cursor, query.Limit)
	if err != nil {
		return nil, err
	}
	base := func() *gorm.DB {
		return dao.DB.WithContext(ctx).Model(&entity.KnowledgeDocumentRepresentationRecord{}).Where("owner_id = ? AND domain_id = ? AND source_id = ? AND source_revision_id = ? AND protocol = ?", owner, domain, query.Source.SourceID, query.Source.RevisionID, entity.KnowledgeParsedProtocol)
	}
	if query.Cursor == "" {
		var upper entity.KnowledgeDocumentRepresentationRecord
		err := base().Select("id").Order("id DESC").Take(&upper).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, finalErr := d.parsedAuthorizedSource(ctx, owner, domain, query.Source); finalErr != nil {
				return nil, finalErr
			}
			return value, nil
		}
		if err != nil {
			return nil, err
		}
		cursor.UpperID = upper.ID
	}
	if !isKnowledgeID(cursor.UpperID, 64) || cursor.LastID != "" && !isKnowledgeID(cursor.LastID, 64) {
		return nil, contentError(400, "invalid_request")
	}
	rows := []entity.KnowledgeDocumentRepresentationRecord{}
	if err := base().Select("id").Where("id > ? AND id <= ?", cursor.LastID, cursor.UpperID).Order("id ASC").Limit(query.Limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows[:min(query.Limit, len(rows))] {
		state, err := d.parsedReadRepresentation(ctx, owner, domain, row.ID)
		if err != nil {
			return nil, err
		}
		if state.Representation.Source != query.Source {
			return nil, contentError(409, "content_integrity")
		}
		value.Items = append(value.Items, entity.KnowledgeContentTarget{Source: state.Representation.Source, Representation: state.Representation.Ref})
	}
	if len(rows) > query.Limit {
		cursor.LastID = rows[query.Limit-1].ID
		value.NextCursor, err = structureSaveReadCursor(ctx, owner, domain, scope, cursor)
		if err != nil {
			return nil, err
		}
	}
	current, err := d.parsedAuthorizedSource(ctx, owner, domain, query.Source)
	if err != nil {
		return nil, err
	}
	if !contentSameSource(query.Source, current) {
		return nil, contentError(409, "content_integrity")
	}
	return value, nil
}
