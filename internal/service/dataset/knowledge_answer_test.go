package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func answerTestRunner(t *testing.T) (*answerRunner, context.Context, *atomic.Int32, entity.KnowledgeAnswerScope) {
	t.Helper()
	d, ctx := lexicalTestContext(t)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeAnswerPolicyRecord{}, &entity.KnowledgeAnswerRequestRecord{}); err != nil {
		t.Fatal(err)
	}
	r := newAnswerRunner(d)
	calls := &atomic.Int32{}
	m := &answerModel{ref: entity.KnowledgeAnswerModelRef{ID: strings.Repeat("b", 32), SHA256: strings.Repeat("c", 64)}, name: "synthetic model", destination: "https://api.deepseek.com", generate: func(context.Context, string, []entity.KnowledgeAnswerCitation) (string, *entity.KnowledgeAnswerUsage, error) {
		calls.Add(1)
		return `{"answer":"47","insufficient_evidence":false,"evidence_ids":["E1"]}`, &entity.KnowledgeAnswerUsage{Input: 120, Output: 18, Total: 138}, nil
	}}
	r.resolve = func(context.Context, string) (*answerModel, error) { return m, nil }
	r.checkScope = func(context.Context, string, string, entity.KnowledgeAnswerScope) error { return nil }
	sha, revision := lexicalSHA([]byte("SYNTHETIC ORION project quota 47")), strings.Repeat("d", 32)
	target := entity.KnowledgeContentTarget{Source: entity.KnowledgeContentSourceRef{DomainID: knowledgeTestDomain, SourceID: strings.Repeat("e", 32), RevisionID: revision, SHA256: sha}, Representation: entity.KnowledgeContentRepresentationRef{Kind: "source_revision", ID: &revision}}
	scope := entity.KnowledgeAnswerScope{Index: entity.KnowledgeLexicalIndexRef{ID: strings.Repeat("1", 64), ManifestSHA256: strings.Repeat("2", 64)}, Selection: entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{target}}}
	text := "SYNTHETIC ORION project quota 47"
	evidence := entity.KnowledgeContentLexicalEvidence{Version: entity.KnowledgeContentVersion, Index: scope.Index, RowID: strings.Repeat("3", 64), Address: entity.KnowledgeContentAddress{Target: target, Node: entity.KnowledgeContentNodeRef{Kind: "c1_node", ID: accessPtr(strings.Repeat("4", 64))}}, CoordinateSpace: "source_utf8", ReadingSHA256: sha, Range: entity.KnowledgeContentRange{EndByte: int64(len(text)), SHA256: &sha}, Locator: entity.KnowledgeContentLocator{Kind: "source_utf8", SourceUTF8: &entity.KnowledgeContentSourceLocator{EndByte: int64(len(text)), StartLine: 1, EndLine: 1}}, CoordinatePrecision: "source_bytes", Assembly: "c1_root_subrange_exact_bytes"}
	r.search = func(context.Context, string, string, entity.KnowledgeContentLexicalSearchRequest) (*entity.KnowledgeContentLexicalSearchData, error) {
		return &entity.KnowledgeContentLexicalSearchData{Hits: []entity.KnowledgeContentLexicalHit{{Evidence: evidence, Text: text}}}, nil
	}
	r.read = func(context.Context, string, string, entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error) {
		return &entity.KnowledgeContentLexicalReadData{Evidence: evidence, Text: text, ByteLength: len(text), Complete: true}, nil
	}
	return r, ctx, calls, scope
}

