//go:build linux

package dataset

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

func contentLexicalMixedFixture(t *testing.T) (*DatasetService, context.Context, entity.KnowledgeContentLexicalSelection, *lexicalTestEngine) {
	t.Helper()
	s, _, r2 := contentLexicalR2(t, "合成 R2 派生文字\r\n")
	ctx := contentLexicalSession(t, s, true, true)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureNodeRecord{}, &entity.KnowledgeStructureRelationRecord{}); err != nil {
		t.Fatal(err)
	}
	store := storage.GetStorageFactory().GetStorage()
	longBody := []byte(strings.Repeat("合成 C1 原文区间\r\n", 800))
	uploadKnowledgeTest(t, store, strings.Repeat("2", 32), "long-source.txt", longBody)
	c1, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: strings.Repeat("2", 32), RelativePath: "long-source.txt", ComponentSide: "non_code"})
	if err != nil {
		t.Fatal(err)
	}
	md := []byte("# 合成 R1 标题\n\n合成固定 Markdown 段落。\n")
	uploadKnowledgeTest(t, store, strings.Repeat("3", 32), "guide.md", md)
	r1Source, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: strings.Repeat("3", 32), RelativePath: "guide.md", ComponentSide: "non_code"})
	if err != nil {
		t.Fatal(err)
	}
	r1 := lexicalPublishedR1Fixture(t, s, ctx, r1Source, "guide.md", md)
	if _, _, err := s.ChangeKnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 2, ParseEnabled: true, LexicalEnabled: true, Confirmed: true, IdempotencyKey: "allow-mixed-fixture-parse"}); err != nil {
		t.Fatal(err)
	}
	csvID, csvBytes := strings.Repeat("5", 32), []byte("A,B\n合成,表格\n1,2\n")
	uploadKnowledgeTest(t, store, csvID, "mixed.csv", csvBytes)
	parsed, err := s.SnapshotKnowledgeParsedSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeParsedSnapshotRequest{DocumentID: csvID, ExpectedPolicyVersion: 1, RelativePath: "mixed.csv", Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}})
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.GetKnowledgeParsedSource(ctx, knowledgeTestOwner, knowledgeTestDomain, csvID)
	if err != nil || state.Profile == nil {
		t.Fatal(state, err)
	}
	task, _, err := s.CreateKnowledgeParsedTask(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeParsedTaskRequest{Source: contentSource(parsed), ExpectedLifecycleVersion: state.LifecycleVersion, ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 3, ExpectedProducerSHA256: state.Profile.ProducerSHA256, MaxUnitBytes: 128, Confirmed: true, IdempotencyKey: "mixed-fixture-owned-child"})
	if err != nil {
		t.Fatal(err)
	}
	var taskRow entity.KnowledgeStructureTaskRecord
	if err := dao.DB.Where("id = ?", task.TaskID).Take(&taskRow).Error; err != nil {
		t.Fatal(err)
	}
	claim, err := claimKnowledgeStructure(ctx, taskRow, common.GenerateUUID())
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}
	s.runKnowledgeParsed(ctx, claim)
	done, err := s.GetKnowledgeParsedTask(ctx, knowledgeTestOwner, knowledgeTestDomain, task.TaskID)
	if err != nil || done.State != "succeeded" || done.Result == nil {
		t.Fatal(done, err)
	}
	selection := entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{
		contentTarget(contentSource(&c1.Source), "source_revision", c1.Source.ID, nil),
		contentTarget(contentSource(&r1Source.Source), r1.Kind, r1.ID, r1.ProducerSHA256),
		r2,
		*done.Result,
	}}
	fake := &lexicalTestEngine{rows: map[string]map[string]any{}}
	s.docEngine = fake
	return s, ctx, selection, fake
}

