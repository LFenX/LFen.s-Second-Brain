package dataset

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
)

func contentLexicalPolicyView(domain string, capture contentLexicalPolicyCapture) entity.KnowledgeContentLexicalPolicy {
	return entity.KnowledgeContentLexicalPolicy{DomainID: domain, LocalVersion: capture.LocalVersion, ContentVersion: capture.ContentVersion, FoundationVersion: capture.FoundationVersion, LocalEnabled: capture.LocalEnabled, ContentEnabled: capture.ContentEnabled, Enabled: capture.LocalEnabled && capture.ContentEnabled, Mode: "owner_only_local_lexical"}
}

func contentLexicalReadiness(selection entity.KnowledgeContentLexicalSelection, sha, kind, current, diagnostic string) entity.KnowledgeContentLexicalReadiness {
	value := entity.KnowledgeContentLexicalReadiness{Kind: kind, Publication: "none", Selection: selection, SelectionSHA256: sha, Currentness: current, Coverage: []entity.KnowledgeContentLexicalCoverage{}, Diagnostics: []string{}}
	if diagnostic != "" {
		value.Diagnostics = append(value.Diagnostics, diagnostic)
	}
	return value
}

func contentLexicalReady(manifest *contentLexicalManifest, index entity.KnowledgeLexicalIndexRef, current string, policies contentLexicalPolicyCapture) entity.KnowledgeContentLexicalReadiness {
	value := contentLexicalReadiness(manifest.Selection, manifest.SelectionSHA256, "ready", current, "")
	value.Publication, value.Index, value.Coverage = "published", &index, manifest.Coverage
	if !policies.LocalEnabled || !policies.ContentEnabled {
		value.Kind, value.Diagnostics = "stale", []string{"policy_disabled"}
	} else if policies != manifest.Policies {
		value.Kind, value.Diagnostics = "stale", []string{"policy_changed"}
	} else if producer, err := tokenizer.ProducerSHA256(); err != nil || producer != manifest.ProducerSHA256 || contentLexicalProfile(policies.Language, producer).SHA256 != manifest.ProfileSHA256 {
		value.Kind, value.Diagnostics = "stale", []string{"profile_changed"}
	}
	return value
}

func (d *DatasetService) contentLexicalJobView(ctx context.Context, job entity.KnowledgeLexicalJobRecord) (*entity.KnowledgeContentLexicalJob, error) {
	policies, err := d.contentLexicalPolicies(ctx, job.OwnerID, job.DomainID, false)
	if err != nil {
		return nil, err
	}
	var selection entity.KnowledgeContentLexicalSelection
	if job.Protocol != entity.KnowledgeContentLexicalProtocol || entity.KnowledgeParsedDecode([]byte(job.Selection), &selection) != nil || selection.Validate(job.DomainID) != nil || lexicalJSON(selection) != job.Selection || !entity.ValidKnowledgeLexicalVersion(job.PolicyVersion, false) || !entity.ValidKnowledgeLexicalVersion(job.ContentVersion, false) || !entity.ValidKnowledgeLexicalVersion(job.FoundationVersion, false) || !entity.ValidKnowledgeLexicalHash(job.ProfileSHA256) || !entity.ValidKnowledgeLexicalHash(job.ProducerSHA256) {
		return nil, lexicalError(409, "integrity_error")
	}
	selection, sha := contentLexicalSelection(selection)
	if sha != job.SelectionSHA256 {
		return nil, lexicalError(409, "integrity_error")
	}
	current, err := d.contentLexicalCurrentness(ctx, job.OwnerID, job.DomainID, selection)
	if err != nil {
		return nil, err
	}
	readiness := contentLexicalReadiness(selection, sha, "building", current, "building")
	var index *entity.KnowledgeLexicalIndexRef
	switch job.State {
	case "queued", "building":
	case "failed":
		readiness = contentLexicalReadiness(selection, sha, "failed", current, job.Diagnostic)
	case "published":
		var record entity.KnowledgeLexicalPublicationRecord
		if err := dao.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND domain_id = ? AND protocol = ?", job.IndexID, job.OwnerID, job.DomainID, entity.KnowledgeContentLexicalProtocol).Take(&record).Error; err != nil {
			return nil, contentLexicalNormalize(err)
		}
		manifest, err := contentLexicalPublication(record)
		if err != nil || manifest.SelectionSHA256 != job.SelectionSHA256 || lexicalJSON(manifest.Selection) != job.Selection || manifest.Policies.LocalVersion != job.PolicyVersion || manifest.Policies.ContentVersion != job.ContentVersion || manifest.Policies.FoundationVersion != job.FoundationVersion || manifest.ProfileSHA256 != job.ProfileSHA256 || manifest.ProducerSHA256 != job.ProducerSHA256 {
			return nil, lexicalError(409, "integrity_error")
		}
		index = &entity.KnowledgeLexicalIndexRef{ID: record.ID, ManifestSHA256: record.ManifestSHA256}
		readiness = contentLexicalReady(manifest, *index, current, policies)
	default:
		return nil, lexicalError(409, "integrity_error")
	}
	if _, _, err := d.lexicalContext(ctx, job.OwnerID, job.DomainID, false); err != nil {
		return nil, err
	}
	return &entity.KnowledgeContentLexicalJob{ID: job.ID, Key: job.Key, RequestSHA256: job.RequestSHA256, State: job.State, Selection: selection, LocalVersion: job.PolicyVersion, ContentVersion: job.ContentVersion, FoundationVersion: job.FoundationVersion, ProfileSHA256: job.ProfileSHA256, ProducerSHA256: job.ProducerSHA256, Index: index, Readiness: readiness, CreatedAt: lexicalTime(job.CreatedAt), UpdatedAt: lexicalTime(job.UpdatedAt)}, nil
}

