package dataset

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type accessCredentialMint struct {
	Row           entity.KnowledgeAccessCredentialRecord
	Data          entity.KnowledgeAccessCredentialMintData
	ViewExpiresAt time.Time
}

func accessCredentialCommand(operation string) bool {
	return operation == "createCredential" || operation == "revokeCredential"
}

func accessCredentialCommandDomain(ctx context.Context, actor string, request any) (string, error) {
	switch value := request.(type) {
	case *entity.KnowledgeAccessCreateCredentialRequest:
		var row entity.KnowledgeAccessViewRecord
		if err := accessDB(ctx).Select("id", "domain_id").Where("id = ? AND subject_user_id = ? AND credential_id IS NULL", value.ViewID, actor).Take(&row).Error; err != nil {
			return "", err
		}
		return row.DomainID, nil
	case *entity.KnowledgeAccessRevokeCredentialRequest:
		var row entity.KnowledgeAccessCredentialRecord
		if err := accessDB(ctx).Select("id", "domain_id").Where("id = ? AND subject_user_id = ?", value.CredentialID, actor).Take(&row).Error; err != nil {
			return "", err
		}
		return row.DomainID, nil
	}
	return "", accessError(400, "invalid_request")
}

// Self metadata access remains available after a content grant or membership
// is lost. A metadata locator never grants source or owner authority.
func accessCredentialMetadataAuthority(ctx context.Context, tx *gorm.DB, actor, domain, tenant, operation string) (*entity.Knowledgebase, error) {
	if accessCredential(ctx) != nil {
		return nil, accessError(401, "unauthenticated")
	}
	if operation == "createCredential" {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", actor).Take(&user).Error; err != nil {
			return nil, err
		}
	}
	if err := accessSession(ctx, tx, actor, true); err != nil {
		return nil, err
	}
	var kb entity.Knowledgebase
	if err := tx.Select("id", "tenant_id").Where("id = ?", domain).Take(&kb).Error; err != nil {
		return nil, err
	}
	if kb.TenantID != tenant {
		return nil, accessError(409, "scope_changed")
	}
	return &kb, nil
}

func accessCredentialDTO(row entity.KnowledgeAccessCredentialRecord) entity.KnowledgeAccessCredential {
	value := entity.KnowledgeAccessCredential{CredentialID: row.ID, DomainID: row.DomainID, ModuleID: row.ModuleID, ModuleVersion: row.ModuleVersion, Mode: row.Mode, ClientLabel: row.ClientLabel, IssuedAt: accessTime(row.IssuedAt), ExpiresAt: accessTime(row.ExpiresAt)}
	if row.RevokedAt != nil {
		value.RevokedAt = accessPtr(accessTime(*row.RevokedAt))
	}
	if row.LastUsedAt != nil {
		value.LastUsedAt = accessPtr(accessTime(*row.LastUsedAt))
	}
	return value
}

