package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type KnowledgeAccessError struct {
	Status       int
	Code         string
	OutcomeKnown bool
	RetrySameKey bool
}

func (e *KnowledgeAccessError) Error() string {
	return "Knowledge access operation could not be completed"
}
func accessError(status int, code string) error {
	return &KnowledgeAccessError{Status: status, Code: code, OutcomeKnown: true}
}
func accessNormalize(err error) *KnowledgeAccessError {
	var access *KnowledgeAccessError
	if errors.As(err, &access) {
		return access
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return accessError(404, "not_found").(*KnowledgeAccessError)
	}
	var lexical *KnowledgeLexicalError
	var knowledge *KnowledgeError
	var document *DocumentOperationError
	status := 503
	if errors.As(err, &lexical) {
		status = lexical.Status
	}
	if errors.As(err, &knowledge) {
		status = knowledge.Status
	}
	if errors.As(err, &document) {
		status = document.Status
	}
	if status == 404 {
		return accessError(404, "not_found").(*KnowledgeAccessError)
	}
	if status == 409 {
		return accessError(409, "integrity_error").(*KnowledgeAccessError)
	}
	if status == 413 {
		return accessError(413, "capacity_exceeded").(*KnowledgeAccessError)
	}
	if status == 415 {
		return accessError(409, "scope_not_supported").(*KnowledgeAccessError)
	}
	return &KnowledgeAccessError{Status: 503, Code: "unavailable", RetrySameKey: true}
}

var accessRequestDefinitions = map[string]string{"setPolicy": "SetPolicyRequest", "setModule": "SetModuleRequest", "getModuleAdmin": "GetModuleAdminRequest", "listModules": "ListModulesRequest", "createGrant": "CreateGrantRequest", "revokeGrant": "RevokeGrantRequest", "inspect": "InspectRequest", "search": "SearchRequest", "read": "ReadRequest"}

func accessDefinition(operation string) string {
	name := accessRequestDefinitions[operation]
	return strings.TrimSuffix(name, "Request") + "Response"
}
func accessJSON(value any) string       { raw, _ := json.Marshal(value); return string(raw) }
func accessTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
func accessPtr[T any](value T) *T       { return &value }

type accessEnvelope struct {
	Operation       string `json:"operation"`
	ContractVersion string `json:"contract_version"`
	RequestID       string `json:"request_id"`
	Data            any    `json:"data"`
}

// KnowledgeAccess encodes and validates the complete bounded response before
// its final primary permit. No partially encoded data escapes this entry.
func (d *DatasetService) KnowledgeAccess(ctx context.Context, actor, operation, requestID string, raw []byte) (int, []byte) {
	ctx, cancel := context.WithTimeout(dao.KnowledgeAccessPrivateReadContext(ctx), 15*time.Second)
	defer cancel()
	status, body, err := d.knowledgeAccess(ctx, actor, operation, requestID, raw)
	if err == nil {
		return status, body
	}
	public := accessNormalize(err)
	response := entity.KnowledgeAccessError{Operation: operation, ContractVersion: entity.KnowledgeAccessVersion, RequestID: requestID, Code: public.Code, Message: "Knowledge access operation could not be completed", OutcomeKnown: public.OutcomeKnown, RetrySameKey: public.RetrySameKey}
	body, _ = json.Marshal(response)
	return public.Status, body
}

func accessEncode(operation, requestID string, data any) ([]byte, error) {
	raw, err := json.Marshal(accessEnvelope{Operation: operation, ContractVersion: entity.KnowledgeAccessVersion, RequestID: requestID, Data: data})
	if err != nil {
		return nil, accessError(503, "unavailable")
	}
	if len(raw) > 1<<20 {
		return nil, accessError(413, "capacity_exceeded")
	}
	if entity.KnowledgeAccessDecode(raw, accessDefinition(operation), nil) != nil {
		return nil, accessError(409, "integrity_error")
	}
	return raw, nil
}

