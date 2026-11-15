package dao

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/entity"
)

var ErrKnowledgeAccessScopeChanged = errors.New("native authorization scope changed")

type KnowledgeAccessNativeMutation struct {
	ActorUserID *string
	Action      string
	UserIDs     []string
	TenantIDs   []string
	DomainIDs   []string
}
type knowledgeAccessNativeScope struct{ Tenants map[string]int64 }

const knowledgeAccessNativeScopeSetting = "knowledge_access_native_locked_scope"

func CheckKnowledgeAccessNativeUser(tx *gorm.DB, id string) error {
	var user entity.User
	return tx.Select("id").Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", id, "1", "1", "1", "0").Take(&user).Error
}
func CheckKnowledgeAccessNativeTenant(tx *gorm.DB, id string) error {
	var tenant entity.Tenant
	return tx.Select("id").Where("id = ? AND status = ?", id, "1").Take(&tenant).Error
}

func knowledgeAccessUnique(ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
func knowledgeAccessDiscover(tx *gorm.DB, mutation KnowledgeAccessNativeMutation) ([]string, error) {
	ids := append([]string(nil), mutation.TenantIDs...)
	users := knowledgeAccessUnique(mutation.UserIDs)
	if len(users) > 0 {
		for _, query := range []struct {
			model any
			where string
		}{
			{&entity.UserTenant{}, "user_id IN ?"},
			{&entity.Knowledgebase{}, "created_by IN ?"},
			{&entity.KnowledgeAccessGrantRecord{}, "issued_by IN ? OR (subject_kind = 'user' AND subject_id IN ?)"},
			{&entity.KnowledgeAccessViewRecord{}, "subject_user_id IN ?"},
		} {
			var found []string
			q := tx.Model(query.model)
			if _, ok := query.model.(*entity.KnowledgeAccessGrantRecord); ok {
				q = q.Where(query.where, users, users)
			} else {
				q = q.Where(query.where, users)
			}
			if err := q.Distinct("tenant_id").Pluck("tenant_id", &found).Error; err != nil {
				return nil, err
			}
			ids = append(ids, found...)
		}
	}
	if len(mutation.DomainIDs) > 0 {
		for _, model := range []any{&entity.Knowledgebase{}, &entity.KnowledgeAccessModuleRecord{}} {
			var found []string
			column := "domain_id"
			if _, ok := model.(*entity.Knowledgebase); ok {
				column = "id"
			}
			if err := tx.Model(model).Where(column+" IN ?", mutation.DomainIDs).Distinct("tenant_id").Pluck("tenant_id", &found).Error; err != nil {
				return nil, err
			}
			ids = append(ids, found...)
		}
	}
	return knowledgeAccessUnique(ids), nil
}

type knowledgeAccessUserAuthority struct {
	ID              string
	Status          *string
	IsActive        string
	IsAuthenticated string
	IsAnonymous     string
	IsSuperuser     *bool
}
type knowledgeAccessMemberAuthority struct {
	ID, UserID, TenantID, Role string
	Status                     *string
}
type knowledgeAccessTenantAuthority struct {
	ID     string
	Status *string
}
type knowledgeAccessDomainAuthority struct {
	ID, TenantID, CreatedBy, Permission string
	Status                              *string
	KnowledgeEnabled                    bool
	KnowledgePolicyVersion              int64
}
type knowledgeAccessNativeSnapshot struct {
	Users      []knowledgeAccessUserAuthority
	Members    []knowledgeAccessMemberAuthority
	Tenants    []knowledgeAccessTenantAuthority
	Domains    []knowledgeAccessDomainAuthority
	References []knowledgeAccessUserReference `json:"-"`
}
type knowledgeAccessUserReference struct{ UserID, TenantID string }

func knowledgeAccessSnapshot(tx *gorm.DB, mutation KnowledgeAccessNativeMutation, tenants []string, lock bool) (knowledgeAccessNativeSnapshot, error) {
	var value knowledgeAccessNativeSnapshot
	query := func() *gorm.DB {
		q := tx.Session(&gorm.Session{})
		if lock {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		return q
	}
	users := knowledgeAccessUnique(mutation.UserIDs)
	if len(users) > 0 {
		if err := query().Model(&entity.User{}).Select("id", "status", "is_active", "is_authenticated", "is_anonymous", "is_superuser").Where("id IN ?", users).Order("id ASC").Find(&value.Users).Error; err != nil {
			return value, err
		}
		var grants []entity.KnowledgeAccessGrantRecord
		if err := query().Select("tenant_id", "issued_by", "subject_kind", "subject_id").Where("issued_by IN ? OR (subject_kind = ? AND subject_id IN ?)", users, "user", users).Order("tenant_id ASC, id ASC").Find(&grants).Error; err != nil {
			return value, err
		}
		for _, row := range grants {
			value.References = append(value.References, knowledgeAccessUserReference{UserID: row.IssuedBy, TenantID: row.TenantID})
			if row.SubjectKind == "user" {
				value.References = append(value.References, knowledgeAccessUserReference{UserID: row.SubjectID, TenantID: row.TenantID})
			}
		}
		var views []entity.KnowledgeAccessViewRecord
		if err := query().Select("tenant_id", "subject_user_id").Where("subject_user_id IN ?", users).Order("tenant_id ASC, id ASC").Find(&views).Error; err != nil {
			return value, err
		}
		for _, row := range views {
			value.References = append(value.References, knowledgeAccessUserReference{UserID: row.SubjectUserID, TenantID: row.TenantID})
		}
	}
	if len(tenants) > 0 {
		if err := query().Model(&entity.Tenant{}).Select("id", "status").Where("id IN ?", tenants).Order("id ASC").Find(&value.Tenants).Error; err != nil {
			return value, err
		}
		if err := query().Model(&entity.UserTenant{}).Select("id", "user_id", "tenant_id", "role", "status").Where("tenant_id IN ?", tenants).Order("tenant_id ASC, id ASC").Find(&value.Members).Error; err != nil {
			return value, err
		}
		if err := query().Model(&entity.Knowledgebase{}).Select("id", "tenant_id", "created_by", "permission", "status", "knowledge_enabled", "knowledge_policy_version").Where("tenant_id IN ?", tenants).Order("tenant_id ASC, id ASC").Find(&value.Domains).Error; err != nil {
			return value, err
		}
	}
	return value, nil
}

// A changed row has the same ID in both definitions; a nil definition records
// absence explicitly for creation/deletion. Only authorization fields are held.
type knowledgeAccessChangedRow[T any] struct {
	ID         string `json:"id"`
	Definition *T     `json:"definition"`
}
type knowledgeAccessNativeDefinition struct {
	TenantID       string                                                      `json:"tenant_id"`
	UserIDs        []string                                                    `json:"user_ids"`
	Users          []knowledgeAccessChangedRow[knowledgeAccessUserAuthority]   `json:"users"`
	Members        []knowledgeAccessChangedRow[knowledgeAccessMemberAuthority] `json:"members"`
	Tenants        []knowledgeAccessChangedRow[knowledgeAccessTenantAuthority] `json:"tenants"`
	Domains        []knowledgeAccessChangedRow[knowledgeAccessDomainAuthority] `json:"domains"`
	SnapshotSHA256 string                                                      `json:"scope_snapshot_sha256"`
}

func knowledgeAccessChangedRows[T any](before, after []T, key func(T) string) ([]knowledgeAccessChangedRow[T], []knowledgeAccessChangedRow[T]) {
	oldRows, newRows := map[string]T{}, map[string]T{}
	ids := []string{}
	for _, row := range before {
		oldRows[key(row)] = row
		ids = append(ids, key(row))
	}
	for _, row := range after {
		newRows[key(row)] = row
		ids = append(ids, key(row))
	}
	oldDelta, newDelta := []knowledgeAccessChangedRow[T]{}, []knowledgeAccessChangedRow[T]{}
	for _, id := range knowledgeAccessUnique(ids) {
		oldRow, had := oldRows[id]
		newRow, has := newRows[id]
		if had == has && reflect.DeepEqual(oldRow, newRow) {
			continue
		}
		oldChange, newChange := knowledgeAccessChangedRow[T]{ID: id}, knowledgeAccessChangedRow[T]{ID: id}
		if had {
			oldChange.Definition = &oldRow
		}
		if has {
			newChange.Definition = &newRow
		}
		oldDelta = append(oldDelta, oldChange)
		newDelta = append(newDelta, newChange)
	}
	return oldDelta, newDelta
}

func knowledgeAccessTenantSnapshot(value knowledgeAccessNativeSnapshot, tenant string, relevant map[string]bool) knowledgeAccessNativeSnapshot {
	scoped := knowledgeAccessNativeSnapshot{}
	for _, row := range value.Members {
		if row.TenantID == tenant {
			scoped.Members = append(scoped.Members, row)
			relevant[row.UserID] = true
		}
	}
	for _, row := range value.Tenants {
		if row.ID == tenant {
			scoped.Tenants = append(scoped.Tenants, row)
		}
	}
	for _, row := range value.Domains {
		if row.TenantID == tenant {
			scoped.Domains = append(scoped.Domains, row)
			relevant[row.CreatedBy] = true
		}
	}
	for _, ref := range value.References {
		if ref.TenantID == tenant {
			relevant[ref.UserID] = true
		}
	}
	return scoped
}

func knowledgeAccessNativeDefinitions(before, after knowledgeAccessNativeSnapshot, tenant string) (string, string, error) {
	// Union relevance prevents deleting a membership from falsely recording its
	// still-existing user as deleted on the after side.
	relevant := map[string]bool{}
	oldScope := knowledgeAccessTenantSnapshot(before, tenant, relevant)
	newScope := knowledgeAccessTenantSnapshot(after, tenant, relevant)
	for _, row := range before.Users {
		if relevant[row.ID] {
			oldScope.Users = append(oldScope.Users, row)
		}
	}
	for _, row := range after.Users {
		if relevant[row.ID] {
			newScope.Users = append(newScope.Users, row)
		}
	}
	oldDefinition, newDefinition := knowledgeAccessNativeDefinition{TenantID: tenant}, knowledgeAccessNativeDefinition{TenantID: tenant}
	oldDefinition.Users, newDefinition.Users = knowledgeAccessChangedRows(oldScope.Users, newScope.Users, func(row knowledgeAccessUserAuthority) string { return row.ID })
	oldDefinition.Members, newDefinition.Members = knowledgeAccessChangedRows(oldScope.Members, newScope.Members, func(row knowledgeAccessMemberAuthority) string { return row.ID })
	oldDefinition.Tenants, newDefinition.Tenants = knowledgeAccessChangedRows(oldScope.Tenants, newScope.Tenants, func(row knowledgeAccessTenantAuthority) string { return row.ID })
	oldDefinition.Domains, newDefinition.Domains = knowledgeAccessChangedRows(oldScope.Domains, newScope.Domains, func(row knowledgeAccessDomainAuthority) string { return row.ID })
	if len(oldDefinition.Users)+len(oldDefinition.Members)+len(oldDefinition.Tenants)+len(oldDefinition.Domains) == 0 {
		return "", "", nil
	}
	users := []string{}
	for _, row := range oldDefinition.Users {
		users = append(users, row.ID)
	}
	for _, side := range []knowledgeAccessNativeDefinition{oldDefinition, newDefinition} {
		for _, row := range side.Members {
			if row.Definition != nil {
				users = append(users, row.Definition.UserID)
			}
		}
		for _, row := range side.Domains {
			if row.Definition != nil {
				users = append(users, row.Definition.CreatedBy)
			}
		}
	}
	oldDefinition.UserIDs, newDefinition.UserIDs = knowledgeAccessUnique(users), knowledgeAccessUnique(users)
	digest := func(value any) string {
		raw, _ := json.Marshal(value)
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:])
	}
	oldDefinition.SnapshotSHA256, newDefinition.SnapshotSHA256 = digest(oldScope), digest(newScope)
	oldJSON, err := json.Marshal(oldDefinition)
	if err != nil {
		return "", "", err
	}
	newJSON, err := json.Marshal(newDefinition)
	if err != nil {
		return "", "", err
	}
	// Background is summarized, but changed targets and facts are never reduced
	// to a digest. Oversized actual deltas abort the owning SQL transaction.
	if len(oldJSON) > 32768 || len(newJSON) > 32768 {
		return "", "", errors.New("native authorization audit definition exceeds capacity")
	}
	return string(oldJSON), string(newJSON), nil
}

