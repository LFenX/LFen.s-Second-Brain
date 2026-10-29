package dataset

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
)

func (d *DatasetService) CreateKnowledgeContentLexicalIndex(ctx context.Context, owner, domain string, req entity.KnowledgeContentLexicalCreateRequest) (*entity.KnowledgeContentLexicalJob, bool, error) {
	if _, _, err := d.lexicalContext(ctx, owner, domain, true); err != nil {
		return nil, false, err
	}
	if req.Version != entity.KnowledgeContentVersion {
		return nil, false, lexicalError(412, "contract_mismatch")
	}
	if req.Validate(domain) != nil {
		return nil, false, lexicalError(400, "invalid_request")
	}
	if d.docEngine == nil || d.docEngine.GetType() != "elasticsearch" {
		return nil, false, lexicalError(503, "unavailable")
	}
	if _, ok := d.docEngine.(lexicalEngine); !ok {
		return nil, false, lexicalError(503, "unavailable")
	}
	req.Selection, _ = contentLexicalSelection(req.Selection)
	opID := lexicalDigest("knowledge-content-operation/0.2.0", owner, domain, "index", req.Key)
	requestSHA := lexicalSHA([]byte(lexicalJSON(req)))
	find := func(tx *gorm.DB, job *entity.KnowledgeLexicalJobRecord) (bool, error) {
		var op entity.KnowledgeLexicalOperationRecord
		err := tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND kind = ?", opID, owner, domain, "content_index").Take(&op).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if op.RequestSHA256 != requestSHA {
			return false, lexicalError(409, "idempotency_conflict")
		}
		err = tx.Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", op.Payload, owner, domain, entity.KnowledgeContentLexicalProtocol).Take(job).Error
		return err == nil, err
	}
	var job entity.KnowledgeLexicalJobRecord
	found, err := find(dao.DB.WithContext(ctx), &job)
	if err != nil {
		return nil, false, contentLexicalNormalize(err)
	}
	if found {
		view, err := d.contentLexicalJobView(ctx, job)
		return view, false, err
	}
	prepared, err := d.prepareKnowledgeContentLexical(ctx, owner, domain, req.Selection, true)
	if err != nil {
		return nil, false, err
	}
	if len(prepared.Rows) == 0 {
		return nil, false, lexicalError(409, "empty_content")
	}
	if prepared.Policies.LocalVersion != req.ExpectedLocalVersion || prepared.Policies.ContentVersion != req.ExpectedContentVersion || prepared.Policies.FoundationVersion != req.ExpectedFoundationVersion {
		return nil, false, lexicalError(409, "policy_changed")
	}
	if prepared.ProfileSHA256 != req.ExpectedProfileSHA256 {
		return nil, false, lexicalError(409, "profile_changed")
	}
	created := false
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kb, local, err := lexicalLock(ctx, tx, owner, domain)
		if err != nil {
			return err
		}
		found, err := find(tx, &job)
		if err != nil || found {
			return err
		}
		content, err := parsedPolicyRecord(tx.Clauses(clause.Locking{Strength: "UPDATE"}), owner, domain)
		if err != nil {
			return err
		}
		if !local.Enabled || !content.LexicalEnabled {
			return lexicalError(409, "policy_disabled")
		}
		if local.Version != req.ExpectedLocalVersion || content.Version != req.ExpectedContentVersion || kb.KnowledgePolicyVersion != req.ExpectedFoundationVersion || lexicalLanguage(kb) != prepared.Policies.Language {
			return lexicalError(409, "policy_changed")
		}
		producer, err := tokenizer.ProducerSHA256()
		if err != nil || producer != prepared.ProducerSHA256 || contentLexicalProfile(lexicalLanguage(kb), producer).SHA256 != prepared.ProfileSHA256 {
			return lexicalError(409, "profile_changed")
		}
		var active int64
		if err := tx.Model(&entity.KnowledgeLexicalJobRecord{}).Where("domain_id = ? AND state IN ?", domain, []string{"queued", "building"}).Count(&active).Error; err != nil {
			return err
		}
		if active >= 4 {
			return lexicalError(429, "capacity_exceeded")
		}
		sources, err := contentLexicalCaptureSources(tx.Clauses(clause.Locking{Strength: "UPDATE"}), kb, prepared.Selection)
		if err != nil {
			return err
		}
		inputs := contentLexicalInputs{Version: entity.KnowledgeContentVersion, SelectionSHA256: prepared.SelectionSHA256, ProfileSHA256: prepared.ProfileSHA256, ProducerSHA256: prepared.ProducerSHA256, Policies: prepared.Policies, Sources: sources}
		job = entity.KnowledgeLexicalJobRecord{Protocol: entity.KnowledgeContentLexicalProtocol, ID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, Key: req.Key, RequestSHA256: requestSHA, SelectionSHA256: prepared.SelectionSHA256, Selection: lexicalJSON(prepared.Selection), PolicyVersion: local.Version, FoundationVersion: kb.KnowledgePolicyVersion, ContentVersion: content.Version, ProducerSHA256: prepared.ProducerSHA256, ProfileSHA256: prepared.ProfileSHA256, ContentInputs: lexicalJSON(inputs), Language: lexicalLanguage(kb), SessionSHA256: ctx.Value(lexicalSessionKey{}).(lexicalSession).SHA256, State: "queued", Diagnostic: "building"}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&entity.KnowledgeLexicalOperationRecord{ID: opID, OwnerID: owner, DomainID: domain, Kind: "content_index", Key: req.Key, RequestSHA256: requestSHA, Payload: job.ID}).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, contentLexicalNormalize(err)
	}
	view, err := d.contentLexicalJobView(ctx, job)
	return view, created, err
}

