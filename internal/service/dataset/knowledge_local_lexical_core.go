package dataset

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// lexicalCorePlan separates representation-specific evidence from the one
// existing engine write, visibility verification and SQL publication sequence.
type lexicalCorePlan struct {
	Rows           []map[string]any
	Sources        []string
	Verify         func(context.Context) error
	ValidateLocked func(*gorm.DB, *entity.Knowledgebase, entity.KnowledgeLexicalPolicyRecord) error
	Publication    func(time.Time) (*entity.KnowledgeLexicalPublicationRecord, error)
}

func (d *DatasetService) lexicalExecutePlan(ctx context.Context, job entity.KnowledgeLexicalJobRecord, kb *entity.Knowledgebase, native lexicalEngine, plan lexicalCorePlan) error {
	if plan.Verify == nil || plan.Publication == nil || len(plan.Sources) == 0 || len(plan.Rows) > 32768 {
		return lexicalError(409, "integrity_error")
	}
	if err := native.PrepareKnowledgeLexical(ctx, "ragflow_"+kb.TenantID, job.DomainID, job.Language); err != nil {
		return err
	}
	if err := native.CreateKnowledgeLexicalRows(ctx, "ragflow_"+kb.TenantID, job.DomainID, plan.Rows); err != nil {
		return err
	}
	if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+kb.TenantID, job.DomainID, plan.Rows); err != nil {
		return lexicalError(409, "integrity_error")
	}
	if _, _, err := d.lexicalContext(ctx, job.OwnerID, job.DomainID, true); err != nil {
		return err
	}
	if err := plan.Verify(ctx); err != nil {
		return err
	}
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, policy, err := lexicalLock(ctx, tx, job.OwnerID, job.DomainID)
		if err != nil {
			return err
		}
		if !policy.Enabled || policy.Version != job.PolicyVersion || current.KnowledgePolicyVersion != job.FoundationVersion || lexicalLanguage(current) != job.Language {
			return lexicalError(409, "policy_changed")
		}
		var claimed entity.KnowledgeLexicalJobRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND state = ? AND claim = ? AND lease_until > ?", job.ID, "building", job.Claim, time.Now().UTC()).Take(&claimed).Error; err != nil {
			return err
		}
		if claimed.OwnerID != job.OwnerID || claimed.DomainID != job.DomainID || claimed.Protocol != job.Protocol || claimed.SelectionSHA256 != job.SelectionSHA256 || claimed.Selection != job.Selection || claimed.RequestSHA256 != job.RequestSHA256 || claimed.SessionSHA256 != job.SessionSHA256 || claimed.PolicyVersion != job.PolicyVersion || claimed.FoundationVersion != job.FoundationVersion || claimed.ContentVersion != job.ContentVersion || claimed.Language != job.Language || claimed.ProducerSHA256 != job.ProducerSHA256 || claimed.ProfileSHA256 != job.ProfileSHA256 || claimed.ContentInputs != job.ContentInputs {
			return lexicalError(409, "integrity_error")
		}
		for _, source := range plan.Sources {
			var doc entity.Document
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND kb_id = ? AND status = ?", source, job.DomainID, "1").Take(&doc).Error; err != nil {
				return lexicalError(409, "source_unavailable")
			}
		}
		if plan.ValidateLocked != nil {
			if err := plan.ValidateLocked(tx, current, policy); err != nil {
				return err
			}
		}
		publication, err := plan.Publication(time.Now())
		if err != nil {
			return err
		}
		if publication == nil || publication.OwnerID != job.OwnerID || publication.DomainID != job.DomainID || publication.SelectionSHA256 != job.SelectionSHA256 {
			return lexicalError(409, "integrity_error")
		}
		if err := tx.Create(publication).Error; err != nil {
			return err
		}
		result := tx.Model(&entity.KnowledgeLexicalJobRecord{}).Where("id = ? AND state = ? AND claim = ?", job.ID, "building", job.Claim).Updates(map[string]any{"state": "published", "manifest": publication.Payload, "index_id": publication.ID, "diagnostic": "", "claim": "", "lease_until": time.Unix(1, 0).UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return lexicalError(409, "policy_changed")
		}
		return nil
	})
}