// WithKnowledgeAccessNativeMutation is called at actual native orchestration
// owners, including direct writers and deletion cascades. SQL changes, epochs
// and safe audit commit together; external ES/storage cleanup stays outside.
func WithKnowledgeAccessNativeMutation(ctx context.Context, db *gorm.DB, mutation KnowledgeAccessNativeMutation, fn func(*gorm.DB) error) error {
	switch mutation.Action {
	case "native_user_create", "native_user_flags", "native_user_delete", "native_membership", "native_domain", "native_tenant_settings":
	default:
		return fmt.Errorf("invalid native access action")
	}
	if existing, ok := db.Get(knowledgeAccessNativeScopeSetting); ok {
		scope := existing.(*knowledgeAccessNativeScope)
		ids, err := knowledgeAccessDiscover(db, mutation)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := scope.Tenants[id]; !ok {
				return ErrKnowledgeAccessScopeChanged
			}
		}
		return fn(db)
	}
	for attempt := 0; attempt < 3; attempt++ {
		err := KnowledgeAccessTransaction(ctx, db, func(tx *gorm.DB) error {
			ids, err := knowledgeAccessDiscover(tx, mutation)
			if err != nil {
				return err
			}
			epochs, err := LockKnowledgeAccessTenants(tx, ids, false)
			if err != nil {
				return err
			}
			before, err := knowledgeAccessSnapshot(tx, mutation, ids, true)
			if err != nil {
				return err
			}
			again, err := knowledgeAccessDiscover(tx, mutation)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(ids, again) {
				return ErrKnowledgeAccessScopeChanged
			}
			if mutation.ActorUserID != nil {
				if err := CheckKnowledgeAccessNativeUser(tx, *mutation.ActorUserID); err != nil {
					return err
				}
			}
			tx = tx.Set(knowledgeAccessNativeScopeSetting, &knowledgeAccessNativeScope{Tenants: epochs}).Session(&gorm.Session{})
			if err = fn(tx); err != nil {
				return err
			}
			after, err := knowledgeAccessSnapshot(tx, mutation, ids, false)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			for _, tenant := range ids {
				oldJSON, newJSON, definitionErr := knowledgeAccessNativeDefinitions(before, after, tenant)
				if definitionErr != nil {
					return definitionErr
				}
				if oldJSON == newJSON {
					continue
				}
				if err = BumpKnowledgeAccessEpoch(tx, tenant); err != nil {
					return err
				}
				digest := sha256.Sum256([]byte(newJSON))
				definition := hex.EncodeToString(digest[:])
				row := entity.KnowledgeAccessAuditRecord{ID: common.GenerateUUID(), TenantID: tenant, ActorUserID: mutation.ActorUserID, Action: mutation.Action, Outcome: "changed", RequestID: common.GenerateUUID(), DefinitionDigest: &definition, BeforeDefinitionJSON: &oldJSON, AfterDefinitionJSON: &newJSON, OccurredAt: now}
				if err = tx.Create(&row).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if !errors.Is(err, ErrKnowledgeAccessScopeChanged) {
			return err
		}
	}
	return ErrKnowledgeAccessScopeChanged
}

// LockKnowledgeAccessDomain enters the epoch-first order before the existing
// owner/core row locks. It does not authorize the caller or change a generation.
func LockKnowledgeAccessDomain(ctx context.Context, tx *gorm.DB, domain string) error {
	var kb entity.Knowledgebase
	if err := tx.WithContext(ctx).Select("id", "tenant_id").Where("id = ?", domain).Take(&kb).Error; err != nil {
		return err
	}
	if existing, ok := tx.Get(knowledgeAccessNativeScopeSetting); ok {
		if _, held := existing.(*knowledgeAccessNativeScope).Tenants[kb.TenantID]; !held {
			return ErrKnowledgeAccessScopeChanged
		}
		return nil
	}
	_, err := LockKnowledgeAccessTenants(tx, []string{kb.TenantID}, false)
	return err
}
