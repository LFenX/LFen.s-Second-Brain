package dataset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// KnowledgeError carries a stable public code without leaking storage or SQL details.
type KnowledgeError struct {
	Status  int
	Code    string
	Message string
}

func (e *KnowledgeError) Error() string { return e.Message }

func knowledgeError(status int, code, message string) error {
	return &KnowledgeError{Status: status, Code: code, Message: message}
}

func knowledgeNotFound() error {
	return knowledgeError(404, "knowledge_not_found", "Knowledge resource not found")
}

func knowledgeInvalid(message string) error {
	return knowledgeError(400, "knowledge_invalid_request", message)
}

func knowledgeConflict(message string) error {
	return knowledgeError(409, "knowledge_conflict", message)
}

func (d *DatasetService) knowledgeBase(ctx context.Context, userID, domainID string, registered bool) (*entity.Knowledgebase, error) {
	id, err := normalizeDatasetID(strings.TrimSpace(domainID))
	if err != nil || id == "" || userID == "" {
		return nil, knowledgeNotFound()
	}
	var kb entity.Knowledgebase
	if err := dao.DB.WithContext(ctx).Where("id = ? AND status = ?", id, string(entity.StatusValid)).Take(&kb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, knowledgeNotFound()
		}
		return nil, err
	}
	if kb.CreatedBy != userID || (registered && !kb.KnowledgeEnabled) {
		return nil, knowledgeNotFound()
	}
	// Reject revoked members before resource loading, which requires an active
	// tenant owner and otherwise treats a disabled sole owner as invalid state.
	var membershipCount int64
	if err := dao.DB.WithContext(ctx).Model(&entity.UserTenant{}).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, kb.TenantID, string(entity.StatusValid)).Count(&membershipCount).Error; err != nil {
		return nil, err
	}
	if membershipCount == 0 {
		return nil, knowledgeNotFound()
	}
	if err := d.CheckAccess(ctx, permission.Subject{UserID: userID}, kb.ID, permission.OperationRead); err != nil {
		if errors.Is(err, permission.ErrPermissionDenied) || errors.Is(err, permission.ErrResourceNotFound) || errors.Is(err, permission.ErrMembershipNotFound) {
			return nil, knowledgeNotFound()
		}
		return nil, err
	}
	return &kb, nil
}

func knowledgeDomain(kb *entity.Knowledgebase) entity.KnowledgeDomain {
	return entity.KnowledgeDomain{ID: kb.ID, Name: kb.Name, Category: kb.KnowledgeCategory,
		OwnerID: kb.CreatedBy, ContractVersion: entity.KnowledgeContractVersion,
		Policy: entity.KnowledgeLocalPolicy(kb.KnowledgePolicyVersion)}
}