func enableAnswerTest(t *testing.T, r *answerRunner, ctx context.Context, scope entity.KnowledgeAnswerScope) entity.KnowledgeAnswerPolicy {
	t.Helper()
	m, err := r.resolve(ctx, knowledgeTestOwner)
	if err != nil {
		t.Fatal(err)
	}
	req := entity.KnowledgeAnswerPolicySetRequest{Version: entity.KnowledgeAnswerVersion, Enabled: true, Scope: &scope, Model: &m.ref, Confirmed: true, ExternalSendConfirmed: true, Key: "enable"}
	policy, err := r.setPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func answerTestRequest() entity.KnowledgeAnswerRequest {
	return entity.KnowledgeAnswerRequest{Version: entity.KnowledgeAnswerVersion, ExpectedVersion: 1, Question: "ORION project quota?", Key: "one-request"}
}

func TestKnowledgeAnswerPolicyUsesPersistedTimestamp(t *testing.T) {
	r, ctx, calls, scope := answerTestRunner(t)
	now := time.Date(2026, 11, 17, 10, 0, 0, 123456789, time.UTC)
	originalNow := dao.DB.NowFunc
	dao.DB.NowFunc = func() time.Time { return now }
	t.Cleanup(func() { dao.DB.NowFunc = originalNow })
	// Model a database column that stores milliseconds while the ORM keeps
	// nanoseconds in its in-memory record after the write.
	truncate := func(tx *gorm.DB) {
		if tx.Statement.Table != "knowledge_answer_policy" {
			return
		}
		row, ok := tx.Statement.Dest.(*entity.KnowledgeAnswerPolicyRecord)
		if !ok {
			return
		}
		err := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
			Table(row.TableName()).Where("domain_id = ?", row.DomainID).
			UpdateColumn("updated_at", row.UpdatedAt.Truncate(time.Millisecond)).Error
		tx.AddError(err)
	}
	const callback = "test:answer_timestamp_precision"
	if err := dao.DB.Callback().Create().After("gorm:create").Register(callback, truncate); err != nil {
		t.Fatal(err)
	}
	if err := dao.DB.Callback().Update().After("gorm:update").Register(callback, truncate); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		dao.DB.Callback().Create().Remove(callback)
		dao.DB.Callback().Update().Remove(callback)
	})
	m, err := r.resolve(ctx, knowledgeTestOwner)
	if err != nil {
		t.Fatal(err)
	}
	req := entity.KnowledgeAnswerPolicySetRequest{Version: entity.KnowledgeAnswerVersion, Enabled: true, Scope: &scope, Model: &m.ref, Confirmed: true, ExternalSendConfirmed: true, Key: "timestamp-enable"}
	for _, enabled := range []bool{true, false} {
		if !enabled {
			req.ExpectedVersion, req.Enabled = 1, false
			req.Scope, req.Model = nil, nil
			req.ExternalSendConfirmed, req.Key = false, "timestamp-disable"
			now = now.Add(time.Second)
		}
		first, err := r.setPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := r.setPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
		if err != nil {
			t.Fatal(err)
		}
		current, err := r.current(ctx, knowledgeTestOwner, knowledgeTestDomain)
		if err != nil || lexicalJSON(first) != lexicalJSON(replay) || lexicalJSON(first) != lexicalJSON(current) {
			t.Fatal("initial policy differs from persisted replay/current policy", err)
		}
		if first.ConsentTime == nil || *first.ConsentTime != accessTime(now.Truncate(time.Millisecond)) || first.Enabled != enabled {
			t.Fatal("policy did not use persisted time/enable state")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("policy changes called the model")
	}
}

func TestKnowledgeAnswerDefaultsConsentReplayAndOldGuard(t *testing.T) {
	r, ctx, calls, scope := answerTestRunner(t)
	p, err := r.current(ctx, knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || p.Enabled || p.Version != 0 {
		t.Fatal(p, err)
	}
	_, _, err = r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
	if normalizeAnswerError(err).Code != "model_use_denied" || calls.Load() != 0 {
		t.Fatal(err, calls.Load())
	}
	if err := permission.CheckDatasetModelUse(ctx, dao.DB, []string{knowledgeTestDomain}, "generation"); !errors.Is(err, permission.ErrKnowledgeModelUseDenied) {
		t.Fatal("old generic guard opened", err)
	}
	enableAnswerTest(t, r, ctx, scope)
	status, data, err := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
	if err != nil || status != 200 {
		t.Fatal(status, err)
	}
	result := data.(entity.KnowledgeAnswerResult)
	if result.Answer != "47" || len(result.Citations) != 1 || calls.Load() != 1 {
		t.Fatal("wrong result/calls")
	}
	if _, err := answerEncode("answer", strings.Repeat("f", 32), result); err != nil {
		t.Fatal("wire schema", err)
	}
	_, data, err = r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
	if err != nil || !data.(entity.KnowledgeAnswerResult).Replayed || calls.Load() != 1 {
		t.Fatal("replay sent model", err)
	}
	changed := answerTestRequest()
	changed.Question = "different question"
	_, _, err = r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, changed)
	if normalizeAnswerError(err).Code != "idempotency_conflict" || calls.Load() != 1 {
		t.Fatal(err)
	}
	if _, err := r.current(ctx, knowledgeTestOwner, knowledgeTestOther); err == nil {
		t.Fatal("unregistered domain accepted")
	}
	if _, err := r.current(ctx, knowledgeTestOther, knowledgeTestDomain); err == nil {
		t.Fatal("foreign owner accepted")
	}
}