func (d *DatasetService) knowledgeAccess(ctx context.Context, actor, operation, requestID string, raw []byte) (int, []byte, error) {
	if len(raw) > 32768 {
		return 0, nil, accessError(413, "capacity_exceeded")
	}
	definition, ok := accessRequestDefinitions[operation]
	if !ok {
		return 0, nil, accessError(400, "invalid_request")
	}
	var version struct {
		Version string `json:"contract_version"`
	}
	// Decode the version only after the duplicate/Unicode gate; a malformed
	// document cannot select a different error path through duplicate keys.
	var checked json.RawMessage
	if entity.KnowledgeParsedDecode(raw, &checked) != nil {
		return 0, nil, accessError(400, "invalid_request")
	}
	var request any
	switch operation {
	case "setPolicy":
		request = &entity.KnowledgeAccessSetPolicyRequest{}
	case "setModule":
		request = &entity.KnowledgeAccessSetModuleRequest{}
	case "getModuleAdmin":
		request = &entity.KnowledgeAccessGetModuleAdminRequest{}
	case "listModules":
		request = &entity.KnowledgeAccessListModulesRequest{}
	case "createGrant":
		request = &entity.KnowledgeAccessCreateGrantRequest{}
	case "revokeGrant":
		request = &entity.KnowledgeAccessRevokeGrantRequest{}
	case "inspect":
		request = &entity.KnowledgeAccessInspectRequest{}
	case "search":
		request = &entity.KnowledgeAccessSearchRequest{}
	case "read":
		request = &entity.KnowledgeAccessReadRequest{}
	}
	if json.Unmarshal(raw, &version) == nil && version.Version != "" && version.Version != entity.KnowledgeAccessVersion {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return 0, nil, accessError(400, "invalid_request")
		}
		object["contract_version"] = json.RawMessage(accessJSON(entity.KnowledgeAccessVersion))
		normalized, _ := json.Marshal(object)
		if entity.KnowledgeAccessDecode(normalized, definition, request) != nil {
			return 0, nil, accessError(400, "invalid_request")
		}
		return 0, nil, accessError(412, "contract_mismatch")
	}
	if entity.KnowledgeAccessDecode(raw, definition, request) != nil {
		return 0, nil, accessError(400, "invalid_request")
	}
	if err := accessSession(ctx, accessDB(ctx), actor, false); err != nil {
		return 0, nil, err
	}
	switch value := request.(type) {
	case *entity.KnowledgeAccessSetPolicyRequest:
		return d.accessCommand(ctx, actor, operation, requestID, value.DomainID, value.IdempotencyKey, value)
	case *entity.KnowledgeAccessSetModuleRequest:
		return d.accessCommand(ctx, actor, operation, requestID, value.DomainID, value.IdempotencyKey, value)
	case *entity.KnowledgeAccessCreateGrantRequest:
		return d.accessCommand(ctx, actor, operation, requestID, value.DomainID, value.IdempotencyKey, value)
	case *entity.KnowledgeAccessRevokeGrantRequest:
		return d.accessCommand(ctx, actor, operation, requestID, value.DomainID, value.IdempotencyKey, value)
	case *entity.KnowledgeAccessGetModuleAdminRequest:
		return d.accessGetModule(ctx, actor, requestID, *value)
	case *entity.KnowledgeAccessListModulesRequest:
		return d.accessListModules(ctx, actor, requestID, *value)
	case *entity.KnowledgeAccessInspectRequest:
		return d.accessInspect(ctx, actor, requestID, *value)
	case *entity.KnowledgeAccessSearchRequest:
		return d.accessSearch(ctx, actor, requestID, *value)
	case *entity.KnowledgeAccessReadRequest:
		return d.accessRead(ctx, actor, requestID, *value)
	}
	return 0, nil, accessError(400, "invalid_request")
}

