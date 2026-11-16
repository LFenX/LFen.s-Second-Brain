package dataset

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func (d *DatasetService) accessCommand(ctx context.Context, actor, operation, requestID, scopeID, key string, request any) (int, []byte, error) {
	digest := accessCommandDigest(actor, operation, request)
	receipt, err := accessReceipt(ctx, actor, operation, key)
	if err != nil {
		return 0, nil, err
	}
	if receipt != nil {
		return d.accessReplay(ctx, actor, operation, requestID, scopeID, digest, receipt)
	}
	tenant, scopeKind, err := accessCommandTenant(ctx, operation, scopeID)
	if err != nil {
		return 0, nil, err
	}
	var options *sql.TxOptions
	if dao.DB.Dialector.Name() != "sqlite" {
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	tx := accessDB(ctx).Begin(options)
	if tx.Error != nil {
		return 0, nil, tx.Error
	}
	defer tx.Rollback()
	now, err := accessNow(tx)
	if err != nil {
		return 0, nil, err
	}
	commandID := common.GenerateUUID()
	pending := "pending"
	domain := scopeID
	if scopeKind == "tenant" {
		domain = ""
	}
	journal := entity.KnowledgeAccessAuditRecord{ID: commandID, ActorUserID: &actor, Action: operation, Outcome: "changed", RequestID: requestID, OccurredAt: now, CommandOperation: &operation, CommandKey: &key, CommandRequestSHA256: digest, CommandScopeKind: &scopeKind, CommandScopeID: &scopeID, CommandState: &pending}
	if err = tx.Create(&journal).Error; err != nil {
		tx.Rollback()
		winner, readErr := accessReceipt(ctx, actor, operation, key)
		if readErr != nil {
			return 0, nil, readErr
		}
		if winner != nil {
			return d.accessReplay(ctx, actor, operation, requestID, scopeID, digest, winner)
		}
		return 0, nil, err
	}
	if _, err = dao.LockKnowledgeAccessTenants(tx, []string{tenant}, false); err != nil {
		return 0, nil, err
	}
	kb, err := accessCommandAuthority(ctx, tx, actor, operation, scopeID, tenant)
	if err != nil {
		return 0, nil, err
	}
	now, err = accessNow(tx)
	if err != nil {
		return 0, nil, err
	}
	proofCtx := accessProofContext(ctx, tx)
	var data, before, after any
	var resource, module string
	changed := true
	status := 200
	outcome := "changed"
	switch value := request.(type) {
	case *entity.KnowledgeAccessSetDepartmentRequest:
		department, old, wasChanged, setErr := accessSetDepartment(tx, actor, now, value)
		if setErr != nil {
			return 0, nil, setErr
		}
		data, after, resource, changed = department, department, department.DepartmentID, wasChanged
		if old != nil {
			before = old
		}
	case *entity.KnowledgeAccessSetPolicyRequest:
		old, readErr := accessPolicy(tx, kb, true)
		if readErr != nil {
			return 0, nil, readErr
		}
		if old.Version != value.ExpectedVersion {
			return 0, nil, accessError(409, "version_conflict")
		}
		if old.Version > 0 && old.Enabled == value.Enabled {
			data, before, after, resource = old, old, old, domain
			changed = false
			break
		}
		if old.Version >= entity.KnowledgeAccessMaxGeneration {
			return 0, nil, accessError(503, "unavailable")
		}
		row := entity.KnowledgeAccessDomainPolicyRecord{DomainID: domain, TenantID: tenant, Version: old.Version + 1, Enabled: value.Enabled, UpdatedBy: actor, UpdatedAt: now}
		if old.Version == 0 {
			err = tx.Create(&row).Error
		} else {
			result := tx.Model(&entity.KnowledgeAccessDomainPolicyRecord{}).Where("domain_id = ? AND version = ?", domain, old.Version).Updates(map[string]any{"version": row.Version, "enabled": row.Enabled, "updated_by": actor, "updated_at": now})
			err = result.Error
			if err == nil && result.RowsAffected != 1 {
				err = accessError(409, "version_conflict")
			}
		}
		if err != nil {
			return 0, nil, err
		}
		data = entity.KnowledgeAccessPolicy{DomainID: domain, Version: row.Version, Enabled: row.Enabled}
		before, after, resource = old, data, domain
	case *entity.KnowledgeAccessSetModuleRequest:
		if value.ModuleID == nil && value.ExpectedVersion != 0 || value.ModuleID != nil && value.ExpectedVersion < 1 {
			return 0, nil, accessError(400, "invalid_request")
		}
		if len(accessJSON(value.Scope)) > 32768 {
			return 0, nil, accessError(413, "capacity_exceeded")
		}
		if _, err = d.accessPublication(proofCtx, tx, kb, value.Scope, true); err != nil {
			return 0, nil, err
		}
		var row entity.KnowledgeAccessModuleRecord
		if value.ModuleID == nil {
			var count int64
			if err = tx.Model(&row).Where("domain_id = ?", domain).Count(&count).Error; err != nil {
				return 0, nil, err
			}
			if count >= 64 {
				return 0, nil, accessError(413, "capacity_exceeded")
			}
			row = entity.KnowledgeAccessModuleRecord{ID: common.GenerateUUID(), DomainID: domain, TenantID: tenant, CreatedBy: actor, CreatedAt: now, Version: 1}
		} else {
			old, oldScope, readErr := accessModule(tx, kb, *value.ModuleID, true)
			if readErr != nil {
				return 0, nil, readErr
			}
			if old.Version != value.ExpectedVersion {
				return 0, nil, accessError(409, "version_conflict")
			}
			if old.Name == value.Name && old.Enabled == value.Enabled && old.ScopeJSON == accessJSON(value.Scope) {
				data, err = accessAdmin(tx, kb, old, oldScope, now)
				if err != nil {
					return 0, nil, err
				}
				before, after, resource, module = accessModuleDefinition(old, oldScope), accessModuleDefinition(old, oldScope), old.ID, old.ID
				changed = false
				break
			}
			if old.Version >= entity.KnowledgeAccessMaxGeneration {
				return 0, nil, accessError(503, "unavailable")
			}
			row = *old
			before = accessModuleDefinition(old, oldScope)
			row.Version++
		}
		row.Name, row.Enabled, row.ScopeJSON, row.ScopeSHA256, row.UpdatedAt = value.Name, value.Enabled, accessJSON(value.Scope), lexicalSHA([]byte(accessJSON(value.Scope))), now
		if value.ModuleID == nil {
			err = tx.Create(&row).Error
		} else {
			result := tx.Model(&entity.KnowledgeAccessModuleRecord{}).Where("id = ? AND domain_id = ? AND version = ?", row.ID, domain, value.ExpectedVersion).Updates(map[string]any{"name": row.Name, "enabled": row.Enabled, "version": row.Version, "scope_json": row.ScopeJSON, "scope_sha256": row.ScopeSHA256, "updated_at": now})
			err = result.Error
			if err == nil && result.RowsAffected != 1 {
				err = accessError(409, "version_conflict")
			}
		}
		if err != nil {
			return 0, nil, err
		}
		data, err = accessAdmin(tx, kb, &row, value.Scope, now)
		if err != nil {
			return 0, nil, err
		}
		after = accessModuleDefinition(&row, value.Scope)
		resource, module = row.ID, row.ID
	case *entity.KnowledgeAccessCreateGrantRequest:
		if value.Subject.Kind == "user" {
			if err = accessLiveMember(tx, value.Subject.ID, tenant); err != nil {
				return 0, nil, err
			}
		} else {
			department, readErr := accessDepartment(tx, tenant, value.Subject.ID)
			if readErr != nil {
				return 0, nil, readErr
			}
			if !department.Enabled {
				return 0, nil, accessError(404, "not_found")
			}
		}
		row, scope, readErr := accessModule(tx, kb, value.ModuleID, true)
		if readErr != nil {
			return 0, nil, readErr
		}
		if row.Version != value.ExpectedModuleVersion {
			return 0, nil, accessError(409, "version_conflict")
		}
		if _, err = d.accessPublication(proofCtx, tx, kb, scope, true); err != nil {
			return 0, nil, err
		}
		expires, parseErr := time.Parse(time.RFC3339Nano, value.ExpiresAt)
		if parseErr != nil || !expires.After(now) || expires.After(now.Add(30*24*time.Hour)) {
			return 0, nil, accessError(400, "invalid_request")
		}
		grants, readErr := accessGrants(tx, kb, row, now)
		if readErr != nil {
			return 0, nil, readErr
		}
		if len(grants) >= 256 {
			return 0, nil, accessError(413, "capacity_exceeded")
		}
		for _, grant := range grants {
			if grant.SubjectKind == value.Subject.Kind && grant.SubjectID == value.Subject.ID {
				return 0, nil, accessError(409, "version_conflict")
			}
		}
		grant := entity.KnowledgeAccessGrantRecord{ID: common.GenerateUUID(), TenantID: tenant, DomainID: domain, ModuleID: row.ID, SubjectKind: value.Subject.Kind, SubjectID: value.Subject.ID, Mode: value.Mode, Delivery: value.Delivery, IssuedBy: actor, DefinitionJSON: accessJSON(value), IssuedAt: now, ExpiresAt: expires.UTC().Truncate(time.Millisecond), ModuleVersion: row.Version, CreateCommandID: commandID}
		if err = tx.Omit("CreationReceipt").Create(&grant).Error; err != nil {
			return 0, nil, err
		}
		data = accessGrantDTO(grant)
		after = data
		resource, module, status = grant.ID, row.ID, 201
	case *entity.KnowledgeAccessRevokeGrantRequest:
		var grant entity.KnowledgeAccessGrantRecord
		if err = accessLocked(tx, true).Where("id = ? AND domain_id = ? AND tenant_id = ?", value.GrantID, domain, tenant).Take(&grant).Error; err != nil {
			return 0, nil, err
		}
		before = accessGrantDTO(grant)
		if grant.RevokedAt == nil {
			result := tx.Model(&entity.KnowledgeAccessGrantRecord{}).Where("id = ? AND revoked_at IS NULL", grant.ID).Update("revoked_at", now)
			if result.Error != nil {
				return 0, nil, result.Error
			}
			if result.RowsAffected != 1 {
				return 0, nil, accessError(409, "version_conflict")
			}
			grant.RevokedAt = &now
		} else {
			changed = false
		}
		data = entity.KnowledgeAccessRevoked{ID: grant.ID, RevokedAt: accessTime(*grant.RevokedAt)}
		after = accessGrantDTO(grant)
		resource, module, outcome = grant.ID, grant.ModuleID, "revoked"
	default:
		return 0, nil, accessError(400, "invalid_request")
	}
	now, err = accessNow(tx)
	if err != nil {
		return 0, nil, err
	}
	if request, ok := request.(*entity.KnowledgeAccessCreateGrantRequest); ok {
		expiry, _ := time.Parse(time.RFC3339Nano, request.ExpiresAt)
		if !expiry.UTC().Truncate(time.Millisecond).After(now) {
			return 0, nil, accessError(400, "invalid_request")
		}
	}
	encoded, err := accessEncode(operation, requestID, data)
	if err != nil {
		return 0, nil, err
	}
	if changed {
		if err = dao.BumpKnowledgeAccessEpoch(tx, tenant); err != nil {
			return 0, nil, err
		}
	}
	safeResult := accessJSON(data)
	completed := "completed"
	updates := map[string]any{"tenant_id": tenant, "domain_id": domain, "outcome": outcome, "command_state": completed, "command_resource_id": resource, "command_http_status": status, "command_result_json": safeResult, "command_committed_at": now, "occurred_at": now}
	if scopeKind == "tenant" {
		updates["domain_id"] = nil
	}
	if module != "" {
		updates["module_id"] = module
	}
	if before != nil {
		updates["before_definition_json"] = accessJSON(before)
	}
	if after != nil {
		definition := accessJSON(after)
		updates["after_definition_json"] = definition
		updates["definition_digest"] = lexicalSHA([]byte(definition))
	}
	result := tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("id = ? AND command_state = ?", commandID, pending).Updates(updates)
	if result.Error != nil {
		return 0, nil, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, nil, accessError(409, "integrity_error")
	}
	if err = tx.Commit().Error; err != nil {
		return 0, nil, &KnowledgeAccessError{Status: 503, Code: "unavailable", OutcomeKnown: false, RetrySameKey: true}
	}
	return status, encoded, nil
}

func accessModuleDefinition(row *entity.KnowledgeAccessModuleRecord, scope entity.KnowledgeAccessModuleScope) any {
	return struct {
		ModuleID string                            `json:"module_id"`
		Name     string                            `json:"name"`
		Version  int64                             `json:"version"`
		Enabled  bool                              `json:"enabled"`
		Scope    entity.KnowledgeAccessModuleScope `json:"scope"`
	}{row.ID, row.Name, row.Version, row.Enabled, scope}
}

func (d *DatasetService) accessGetModule(ctx context.Context, actor, requestID string, req entity.KnowledgeAccessGetModuleAdminRequest) (int, []byte, error) {
	tenant, err := accessTenantLocator(ctx, req.DomainID)
	if err != nil {
		return 0, nil, err
	}
	var encoded []byte
	err = dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		if _, err := dao.LockKnowledgeAccessTenants(tx, []string{tenant}, true); err != nil {
			return err
		}
		kb, err := accessDomain(ctx, tx, actor, req.DomainID, true, true)
		if err != nil {
			return err
		}
		if kb.TenantID != tenant {
			return accessError(409, "scope_changed")
		}
		module, scope, err := accessModule(tx, kb, req.ModuleID, true)
		if err != nil {
			return err
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		data, err := accessAdmin(tx, kb, module, scope, now)
		if err != nil {
			return err
		}
		encoded, err = accessEncode("getModuleAdmin", requestID, data)
		if err != nil {
			return err
		}
		return accessAudit(tx, actor, "getModuleAdmin", requestID, kb, module.ID, "allowed")
	})
	return 200, encoded, err
}