// RegisterKnowledgeDomain only enables an empty private native dataset.
// Existing real datasets are never reclassified or reindexed as a side effect.
func (d *DatasetService) RegisterKnowledgeDomain(ctx context.Context, userID, domainID string, req entity.RegisterKnowledgeDomainRequest) (*entity.KnowledgeDomain, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, false)
	if err != nil {
		return nil, err
	}
	if err := d.CheckAccess(ctx, permission.Subject{UserID: userID}, kb.ID, permission.OperationUpdate); err != nil {
		return nil, knowledgeNotFound()
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "custom"
	}
	switch category {
	case "family", "personal", "project", "experience", "resource", "custom":
	default:
		return nil, knowledgeInvalid("Unsupported domain category")
	}
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND created_by = ?", kb.ID, userID).Take(kb).Error; err != nil {
			return err
		}
		if kb.KnowledgeEnabled {
			if kb.KnowledgeCategory != category {
				return knowledgeConflict("Domain category is already frozen")
			}
			_, err := dao.NewFileDAO().EnsureKnowledgeFolder(ctx, tx, kb.ID)
			return err
		}
		if kb.Permission != string(entity.TenantPermissionMe) {
			return knowledgeConflict("Only a private dataset can become a domain")
		}
		var count int64
		if err := tx.Model(&entity.Document{}).Where("kb_id = ?", kb.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 || kb.ChunkNum != 0 || kb.TokenNum != 0 {
			return knowledgeConflict("Register an empty dataset before adding sources")
		}
		if err := tx.Model(&entity.Connector2Kb{}).Where("kb_id = ?", kb.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return knowledgeConflict("Disconnect automatic sources before registering a domain")
		}
		if err := tx.Model(&entity.IngestionTask{}).Where("dataset_id = ?", kb.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return knowledgeConflict("Register a fresh dataset without ingestion history")
		}
		if _, err := dao.NewFileDAO().EnsureKnowledgeFolder(ctx, tx, kb.ID); err != nil {
			return err
		}
		if err := tx.Model(&entity.Knowledgebase{}).Where("id = ?", kb.ID).Updates(map[string]interface{}{
			"knowledge_enabled": true, "knowledge_category": category, "knowledge_policy_version": int64(1),
		}).Error; err != nil {
			return err
		}
		kb.KnowledgeEnabled, kb.KnowledgeCategory, kb.KnowledgePolicyVersion = true, category, 1
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := knowledgeDomain(kb)
	return &result, nil
}

func (d *DatasetService) GetKnowledgeDomain(ctx context.Context, userID, domainID string) (*entity.KnowledgeDomain, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	result := knowledgeDomain(kb)
	return &result, nil
}

type knowledgeCursor struct {
	DomainID string `json:"domain"`
	Kind     string `json:"kind"`
	Parent   string `json:"parent"`
	Revision string `json:"revision"`
	LastID   string `json:"last"`
}

func knowledgePageQuery(query *gorm.DB, cursor string, scope knowledgeCursor, limit int) (*gorm.DB, error) {
	if limit < 1 || limit > 200 {
		return nil, knowledgeInvalid("limit must be between 1 and 200")
	}
	if cursor != "" {
		if len(cursor) > 2048 {
			return nil, knowledgeInvalid("Invalid cursor")
		}
		encoded, err := base64.RawURLEncoding.DecodeString(cursor)
		var parsed knowledgeCursor
		if err != nil || json.Unmarshal(encoded, &parsed) != nil || parsed.DomainID != scope.DomainID || parsed.Kind != scope.Kind || parsed.Parent != scope.Parent || parsed.Revision != scope.Revision || parsed.LastID == "" {
			return nil, knowledgeInvalid("Cursor does not match this scope")
		}
		query = query.Where("id > ?", parsed.LastID)
	}
	return query.Order("id ASC").Limit(limit + 1), nil
}

func knowledgeNextCursor(scope knowledgeCursor, lastID string) string {
	scope.LastID = lastID
	b, _ := json.Marshal(scope)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (d *DatasetService) ListKnowledgeDomains(ctx context.Context, userID, cursor string, limit int) (*entity.KnowledgePage[entity.KnowledgeDomain], error) {
	scope := knowledgeCursor{DomainID: userID, Kind: "domains"}
	query, err := knowledgePageQuery(dao.DB.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("knowledge_enabled = ? AND created_by = ? AND status = ?", true, userID, string(entity.StatusValid)), cursor, scope, limit)
	if err != nil {
		return nil, err
	}
	var rows []entity.Knowledgebase
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	page := &entity.KnowledgePage[entity.KnowledgeDomain]{Items: []entity.KnowledgeDomain{}}
	if len(rows) > limit {
		page.NextCursor = knowledgeNextCursor(scope, rows[limit-1].ID)
		rows = rows[:limit]
	}
	for i := range rows {
		if _, err := d.knowledgeBase(ctx, userID, rows[i].ID, true); err != nil {
			return nil, err
		}
		page.Items = append(page.Items, knowledgeDomain(&rows[i]))
	}
	return page, nil
}

func validKnowledgeText(s string, max int) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, 0)
}

