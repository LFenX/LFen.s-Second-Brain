package dataset

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type accessCredentialKey struct{}

type accessCredentialCapture struct {
	Scope   entity.KnowledgeAccessModuleScope `json:"scope"`
	Capture accessCapture                     `json:"capture"`
}

type accessCredentialIdentity struct {
	ID, TenantID, UserID, DomainID, ModuleID, GrantID, Mode, Audience string
	VerifierSHA256                                                    []byte
	RequestID, ReservationID                                          string
}

func accessCredential(ctx context.Context) *accessCredentialIdentity {
	value, _ := ctx.Value(accessCredentialKey{}).(*accessCredentialIdentity)
	return value
}

func accessCredentialVerifier(id string, secret []byte) []byte {
	hash := sha256.New()
	for _, part := range [][]byte{[]byte("knowledge-access-credential/1"), []byte(id), secret} {
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(part)))
		hash.Write(size[:])
		hash.Write(part)
	}
	return hash.Sum(nil)
}

func accessParseCredential(authorization string) (*accessCredentialIdentity, error) {
	if len(authorization) != 88 || !strings.EqualFold(authorization[:7], "Bearer ") || !strings.HasPrefix(authorization[7:], "kac1_") {
		return nil, accessError(401, "unauthenticated")
	}
	id, encoded, ok := strings.Cut(authorization[12:], ".")
	if !ok || !isKnowledgeID(id, 32) || len(encoded) != 43 {
		return nil, accessError(401, "unauthenticated")
	}
	secret, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	defer clear(secret)
	if err != nil || len(secret) != 32 || base64.RawURLEncoding.EncodeToString(secret) != encoded {
		return nil, accessError(401, "unauthenticated")
	}
	return &accessCredentialIdentity{ID: id, VerifierSHA256: accessCredentialVerifier(id, secret)}, nil
}

// Credential authentication checks account status without loading native tokens.
func accessCredentialUser(ctx context.Context, tx *gorm.DB, actor string, lock bool) error {
	identity := accessCredential(ctx)
	if identity == nil || identity.UserID != actor || !isKnowledgeID(actor, 32) {
		return accessError(401, "unauthenticated")
	}
	var user entity.User
	err := accessLocked(tx.WithContext(ctx), lock).Select("id", "status", "is_active", "is_authenticated", "is_anonymous").Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", actor, "1", "1", "1", "0").Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return accessError(401, "unauthenticated")
	}
	return err
}

func accessViewCredentialMatches(ctx context.Context, view entity.KnowledgeAccessViewRecord) bool {
	identity := accessCredential(ctx)
	if identity == nil {
		return view.CredentialID == nil
	}
	return view.CredentialID != nil && *view.CredentialID == identity.ID && view.SubjectUserID == identity.UserID && view.TenantID == identity.TenantID && view.DomainID == identity.DomainID && view.ModuleID == identity.ModuleID && view.GrantID == identity.GrantID
}

func accessViewQuery(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if identity := accessCredential(ctx); identity != nil {
		return tx.Where("credential_id = ?", identity.ID)
	}
	return tx.Where("credential_id IS NULL")
}