func TestKnowledgeContentLexicalMixedFourKindsSharedQueueSearchRead(t *testing.T) {
	contentLexicalTokenizer(t)
	s, ctx, selection, fake := contentLexicalMixedFixture(t)
	inspect, err := s.InspectKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalInspectRequest{Version: entity.KnowledgeContentVersion, Selection: selection})
	if err != nil || inspect.Readiness.Kind != "unindexed" || len(inspect.Readiness.Coverage) != 4 || !inspect.Policy.Enabled {
		t.Fatal(inspect, err)
	}
	create := entity.KnowledgeContentLexicalCreateRequest{Version: entity.KnowledgeContentVersion, Key: "one-mixed-job", ExpectedLocalVersion: inspect.Policy.LocalVersion, ExpectedContentVersion: inspect.Policy.ContentVersion, ExpectedFoundationVersion: inspect.Policy.FoundationVersion, ExpectedProfileSHA256: inspect.Profile.SHA256, Selection: selection}
	job, created, err := s.CreateKnowledgeContentLexicalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, create)
	if err != nil || !created || job.State != "queued" {
		t.Fatal(job, created, err)
	}
	replay, created, err := s.CreateKnowledgeContentLexicalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, create)
	if err != nil || created || replay.ID != job.ID {
		t.Fatal("same key did not recover the one mixed job", replay, created, err)
	}
	runtime, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	s.startKnowledgeLocalLexicalRuntime(runtime)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err = s.GetKnowledgeContentLexicalJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == "published" || job.State == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if job.State != "published" || job.Index == nil {
		t.Fatal("same queue did not publish", job.State, job.Readiness.Diagnostics)
	}
	search := entity.KnowledgeContentLexicalSearchRequest{Version: entity.KnowledgeContentVersion, Index: *job.Index, Selection: selection, Query: "合成", Limit: 20}
	result, err := s.SearchKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, search)
	if err != nil || result.ReturnedCount < 4 || fake.searches != 1 {
		t.Fatal(result, fake.searches, err)
	}
	kinds := map[string]bool{}
	reads := []entity.KnowledgeContentLexicalReadData{}
	var c1Units int
	for _, hit := range result.Hits {
		kind := hit.Evidence.Address.Target.Representation.Kind
		kinds[kind] = true
		read, err := s.ReadKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: hit.Evidence})
		if err != nil || read.Text != hit.Text || !read.Complete || lexicalSHA([]byte(read.Text)) != *hit.Evidence.Range.SHA256 {
			t.Fatal(read, err)
		}
		if read.Context.Node == nil || lexicalJSON(read.Context.Node.Ref) != lexicalJSON(hit.Evidence.Address.Node) || read.Context.MetadataBytes > 65536 || read.Context.MetadataBytes != len(lexicalJSON(read.Context)) || len(read.Context.TypedMembers) > 32 || read.Context.ParentExpansion != "disabled" {
			t.Fatal("bounded owning node context lost", read.Context)
		}
		for _, member := range read.Context.TypedMembers {
			if member.Node.Text.Range == nil || member.Node.Text.Range.StartByte < hit.Evidence.Range.StartByte || member.Node.Text.Range.EndByte > hit.Evidence.Range.EndByte || !entity.ValidKnowledgeLexicalHash(member.PayloadSHA256) {
				t.Fatal("unproved or expanded typed member", member)
			}
		}
		reads = append(reads, *read)
		if kind == "source_revision" {
			c1Units++
			if hit.Evidence.Assembly != "c1_root_subrange_exact_bytes" || hit.Evidence.Range.EndByte-hit.Evidence.Range.StartByte > 8192 || hit.Evidence.Locator.SourceUTF8.EndByte <= 8192 {
				t.Fatal("C1 fragment was promoted to the whole root", hit.Evidence)
			}
		} else if hit.Evidence.Assembly != "existing_passage_full_range" {
			t.Fatal("owning passage scope lost", hit.Evidence)
		}
	}
	if len(kinds) != 4 || c1Units < 2 {
		t.Fatal("new consumer had to split the four kinds or C1 remained limited to one root", kinds, c1Units)
	}
	if _, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID); err == nil {
		t.Fatal("old decoder accepted the new job protocol")
	}
	tampered := result.Hits[0].Evidence
	tampered.Range.EndByte--
	_, err = s.ReadKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: tampered})
	lexicalAssertCode(t, err, "integrity_error")
	if _, _, err := s.ChangeKnowledgeContentPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentPolicyRequest{ExpectedPolicyVersion: 1, ExpectedContentPolicyVersion: 3, ParseEnabled: true, Confirmed: true, IdempotencyKey: "disable-search-retain-fixed-read"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.SearchKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, search)
	lexicalAssertCode(t, err, "policy_disabled")
	if _, err := s.ReadKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: result.Hits[0].Evidence}); err != nil {
		t.Fatal("index opt-in replaced current fixed content read authority", err)
	}
	proof, err := json.MarshalIndent(map[string]any{"status": "Passed_OneMixedQueuePublicationQuery", "inspect": inspect, "job": job, "search": result, "reads": reads, "kinds": kinds, "c1_subrange_units": c1Units, "native_tokenizer": true, "es": "substitute", "http": false, "quality": "NotStarted"}, "", "  ")
	if err != nil || os.WriteFile("lexical-unified-public-proof.json", append(proof, '\n'), 0600) != nil {
		t.Fatal("public service proof export failed", err)
	}
	t.Logf("same_queue=true same_publication=true single_query=true kinds=%d c1_subrange_units=%d native_tokenizer=true es=substitute", len(kinds), c1Units)
}