func isKnowledgeID(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func validKnowledgeSide(side string) bool {
	switch side {
	case "frontend", "backend", "shared", "cli", "non_code":
		return true
	}
	return false
}

func (d *DatasetService) knowledgeGroup(ctx context.Context, domainID, groupID string) (*entity.KnowledgeGroup, error) {
	var group entity.KnowledgeGroup
	if err := dao.DB.WithContext(ctx).Where("id = ? AND domain_id = ?", groupID, domainID).Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, knowledgeNotFound()
		}
		return nil, err
	}
	return &group, nil
}

func (d *DatasetService) CreateKnowledgeGroup(ctx context.Context, userID, domainID string, req entity.CreateKnowledgeGroupRequest) (*entity.KnowledgeGroup, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if !validKnowledgeText(name, 256) {
		return nil, knowledgeInvalid("Invalid group name")
	}
	switch req.Kind {
	case "project", "topic", "module":
	default:
		return nil, knowledgeInvalid("Unsupported group kind")
	}
	if req.ComponentSide == "" {
		req.ComponentSide = "non_code"
	}
	if !validKnowledgeSide(req.ComponentSide) {
		return nil, knowledgeInvalid("Unsupported component_side")
	}
	if req.ParentID != nil {
		if !isKnowledgeID(*req.ParentID, 32) {
			return nil, knowledgeInvalid("parent_id must be 32 lowercase hexadecimal characters")
		}
		parentID := *req.ParentID
		depth := 1
		for parentID != "" {
			depth++
			if depth > 16 {
				return nil, knowledgeInvalid("Group depth exceeds 16")
			}
			parent, err := d.knowledgeGroup(ctx, kb.ID, parentID)
			if err != nil {
				return nil, err
			}
			parentID = ""
			if parent.ParentID != nil {
				parentID = *parent.ParentID
			}
		}
	}
	group := &entity.KnowledgeGroup{ID: common.GenerateUUID(), DomainID: kb.ID, ParentID: req.ParentID, Kind: req.Kind, Name: name, ComponentSide: req.ComponentSide}
	if err := dao.DB.WithContext(ctx).Create(group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

func (d *DatasetService) ListKnowledgeGroups(ctx context.Context, userID, domainID, parentID, cursor string, limit int) (*entity.KnowledgePage[entity.KnowledgeGroup], error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	query := dao.DB.WithContext(ctx).Model(&entity.KnowledgeGroup{}).Where("domain_id = ?", kb.ID)
	if parentID == "" {
		query = query.Where("parent_id IS NULL")
	} else {
		if _, err := d.knowledgeGroup(ctx, kb.ID, parentID); err != nil {
			return nil, err
		}
		query = query.Where("parent_id = ?", parentID)
	}
	scope := knowledgeCursor{DomainID: kb.ID, Kind: "groups", Parent: parentID}
	query, err = knowledgePageQuery(query, cursor, scope, limit)
	if err != nil {
		return nil, err
	}
	items := []entity.KnowledgeGroup{}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	page := &entity.KnowledgePage[entity.KnowledgeGroup]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = knowledgeNextCursor(scope, items[limit-1].ID)
	}
	return page, nil
}

func (d *DatasetService) CheckKnowledgePolicy(ctx context.Context, userID, domainID, purpose string) (*entity.KnowledgePolicyDecision, error) {
	kb, err := d.knowledgeBase(ctx, userID, domainID, true)
	if err != nil {
		return nil, err
	}
	if !permission.IsKnowledgeModelPurpose(purpose) {
		return nil, knowledgeInvalid("Unsupported model purpose")
	}
	return &entity.KnowledgePolicyDecision{Purpose: purpose, Allowed: false, Reason: "domain_model_calls_disabled", PolicyVersion: kb.KnowledgePolicyVersion, ContractVersion: entity.KnowledgeContractVersion}, nil
}

func knowledgeDBError(err error, action string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return knowledgeNotFound()
	}
	return fmt.Errorf("knowledge %s failed: %w", action, err)
}