func (d *DatasetService) accessListModules(ctx context.Context, actor, requestID string, req entity.KnowledgeAccessListModulesRequest) (int, []byte, error) {
	var encoded []byte
	err := dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		if _, err := dao.LockKnowledgeAccessTenants(tx, []string{req.TenantID}, true); err != nil {
			return err
		}
		if err := accessTenant(ctx, tx, actor, req.TenantID, true); err != nil {
			return err
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		var domains []entity.Knowledgebase
		q := tx.Where("tenant_id = ? AND status = ? AND knowledge_enabled = ?", req.TenantID, "1", true)
		if req.DomainID != nil {
			q = q.Where("id = ?", *req.DomainID)
		}
		if err = q.Order("id ASC").Find(&domains).Error; err != nil {
			return err
		}
		data := entity.KnowledgeAccessListModulesData{Items: []entity.KnowledgeAccessModuleSummary{}, Pagination: "none", OwnedDomainPolicies: []entity.KnowledgeAccessPolicy{}}
		for _, domain := range domains {
			kb, readErr := accessDomain(ctx, tx, actor, domain.ID, false, true)
			if readErr != nil {
				if errors.Is(readErr, gorm.ErrRecordNotFound) {
					continue
				}
				var public *KnowledgeAccessError
				if errors.As(readErr, &public) && public.Status == 404 {
					continue
				}
				return readErr
			}
			owner := kb.CreatedBy == actor
			policy, err := accessPolicy(tx, kb, true)
			if err != nil {
				return err
			}
			if owner {
				data.OwnedDomainPolicies = append(data.OwnedDomainPolicies, policy)
				if len(data.OwnedDomainPolicies) > 64 {
					return accessError(413, "capacity_exceeded")
				}
			}
			var modules []entity.KnowledgeAccessModuleRecord
			if err = tx.Where("domain_id = ? AND tenant_id = ?", kb.ID, kb.TenantID).Order("id ASC").Limit(65).Find(&modules).Error; err != nil {
				return err
			}
			if len(modules) > 64 {
				return accessError(413, "capacity_exceeded")
			}
			for _, candidate := range modules {
				module, scope, err := accessModule(tx, kb, candidate.ID, true)
				if err != nil {
					return err
				}
				modes := []string{}
				mcp := false
				if policy.Enabled && module.Enabled {
					grants, err := accessGrants(tx, kb, module, now)
					if err != nil {
						return err
					}
					read, search := false, false
					for _, grant := range grants {
						matches, err := accessGrantSubject(tx, grant, actor)
						if err != nil {
							return err
						}
						if grant.IssuedBy == kb.CreatedBy && matches && accessCovers(grant, "read", "web_only") {
							read = true
							search = search || grant.Mode == "read_search"
							mcp = mcp || grant.Delivery == "web_and_mcp"
						}
					}
					if read {
						modes = append(modes, "read")
					}
					if search {
						modes = append(modes, "read_search")
					}
				}
				if !owner && len(modes) == 0 {
					continue
				}
				data.Items = append(data.Items, entity.KnowledgeAccessModuleSummary{ModuleID: module.ID, DomainID: kb.ID, Name: module.Name, Version: module.Version, TargetCount: int64(len(scope.Targets)), Modes: modes, MCPDeliveryAllowed: mcp, Enabled: module.Enabled, AdministrativeOwner: owner})
				if len(data.Items) > 64 {
					return accessError(413, "capacity_exceeded")
				}
			}
		}
		encoded, err = accessEncode("listModules", requestID, data)
		if err != nil {
			return err
		}
		return tx.Create(&entity.KnowledgeAccessAuditRecord{ID: common.GenerateUUID(), TenantID: req.TenantID, ActorUserID: &actor, Action: "listModules", Outcome: "allowed", RequestID: requestID, OccurredAt: now}).Error
	})
	return 200, encoded, err
}

func accessCommandTenant(ctx context.Context, operation, scopeID string) (string, string, error) {
	if operation == "setDepartment" {
		return scopeID, "tenant", nil
	}
	tenant, err := accessTenantLocator(ctx, scopeID)
	return tenant, "domain", err
}

func accessCommandAuthority(ctx context.Context, tx *gorm.DB, actor, operation, scopeID, tenant string) (*entity.Knowledgebase, error) {
	if operation == "setDepartment" {
		return nil, accessTenantOwner(ctx, tx, actor, tenant)
	}
	kb, err := accessDomain(ctx, tx, actor, scopeID, true, true)
	if err != nil {
		return nil, err
	}
	if kb.TenantID != tenant {
		return nil, accessError(409, "scope_changed")
	}
	return kb, nil
}
