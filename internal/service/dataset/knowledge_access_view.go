package dataset

import (
	"context"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func accessReserveRequest(ctx context.Context, tx *gorm.DB, actor, operation, requestID string, kb *entity.Knowledgebase, module string, now time.Time) (string, error) {
	if identity := accessCredential(ctx); identity != nil {
		if identity.UserID != actor || identity.DomainID != kb.ID || identity.TenantID != kb.TenantID || identity.ModuleID != module || identity.RequestID != requestID {
			return "", accessError(401, "unauthenticated")
		}
		result := tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("id = ? AND tenant_id = ? AND actor_user_id = ? AND credential_id = ? AND domain_id = ? AND request_id = ? AND action = ? AND outcome = ?", identity.ReservationID, kb.TenantID, actor, identity.ID, kb.ID, requestID, "mcpTransport", "denied").Updates(map[string]any{"action": operation, "module_id": module})
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected != 1 {
			return "", accessError(409, "integrity_error")
		}
		return identity.ReservationID, nil
	}
	var count int64
	if err := tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("actor_user_id = ? AND domain_id = ? AND action IN ? AND occurred_at > ?", actor, kb.ID, []string{"inspect", "search", "read", "mcpTransport"}, now.Add(-time.Minute)).Count(&count).Error; err != nil {
		return "", err
	}
	if count >= 60 {
		return "", accessError(429, "rate_limited")
	}
	id := common.GenerateUUID()
	// A reservation is conservatively denied until the fresh final permit
	// commits. This same audit row counts one request and becomes allowed.
	row := entity.KnowledgeAccessAuditRecord{ID: id, TenantID: kb.TenantID, ActorUserID: &actor, DomainID: accessPtr(kb.ID), ModuleID: &module, Action: operation, Outcome: "denied", RequestID: requestID, OccurredAt: now}
	return id, tx.Create(&row).Error
}

func (d *DatasetService) accessInspect(ctx context.Context, actor, requestID string, req entity.KnowledgeAccessInspectRequest) (int, []byte, error) {
	tenant, err := accessTenantLocator(ctx, req.DomainID)
	if err != nil {
		return 0, nil, err
	}
	var verified *verifiedAccessActor
	err = dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		epochs, err := dao.LockKnowledgeAccessTenants(tx, []string{tenant}, false)
		if err != nil {
			return err
		}
		kb, err := accessDomain(ctx, tx, actor, req.DomainID, false, true)
		if err != nil {
			return err
		}
		if kb.TenantID != tenant {
			return accessError(409, "scope_changed")
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		module, scope, err := accessModule(tx, kb, req.ModuleID, true)
		if err != nil {
			return err
		}
		var parent *verifiedAccessActor
		var grant *entity.KnowledgeAccessGrantRecord
		if identity := accessCredential(ctx); identity != nil {
			if req.DomainID != identity.DomainID || req.ModuleID != identity.ModuleID {
				return accessError(404, "not_found")
			}
			if req.Delivery != "web_and_mcp" || !(req.Mode == identity.Mode || identity.Mode == "read_search" && req.Mode == "read") {
				return accessError(403, "action_denied")
			}
			_, parent, err = d.accessCurrentCredential(ctx, tx, epochs[tenant], now, false)
			if err != nil {
				return err
			}
			grant = parent.Grant
			scope = parent.Scope
		} else {
			grant, err = accessPickGrant(tx, kb, module, actor, req.Mode, req.Delivery, now)
			if err != nil {
				return err
			}
		}
		if req.SourceIDs != nil {
			requested := map[string]bool{}
			for _, id := range *req.SourceIDs {
				requested[id] = true
			}
			selected := []entity.KnowledgeAccessTarget{}
			for _, target := range scope.Targets {
				if requested[target.Source.SourceID] {
					selected = append(selected, target)
					delete(requested, target.Source.SourceID)
				}
			}
			if len(requested) > 0 || len(selected) == 0 {
				return accessError(404, "not_found")
			}
			scope.Targets = selected
		}
		accessSortTargets(scope.Targets)
		proofCtx := accessProofContext(ctx, tx)
		manifest, err := d.accessPublication(proofCtx, tx, kb, scope, true)
		if err != nil {
			return err
		}
		policy, err := accessPolicy(tx, kb, true)
		if err != nil {
			return err
		}
		selection := entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{}}
		for _, target := range scope.Targets {
			selection.Targets = append(selection.Targets, accessContentTarget(target))
		}
		fences, err := contentLexicalCaptureSources(accessLocked(tx, true), kb, selection)
		if err != nil {
			return err
		}
		private := accessJSON(scope)
		capture := accessCapture{OwnerID: kb.CreatedBy, Mode: req.Mode, Delivery: req.Delivery, ScopeSHA256: module.ScopeSHA256, SelectedSHA256: lexicalSHA([]byte(private)), ShareVersion: policy.Version, Policies: manifest.Policies, Fences: fences}
		if parent != nil {
			if err = accessCaptureSubset(parent, scope, capture); err != nil {
				return err
			}
		}
		captureJSON := accessJSON(capture)
		if len(private)+len(captureJSON) > 65536 {
			return accessError(413, "capacity_exceeded")
		}
		expires := now.Add(300 * time.Second)
		if grant.ExpiresAt.Before(expires) {
			expires = grant.ExpiresAt
		}
		if parent != nil && parent.View.ExpiresAt.Before(expires) {
			expires = parent.View.ExpiresAt
		}
		credentialID := accessCurrentCredentialID(ctx)
		var existing []entity.KnowledgeAccessViewRecord
		if err = tx.Where("subject_user_id = ? AND domain_id = ? AND tenant_epoch = ? AND expires_at > ?", actor, kb.ID, epochs[tenant], now).Order("issued_at DESC, id ASC").Limit(21).Find(&existing).Error; err != nil {
			return err
		}
		var view entity.KnowledgeAccessViewRecord
		for _, row := range existing {
			if accessSameCredential(row.CredentialID, credentialID) && row.TenantID == tenant && row.GrantID == grant.ID && row.ModuleID == module.ID && row.ModuleVersion == module.Version && row.TenantEpoch == epochs[tenant] && row.PrivateManifestJSON == private && row.CaptureJSON == captureJSON && !row.ExpiresAt.After(expires) {
				view = row
				break
			}
		}
		if view.ID == "" {
			if len(existing) >= 20 {
				return accessError(429, "rate_limited")
			}
			view = entity.KnowledgeAccessViewRecord{ID: common.GenerateUUID(), TenantID: tenant, SubjectUserID: actor, CredentialID: credentialID, GrantID: grant.ID, DomainID: kb.ID, ModuleID: module.ID, ModuleVersion: module.Version, TenantEpoch: epochs[tenant], PrivateManifestJSON: private, CaptureJSON: captureJSON, IssuedAt: now, ExpiresAt: expires}
			if err = tx.Create(&view).Error; err != nil {
				return err
			}
		}
		rows, byTarget, err := accessSelectedRows(manifest, scope.Targets)
		if err != nil {
			return err
		}
		auditID, err := accessReserveRequest(ctx, tx, actor, "inspect", requestID, kb, module.ID, now)
		if err != nil {
			return err
		}
		verified = &verifiedAccessActor{UserID: actor, View: view, KB: kb, Scope: scope, Capture: capture, Manifest: manifest, Rows: rows, ByTarget: byTarget, AuditID: auditID, Grant: grant}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	catalog := make([]entity.KnowledgeAccessTargetCatalogEntry, 0, len(verified.Scope.Targets))
	for i, target := range verified.Scope.Targets {
		catalog = append(catalog, entity.KnowledgeAccessTargetCatalogEntry{Target: target, UnitCount: int64(len(verified.ByTarget[i]))})
	}
	data := entity.KnowledgeAccessInspectData{ViewID: verified.View.ID, DomainID: verified.KB.ID, ModuleID: verified.View.ModuleID, ModuleVersion: verified.View.ModuleVersion, Mode: verified.Capture.Mode, Delivery: verified.Capture.Delivery, ExpiresAt: accessTime(verified.View.ExpiresAt), Targets: catalog, SourceCount: int64(len(catalog)), UnitCount: int64(len(verified.Rows)), Readiness: "ready", Pagination: "none"}
	encoded, err := accessEncode("inspect", requestID, data)
	if err != nil {
		return 0, nil, err
	}
	if err = d.accessFinalPermit(ctx, verified, "inspect", requestID); err != nil {
		return 0, nil, err
	}
	return 200, encoded, nil
}

func (d *DatasetService) accessOpenView(ctx context.Context, actor, viewID, operation, requestID string) (*verifiedAccessActor, error) {
	var locator entity.KnowledgeAccessViewRecord
	if err := accessViewQuery(ctx, accessDB(ctx)).Select("id", "tenant_id").Where("id = ? AND subject_user_id = ?", viewID, actor).Take(&locator).Error; err != nil {
		return nil, err
	}
	var verified *verifiedAccessActor
	err := dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		epochs, err := dao.LockKnowledgeAccessTenants(tx, []string{locator.TenantID}, false)
		if err != nil {
			return err
		}
		var view entity.KnowledgeAccessViewRecord
		if err = accessViewQuery(ctx, accessLocked(tx, true)).Where("id = ? AND subject_user_id = ?", viewID, actor).Take(&view).Error; err != nil {
			return err
		}
		if view.TenantID != locator.TenantID {
			return accessError(409, "scope_changed")
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		verified, err = d.accessValidateView(ctx, tx, actor, view, epochs[view.TenantID], now)
		if err != nil {
			return err
		}
		if operation == "search" && verified.Capture.Mode != "read_search" {
			return accessError(403, "action_denied")
		}
		verified.AuditID, err = accessReserveRequest(ctx, tx, actor, operation, requestID, verified.KB, view.ModuleID, now)
		return err
	})
	return verified, err
}