func (d *DatasetService) GetKnowledgeContentLexicalJob(ctx context.Context, owner, domain, id string) (*entity.KnowledgeContentLexicalJob, error) {
	if _, _, err := d.lexicalContext(ctx, owner, domain, false); err != nil {
		return nil, err
	}
	if !entity.ValidKnowledgeLexicalID(id) {
		return nil, lexicalError(404, "not_found")
	}
	var job entity.KnowledgeLexicalJobRecord
	if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", id, owner, domain, entity.KnowledgeContentLexicalProtocol).Take(&job).Error; err != nil {
		return nil, contentLexicalNormalize(err)
	}
	return d.contentLexicalJobView(ctx, job)
}

func (d *DatasetService) buildKnowledgeContentLexical(ctx context.Context, job entity.KnowledgeLexicalJobRecord) error {
	ctx = context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: job.OwnerID, SHA256: job.SessionSHA256})
	kb, _, err := d.lexicalContext(ctx, job.OwnerID, job.DomainID, true)
	if err != nil {
		return err
	}
	var selection entity.KnowledgeContentLexicalSelection
	if job.Protocol != entity.KnowledgeContentLexicalProtocol || entity.KnowledgeParsedDecode([]byte(job.Selection), &selection) != nil || selection.Validate(job.DomainID) != nil || lexicalJSON(selection) != job.Selection {
		return lexicalError(409, "integrity_error")
	}
	prepared, err := d.prepareKnowledgeContentLexical(ctx, job.OwnerID, job.DomainID, selection, true)
	if err != nil {
		return err
	}
	if prepared.SelectionSHA256 != job.SelectionSHA256 || prepared.Policies.LocalVersion != job.PolicyVersion || prepared.Policies.ContentVersion != job.ContentVersion || prepared.Policies.FoundationVersion != job.FoundationVersion || prepared.Policies.Language != job.Language {
		return lexicalError(409, "policy_changed")
	}
	if prepared.ProducerSHA256 != job.ProducerSHA256 || prepared.ProfileSHA256 != job.ProfileSHA256 {
		return lexicalError(409, "profile_changed")
	}
	if err := contentLexicalValidateInputs(dao.DB.WithContext(ctx), kb, job, prepared); err != nil {
		return err
	}
	native, ok := d.docEngine.(lexicalEngine)
	if !ok || d.docEngine.GetType() != "elasticsearch" {
		return lexicalError(503, "unavailable")
	}
	rows, tokenless, err := lexicalTokenizeInputs(ctx, job, prepared.Inputs)
	if err != nil {
		return err
	}
	var captured contentLexicalInputs
	if entity.KnowledgeParsedDecode([]byte(job.ContentInputs), &captured) != nil {
		return lexicalError(409, "integrity_error")
	}
	manifest := contentLexicalManifest{Version: entity.KnowledgeContentVersion, Protocol: entity.KnowledgeContentLexicalProtocol, Owner: job.OwnerID, Domain: job.DomainID, Selection: prepared.Selection, SelectionSHA256: prepared.SelectionSHA256, Policies: prepared.Policies, ProducerSHA256: prepared.ProducerSHA256, ProfileSHA256: prepared.ProfileSHA256, Coverage: prepared.Coverage, TokenlessBytes: tokenless, Rows: prepared.Rows, Sources: captured.Sources}
	planned := lexicalJSON(manifest)
	if len(prepared.Rows) == 0 || len(planned) > 32<<20 || job.Manifest != "" && job.Manifest != planned {
		return lexicalError(409, "integrity_error")
	}
	stored := dao.DB.WithContext(ctx).Model(&entity.KnowledgeLexicalJobRecord{}).Where("id = ? AND protocol = ? AND state = ? AND claim = ?", job.ID, entity.KnowledgeContentLexicalProtocol, "building", job.Claim).Update("manifest", planned)
	if stored.Error != nil {
		return stored.Error
	}
	if stored.RowsAffected != 1 {
		return lexicalError(409, "policy_changed")
	}
	sources := make([]string, len(selection.Targets))
	for i, target := range selection.Targets {
		sources[i] = target.Source.SourceID
	}
	return d.lexicalExecutePlan(ctx, job, kb, native, lexicalCorePlan{
		Rows: rows, Sources: sources,
		Verify: func(ctx context.Context) error {
			if err := d.verifyKnowledgeContentLexical(ctx, job.OwnerID, job.DomainID, prepared); err != nil {
				return err
			}
			return contentLexicalValidateInputs(dao.DB.WithContext(ctx), kb, job, prepared)
		},
		ValidateLocked: func(tx *gorm.DB, current *entity.Knowledgebase, _ entity.KnowledgeLexicalPolicyRecord) error {
			return contentLexicalValidateInputs(tx.Clauses(clause.Locking{Strength: "UPDATE"}), current, job, prepared)
		},
		Publication: func(now time.Time) (*entity.KnowledgeLexicalPublicationRecord, error) {
			manifest.PublishedAt = lexicalTime(now)
			payload := lexicalJSON(manifest)
			sha := lexicalSHA([]byte(payload))
			publication := &entity.KnowledgeLexicalPublicationRecord{Protocol: entity.KnowledgeContentLexicalProtocol, ID: lexicalDigest("knowledge-content-index-publication/0.2.0", job.OwnerID, job.DomainID, sha), OwnerID: job.OwnerID, DomainID: job.DomainID, SelectionSHA256: job.SelectionSHA256, ManifestSHA256: sha, Payload: payload}
			if _, err := contentLexicalPublication(*publication); err != nil {
				return nil, err
			}
			return publication, nil
		},
	})
}