func accessSameCredential(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func accessCurrentCredentialID(ctx context.Context) *string {
	if identity := accessCredential(ctx); identity != nil {
		return accessPtr(identity.ID)
	}
	return nil
}

func accessCredentialDefinition(row entity.KnowledgeAccessCredentialRecord) any {
	digest := lexicalSHA([]byte(accessJSON([]any{"knowledge-access-credential-scope/0.3.0", row.ID, row.TenantID, row.SubjectUserID, row.GrantID, row.DomainID, row.ModuleID, row.ModuleVersion, row.TenantEpoch, row.CaptureJSON, row.Mode, row.Audience, row.ClientLabel, accessTime(row.IssuedAt), accessTime(row.ExpiresAt)})))
	return struct {
		CredentialID string `json:"credential_id"`
		Mode         string `json:"mode"`
		ExpiresAt    string `json:"expires_at"`
		ScopeSHA256  string `json:"scope_sha256"`
	}{row.ID, row.Mode, accessTime(row.ExpiresAt), digest}
}

func accessCredentialIntegrity(tx *gorm.DB, row entity.KnowledgeAccessCredentialRecord) error {
	var receipt entity.KnowledgeAccessAuditRecord
	if err := accessLocked(tx, true).Where("id = ? AND tenant_id = ?", row.CreateCommandID, row.TenantID).Take(&receipt).Error; err != nil {
		return err
	}
	definition := accessJSON(accessCredentialDefinition(row))
	if receipt.ActorUserID == nil || *receipt.ActorUserID != row.SubjectUserID || receipt.CredentialID == nil || *receipt.CredentialID != row.ID || receipt.DomainID == nil || *receipt.DomainID != row.DomainID || receipt.ModuleID == nil || *receipt.ModuleID != row.ModuleID || receipt.CommandOperation == nil || *receipt.CommandOperation != "createCredential" || receipt.CommandResourceID == nil || *receipt.CommandResourceID != row.ID || receipt.CommandState == nil || *receipt.CommandState != "completed" || receipt.AfterDefinitionJSON == nil || *receipt.AfterDefinitionJSON != definition || receipt.DefinitionDigest == nil || *receipt.DefinitionDigest != lexicalSHA([]byte(definition)) {
		return accessError(409, "integrity_error")
	}
	return nil
}

// Parent authorization shares the view metadata checks. Publication readiness
// remains a business check, so a live credential can report index_stale safely.
func (d *DatasetService) accessCurrentCredential(ctx context.Context, tx *gorm.DB, epoch int64, now time.Time, used bool) (*entity.KnowledgeAccessCredentialRecord, *verifiedAccessActor, error) {
	identity := accessCredential(ctx)
	if identity == nil {
		return nil, nil, accessError(401, "unauthenticated")
	}
	var row entity.KnowledgeAccessCredentialRecord
	if err := tx.Session(&gorm.Session{}).Where("id = ?", identity.ID).Take(&row).Error; err != nil {
		return nil, nil, err
	}
	audience, err := config.KnowledgeAccessMCPAudience()
	if err != nil {
		return nil, nil, accessError(503, "unavailable")
	}
	if row.ID != identity.ID || row.TenantID != identity.TenantID || row.SubjectUserID != identity.UserID || row.DomainID != identity.DomainID || row.ModuleID != identity.ModuleID || row.GrantID != identity.GrantID || row.Mode != identity.Mode || row.Audience != audience || row.Audience != identity.Audience || len(row.VerifierSHA256) != 32 || subtle.ConstantTimeCompare(row.VerifierSHA256, identity.VerifierSHA256) != 1 || row.RevokedAt != nil || !now.Before(row.ExpiresAt) || row.IssuedAt.After(now) || !row.ExpiresAt.After(row.IssuedAt) || row.ExpiresAt.After(row.IssuedAt.Add(8*time.Hour)) || row.TenantEpoch != epoch {
		return nil, nil, accessError(401, "unauthenticated")
	}
	var capture accessCredentialCapture
	if len(row.CaptureJSON) > 65536 || entity.KnowledgeParsedDecode([]byte(row.CaptureJSON), &capture) != nil || accessJSON(capture) != row.CaptureJSON || entity.KnowledgeAccessDecode([]byte(accessJSON(capture.Scope)), "ModuleScope", nil) != nil || capture.Capture.Mode != row.Mode || capture.Capture.Delivery != "web_and_mcp" {
		return nil, nil, accessError(401, "unauthenticated")
	}
	parent := entity.KnowledgeAccessViewRecord{ID: row.ID, TenantID: row.TenantID, SubjectUserID: row.SubjectUserID, CredentialID: &row.ID, GrantID: row.GrantID, DomainID: row.DomainID, ModuleID: row.ModuleID, ModuleVersion: row.ModuleVersion, TenantEpoch: row.TenantEpoch, PrivateManifestJSON: accessJSON(capture.Scope), CaptureJSON: accessJSON(capture.Capture), IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt}
	verified, err := d.accessAuthorizeView(ctx, tx, row.SubjectUserID, parent, epoch, now)
	if err != nil {
		return nil, nil, err
	}
	query := accessLocked(tx, true)
	if used {
		query = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var current entity.KnowledgeAccessCredentialRecord
	if err = query.Where("id = ?", identity.ID).Take(&current).Error; err != nil {
		return nil, nil, err
	}
	if current.RevokedAt != nil || accessJSON(accessCredentialDefinition(current)) != accessJSON(accessCredentialDefinition(row)) || len(current.VerifierSHA256) != 32 || subtle.ConstantTimeCompare(current.VerifierSHA256, identity.VerifierSHA256) != 1 {
		return nil, nil, accessError(401, "unauthenticated")
	}
	if err = accessCredentialIntegrity(tx, current); err != nil {
		return nil, nil, err
	}
	return &current, verified, nil
}

func accessCredentialAuthError(err error) error {
	if err == nil {
		return nil
	}
	var public *KnowledgeAccessError
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.As(err, &public) && public.Status >= 400 && public.Status < 500 && public.Status != 429 {
		return accessError(401, "unauthenticated")
	}
	if public != nil && public.Status == 429 {
		return public
	}
	return &KnowledgeAccessError{Status: 503, Code: "unavailable", RetrySameKey: true}
}

// AuthenticateKnowledgeAccessMCP creates a request-local verified principal.
// The caller owns its bounded context; no native/owner session is constructed.
func (d *DatasetService) AuthenticateKnowledgeAccessMCP(ctx context.Context, authorization, requestID string) (context.Context, string, string, error) {
	if !isKnowledgeID(requestID, 32) {
		return nil, "", "", accessError(400, "invalid_request")
	}
	identity, err := accessParseCredential(authorization)
	if err != nil {
		return nil, "", "", err
	}
	audience, err := config.KnowledgeAccessMCPAudience()
	if err != nil {
		return nil, "", "", accessError(503, "unavailable")
	}
	ctx = dao.KnowledgeAccessPrivateReadContext(ctx)
	var locator entity.KnowledgeAccessCredentialRecord
	if err = accessDB(ctx).Where("id = ?", identity.ID).Take(&locator).Error; err != nil {
		return nil, "", "", accessCredentialAuthError(err)
	}
	if len(locator.VerifierSHA256) != 32 || subtle.ConstantTimeCompare(locator.VerifierSHA256, identity.VerifierSHA256) != 1 || locator.Audience != audience {
		return nil, "", "", accessError(401, "unauthenticated")
	}
	identity.TenantID, identity.UserID, identity.DomainID, identity.ModuleID, identity.GrantID, identity.Mode, identity.Audience, identity.RequestID = locator.TenantID, locator.SubjectUserID, locator.DomainID, locator.ModuleID, locator.GrantID, locator.Mode, audience, requestID
	ctx = context.WithValue(ctx, accessCredentialKey{}, identity)
	err = dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		epochs, err := dao.LockKnowledgeAccessTenants(tx, []string{identity.TenantID}, false)
		if err != nil {
			return err
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		_, verified, err := d.accessCurrentCredential(ctx, tx, epochs[identity.TenantID], now, false)
		if err != nil {
			return err
		}
		var count int64
		if err = tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("actor_user_id = ? AND domain_id = ? AND action IN ? AND occurred_at > ?", identity.UserID, identity.DomainID, []string{"inspect", "search", "read", "mcpTransport"}, now.Add(-time.Minute)).Count(&count).Error; err != nil {
			return err
		}
		if count >= 60 {
			return accessError(429, "rate_limited")
		}
		identity.ReservationID = common.GenerateUUID()
		row := entity.KnowledgeAccessAuditRecord{ID: identity.ReservationID, TenantID: identity.TenantID, ActorUserID: &identity.UserID, CredentialID: &identity.ID, DomainID: &identity.DomainID, ModuleID: &verified.View.ModuleID, Action: "mcpTransport", Outcome: "denied", RequestID: requestID, OccurredAt: now}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, "", "", accessCredentialAuthError(err)
	}
	return ctx, identity.UserID, identity.Mode, nil
}

func (d *DatasetService) accessCheckCredential(ctx context.Context, used bool) error {
	identity := accessCredential(ctx)
	if identity == nil {
		return accessError(401, "unauthenticated")
	}
	err := dao.KnowledgeAccessTransaction(ctx, accessDB(ctx), func(tx *gorm.DB) error {
		epochs, err := dao.LockKnowledgeAccessTenants(tx, []string{identity.TenantID}, true)
		if err != nil {
			return err
		}
		now, err := accessNow(tx)
		if err != nil {
			return err
		}
		row, verified, err := d.accessCurrentCredential(ctx, tx, epochs[identity.TenantID], now, used)
		if err != nil {
			return err
		}
		now, err = accessNow(tx)
		if err != nil {
			return err
		}
		if !now.Before(row.ExpiresAt) || !now.Before(verified.Grant.ExpiresAt) {
			return accessError(401, "unauthenticated")
		}
		if !used {
			return nil
		}
		if row.LastUsedAt == nil || now.After(*row.LastUsedAt) {
			result := tx.Model(&entity.KnowledgeAccessCredentialRecord{}).Where("id = ? AND revoked_at IS NULL AND expires_at > ?", row.ID, now).Update("last_used_at", now)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return accessError(503, "unavailable")
			}
		}
		return tx.Model(&entity.KnowledgeAccessAuditRecord{}).Where("id = ? AND actor_user_id = ? AND credential_id = ? AND request_id = ? AND action = ? AND outcome = ?", identity.ReservationID, identity.UserID, identity.ID, identity.RequestID, "mcpTransport", "denied").Updates(map[string]any{"outcome": "allowed", "occurred_at": now}).Error
	})
	return accessCredentialAuthError(err)
}

func (d *DatasetService) CheckKnowledgeAccessMCP(ctx context.Context) error {
	return d.accessCheckCredential(ctx, false)
}

// FinalizeKnowledgeAccessMCP rechecks current primary authority even when the
// SDK planned an error. A revoked/expired credential discards the whole result.
func (d *DatasetService) FinalizeKnowledgeAccessMCP(ctx context.Context) error {
	return d.accessCheckCredential(ctx, true)
}

func accessCaptureSubset(parent *verifiedAccessActor, scope entity.KnowledgeAccessModuleScope, capture accessCapture) error {
	if parent.Scope.Publication != scope.Publication || parent.Capture.OwnerID != capture.OwnerID || parent.Capture.ScopeSHA256 != capture.ScopeSHA256 || parent.Capture.ShareVersion != capture.ShareVersion || parent.Capture.Policies != capture.Policies || capture.Delivery != "web_and_mcp" || parent.Capture.Delivery != "web_and_mcp" || !(parent.Capture.Mode == capture.Mode || parent.Capture.Mode == "read_search" && capture.Mode == "read") {
		return accessError(409, "scope_changed")
	}
	allowed := map[string]bool{}
	for _, target := range parent.Scope.Targets {
		allowed[accessJSON(target)] = true
	}
	for _, target := range scope.Targets {
		if !allowed[accessJSON(target)] {
			return accessError(404, "not_found")
		}
	}
	fences := map[string]string{}
	for _, fence := range parent.Capture.Fences {
		fences[fence.SourceID] = accessJSON(fence)
	}
	for _, fence := range capture.Fences {
		if fences[fence.SourceID] != accessJSON(fence) {
			return accessError(409, "scope_changed")
		}
	}
	return nil
}
