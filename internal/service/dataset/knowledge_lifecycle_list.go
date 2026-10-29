package dataset

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

type KnowledgeLifecycleListQuery struct {
	State         string  `json:"state"`
	GroupScope    string  `json:"group_scope"`
	GroupID       string  `json:"group_id"`
	ComponentSide string  `json:"component_side"`
	Language      *string `json:"language"`
	Limit         int     `json:"limit"`
	Cursor        string  `json:"-"`
}

type lifecycleCursor struct {
	Query KnowledgeLifecycleListQuery `json:"query"`
	After string                      `json:"after"`
}

func (d *DatasetService) ListKnowledgeSourceLifecycles(ctx context.Context, user, domain string, query KnowledgeLifecycleListQuery) (*entity.KnowledgeLifecyclePage, error) {
	if query.State == "" {
		query.State = "active"
	}
	if query.GroupScope == "" {
		query.GroupScope = "all"
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.State != "active" && query.State != "archived" || query.GroupScope != "all" && query.GroupScope != "ungrouped" && query.GroupScope != "exact" || query.Limit < 1 || query.Limit > 20 || query.ComponentSide != "" && !validKnowledgeSide(query.ComponentSide) || query.Language != nil && (!utf8.ValidString(*query.Language) || utf8.RuneCountInString(*query.Language) > 32) || query.GroupScope == "exact" && !isKnowledgeID(query.GroupID, 32) || query.GroupScope != "exact" && query.GroupID != "" || query.Cursor != "" && !isKnowledgeID(query.Cursor, 32) {
		return nil, knowledgeInvalid("Invalid lifecycle filters")
	}
	var result *entity.KnowledgeLifecyclePage
	err := d.lifecycleTransaction(ctx, user, domain, func(tx *gorm.DB, kb *entity.Knowledgebase) error {
		if query.GroupScope == "exact" {
			var group entity.KnowledgeGroup
			if err := tx.Where("id = ? AND domain_id = ?", query.GroupID, domain).Take(&group).Error; err != nil {
				return err
			}
		}
		after := ""
		if query.Cursor != "" {
			var row entity.KnowledgeDirectoryState
			if err := tx.Where("id = ? AND domain_id = ? AND user_id = ? AND kind = ? AND expires_at > ?", query.Cursor, domain, user, "lifecycle_cursor", time.Now().UTC()).Take(&row).Error; err != nil {
				return knowledgeInvalid("Invalid or expired lifecycle cursor")
			}
			var cursor lifecycleCursor
			if json.Unmarshal([]byte(row.Payload), &cursor) != nil || directoryJSON(cursor.Query) != directoryJSON(query) || !isKnowledgeID(cursor.After, 32) {
				return knowledgeInvalid("Lifecycle cursor scope changed")
			}
			after = cursor.After
		}
		db := tx.Model(&entity.KnowledgeSourceLifecycle{}).Select("knowledge_source_lifecycle.source_id").
			Joins("JOIN document ON document.id = knowledge_source_lifecycle.source_id AND document.kb_id = knowledge_source_lifecycle.domain_id").
			Joins("JOIN knowledge_source_revision ON knowledge_source_revision.document_id = knowledge_source_lifecycle.source_id AND knowledge_source_revision.domain_id = knowledge_source_lifecycle.domain_id AND knowledge_source_revision.is_current = ?", true).
			Where("knowledge_source_lifecycle.domain_id = ? AND knowledge_source_lifecycle.state = ? AND document.status = ? AND knowledge_source_lifecycle.source_id > ?", domain, query.State, "1", after)
		db = db.Where("NOT EXISTS (?)", tx.Model(&entity.KnowledgeSourceRegistration{}).Select("1").Where("knowledge_source_registration.id = knowledge_source_revision.original_registration_id AND knowledge_source_registration.domain_id = knowledge_source_revision.domain_id AND knowledge_source_registration.source_kind = ?", "parsed_file"))
		if query.GroupScope == "ungrouped" {
			db = db.Where("knowledge_source_revision.group_id IS NULL")
		}
		if query.GroupScope == "exact" {
			db = db.Where("knowledge_source_revision.group_id = ?", query.GroupID)
		}
		if query.ComponentSide != "" {
			db = db.Where("knowledge_source_revision.component_side = ?", query.ComponentSide)
		}
		if query.Language != nil {
			column := "knowledge_source_revision.language"
			if tx.Dialector.Name() == "mysql" {
				column += " COLLATE utf8mb4_bin"
			}
			db = db.Where(column+" = ?", *query.Language)
		}
		var ids []string
		if err := db.Order("knowledge_source_lifecycle.source_id ASC").Limit(query.Limit + 1).Scan(&ids).Error; err != nil {
			return err
		}
		result = &entity.KnowledgeLifecyclePage{Items: []entity.KnowledgeLifecycleDetail{}}
		for _, id := range ids[:min(len(ids), query.Limit)] {
			data, err := loadLifecycleSource(tx, kb, id)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, lifecycleDetail(kb, data))
		}
		for len(result.Items) > 0 {
			raw, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if len(raw)+512 <= entity.KnowledgeLifecycleJSONBytes {
				break
			}
			result.Items = result.Items[:len(result.Items)-1]
		}
		if len(ids) > 0 && len(result.Items) == 0 {
			return knowledgeError(413, "lifecycle_budget_exceeded", "One lifecycle detail exceeds the JSON budget")
		}
		if len(ids) > len(result.Items) {
			last := result.Items[len(result.Items)-1].SourceID
			now := time.Now().UTC()
			row := entity.KnowledgeDirectoryState{ID: common.GenerateUUID(), DomainID: domain, UserID: user, Kind: "lifecycle_cursor", Payload: directoryJSON(lifecycleCursor{Query: query, After: last}), PrivateData: "{}", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			result.NextCursor = &row.ID
		}
		return nil
	})
	return result, err
}