func accessLocked(tx *gorm.DB, lock bool) *gorm.DB {
	if lock {
		return tx.Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	}
	return tx.Session(&gorm.Session{})
}
func accessSession(ctx context.Context, tx *gorm.DB, actor string, lock bool) error {
	if !isKnowledgeID(actor, 32) {
		return accessError(401, "unauthenticated")
	}
	session, ok := ctx.Value(lexicalSessionKey{}).(lexicalSession)
	if !ok || session.UserID != actor || len(session.SHA256) != 64 {
		return accessError(401, "unauthenticated")
	}
	var user entity.User
	if err := accessLocked(tx.WithContext(ctx), lock).Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", actor, "1", "1", "1", "0").Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return accessError(401, "unauthenticated")
		}
		return err
	}
	if user.AccessToken == nil || *user.AccessToken == "" || lexicalSHA([]byte(*user.AccessToken)) != session.SHA256 {
		return accessError(401, "unauthenticated")
	}
	return nil
}
func accessMembership(tx *gorm.DB, user, tenant string, lock bool) error {
	var rows []entity.UserTenant
	if err := accessLocked(tx, lock).Where("user_id = ? AND tenant_id = ? AND status = ?", user, tenant, "1").Order("id ASC").Limit(2).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return gorm.ErrRecordNotFound
	}
	if len(rows) != 1 {
		return accessError(409, "integrity_error")
	}
	switch rows[0].Role {
	case "owner", "admin", "normal":
		return nil
	}
	return gorm.ErrRecordNotFound
}
func accessTenant(ctx context.Context, tx *gorm.DB, actor, tenant string, lock bool) error {
	if !isKnowledgeID(tenant, 32) {
		return accessError(409, "scope_not_supported")
	}
	if err := accessSession(ctx, tx, actor, lock); err != nil {
		return err
	}
	var row entity.Tenant
	if err := accessLocked(tx, lock).Where("id = ? AND status = ?", tenant, "1").Take(&row).Error; err != nil {
		return err
	}
	return accessMembership(tx, actor, tenant, lock)
}
func accessDomain(ctx context.Context, tx *gorm.DB, actor, domain string, management, lock bool) (*entity.Knowledgebase, error) {
	var kb entity.Knowledgebase
	if err := accessLocked(tx, lock).Where("id = ? AND status = ? AND knowledge_enabled = ?", domain, "1", true).Take(&kb).Error; err != nil {
		return nil, err
	}
	if !isKnowledgeID(kb.CreatedBy, 32) {
		return nil, accessError(409, "scope_not_supported")
	}
	if management && kb.CreatedBy != actor {
		return nil, accessError(403, "action_denied")
	}
	if err := accessTenant(ctx, tx, actor, kb.TenantID, lock); err != nil {
		return nil, err
	}
	var owner entity.User
	if err := accessLocked(tx, lock).Select("id", "status", "is_active", "is_authenticated", "is_anonymous").Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", kb.CreatedBy, "1", "1", "1", "0").Take(&owner).Error; err != nil {
		return nil, err
	}
	if err := accessMembership(tx, kb.CreatedBy, kb.TenantID, lock); err != nil {
		return nil, err
	}
	if kb.Permission != "me" || kb.KnowledgePolicyVersion < 1 || kb.KnowledgePolicyVersion > entity.KnowledgeAccessMaxGeneration {
		return nil, accessError(409, "scope_not_supported")
	}
	return &kb, nil
}