func TestKnowledgeAnswerNoHitAndInvalidProviderOutput(t *testing.T) {
	for _, test := range []string{"no-hit", "unknown-id", "duplicate-id", "byte-changed", "timeout"} {
		t.Run(test, func(t *testing.T) {
			r, ctx, calls, scope := answerTestRunner(t)
			enableAnswerTest(t, r, ctx, scope)
			m, _ := r.resolve(ctx, knowledgeTestOwner)
			switch test {
			case "no-hit":
				r.search = func(context.Context, string, string, entity.KnowledgeContentLexicalSearchRequest) (*entity.KnowledgeContentLexicalSearchData, error) {
					return &entity.KnowledgeContentLexicalSearchData{}, nil
				}
			case "unknown-id":
				m.generate = func(context.Context, string, []entity.KnowledgeAnswerCitation) (string, *entity.KnowledgeAnswerUsage, error) {
					calls.Add(1)
					return `{"answer":"47","insufficient_evidence":false,"evidence_ids":["E5"]}`, nil, nil
				}
			case "duplicate-id":
				m.generate = func(context.Context, string, []entity.KnowledgeAnswerCitation) (string, *entity.KnowledgeAnswerUsage, error) {
					calls.Add(1)
					return `{"answer":"47","insufficient_evidence":false,"evidence_ids":["E1","E1"]}`, nil, nil
				}
			case "byte-changed":
				read := r.read
				r.read = func(c context.Context, o, d string, q entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error) {
					v, e := read(c, o, d, q)
					if calls.Load() > 0 {
						v.Text = "PRIVATE-MUTATED-EVIDENCE"
					}
					return v, e
				}
			case "timeout":
				m.generate = func(context.Context, string, []entity.KnowledgeAnswerCitation) (string, *entity.KnowledgeAnswerUsage, error) {
					calls.Add(1)
					return "", nil, context.DeadlineExceeded
				}
			}
			status, result, err := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
			if test == "no-hit" {
				if err != nil || status != 200 || calls.Load() != 0 || !result.(entity.KnowledgeAnswerResult).Insufficient {
					t.Fatal(status, err, calls.Load())
				}
				return
			}
			if err == nil || result != nil || calls.Load() != 1 {
				t.Fatal("partial/error result escaped", err)
			}
			_, _, again := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
			if again == nil || calls.Load() != 1 {
				t.Fatal("failed request resent model", again)
			}
			_, body := AnswerErrorResponse("answer", strings.Repeat("f", 32), err)
			if entity.KnowledgeAnswerDecode(body, "Error", nil) != nil || strings.Contains(string(body), "PRIVATE-MUTATED-EVIDENCE") {
				t.Fatal("error wire/body leak")
			}
		})
	}
}