func (d *DatasetService) InspectKnowledgeContentLexical(ctx context.Context, owner, domain string, req entity.KnowledgeContentLexicalInspectRequest) (*entity.KnowledgeContentLexicalInspectData, error) {
	if _, _, err := d.lexicalContext(ctx, owner, domain, false); err != nil {
		return nil, err
	}
	if req.Version != entity.KnowledgeContentVersion {
		return nil, lexicalError(412, "contract_mismatch")
	}
	if req.Selection.Validate(domain) != nil {
		return nil, lexicalError(400, "invalid_request")
	}
	prepared, err := d.prepareKnowledgeContentLexical(ctx, owner, domain, req.Selection, false)
	if err != nil {
		return nil, err
	}
	current, err := d.contentLexicalCurrentness(ctx, owner, domain, prepared.Selection)
	if err != nil {
		return nil, err
	}
	readiness := contentLexicalReadiness(prepared.Selection, prepared.SelectionSHA256, "unindexed", current, "none")
	readiness.Coverage = prepared.Coverage
	if len(prepared.Rows) == 0 {
		readiness.Kind, readiness.Diagnostics = "empty", []string{"empty_content"}
	}
	var record entity.KnowledgeLexicalPublicationRecord
	err = dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND selection_sha256 = ? AND protocol = ?", owner, domain, prepared.SelectionSHA256, entity.KnowledgeContentLexicalProtocol).Order("created_at DESC, id DESC").Take(&record).Error
	if err == nil {
		manifest, err := contentLexicalPublication(record)
		if err != nil {
			return nil, err
		}
		index := entity.KnowledgeLexicalIndexRef{ID: record.ID, ManifestSHA256: record.ManifestSHA256}
		readiness = contentLexicalReady(manifest, index, current, prepared.Policies)
		if readiness.Kind == "ready" {
			kb, _, err := d.lexicalContext(ctx, owner, domain, true)
			if err != nil {
				return nil, err
			}
			native, ok := d.docEngine.(lexicalEngine)
			if !ok || d.docEngine.GetType() != "elasticsearch" {
				return nil, lexicalError(503, "unavailable")
			}
			rows, err := d.contentLexicalExpectedRows(ctx, manifest)
			if err != nil {
				return nil, err
			}
			if err := native.VerifyKnowledgeLexicalRows(ctx, "ragflow_"+kb.TenantID, domain, rows); err != nil {
				return nil, lexicalError(409, "integrity_error")
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, contentLexicalNormalize(err)
	} else {
		var job entity.KnowledgeLexicalJobRecord
		err = dao.DB.WithContext(ctx).Where("owner_id = ? AND domain_id = ? AND selection_sha256 = ? AND protocol = ?", owner, domain, prepared.SelectionSHA256, entity.KnowledgeContentLexicalProtocol).Order("created_at DESC, id DESC").Take(&job).Error
		if err == nil {
			view, err := d.contentLexicalJobView(ctx, job)
			if err != nil {
				return nil, err
			}
			readiness = view.Readiness
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contentLexicalNormalize(err)
		}
	}
	latest, err := d.contentLexicalPolicies(ctx, owner, domain, false)
	if err != nil {
		return nil, err
	}
	if latest != prepared.Policies {
		readiness.Kind, readiness.Diagnostics = "stale", []string{"policy_changed"}
	}
	if !latest.LocalEnabled || !latest.ContentEnabled {
		if readiness.Index != nil {
			readiness.Kind = "stale"
		}
		readiness.Diagnostics = []string{"policy_disabled"}
	}
	return &entity.KnowledgeContentLexicalInspectData{Policy: contentLexicalPolicyView(domain, latest), Profile: contentLexicalProfile(prepared.Policies.Language, prepared.ProducerSHA256), Readiness: readiness}, nil
}