func accessTenantLocator(ctx context.Context, domain string) (string, error) {
	var row entity.Knowledgebase
	if err := accessDB(ctx).Select("id", "tenant_id").Where("id = ?", domain).Take(&row).Error; err != nil {
		return "", err
	}
	return row.TenantID, nil
}
func accessPolicy(tx *gorm.DB, kb *entity.Knowledgebase, lock bool) (entity.KnowledgeAccessPolicy, error) {
	var row entity.KnowledgeAccessDomainPolicyRecord
	err := accessLocked(tx, lock).Where("domain_id = ?", kb.ID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entity.KnowledgeAccessPolicy{DomainID: kb.ID}, nil
	}
	if err != nil {
		return entity.KnowledgeAccessPolicy{}, err
	}
	if row.TenantID != kb.TenantID || row.Version < 1 || row.Version > entity.KnowledgeAccessMaxGeneration {
		return entity.KnowledgeAccessPolicy{}, accessError(409, "integrity_error")
	}
	return entity.KnowledgeAccessPolicy{DomainID: kb.ID, Version: row.Version, Enabled: row.Enabled}, nil
}
func accessModule(tx *gorm.DB, kb *entity.Knowledgebase, id string, lock bool) (*entity.KnowledgeAccessModuleRecord, entity.KnowledgeAccessModuleScope, error) {
	var row entity.KnowledgeAccessModuleRecord
	var scope entity.KnowledgeAccessModuleScope
	if err := accessLocked(tx, lock).Where("id = ? AND domain_id = ? AND tenant_id = ?", id, kb.ID, kb.TenantID).Take(&row).Error; err != nil {
		return nil, scope, err
	}
	if len(row.ScopeJSON) > 32768 || lexicalSHA([]byte(row.ScopeJSON)) != row.ScopeSHA256 || entity.KnowledgeAccessDecode([]byte(row.ScopeJSON), "ModuleScope", &scope) != nil || row.Version < 1 || row.Version > entity.KnowledgeAccessMaxGeneration {
		return nil, scope, accessError(409, "integrity_error")
	}
	return &row, scope, nil
}
func accessGrantDTO(row entity.KnowledgeAccessGrantRecord) entity.KnowledgeAccessGrant {
	var revoked *string
	if row.RevokedAt != nil {
		revoked = accessPtr(accessTime(*row.RevokedAt))
	}
	return entity.KnowledgeAccessGrant{GrantID: row.ID, DomainID: row.DomainID, ModuleID: row.ModuleID, ModuleVersion: row.ModuleVersion, Subject: entity.KnowledgeAccessSubject{Kind: row.SubjectKind, ID: row.SubjectID}, Mode: row.Mode, Delivery: row.Delivery, IssuedAt: accessTime(row.IssuedAt), ExpiresAt: accessTime(row.ExpiresAt), RevokedAt: revoked, IssuedByUserID: row.IssuedBy}
}
func accessGrants(tx *gorm.DB, kb *entity.Knowledgebase, module *entity.KnowledgeAccessModuleRecord, now time.Time) ([]entity.KnowledgeAccessGrantRecord, error) {
	var rows []entity.KnowledgeAccessGrantRecord
	err := tx.Where("tenant_id = ? AND domain_id = ? AND module_id = ? AND module_version = ? AND revoked_at IS NULL AND expires_at > ?", kb.TenantID, kb.ID, module.ID, module.Version, now).Order("issued_at DESC, id ASC").Limit(257).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > 256 {
		return nil, accessError(413, "capacity_exceeded")
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if err := accessGrantIntegrity(tx, row); err != nil {
			return nil, err
		}
		subject := row.SubjectKind + "/" + row.SubjectID
		if seen[subject] {
			return nil, accessError(409, "integrity_error")
		}
		seen[subject] = true
	}
	return rows, nil
}
func accessAdmin(tx *gorm.DB, kb *entity.Knowledgebase, module *entity.KnowledgeAccessModuleRecord, scope entity.KnowledgeAccessModuleScope, now time.Time) (*entity.KnowledgeAccessModuleAdmin, error) {
	policy, err := accessPolicy(tx, kb, true)
	if err != nil {
		return nil, err
	}
	rows, err := accessGrants(tx, kb, module, now)
	if err != nil {
		return nil, err
	}
	grants := make([]entity.KnowledgeAccessGrant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, accessGrantDTO(row))
	}
	return &entity.KnowledgeAccessModuleAdmin{ModuleID: module.ID, DomainID: kb.ID, Name: module.Name, Version: module.Version, Enabled: module.Enabled, Scope: scope, Grants: grants, Policy: policy}, nil
}
func accessCovers(grant entity.KnowledgeAccessGrantRecord, actor, mode, delivery string) bool {
	return grant.SubjectKind == "user" && grant.SubjectID == actor && (grant.Mode == mode || grant.Mode == "read_search" && mode == "read") && (grant.Delivery == delivery || grant.Delivery == "web_and_mcp" && delivery == "web_only")
}
func accessPickGrant(tx *gorm.DB, kb *entity.Knowledgebase, module *entity.KnowledgeAccessModuleRecord, actor, mode, delivery string, now time.Time) (*entity.KnowledgeAccessGrantRecord, error) {
	if !module.Enabled {
		return nil, accessError(403, "action_denied")
	}
	policy, err := accessPolicy(tx, kb, true)
	if err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, accessError(403, "action_denied")
	}
	rows, err := accessGrants(tx, kb, module, now)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.IssuedBy == kb.CreatedBy && accessCovers(row, actor, mode, delivery) {
			return &row, nil
		}
	}
	return nil, accessError(403, "action_denied")
}

