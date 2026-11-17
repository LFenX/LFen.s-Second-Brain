package dataset

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func answerRequestRow(tx *gorm.DB, owner, domain, key string) (entity.KnowledgeAnswerRequestRecord, error) {
	var row entity.KnowledgeAnswerRequestRecord
	err := tx.Where("owner_id = ? AND domain_id = ? AND operation = ? AND key_sha256 = ?", owner, domain, "answer", lexicalSHA([]byte(key))).Take(&row).Error
	return row, err
}

func (r *answerRunner) answer(ctx context.Context, owner, domain string, req entity.KnowledgeAnswerRequest) (int, any, error) {
	p, err := r.current(ctx, owner, domain)
	if err != nil {
		return 0, nil, err
	}
	if !p.Enabled {
		return 0, nil, answerFailure(403, "model_use_denied")
	}
	if p.Version != req.ExpectedVersion {
		return 0, nil, answerFailure(409, "version_conflict")
	}
	m, err := r.validatePermit(ctx, owner, domain, p)
	if err != nil {
		return 0, nil, err
	}
	digest := lexicalSHA([]byte(lexicalJSON(req)))
	previous, err := answerRequestRow(dao.DB.WithContext(ctx), owner, domain, req.Key)
	if err == nil {
		return r.replay(ctx, owner, domain, p, previous, digest)
	}
	if err != gorm.ErrRecordNotFound {
		return 0, nil, err
	}
	query := req.Question
	if req.Query != nil {
		query = *req.Query
	}
	if !entity.ValidKnowledgeLexicalQuery(query) {
		return 0, nil, answerFailure(400, "invalid_request")
	}
	hits, err := r.search(ctx, owner, domain, entity.KnowledgeContentLexicalSearchRequest{Version: entity.KnowledgeContentVersion, Index: p.Scope.Index, Selection: p.Scope.Selection, Query: query, Limit: 5})
	if err != nil {
		return 0, nil, r.scopeError(ctx, owner, domain, err)
	}
	if hits == nil || len(hits.Hits) > 5 {
		return 0, nil, answerFailure(503, "unavailable")
	}
	evidence := make([]entity.KnowledgeAnswerCitation, 0, len(hits.Hits))
	bytes := 0
	for _, hit := range hits.Hits {
		text := hit.Text
		if len(text) == 0 || len(text) > 8192 || hit.Evidence.Validate(domain) != nil || hit.Evidence.Index != p.Scope.Index || hit.Evidence.Range.SHA256 == nil || *hit.Evidence.Range.SHA256 != lexicalSHA([]byte(text)) {
			return 0, nil, answerFailure(412, "scope_changed")
		}
		if bytes+len(text) > 32768 {
			break
		}
		citation := entity.KnowledgeAnswerCitation{ID: fmt.Sprintf("E%d", len(evidence)+1), Evidence: hit.Evidence, Text: text, ByteLength: len(text), SHA256: lexicalSHA([]byte(text))}
		evidence = append(evidence, citation)
		bytes += len(text)
	}
	// Search results are reread through the fixed-source owner before any model
	// request. Neither an index snippet nor an ID supplied by a client is trusted.
	if err := r.verifyCitations(ctx, owner, domain, evidence); err != nil {
		return 0, nil, err
	}
	row := entity.KnowledgeAnswerRequestRecord{ID: common.GenerateUUID(), OwnerID: owner, DomainID: domain, Operation: "answer", KeySHA256: lexicalSHA([]byte(req.Key)), RequestSHA256: digest, PolicyVersion: p.Version, ScopeSHA256: *p.ScopeSHA256, State: "running", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if len(evidence) > 0 {
		row.ProviderCalls = 1
	}
	found := false
	err = r.locked(ctx, owner, domain, func(tx *gorm.DB, policy entity.KnowledgeAnswerPolicyRecord) error {
		if err := r.permitLocked(ctx, owner, domain, policy, p); err != nil {
			return err
		}
		existing, err := answerRequestRow(tx, owner, domain, req.Key)
		if err == nil {
			previous, found = existing, true
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		var active, recent int64
		if err := tx.Model(&entity.KnowledgeAnswerRequestRecord{}).Where("owner_id = ? AND domain_id = ? AND operation = ? AND state = ? AND created_at > ?", owner, domain, "answer", "running", time.Now().UTC().Add(-90*time.Second)).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return answerFailure(429, "rate_limited")
		}
		if err := tx.Model(&entity.KnowledgeAnswerRequestRecord{}).Where("owner_id = ? AND operation = ? AND created_at > ?", owner, "answer", time.Now().UTC().Add(-time.Minute)).Count(&recent).Error; err != nil {
			return err
		}
		if recent >= 30 {
			return answerFailure(429, "rate_limited")
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return 0, nil, err
	}
	if found {
		return r.replay(ctx, owner, domain, p, previous, digest)
	}
	result := entity.KnowledgeAnswerResult{ID: row.ID, State: "completed", PolicyVersion: p.Version, ScopeSHA256: *p.ScopeSHA256, Model: *p.Model, Citations: []entity.KnowledgeAnswerCitation{}, ProviderCalls: row.ProviderCalls}
	if len(evidence) == 0 {
		result.Answer, result.Insufficient = "所选固定资料中没有找到支持答案的证据。", true
	} else {
		// The durable dispatch marker is committed and SQL locks are released.
		// This is the only provider call; no retry or failover path exists here.
		output, usage, issue := m.generate(ctx, req.Question, evidence)
		if issue != nil {
			return r.fail(ctx, row, answerFailure(502, "provider_outcome_unknown"))
		}
		var parsed entity.KnowledgeAnswerModelOutput
		if entity.KnowledgeAnswerDecode([]byte(output), "ModelOutput", &parsed) != nil || strings.TrimSpace(parsed.Answer) == "" {
			return r.fail(ctx, row, answerFailure(502, "model_output_invalid"))
		}
		result.Answer, result.Insufficient, result.Usage = parsed.Answer, parsed.Insufficient, usage
		byID := make(map[string]entity.KnowledgeAnswerCitation, len(evidence))
		for _, item := range evidence {
			byID[item.ID] = item
		}
		for _, id := range parsed.EvidenceIDs {
			item, ok := byID[id]
			if !ok {
				return r.fail(ctx, row, answerFailure(502, "model_output_invalid"))
			}
			result.Citations = append(result.Citations, item)
		}
	}
	if err := r.verifyCitations(ctx, owner, domain, result.Citations); err != nil {
		return r.fail(ctx, row, err)
	}
	err = r.locked(ctx, owner, domain, func(tx *gorm.DB, policy entity.KnowledgeAnswerPolicyRecord) error {
		if err := r.permitLocked(ctx, owner, domain, policy, p); err != nil {
			return err
		}
		if _, err := answerEncode("answer", common.GenerateUUID(), result); err != nil {
			return err
		}
		update := tx.Model(&entity.KnowledgeAnswerRequestRecord{}).Where("id = ? AND owner_id = ? AND state = ? AND request_sha256 = ?", row.ID, owner, "running", digest).Updates(map[string]any{"state": "completed", "response_json": lexicalJSON(result), "updated_at": time.Now().UTC()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return answerFailure(503, "unavailable")
		}
		return nil
	})
	if err != nil {
		return r.fail(ctx, row, err)
	}
	return 200, result, nil
}

func (r *answerRunner) verifyCitations(ctx context.Context, owner, domain string, citations []entity.KnowledgeAnswerCitation) error {
	for _, item := range citations {
		read, err := r.read(ctx, owner, domain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: item.Evidence})
		if err != nil {
			return r.scopeError(ctx, owner, domain, err)
		}
		if read == nil || !read.Complete || read.Text != item.Text || read.ByteLength != item.ByteLength || lexicalJSON(read.Evidence) != lexicalJSON(item.Evidence) || lexicalSHA([]byte(read.Text)) != item.SHA256 {
			return answerFailure(412, "scope_changed")
		}
	}
	return nil
}

func (r *answerRunner) replay(ctx context.Context, owner, domain string, p entity.KnowledgeAnswerPolicy, row entity.KnowledgeAnswerRequestRecord, digest string) (int, any, error) {
	if row.RequestSHA256 != digest {
		return 0, nil, answerFailure(409, "idempotency_conflict")
	}
	if row.PolicyVersion != p.Version || row.ScopeSHA256 != *p.ScopeSHA256 {
		return 0, nil, answerFailure(409, "version_conflict")
	}
	if row.State == "failed" {
		return 0, nil, &KnowledgeAnswerError{Status: 502, Code: row.FailureCode, ID: accessPtr(row.ID)}
	}
	if row.State == "running" {
		if time.Since(row.CreatedAt) > 90*time.Second {
			return 0, nil, &KnowledgeAnswerError{Status: 502, Code: "provider_outcome_unknown", ID: accessPtr(row.ID)}
		}
		return 202, entity.KnowledgeAnswerRunning{ID: row.ID, State: "running", PolicyVersion: p.Version, ScopeSHA256: *p.ScopeSHA256, Model: *p.Model, ProviderCalls: row.ProviderCalls, Replayed: true}, nil
	}
	if row.State != "completed" || time.Since(row.CreatedAt) >= 24*time.Hour || row.ResponseJSON == "" {
		return 0, nil, answerFailure(409, "idempotency_conflict")
	}
	var result entity.KnowledgeAnswerResult
	if entity.KnowledgeAnswerDecode([]byte(row.ResponseJSON), "Completed", &result) != nil || result.ID != row.ID || result.Model != *p.Model || result.PolicyVersion != p.Version || result.ScopeSHA256 != *p.ScopeSHA256 || result.ProviderCalls != row.ProviderCalls {
		return 0, nil, answerFailure(503, "unavailable")
	}
	if err := r.verifyCitations(ctx, owner, domain, result.Citations); err != nil {
		return 0, nil, err
	}
	if err := r.locked(ctx, owner, domain, func(_ *gorm.DB, policy entity.KnowledgeAnswerPolicyRecord) error {
		return r.permitLocked(ctx, owner, domain, policy, p)
	}); err != nil {
		return 0, nil, err
	}
	result.Replayed = true
	return 200, result, nil
}

func (r *answerRunner) fail(ctx context.Context, row entity.KnowledgeAnswerRequestRecord, err error) (int, any, error) {
	e := normalizeAnswerError(err)
	e.ID = accessPtr(row.ID)
	// A client disconnect cannot erase the dispatch marker or cause an automatic
	// resend. Failure persistence carries no answer/context/provider diagnostics.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	update := dao.DB.WithContext(cleanup).Model(&entity.KnowledgeAnswerRequestRecord{}).Where("id = ? AND state = ?", row.ID, "running").Updates(map[string]any{"state": "failed", "failure_code": e.Code, "response_json": "", "updated_at": time.Now().UTC()})
	if update.Error != nil {
		e.Code, e.Status = "provider_outcome_unknown", 502
	}
	return 0, nil, e
}