func TestKnowledgeAnswerInflightDisableSameKeyAndRetention(t *testing.T) {
	r, ctx, calls, scope := answerTestRunner(t)
	enableAnswerTest(t, r, ctx, scope)
	started, release := make(chan struct{}), make(chan struct{})
	m, _ := r.resolve(ctx, knowledgeTestOwner)
	m.generate = func(context.Context, string, []entity.KnowledgeAnswerCitation) (string, *entity.KnowledgeAnswerUsage, error) {
		calls.Add(1)
		close(started)
		<-release
		return `{"answer":"47","insufficient_evidence":false,"evidence_ids":["E1"]}`, nil, nil
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch did not start")
	}
	status, _, err := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
	if err != nil || status != 202 || calls.Load() != 1 {
		t.Fatal("concurrent request resent", status, err)
	}
	var epochBefore entity.KnowledgeAccessTenantStateRecord
	dao.DB.Where("tenant_id = ?", knowledgeTestOwner).Take(&epochBefore)
	_, err = r.setPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeAnswerPolicySetRequest{Version: entity.KnowledgeAnswerVersion, ExpectedVersion: 1, Confirmed: true, Key: "disable"})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil || normalizeAnswerError(err).Code != "model_use_denied" {
		t.Fatal("revoked in-flight answer returned", err)
	}
	var row entity.KnowledgeAnswerRequestRecord
	dao.DB.Where("operation = ? AND key_sha256 = ?", "answer", lexicalSHA([]byte("one-request"))).Take(&row)
	if row.ResponseJSON != "" || row.State != "failed" || calls.Load() != 1 {
		t.Fatal("revoked answer cached")
	}
	var epochAfter entity.KnowledgeAccessTenantStateRecord
	dao.DB.Where("tenant_id = ?", knowledgeTestOwner).Take(&epochAfter)
	if epochAfter.Epoch != epochBefore.Epoch {
		t.Fatal("generation toggle revoked read-sharing epoch")
	}
	row.ResponseJSON = "synthetic-expired-answer"
	row.CreatedAt = time.Now().UTC().Add(-25 * time.Hour)
	if err := dao.DB.Save(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := answerPrune(ctx); err != nil {
		t.Fatal(err)
	}
	dao.DB.Where("id = ?", row.ID).Take(&row)
	if row.ResponseJSON != "" {
		t.Fatal("expired answer body retained")
	}
	row.CreatedAt = time.Now().UTC().Add(-8 * 24 * time.Hour)
	dao.DB.Save(&row)
	if err := answerPrune(ctx); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(dao.DB.Where("id = ?", row.ID).Take(&entity.KnowledgeAnswerRequestRecord{}).Error, gorm.ErrRecordNotFound) {
		t.Fatal("expired ledger retained")
	}
}

func TestKnowledgeAnswerRequestDoesNotPersistQuestionOrProviderSecrets(t *testing.T) {
	r, ctx, _, scope := answerTestRunner(t)
	enableAnswerTest(t, r, ctx, scope)
	if _, _, err := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest()); err != nil {
		t.Fatal(err)
	}
	var row entity.KnowledgeAnswerRequestRecord
	dao.DB.Where("operation = ?", "answer").Take(&row)
	raw, _ := json.Marshal(row)
	if strings.Contains(string(raw), "ORION project quota?") || strings.Contains(string(raw), "api_key") {
		t.Fatal("private question/config persisted")
	}
	probe := dao.DB.Session(&gorm.Session{DryRun: true}).Where("id = ?", row.ID).Take(&entity.KnowledgeAnswerRequestRecord{})
	if !strings.Contains(probe.Statement.SQL.String(), "id = ?") || len(probe.Statement.Vars) != 1 {
		t.Fatal("unbound UUID predicate")
	}
}

func TestKnowledgeAnswerMissingSourceKeepsDomainDenialsSeparate(t *testing.T) {
	r, ctx, calls, scope := answerTestRunner(t)
	enableAnswerTest(t, r, ctx, scope)
	r.read = func(context.Context, string, string, entity.KnowledgeContentLexicalReadRequest) (*entity.KnowledgeContentLexicalReadData, error) {
		return nil, lexicalError(404, "not_found")
	}
	_, result, err := r.answer(ctx, knowledgeTestOwner, knowledgeTestDomain, answerTestRequest())
	if result != nil || normalizeAnswerError(err).Code != "scope_changed" || normalizeAnswerError(err).Status != 412 || calls.Load() != 0 {
		t.Fatal("missing fixed source was not a closed scope conflict", err)
	}
	if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestDomain).Update("knowledge_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	err = r.scopeError(ctx, knowledgeTestOwner, knowledgeTestDomain, lexicalError(404, "not_found"))
	if normalizeAnswerError(err).Status != 404 {
		t.Fatal("lost domain authority was exposed as source state", err)
	}
}
