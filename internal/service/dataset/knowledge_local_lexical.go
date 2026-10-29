package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"time"
)

type KnowledgeLexicalError struct {
	Status       int
	Code         string
	OutcomeKnown bool
	RetrySameKey bool
}

type lexicalSessionKey struct{}
type lexicalSession struct {
	UserID string
	SHA256 string
}

func lexicalLanguage(kb *entity.Knowledgebase) string {
	if kb.Language == nil {
		return ""
	}
	return *kb.Language
}

// AuthenticateKnowledgeLocalSession verifies only the signed native session.
// There is no API/beta token or configured-user fallback in this adapter.
func (d *DatasetService) AuthenticateKnowledgeLocalSession(ctx context.Context, authorization string) (context.Context, string, error) {
	if authorization == "" {
		return ctx, "", lexicalError(401, "unauthenticated")
	}
	u, code, err := service.NewUserService().GetUserByToken(ctx, authorization)
	if err != nil || code != common.CodeSuccess || u == nil || u.AccessToken == nil {
		return ctx, "", lexicalError(401, "unauthenticated")
	}
	session := lexicalSession{UserID: u.ID, SHA256: lexicalSHA([]byte(*u.AccessToken))}
	ctx = context.WithValue(ctx, lexicalSessionKey{}, session)
	if err := lexicalVerifySession(ctx, dao.DB, u.ID, false); err != nil {
		return ctx, "", err
	}
	return ctx, u.ID, nil
}
func lexicalVerifySession(ctx context.Context, db *gorm.DB, user string, lock bool) error {
	session, ok := ctx.Value(lexicalSessionKey{}).(lexicalSession)
	if !ok || session.UserID != user || len(session.SHA256) != 64 || db == nil {
		return lexicalError(401, "unauthenticated")
	}
	q := db.WithContext(ctx)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var current entity.User
	if err := q.Where("id = ? AND status = ? AND is_active = ? AND is_authenticated = ? AND is_anonymous = ?", user, string(entity.StatusValid), "1", "1", "0").Take(&current).Error; err != nil {
		return lexicalError(401, "unauthenticated")
	}
	if current.AccessToken == nil || *current.AccessToken == "" || lexicalSHA([]byte(*current.AccessToken)) != session.SHA256 || current.IsSuperuser != nil && *current.IsSuperuser {
		return lexicalError(401, "unauthenticated")
	}
	return nil
}
func (e *KnowledgeLexicalError) Error() string { return "Local lexical operation: " + e.Code }
func lexicalError(status int, code string) error {
	return &KnowledgeLexicalError{Status: status, Code: code, OutcomeKnown: true}
}
func lexicalNormalize(err error) error {
	if err == nil {
		return nil
	}
	var e *KnowledgeLexicalError
	if errors.As(err, &e) {
		return err
	}
	var k *KnowledgeError
	if errors.As(err, &k) {
		if k.Status == 404 {
			return lexicalError(404, "not_found")
		}
		if k.Status == 415 {
			return lexicalError(415, "unsupported_representation")
		}
		if k.Code == "knowledge_source_integrity" || k.Code == "structure_integrity" {
			return lexicalError(409, "integrity_error")
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return lexicalError(404, "not_found")
	}
	return &KnowledgeLexicalError{Status: 503, Code: "unavailable", RetrySameKey: true}
}
func lexicalPolicy(kb *entity.Knowledgebase, p entity.KnowledgeLexicalPolicyRecord) entity.KnowledgeLexicalPolicy {
	return entity.KnowledgeLexicalPolicy{DomainID: kb.ID, Version: p.Version, FoundationVersion: kb.KnowledgePolicyVersion, Enabled: p.Enabled, Mode: "owner_only_lexical", ReadAuthority: "existing_foundation_owner", AcceptedInputs: []string{"c1_text_root_at_most_8192_bytes", "r1_published_passage"}, AllowedModelUses: []string{}, VectorIndex: "not_created"}
}
func lexicalPolicyRow(ctx context.Context, db *gorm.DB, domain string) (entity.KnowledgeLexicalPolicyRecord, error) {
	var p entity.KnowledgeLexicalPolicyRecord
	err := db.WithContext(ctx).Where("domain_id = ?", domain).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entity.KnowledgeLexicalPolicyRecord{DomainID: domain}, nil
	}
	return p, err
}
func (d *DatasetService) lexicalContext(ctx context.Context, user, domain string, enabled bool) (*entity.Knowledgebase, entity.KnowledgeLexicalPolicy, error) {
	if err := lexicalVerifySession(ctx, dao.DB, user, false); err != nil {
		return nil, entity.KnowledgeLexicalPolicy{}, err
	}
	kb, err := d.knowledgeBase(ctx, user, domain, true)
	if err != nil {
		return nil, entity.KnowledgeLexicalPolicy{}, lexicalNormalize(err)
	}
	if kb.Permission != "me" || !entity.ValidKnowledgeLexicalVersion(kb.KnowledgePolicyVersion, false) {
		return nil, entity.KnowledgeLexicalPolicy{}, lexicalError(404, "not_found")
	}
	p, err := lexicalPolicyRow(ctx, dao.DB, kb.ID)
	if err != nil {
		return nil, entity.KnowledgeLexicalPolicy{}, lexicalNormalize(err)
	}
	if p.Version < 0 || p.Version > 9007199254740991 || p.Enabled && p.Version == 0 || p.OwnerID != "" && p.OwnerID != user {
		return nil, entity.KnowledgeLexicalPolicy{}, lexicalError(409, "integrity_error")
	}
	policy := lexicalPolicy(kb, p)
	if enabled && !policy.Enabled {
		return nil, policy, lexicalError(409, "policy_disabled")
	}
	return kb, policy, nil
}

