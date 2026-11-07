package dataset

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

// The unavailable engine fails before tokenization so retry and claim tests do
// not need native resource files or an external search service.
type lexicalRetryEngine struct{ *lexicalTestEngine }

func (*lexicalRetryEngine) GetType() string { return "synthetic-unavailable" }

func newLexicalRetryJob(t *testing.T, state string, attempts int64, lease time.Time) (*DatasetService, context.Context, entity.KnowledgeLexicalJobRecord) {
	t.Helper()
	s, ctx := lexicalTestContext(t)
	if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "retry-enable", ExpectedFoundationVersion: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	kb, policy, err := s.lexicalContext(ctx, knowledgeTestOwner, knowledgeTestDomain, true)
	if err != nil {
		t.Fatal(err)
	}
	s.docEngine = &lexicalRetryEngine{&lexicalTestEngine{rows: map[string]map[string]any{}}}
	job := entity.KnowledgeLexicalJobRecord{Protocol: entity.KnowledgeLocalLexicalProtocol, ID: strings.Repeat("c", 32), OwnerID: knowledgeTestOwner, DomainID: knowledgeTestDomain, Key: "retry-job", RequestSHA256: strings.Repeat("d", 64), SelectionSHA256: strings.Repeat("e", 64), Selection: "{}", PolicyVersion: policy.Version, FoundationVersion: policy.FoundationVersion, Language: lexicalLanguage(kb), SessionSHA256: ctx.Value(lexicalSessionKey{}).(lexicalSession).SHA256, State: state, AttemptCount: attempts, LeaseUntil: lease, CreatedAt: time.Now().UTC()}
	if err := dao.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	return s, ctx, job
}