func accessCommandDigest(actor, operation string, request any) []byte {
	raw, _ := json.Marshal([]any{"knowledge-access-command/0.3.0", actor, operation, request})
	digest := sha256.Sum256(raw)
	return digest[:]
}
func accessReceipt(ctx context.Context, actor, operation, key string) (*entity.KnowledgeAccessAuditRecord, error) {
	var row entity.KnowledgeAccessAuditRecord
	err := accessDB(ctx).Where("actor_user_id = ? AND command_operation = ? AND command_key = ?", actor, operation, key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (d *DatasetService) accessReplay(ctx context.Context, actor, operation, requestID, domain string, digest []byte, row *entity.KnowledgeAccessAuditRecord) (int, []byte, error) {
	if row.CommandScopeID == nil || *row.CommandScopeID != domain || !bytes.Equal(digest, row.CommandRequestSHA256) {
		return 0, nil, accessError(409, "idempotency_conflict")
	}
	tenant, err := accessTenantLocator(ctx, domain)
	if err != nil {
		return 0, nil, err
	}
	var encoded []byte
	err = dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		if _, err := dao.LockKnowledgeAccessTenants(tx, []string{tenant}, true); err != nil {
			return err
		}
		kb, err := accessDomain(ctx, tx, actor, domain, true, true)
		if err != nil {
			return err
		}
		if kb.TenantID != tenant || row.TenantID != tenant {
			return accessError(409, "scope_changed")
		}
		if row.CommandCompactedAt != nil || row.CommandState == nil || *row.CommandState != "completed" || row.CommandResultJSON == nil || row.CommandHTTPStatus == nil {
			return accessError(409, "idempotency_conflict")
		}
		var data json.RawMessage = []byte(*row.CommandResultJSON)
		encoded, err = accessEncode(operation, requestID, data)
		return err
	})
	return 200, encoded, err
}

func accessNow(tx *gorm.DB) (time.Time, error) {
	now, err := batchNow(tx)
	return now.UTC().Truncate(time.Millisecond), err
}
func accessAudit(tx *gorm.DB, actor, operation, requestID string, kb *entity.Knowledgebase, module string, outcome string) error {
	now, err := accessNow(tx)
	if err != nil {
		return err
	}
	row := entity.KnowledgeAccessAuditRecord{ID: common.GenerateUUID(), TenantID: kb.TenantID, ActorUserID: &actor, DomainID: accessPtr(kb.ID), Action: operation, Outcome: outcome, RequestID: requestID, OccurredAt: now}
	if module != "" {
		row.ModuleID = &module
	}
	return tx.Create(&row).Error
}

func accessSortTargets(targets []entity.KnowledgeAccessTarget) {
	sort.Slice(targets, func(i, j int) bool {
		a, b := targets[i], targets[j]
		left := []string{a.Source.DomainID, a.Source.SourceID, a.Source.SourceRevisionID, a.Source.SourceSHA256, a.Representation.Kind, a.Representation.ID}
		right := []string{b.Source.DomainID, b.Source.SourceID, b.Source.SourceRevisionID, b.Source.SourceSHA256, b.Representation.Kind, b.Representation.ID}
		if a.Representation.ProducerSHA256 != nil {
			left = append(left, *a.Representation.ProducerSHA256)
		}
		if b.Representation.ProducerSHA256 != nil {
			right = append(right, *b.Representation.ProducerSHA256)
		}
		return strings.Join(left, "\x00") < strings.Join(right, "\x00")
	})
}

func accessDB(ctx context.Context) *gorm.DB {
	return dao.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}