// Lock the current foundation owner, membership and policy in the publication
// transaction. The earlier native permission check is repeated at every entry.
func lexicalLock(ctx context.Context, tx *gorm.DB, user, domain string) (*entity.Knowledgebase, entity.KnowledgeLexicalPolicyRecord, error) {
	if err := lexicalVerifySession(ctx, tx, user, true); err != nil {
		return nil, entity.KnowledgeLexicalPolicyRecord{}, err
	}
	var kb entity.Knowledgebase
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND created_by = ? AND knowledge_enabled = ? AND permission = ? AND status = ?", domain, user, true, "me", string(entity.StatusValid)).Take(&kb).Error; err != nil {
		return nil, entity.KnowledgeLexicalPolicyRecord{}, lexicalNormalize(err)
	}
	var member entity.UserTenant
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND tenant_id = ? AND status = ?", user, kb.TenantID, string(entity.StatusValid)).Take(&member).Error; err != nil {
		return nil, entity.KnowledgeLexicalPolicyRecord{}, lexicalNormalize(err)
	}
	if err := permission.NewDatabaseChecker(tx).CheckResource(ctx, permission.Subject{UserID: user}, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: domain}, permission.OperationRead); err != nil {
		return nil, entity.KnowledgeLexicalPolicyRecord{}, lexicalError(404, "not_found")
	}
	p, err := lexicalPolicyRow(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), domain)
	if err != nil {
		return nil, p, lexicalNormalize(err)
	}
	if p.OwnerID != "" && p.OwnerID != user {
		return nil, p, lexicalError(404, "not_found")
	}
	return &kb, p, nil
}
func (d *DatasetService) GetKnowledgeLocalPolicy(ctx context.Context, user, domain string) (*entity.KnowledgeLexicalPolicy, error) {
	_, p, err := d.lexicalContext(ctx, user, domain, false)
	return &p, err
}
func (d *DatasetService) ChangeKnowledgeLocalPolicy(ctx context.Context, user, domain string, req entity.KnowledgeLexicalPolicyChange) (*entity.KnowledgeLexicalPolicyReceipt, error) {
	if _, _, err := d.lexicalContext(ctx, user, domain, false); err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeLocalLexicalVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if !entity.ValidKnowledgeLexicalKey(req.Key) || !entity.ValidKnowledgeLexicalVersion(req.ExpectedVersion, true) || !entity.ValidKnowledgeLexicalVersion(req.ExpectedFoundationVersion, false) {
		return nil, lexicalError(400, "invalid_request")
	}
	id := lexicalDigest("knowledge-local-operation/1", user, domain, "policy", req.Key)
	sha := lexicalSHA([]byte(lexicalJSON(req)))
	var out entity.KnowledgeLexicalPolicyReceipt
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, p, err := lexicalLock(ctx, tx, user, domain)
		if err != nil {
			return err
		}
		var op entity.KnowledgeLexicalOperationRecord
		err = tx.Where("id = ?", id).Take(&op).Error
		if err == nil {
			if op.RequestSHA256 != sha {
				return lexicalError(409, "idempotency_conflict")
			}
			if json.Unmarshal([]byte(op.Payload), &out) != nil {
				return lexicalError(409, "integrity_error")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if p.Version != req.ExpectedVersion || kb.KnowledgePolicyVersion != req.ExpectedFoundationVersion {
			return lexicalError(409, "policy_changed")
		}
		outcome := "no_change"
		if p.Enabled != req.Enabled {
			if p.Version == 9007199254740991 {
				return lexicalError(409, "capacity_exceeded")
			}
			p.Version++
			p.Enabled = req.Enabled
			outcome = "changed"
		}
		p.DomainID, p.OwnerID = domain, user
		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		out = entity.KnowledgeLexicalPolicyReceipt{Key: req.Key, RequestSHA256: sha, Outcome: outcome, Policy: lexicalPolicy(kb, p)}
		return tx.Create(&entity.KnowledgeLexicalOperationRecord{ID: id, OwnerID: user, DomainID: domain, Kind: "policy", Key: req.Key, RequestSHA256: sha, Payload: lexicalJSON(out)}).Error
	})
	return &out, lexicalNormalize(err)
}
func lexicalReadiness(s entity.KnowledgeLexicalSelection, sha, kind, current, diagnostic string) entity.KnowledgeLexicalReadiness {
	diagnostics := []string{}
	if diagnostic != "" {
		diagnostics = append(diagnostics, diagnostic)
	}
	return entity.KnowledgeLexicalReadiness{Kind: kind, Publication: "not_published", Selection: s, SelectionSHA256: sha, Currentness: current, Diagnostics: diagnostics}
}
func lexicalTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