func loadLexicalRetryJob(t *testing.T, id string) entity.KnowledgeLexicalJobRecord {
	t.Helper()
	var job entity.KnowledgeLexicalJobRecord
	if err := dao.DB.Where("id = ?", id).Take(&job).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func TestKnowledgeLocalLexicalStaleClaimRetryLimit(t *testing.T) {
	s, ctx, listed := newLexicalRetryJob(t, "queued", 1, time.Unix(1, 0).UTC())
	// A and B listed the same first retry. Keep A's value while B owns the
	// second attempt and the real worker handles a transient engine failure.
	workerA := listed
	workerB, err := claimKnowledgeLocalLexical(ctx, listed)
	if err != nil || workerB == nil || workerB.AttemptCount != 2 {
		t.Fatalf("B claim = %+v, error = %v", workerB, err)
	}
	s.runKnowledgeLocalLexical(ctx, *workerB)
	requeued := loadLexicalRetryJob(t, listed.ID)
	if requeued.State != "queued" || requeued.AttemptCount != 2 || requeued.Diagnostic != "index_failed" || requeued.Claim != "" {
		t.Fatalf("second transient failure did not requeue: %+v", requeued)
	}

	stale, err := claimKnowledgeLocalLexical(ctx, workerA)
	if err != nil {
		t.Fatal(err)
	}
	if stale != nil {
		// On the old WHERE this executes SQL attempt three with local count two.
		// Exercise the retry decision too, rather than only testing the claim.
		s.runKnowledgeLocalLexical(ctx, *stale)
		actual := loadLexicalRetryJob(t, listed.ID)
		t.Fatalf("stale A claimed SQL attempt %d with local count %d; third failure left state=%s diagnostic=%s", actual.AttemptCount, stale.AttemptCount, actual.State, actual.Diagnostic)
	}
	unchanged := loadLexicalRetryJob(t, listed.ID)
	if unchanged.AttemptCount != 2 || unchanged.State != "queued" || unchanged.Claim != "" {
		t.Fatalf("losing stale claim changed the row: %+v", unchanged)
	}

	third, err := claimKnowledgeLocalLexical(ctx, unchanged)
	if err != nil || third == nil || third.AttemptCount != 3 {
		t.Fatalf("fresh third claim = %+v, error = %v", third, err)
	}
	s.runKnowledgeLocalLexical(ctx, *third)
	failed := loadLexicalRetryJob(t, listed.ID)
	if failed.State != "failed" || failed.AttemptCount != 3 || failed.Diagnostic != "index_failed" || failed.Claim != "" {
		t.Fatalf("third transient failure did not terminate: %+v", failed)
	}
	if again, err := claimKnowledgeLocalLexical(ctx, failed); err != nil || again != nil {
		t.Fatalf("terminal job was claimed again: %+v, error = %v", again, err)
	}
}

func TestKnowledgeLocalLexicalClaimStatesAndRetry(t *testing.T) {
	check := func(t *testing.T, state string, attempts int64, lease time.Time, wantClaim bool) {
		t.Helper()
		s, ctx, listed := newLexicalRetryJob(t, state, attempts, lease)
		claimed, err := claimKnowledgeLocalLexical(ctx, listed)
		if err != nil || (claimed != nil) != wantClaim {
			t.Fatalf("claim = %+v, error = %v, want claimed=%t", claimed, err, wantClaim)
		}
		actual := loadLexicalRetryJob(t, listed.ID)
		if !wantClaim {
			if actual.State != listed.State || actual.AttemptCount != listed.AttemptCount || actual.Claim != listed.Claim || !actual.LeaseUntil.Equal(listed.LeaseUntil) {
				t.Fatalf("ineligible claim changed the row: %+v", actual)
			}
			return
		}
		if claimed.State != "building" || claimed.AttemptCount != attempts+1 || actual.AttemptCount != claimed.AttemptCount || actual.Claim == "" || actual.Claim != claimed.Claim || !actual.LeaseUntil.After(time.Now()) {
			t.Fatalf("claim and SQL differ: claimed=%+v actual=%+v", claimed, actual)
		}
		if duplicate, err := claimKnowledgeLocalLexical(ctx, listed); err != nil || duplicate != nil {
			t.Fatalf("active claim was taken twice: %+v, error = %v", duplicate, err)
		}
		s.runKnowledgeLocalLexical(ctx, *claimed)
		retry := loadLexicalRetryJob(t, listed.ID)
		if retry.State != "queued" || retry.AttemptCount != attempts+1 || retry.Diagnostic != "index_failed" || retry.Claim != "" {
			t.Fatalf("ordinary transient retry changed: %+v", retry)
		}
	}
	t.Run("queued", func(t *testing.T) {
		check(t, "queued", 0, time.Unix(1, 0).UTC(), true)
	})
	t.Run("expired_building", func(t *testing.T) {
		check(t, "building", 1, time.Now().Add(-time.Hour), true)
	})
	t.Run("live_building", func(t *testing.T) {
		check(t, "building", 1, time.Now().Add(time.Hour), false)
	})
	t.Run("published", func(t *testing.T) {
		check(t, "published", 1, time.Unix(1, 0).UTC(), false)
	})
	t.Run("failed", func(t *testing.T) {
		check(t, "failed", 3, time.Unix(1, 0).UTC(), false)
	})
	t.Run("cancelled_context", func(t *testing.T) {
		_, ctx, listed := newLexicalRetryJob(t, "queued", 0, time.Unix(1, 0).UTC())
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		claimed, err := claimKnowledgeLocalLexical(cancelled, listed)
		if claimed != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled claim = %+v, error = %v", claimed, err)
		}
		actual := loadLexicalRetryJob(t, listed.ID)
		if actual.State != "queued" || actual.AttemptCount != 0 || actual.Claim != "" {
			t.Fatalf("cancelled claim changed the row: %+v", actual)
		}
	})
}