func (d *DatasetService) accessMintCredential(ctx context.Context, tx *gorm.DB, actor, commandID string, kb *entity.Knowledgebase, request entity.KnowledgeAccessCreateCredentialRequest) (*accessCredentialMint, error) {
	if accessCredential(ctx) != nil {
		return nil, accessError(401, "unauthenticated")
	}
	if strings.TrimSpace(request.ClientLabel) == "" || len(request.ClientLabel) > 128 {
		return nil, accessError(400, "invalid_request")
	}
	audience, err := config.KnowledgeAccessMCPAudience()
	if err != nil {
		return nil, accessError(503, "unavailable")
	}
	var state entity.KnowledgeAccessTenantStateRecord
	if err = tx.Where("tenant_id = ?", kb.TenantID).Take(&state).Error; err != nil {
		return nil, err
	}
	var view entity.KnowledgeAccessViewRecord
	if err = accessLocked(tx, true).Where("id = ? AND subject_user_id = ? AND domain_id = ? AND tenant_id = ? AND credential_id IS NULL", request.ViewID, actor, kb.ID, kb.TenantID).Take(&view).Error; err != nil {
		return nil, err
	}
	now, err := accessNow(tx)
	if err != nil {
		return nil, err
	}
	verified, err := d.accessValidateView(ctx, tx, actor, view, state.Epoch, now)
	if err != nil {
		return nil, err
	}
	if verified.Capture.Delivery != "web_and_mcp" || !(verified.Capture.Mode == request.Mode || verified.Capture.Mode == "read_search" && request.Mode == "read") {
		return nil, accessError(403, "action_denied")
	}
	var count int64
	if err = tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("actor_user_id = ? AND command_operation = ? AND command_state = ? AND command_committed_at > ?", actor, "createCredential", "completed", now.Add(-time.Minute)).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= 5 {
		return nil, accessError(429, "rate_limited")
	}
	if err = tx.Model(&entity.KnowledgeAccessCredentialRecord{}).Where("subject_user_id = ? AND domain_id = ? AND tenant_epoch = ? AND revoked_at IS NULL AND expires_at > ?", actor, kb.ID, state.Epoch, now).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= 20 {
		return nil, accessError(429, "rate_limited")
	}
	now, err = accessNow(tx)
	if err != nil {
		return nil, err
	}
	if !now.Before(view.ExpiresAt) {
		return nil, accessError(409, "view_stale")
	}
	ttl := int64(3600)
	if request.TTLSeconds != nil {
		ttl = *request.TTLSeconds
	}
	if ttl < 60 || ttl > 28800 {
		return nil, accessError(400, "invalid_request")
	}
	expires := now.Add(time.Duration(ttl) * time.Second)
	if verified.Grant.ExpiresAt.Before(expires) {
		expires = verified.Grant.ExpiresAt
	}
	if !expires.After(now) {
		return nil, accessError(409, "view_stale")
	}
	capture := verified.Capture
	capture.Mode = request.Mode
	private := accessJSON(accessCredentialCapture{Scope: verified.Scope, Capture: capture})
	if len(private) > 65536 {
		return nil, accessError(413, "capacity_exceeded")
	}
	var identifier [16]byte
	var secret [32]byte
	defer clear(secret[:])
	if _, err = rand.Read(identifier[:]); err != nil {
		return nil, accessError(503, "unavailable")
	}
	if _, err = rand.Read(secret[:]); err != nil {
		return nil, accessError(503, "unavailable")
	}
	id := hex.EncodeToString(identifier[:])
	row := entity.KnowledgeAccessCredentialRecord{ID: id, TenantID: kb.TenantID, SubjectUserID: actor, GrantID: verified.Grant.ID, DomainID: kb.ID, ModuleID: verified.View.ModuleID, ModuleVersion: verified.View.ModuleVersion, TenantEpoch: state.Epoch, CaptureJSON: private, Mode: request.Mode, Audience: audience, VerifierSHA256: accessCredentialVerifier(id, secret[:]), ClientLabel: request.ClientLabel, IssuedAt: now, ExpiresAt: expires, CreateCommandID: commandID}
	if err = tx.Omit("CreationReceipt").Create(&row).Error; err != nil {
		return nil, err
	}
	token := "kac1_" + id + "." + base64.RawURLEncoding.EncodeToString(secret[:])
	return &accessCredentialMint{Row: row, Data: entity.KnowledgeAccessCredentialMintData{Credential: accessCredentialDTO(row), Token: &token, Delivery: "issued_once"}, ViewExpiresAt: view.ExpiresAt}, nil
}

func accessRevokeCredential(tx *gorm.DB, actor string, kb *entity.Knowledgebase, request entity.KnowledgeAccessRevokeCredentialRequest, now time.Time) (*entity.KnowledgeAccessCredentialRecord, error) {
	var row entity.KnowledgeAccessCredentialRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND subject_user_id = ? AND domain_id = ? AND tenant_id = ?", request.CredentialID, actor, kb.ID, kb.TenantID).Take(&row).Error; err != nil {
		return nil, err
	}
	if row.RevokedAt == nil {
		result := tx.Model(&entity.KnowledgeAccessCredentialRecord{}).Where("id = ? AND subject_user_id = ? AND revoked_at IS NULL", row.ID, actor).Update("revoked_at", now)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, accessError(409, "version_conflict")
		}
		row.RevokedAt = &now
	}
	return &row, nil
}