func (d *DatasetService) accessFinalPermit(ctx context.Context, verified *verifiedAccessActor, operation, requestID string) error {
	return dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		epochs, err := dao.LockKnowledgeAccessTenants(tx, []string{verified.View.TenantID}, true)
		if err != nil {
			return err
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		var view entity.KnowledgeAccessViewRecord
		if err = accessViewQuery(ctx, accessLocked(tx, true)).Where("id = ? AND subject_user_id = ?", verified.View.ID, verified.UserID).Take(&view).Error; err != nil {
			return err
		}
		if view.TenantID != verified.View.TenantID || view.GrantID != verified.View.GrantID || view.ModuleID != verified.View.ModuleID || view.PrivateManifestJSON != verified.View.PrivateManifestJSON || view.CaptureJSON != verified.View.CaptureJSON {
			return accessError(409, "view_stale")
		}
		current, err := d.accessValidateView(ctx, tx, verified.UserID, view, epochs[view.TenantID], now)
		if err != nil {
			return err
		}
		if operation == "search" && current.Capture.Mode != "read_search" {
			return accessError(403, "action_denied")
		}
		// Time is checked again after all current source/dependency proofs.
		now, err = accessNow(tx)
		if err != nil {
			return err
		}
		if !now.Before(view.ExpiresAt) {
			return accessError(409, "view_stale")
		}
		var grant entity.KnowledgeAccessGrantRecord
		if err = accessLocked(tx, true).Where("id = ? AND revoked_at IS NULL AND expires_at > ?", view.GrantID, now).Take(&grant).Error; err != nil {
			return accessError(409, "view_stale")
		}
		result := tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("id = ? AND tenant_id = ? AND actor_user_id = ? AND action = ? AND request_id = ? AND outcome = ?", verified.AuditID, view.TenantID, verified.UserID, operation, requestID, "denied").Updates(map[string]any{"outcome": "allowed", "occurred_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return accessError(409, "integrity_error")
		}
		return nil
	})
}