func TestKnowledgeLocalLexicalClaimSQLBindings(t *testing.T) {
	_, ctx, listed := newLexicalRetryJob(t, "queued", 2, time.Unix(1, 0).UTC())
	base := dao.DB
	const callback = "test:knowledge_lexical_claim_sql"
	var statement string
	var values []any
	if err := base.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		statement = tx.Statement.SQL.String()
		values = append([]any(nil), tx.Statement.Vars...)
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		dao.DB = base
		_ = base.Callback().Update().Remove(callback)
	}()
	// Invoke the same helper with a DryRun session. No second copy of the
	// production WHERE is used to generate the probed SQL.
	dao.DB = base.Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true})
	claimed, err := claimKnowledgeLocalLexical(ctx, listed)
	dao.DB = base
	if err != nil || claimed != nil {
		t.Fatalf("DryRun claim = %+v, error = %v", claimed, err)
	}
	t.Logf("DryRun claim SQL: %s; bound variables: %#v", statement, values)
	if !strings.Contains(statement, "attempt_count + 1") || !strings.Contains(statement, "id = ? AND attempt_count = ? AND (state = ? OR (state = ? AND lease_until < ?))") || strings.Contains(statement, listed.ID) || len(values) < 5 {
		t.Fatalf("claim SQL lost its bound CAS: %s; %#v", statement, values)
	}
	where := values[len(values)-5:]
	if where[0] != listed.ID || where[1] != listed.AttemptCount || where[2] != "queued" || where[3] != "building" {
		t.Fatalf("claim WHERE variables differ: %#v", where)
	}
	if deadline, ok := where[4].(time.Time); !ok || deadline.IsZero() {
		t.Fatalf("lease deadline is not bound: %#v", where[4])
	}
	actual := loadLexicalRetryJob(t, listed.ID)
	if actual.State != "queued" || actual.AttemptCount != 2 || actual.Claim != "" {
		t.Fatalf("DryRun mutated the job: %+v", actual)
	}
}

func TestKnowledgeLocalLexicalClaimPublishes(t *testing.T) {
	const resources = "/usr/share/infinity/resource"
	if _, err := os.Stat(resources + "/rag/huqie.trie"); err != nil {
		t.Skip("native lexical resources not prepared")
	}
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: resources, MinSize: 1, MaxSize: 2}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tokenizer.Close)
	s, ctx := lexicalTestContext(t)
	s.docEngine = &lexicalTestEngine{rows: map[string]map[string]any{}}
	if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "claim-publish-enable", ExpectedFoundationVersion: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	docID := strings.Repeat("2", 32)
	uploadKnowledgeTest(t, storage.GetStorageFactory().GetStorage(), docID, "retry-publication.txt", []byte("Synthetic claimed publication keeps its immutable source.\n"))
	snapshot, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID, RelativePath: "retry-publication.txt", ComponentSide: "non_code"})
	if err != nil {
		t.Fatal(err)
	}
	selection := entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{{Source: entity.KnowledgeLexicalSourceRef{DomainID: knowledgeTestDomain, SourceID: docID, RevisionID: snapshot.Source.ID, SHA256: snapshot.Source.SHA256}, Representation: entity.KnowledgeLexicalRepresentationRef{Kind: "source_revision", ID: snapshot.Source.ID}}}}
	job, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "claim-publish", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	listed := loadLexicalRetryJob(t, job.ID)
	claimed, err := claimKnowledgeLocalLexical(ctx, listed)
	if err != nil || claimed == nil || claimed.AttemptCount != 1 {
		t.Fatalf("publication claim = %+v, error = %v", claimed, err)
	}
	s.runKnowledgeLocalLexical(ctx, *claimed)
	finished, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID)
	if err != nil || finished.State != "published" || finished.Index == nil {
		t.Fatalf("claimed job did not publish: %+v, error = %v", finished, err)
	}
	actual := loadLexicalRetryJob(t, job.ID)
	if actual.AttemptCount != 1 || actual.Claim != "" || actual.IndexID != finished.Index.ID {
		t.Fatalf("published claim identity changed: %+v", actual)
	}
	var publications int64
	if err := dao.DB.Model(&entity.KnowledgeLexicalPublicationRecord{}).Where("owner_id = ? AND domain_id = ?", knowledgeTestOwner, knowledgeTestDomain).Count(&publications).Error; err != nil || publications != 1 {
		t.Fatalf("publication count = %d, error = %v", publications, err)
	}
	if again, err := claimKnowledgeLocalLexical(ctx, actual); err != nil || again != nil {
		t.Fatalf("published job was reclaimed: %+v, error = %v", again, err)
	}
}