func (d *DatasetService) accessListCredentials(ctx context.Context, actor, requestID string, request entity.KnowledgeAccessListCredentialsRequest) (int, []byte, error) {
	var encoded []byte
	err := dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		if accessCredential(ctx) != nil {
			return accessError(401, "unauthenticated")
		}
		if err := accessSession(ctx, tx, actor, false); err != nil {
			return err
		}
		data := entity.KnowledgeAccessListCredentialsData{Items: []entity.KnowledgeAccessCredential{}, Pagination: "none"}
		query := tx.Model(&entity.KnowledgeAccessCredentialRecord{}).Select("id", "tenant_id", "subject_user_id", "domain_id", "module_id", "module_version", "tenant_epoch", "mode", "client_label", "issued_at", "expires_at", "revoked_at", "last_used_at", "create_command_id").Where("subject_user_id = ? AND domain_id = ?", actor, request.DomainID)
		var rows []entity.KnowledgeAccessCredentialRecord
		if request.IdempotencyKey != nil {
			var receipt entity.KnowledgeAccessAuditRecord
			err := tx.Where("actor_user_id = ? AND command_operation = ? AND command_key = ? AND command_scope_id = ?", actor, "createCredential", *request.IdempotencyKey, request.DomainID).Take(&receipt).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && receipt.CommandResourceID != nil {
				if err = query.Where("id = ? AND create_command_id = ?", *receipt.CommandResourceID, receipt.ID).Limit(1).Find(&rows).Error; err != nil {
					return err
				}
			}
		} else {
			var locator entity.KnowledgeAccessCredentialRecord
			err := query.Session(&gorm.Session{}).Select("id", "tenant_id").Order("issued_at DESC, id ASC").Take(&locator).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				epochs, err := dao.LockKnowledgeAccessTenants(tx, []string{locator.TenantID}, true)
				if err != nil {
					return err
				}
				now, err := accessNow(tx)
				if err != nil {
					return err
				}
				if err = query.Session(&gorm.Session{}).Where("tenant_id = ? AND tenant_epoch = ? AND revoked_at IS NULL AND expires_at > ?", locator.TenantID, epochs[locator.TenantID], now).Order("issued_at DESC, id ASC").Limit(21).Find(&rows).Error; err != nil {
					return err
				}
				if len(rows) > 20 {
					return accessError(413, "capacity_exceeded")
				}
				ids := []string{}
				for _, row := range rows {
					ids = append(ids, row.ID)
				}
				if len(rows) < 20 {
					var recent []entity.KnowledgeAccessCredentialRecord
					remaining := query.Session(&gorm.Session{})
					if len(ids) > 0 {
						remaining = remaining.Where("id NOT IN ?", ids)
					}
					if err = remaining.Order("issued_at DESC, id ASC").Limit(20 - len(rows)).Find(&recent).Error; err != nil {
						return err
					}
					rows = append(rows, recent...)
				}
			}
		}
		for _, row := range rows {
			if row.SubjectUserID != actor || row.DomainID != request.DomainID {
				return accessError(409, "integrity_error")
			}
			data.Items = append(data.Items, accessCredentialDTO(row))
		}
		if len(rows) > 0 && request.IdempotencyKey != nil {
			if _, err := dao.LockKnowledgeAccessTenants(tx, []string{rows[0].TenantID}, true); err != nil {
				return err
			}
		}
		if err := accessSession(ctx, tx, actor, true); err != nil {
			return err
		}
		var err error
		encoded, err = accessEncode("listCredentials", requestID, data)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			locator := &entity.Knowledgebase{ID: request.DomainID, TenantID: rows[0].TenantID}
			return accessAudit(tx, actor, "listCredentials", requestID, locator, "", "allowed")
		}
		return nil
	})
	return 200, encoded, err
}
