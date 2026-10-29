package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
	"time"
)

// StartKnowledgeLocalLexicalRuntime resumes only explicitly admitted local
// lexical jobs. It never opens the ordinary ingestion or model-use gates.
func StartKnowledgeLocalLexicalRuntime(ctx context.Context) {
	if dao.DB == nil {
		return
	}
	d := NewDatasetService()
	d.startKnowledgeLocalLexicalRuntime(ctx)
}

func (d *DatasetService) startKnowledgeLocalLexicalRuntime(ctx context.Context) {
	slots := make(chan struct{}, 2)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var candidates []entity.KnowledgeLexicalJobRecord
				if dao.DB.WithContext(ctx).Where("state = ? OR (state = ? AND lease_until < ?)", "queued", "building", time.Now().UTC()).Order("created_at ASC, id ASC").Limit(8).Find(&candidates).Error != nil {
					continue
				}
				for _, candidate := range candidates {
					select {
					case slots <- struct{}{}:
						go func(j entity.KnowledgeLexicalJobRecord) {
							defer func() { <-slots }()
							claim := common.GenerateUUID()
							now := time.Now().UTC()
							result := dao.DB.WithContext(ctx).Model(&entity.KnowledgeLexicalJobRecord{}).Where("id = ? AND (state = ? OR (state = ? AND lease_until < ?))", j.ID, "queued", "building", now).Updates(map[string]any{"state": "building", "claim": claim, "lease_until": now.Add(2 * time.Minute), "attempt_count": gorm.Expr("attempt_count + 1")})
							if result.Error == nil && result.RowsAffected == 1 {
								j.Claim = claim
								j.AttemptCount++
								j.State = "building"
								d.runKnowledgeLocalLexical(ctx, j)
							}
						}(candidate)
					default:
					}
				}
			}
		}
	}()
}

func (d *DatasetService) runKnowledgeLocalLexical(parent context.Context, job entity.KnowledgeLexicalJobRecord) {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	var err error
	switch job.Protocol {
	case "", entity.KnowledgeLocalLexicalProtocol:
		err = d.buildKnowledgeLocalLexical(ctx, job)
	case entity.KnowledgeContentLexicalProtocol:
		err = d.buildKnowledgeContentLexical(ctx, job)
	default:
		err = lexicalError(409, "integrity_error")
	}
	if err == nil {
		return
	}
	code := "index_failed"
	var public *KnowledgeLexicalError
	if errors.As(lexicalNormalize(err), &public) {
		code = public.Code
	}
	allowed := map[string]bool{"policy_disabled": true, "policy_changed": true, "profile_changed": true, "unit_oversize": true, "unsupported_representation": true, "source_unavailable": true, "source_changed": true, "empty_content": true, "integrity_error": true, "coverage_gap": true}
	if !allowed[code] {
		code = "index_failed"
	}
	state := "failed"
	if code == "index_failed" && job.AttemptCount < 3 {
		state = "queued"
	}
	finishCtx, done := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer done()
	dao.DB.WithContext(finishCtx).Model(&entity.KnowledgeLexicalJobRecord{}).Where("id = ? AND state = ? AND claim = ?", job.ID, "building", job.Claim).Updates(map[string]any{"state": state, "diagnostic": code, "claim": "", "lease_until": time.Unix(1, 0).UTC()})
}

func (d *DatasetService) buildKnowledgeLocalLexical(ctx context.Context, job entity.KnowledgeLexicalJobRecord) error {
	ctx = context.WithValue(ctx, lexicalSessionKey{}, lexicalSession{UserID: job.OwnerID, SHA256: job.SessionSHA256})
	kb, p, err := d.lexicalContext(ctx, job.OwnerID, job.DomainID, true)
	if err != nil {
		return err
	}
	if p.Version != job.PolicyVersion || p.FoundationVersion != job.FoundationVersion || lexicalLanguage(kb) != job.Language {
		return lexicalError(409, "policy_changed")
	}
	native, ok := d.docEngine.(lexicalEngine)
	if !ok || d.docEngine.GetType() != "elasticsearch" {
		return lexicalError(503, "unavailable")
	}
	producer, err := tokenizer.ProducerSHA256()
	if err != nil {
		return err
	}
	var selection entity.KnowledgeLexicalSelection
	if json.Unmarshal([]byte(job.Selection), &selection) != nil || selection.Validate(job.DomainID) != nil {
		return lexicalError(409, "integrity_error")
	}
	_, sha := lexicalSelection(selection)
	if sha != job.SelectionSHA256 {
		return lexicalError(409, "integrity_error")
	}
	units, coverage, _, err := d.lexicalUnits(ctx, job.OwnerID, job.DomainID, job.Language, producer, selection)
	if err != nil {
		return err
	}
	rows, tokenless, err := lexicalTokenize(ctx, job, units)
	if err != nil {
		return err
	}
	coverage.TokenlessBytes = tokenless
	m := lexicalManifest{Version: entity.KnowledgeLocalLexicalVersion, Owner: job.OwnerID, Domain: job.DomainID, Selection: selection, SelectionSHA256: sha, PolicyVersion: job.PolicyVersion, FoundationVersion: job.FoundationVersion, Producer: producer, Language: job.Language, Profile: lexicalProfile(job.Language, producer), Coverage: coverage, Rows: []lexicalManifestRow{}}
	for _, u := range units {
		m.Rows = append(m.Rows, u.Row)
	}
	planned := lexicalJSON(m)
	if job.Manifest != "" && job.Manifest != planned {
		return lexicalError(409, "integrity_error")
	}
	stored := dao.DB.WithContext(ctx).Model(&entity.KnowledgeLexicalJobRecord{}).Where("id = ? AND state = ? AND claim = ?", job.ID, "building", job.Claim).Update("manifest", planned)
	if stored.Error != nil {
		return stored.Error
	}
	if stored.RowsAffected != 1 {
		return lexicalError(409, "policy_changed")
	}
	if !coverage.Complete {
		return lexicalError(409, "coverage_gap")
	}
	sources := make([]string, len(selection.Targets))
	for i, target := range selection.Targets {
		sources[i] = target.Source.SourceID
	}
	return d.lexicalExecutePlan(ctx, job, kb, native, lexicalCorePlan{
		Rows: rows, Sources: sources,
		Verify: func(ctx context.Context) error { _, err := d.lexicalExpectedRows(ctx, &m); return err },
		Publication: func(now time.Time) (*entity.KnowledgeLexicalPublicationRecord, error) {
			m.PublishedAt = lexicalTime(now)
			payload := lexicalJSON(m)
			manifestSHA := lexicalSHA([]byte(payload))
			id := lexicalDigest("knowledge-local-index-publication/1", job.OwnerID, job.DomainID, manifestSHA)
			publication := &entity.KnowledgeLexicalPublicationRecord{ID: id, OwnerID: job.OwnerID, DomainID: job.DomainID, SelectionSHA256: sha, ManifestSHA256: manifestSHA, Payload: payload}
			if _, err := lexicalPublication(*publication); err != nil {
				return nil, err
			}
			return publication, nil
		},
	})
}
