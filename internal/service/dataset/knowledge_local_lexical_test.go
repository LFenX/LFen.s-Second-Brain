package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	es "ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	structure "ragflow/internal/parser/structure"
	"ragflow/internal/service/nlp"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
	"strings"
	"testing"
	"time"
)

type lexicalTestEngine struct {
	engine.DocEngine
	rows           map[string]map[string]any
	partialFailure bool
	searches       int
}

func (e *lexicalTestEngine) GetType() string { return "elasticsearch" }
func (e *lexicalTestEngine) PrepareKnowledgeLexical(context.Context, string, string, string) error {
	return nil
}
func (e *lexicalTestEngine) CreateKnowledgeLexicalRows(ctx context.Context, index, domain string, rows []map[string]any) error {
	for _, row := range rows {
		id := row["id"].(string)
		if old, ok := e.rows[id]; ok {
			for _, key := range []string{"knowledge_ref", "content_with_weight", "knowledge_binding_kwd"} {
				if old[key] != row[key] {
					return errors.New("conflict")
				}
			}
		}
		copy := map[string]any{}
		for k, v := range row {
			copy[k] = v
		}
		e.rows[id] = copy
		if e.partialFailure {
			return errors.New("synthetic partial write")
		}
	}
	return nil
}
func (e *lexicalTestEngine) VerifyKnowledgeLexicalRows(ctx context.Context, index, domain string, rows []map[string]any) error {
	if len(e.rows) != len(rows) {
		return errors.New("incomplete row set")
	}
	for _, row := range rows {
		got := e.rows[row["id"].(string)]
		for _, key := range []string{"id", "kb_id", "doc_id", "available_int", "knowledge_ref", "knowledge_binding_kwd", "content_with_weight"} {
			if got[key] != row[key] {
				return errors.New("row mismatch")
			}
		}
	}
	return nil
}
func (e *lexicalTestEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searches++
	if len(req.MatchExprs) != 1 || req.Filter["knowledge_available_exact"] != 0 {
		return nil, errors.New("unsafe native search request")
	}
	if _, ok := req.MatchExprs[0].(*types.MatchTextExpr); !ok {
		return nil, errors.New("not lexical")
	}
	rows := []map[string]any{}
	for _, row := range e.rows {
		copy := map[string]any{}
		for k, v := range row {
			copy[k] = v
		}
		copy["_score"] = 3.5
		rows = append(rows, copy)
		if len(rows) == req.Limit {
			break
		}
	}
	return &types.SearchResult{Chunks: rows, Total: int64(len(rows))}, nil
}
func (e *lexicalTestEngine) GetFields(rows []map[string]any, fields []string) map[string]map[string]any {
	return (&es.Engine{}).GetFields(rows, fields)
}
func (e *lexicalTestEngine) GetChunkIDs(rows []map[string]any) []string {
	return (&es.Engine{}).GetChunkIDs(rows)
}
func (e *lexicalTestEngine) GetAggregation([]map[string]any, string) []map[string]any { return nil }

func lexicalTestContext(t *testing.T) (*DatasetService, context.Context) {
	t.Helper()
	s, _ := setupKnowledgeTest(t)
	token := "synthetic-native-session-access-token"
	valid := "1"
	if err := dao.DB.Save(&entity.User{ID: knowledgeTestOwner, Nickname: "Synthetic", Email: "synthetic@example.invalid", AccessToken: &token, Status: &valid, IsActive: "1", IsAuthenticated: "1", IsAnonymous: "0"}).Error; err != nil {
		t.Fatal(err)
	}
	registerKnowledgeTest(t, s, knowledgeTestDomain)
	if err := dao.DB.AutoMigrate(&entity.KnowledgeLexicalPolicyRecord{}, &entity.KnowledgeLexicalOperationRecord{}, &entity.KnowledgeLexicalJobRecord{}, &entity.KnowledgeLexicalPublicationRecord{}, &entity.KnowledgeStructureViewRecord{}, &entity.KnowledgeStructureNodeRecord{}); err != nil {
		t.Fatal(err)
	}
	return s, context.WithValue(context.Background(), lexicalSessionKey{}, lexicalSession{UserID: knowledgeTestOwner, SHA256: lexicalSHA([]byte(token))})
}
func lexicalAssertCode(t *testing.T, err error, code string) {
	t.Helper()
	var got *KnowledgeLexicalError
	if !errors.As(err, &got) || got.Code != code {
		t.Fatalf("error=%v want=%s", err, code)
	}
}
func TestKnowledgeLocalLexicalPolicySessionCASAndSQL(t *testing.T) {
	s, ctx := lexicalTestContext(t)
	p, err := s.GetKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain)
	if err != nil || p.Enabled || p.Version != 0 {
		t.Fatalf("default policy: %+v %v", p, err)
	}
	req := entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "enable", ExpectedFoundationVersion: 1, Enabled: true}
	receipt, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || receipt.Policy.Version != 1 {
		t.Fatalf("change: %+v %v", receipt, err)
	}
	replay, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	if err != nil || lexicalJSON(receipt) != lexicalJSON(replay) {
		t.Fatalf("replay: %v", err)
	}
	req.Enabled = false
	_, err = s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, req)
	lexicalAssertCode(t, err, "idempotency_conflict")
	_, err = s.GetKnowledgeLocalPolicy(context.Background(), knowledgeTestOwner, knowledgeTestDomain)
	lexicalAssertCode(t, err, "unauthenticated")
	stmt := dao.DB.Session(&gorm.Session{DryRun: true}).Where("id = ? AND domain_id = ?", strings.Repeat("f", 64), knowledgeTestDomain).Take(&entity.KnowledgeLexicalPublicationRecord{})
	if !strings.Contains(stmt.Statement.SQL.String(), "id = ?") || len(stmt.Statement.Vars) < 2 || stmt.Statement.Vars[0] != strings.Repeat("f", 64) {
		t.Fatalf("unbound SQL: %s %#v", stmt.Statement.SQL.String(), stmt.Statement.Vars)
	}
	if err := dao.DB.Model(&entity.User{}).Where("id = ?", knowledgeTestOwner).Update("is_active", "0").Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.GetKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain)
	lexicalAssertCode(t, err, "unauthenticated")
}

// This test uses the locked synthetic bytes, real native Tokenizer and SQLite.
// The ES transport below is a substitute; it is not real ES integration proof.
func TestKnowledgeLocalLexicalSyntheticNativeChain(t *testing.T) {
	if _, err := os.Stat("/usr/share/infinity/resource/rag/huqie.trie"); err != nil {
		t.Skip("native lexical resources not prepared")
	}
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: "/usr/share/infinity/resource", MinSize: 1, MaxSize: 2}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tokenizer.Close)
	if err := nlp.InitQueryBuilderFromTokenizer("/usr/share/infinity/resource"); err != nil {
		t.Fatal(err)
	}
	oldResolver := component.DefaultEmbedderResolver
	attempts := 0
	component.DefaultEmbedderResolver = func(context.Context, string, string) (component.Embedder, string, error) {
		attempts++
		return nil, "", errors.New("model attempt forbidden")
	}
	t.Cleanup(func() { component.DefaultEmbedderResolver = oldResolver })
	s, ctx := lexicalTestContext(t)
	store := storage.GetStorageFactory().GetStorage()
	fake := &lexicalTestEngine{rows: map[string]map[string]any{}, partialFailure: true}
	s.docEngine = fake
	if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "enable", ExpectedFoundationVersion: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	selection := lexicalFixtureSelection(t, s, ctx, store)
	job, err := s.CreateKnowledgeLocalIndex(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalCreateRequest{Version: entity.KnowledgeLocalLexicalVersion, Key: "index", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	var record entity.KnowledgeLexicalJobRecord
	if err := dao.DB.Where("id = ?", job.ID).Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	record.Claim = "synthetic-claim"
	record.State = "building"
	record.LeaseUntil = time.Now().Add(time.Minute)
	if err := dao.DB.Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.buildKnowledgeLocalLexical(ctx, record); err == nil {
		t.Fatal("partial write published")
	}
	var count int64
	dao.DB.Model(&entity.KnowledgeLexicalPublicationRecord{}).Count(&count)
	if count != 0 {
		t.Fatal("partial publication escaped")
	}
	if err := dao.DB.Where("id = ?", job.ID).Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	fake.partialFailure = false
	if err := s.buildKnowledgeLocalLexical(ctx, record); err != nil {
		t.Fatal(err)
	}
	finished, err := s.GetKnowledgeLocalIndexJob(ctx, knowledgeTestOwner, knowledgeTestDomain, job.ID)
	if err != nil || finished.State != "published" || finished.Index == nil {
		t.Fatalf("job=%+v %v", finished, err)
	}
	search := entity.KnowledgeLexicalSearchRequest{Version: entity.KnowledgeLocalLexicalVersion, Index: *finished.Index, Selection: selection, Query: "合成", Limit: 8}
	result, err := s.SearchKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, search)
	if err != nil || len(result.Hits) != 3 {
		t.Fatalf("search=%+v %v", result, err)
	}
	for _, hit := range result.Hits {
		read, err := s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: hit.Evidence})
		if err != nil || read.Text != hit.Text || lexicalSHA([]byte(read.Text)) != hit.Evidence.FragmentSHA256 {
			t.Fatalf("round trip: %+v %v", read, err)
		}
	}
	tampered := result.Hits[0].Evidence
	tampered.Range.End--
	_, err = s.ReadKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalReadRequest{Version: entity.KnowledgeLocalLexicalVersion, Evidence: tampered})
	lexicalAssertCode(t, err, "integrity_error")
	row := fake.rows[result.Hits[0].Evidence.RowID]
	ref := row["knowledge_ref"]
	row["knowledge_ref"] = "same binding, wrong node reference"
	_, err = s.SearchKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, search)
	lexicalAssertCode(t, err, "integrity_error")
	row["knowledge_ref"] = ref
	if _, err := s.ChangeKnowledgeLocalPolicy(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeLexicalPolicyChange{Version: entity.KnowledgeLocalLexicalVersion, Key: "disable", ExpectedVersion: 1, ExpectedFoundationVersion: 1, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	_, err = s.SearchKnowledgeLocal(ctx, knowledgeTestOwner, knowledgeTestDomain, search)
	lexicalAssertCode(t, err, "policy_disabled")
	if attempts != 0 {
		t.Fatalf("model attempts=%d", attempts)
	}
	t.Logf("synthetic_formats=3 real_tokenizer=true sql=SQLite es=substitute published_rows=%d model_attempts=%d", len(fake.rows), attempts)
}

func TestKnowledgeLocalLexicalApprovedIdentityVectors(t *testing.T) {
	var vectors []struct {
		Binding string                 `json:"binding_id"`
		Row     string                 `json:"index_row_id"`
		Plan    string                 `json:"chunk_plan_sha256"`
		Ref     lexicalStoredReference `json:"knowledge_ref"`
	}
	if err := json.Unmarshal([]byte(lexicalVectorsJSON), &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		target := entity.KnowledgeLexicalTarget{Source: v.Ref.Source, Representation: v.Ref.Representation}
		if lexicalBinding(target, v.Plan) != v.Binding {
			t.Fatal("binding vector mismatch")
		}
		member := v.Ref.Members[0]
		if lexicalRowID(v.Binding, member.Node, entity.KnowledgeLexicalRange{Start: member.Range.Start, End: member.Range.End}, member.Range.SHA256) != v.Row {
			t.Fatal("row vector mismatch")
		}
	}
	if lexicalAssembly() != "d75aee517ad68546e6cb354b99018dd340de2a6e6c7aec10c1fe48941179934c" {
		t.Fatal("assembly mismatch")
	}
}

func lexicalPublishedR1Fixture(t *testing.T, s *DatasetService, ctx context.Context, snapshot *entity.KnowledgeSnapshot, name string, body []byte) entity.KnowledgeLexicalRepresentationRef {
	t.Helper()
	parser := "markdown"
	if strings.HasSuffix(name, ".tsx") {
		parser = "tsx"
	}
	producer, err := entity.KnowledgeStructureProducerDigest(nil)
	if err != nil {
		t.Fatal(err)
	}
	member := entity.KnowledgeStructureViewMember{SourceID: snapshot.Source.DocumentID, SourceRevisionID: snapshot.Source.ID, SourceNodeID: snapshot.Root.ID, SHA256: snapshot.Source.SHA256, ByteLength: int64(len(body)), RelativePath: name, Parser: parser, LifecycleVersion: 1}
	member.Classification = entity.KnowledgeStructureClassification{GroupID: snapshot.Source.GroupID, ComponentSide: snapshot.Source.ComponentSide, Language: snapshot.Source.Language}
	inputs := &structureInputs{ManifestID: strings.Repeat("9", 32), ManifestSHA256: lexicalSHA([]byte("synthetic fixture manifest")), FoundationPolicyVersion: 1, StructurePolicyVersion: 1, Members: []entity.KnowledgeStructureViewMember{member}, Aliases: []entity.KnowledgeStructureAlias{}}
	parsed, err := structure.Build(ctx, []structure.Input{{SourceID: member.SourceID, RevisionID: member.SourceRevisionID, Path: name, Parser: parser, Content: body}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := structureBindBuild(knowledgeTestOwner, knowledgeTestDomain, strings.Repeat("8", 32), producer, inputs, parsed, map[string][]byte{member.SourceID: body})
	if err != nil {
		t.Fatal(err)
	}
	bound.View.PublishedAt = lexicalTime(time.Now())
	if err := dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entity.KnowledgeStructureViewRecord{ID: bound.View.ViewID, OwnerID: knowledgeTestOwner, DomainID: knowledgeTestDomain, Published: true, Payload: lexicalJSON(bound.View), CreatedAt: time.Now().UTC(), PublishedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		return tx.CreateInBatches(&bound.Nodes, 128).Error
	}); err != nil {
		t.Fatal(err)
	}
	return entity.KnowledgeLexicalRepresentationRef{Kind: "structure_view", ID: bound.View.ViewID, ProducerSHA256: &producer}
}

func lexicalSaveEvidence(t *testing.T, operation, name string, data any) {
	t.Helper()
	dir := os.Getenv("LFEN_KCLS_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if !strings.HasPrefix(filepath.Clean(dir), "/tmp/lfen-kcls-be-20261028-01") {
		t.Fatal("evidence output boundary")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	filename := t.Name() + "-" + name + ".service-data.json"
	if strings.ContainsAny(filename, "/\\") {
		t.Fatal("evidence filename")
	}
	f, err := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	kind := "ServiceData"
	if operation == "verification" {
		kind = "VerificationEvidence"
	}
	if operation == "serviceError" {
		kind = "ServiceError"
	}
	meta := map[string]any{"kind": kind, "operation": operation, "file": filename, "byte_length": len(raw), "sha256": lexicalSHA(raw), "http_envelope": false}
	b, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, t.Name()+"-"+name+".meta.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func lexicalSaveError(t *testing.T, name string, err error) {
	t.Helper()
	var e *KnowledgeLexicalError
	if !errors.As(err, &e) {
		t.Fatalf("expected controlled error: %v", err)
	}
	lexicalSaveEvidence(t, "serviceError", name, map[string]any{"status": e.Status, "code": e.Code, "outcome_known": e.OutcomeKnown, "retry_same_key": e.RetrySameKey})
}

const lexicalFixturesJSON = "[{\"path\":\"field-guide-v1.md\",\"content_utf8\":\"# 星栈运维样例\\n\\n> 本文为原创合成资料，所有名称、数值与链接只用于离线验证。\\n\\n## 交接规则\\n\\n星栈演练代号为 MIST-42，交接窗口为 09:10—09:25。\\n\\n- 先核对来源修订。\\n- 再记录处理结果。\\n\\n## 阈值表\\n\\n| 指标 | 阈值 | 单位 |\\n| --- | --- | --- |\\n| 缓冲上限 | 12 | 条 |\\n| 等待上限 | 40 | 秒 |\\n\\n## 代码与附件\\n\\n```ts\\nexport const bufferLimit = 12;\\n```\\n\\n[离线附件：星栈示意图](https://fixtures.invalid/attachments/star-stack-diagram.svg)\\n\\n附件只保留链接；本批不抓取附件正文。\\n\"},{\"path\":\"field-guide-v2.md\",\"content_utf8\":\"# 星栈运维样例\\n\\n> 本文为原创合成资料，所有名称、数值与链接只用于离线验证。\\n\\n## 交接规则\\n\\n星栈演练代号为 MIST-42，交接窗口为 09:10—09:25。\\n\\n- 先核对来源修订。\\n- 再记录处理结果。\\n\\n## 阈值表\\n\\n| 指标 | 阈值 | 单位 |\\n| --- | --- | --- |\\n| 缓冲上限 | 18 | 条 |\\n| 等待上限 | 40 | 秒 |\\n\\n## 代码与附件\\n\\n```ts\\nexport const bufferLimit = 18;\\n```\\n\\n[离线附件：星栈示意图](https://fixtures.invalid/attachments/star-stack-diagram.svg)\\n\\n附件只保留链接；本批不抓取附件正文。\\n\"},{\"path\":\"run-notes-v1.txt\",\"content_utf8\":\"合成运行笔记\\r\\n记录编号：TEXT-SYN-07\\r\\n\\r\\n第一轮\\r\\n交接人代号：青屿。\\r\\n重复片段：星栈信号\\r\\n结果：先保存原文，再建立索引。\\r\\n\\r\\n第二轮\\r\\n重复片段：星栈信号\\r\\n备注：空行保留，重复内容不合并为同一来源位置。\\r\\n\"},{\"path\":\"signal-card-v1.tsx\",\"content_utf8\":\"import { useState } from 'react';\\nimport type { ReactNode } from 'react';\\n\\nexport type SignalCardProps = {\\n  label: string;\\n  badge?: ReactNode;\\n};\\n\\nexport const defaultSignalLabel = '星栈就绪';\\n\\nexport function SignalCard({ label, badge }: SignalCardProps) {\\n  const [expanded, setExpanded] = useState(false);\\n\\n  function toggleDetails() {\\n    setExpanded((current) => !current);\\n  }\\n\\n  return (\\n    <section aria-label={label}>\\n      <h2>{label}</h2>\\n      <button type=\\\"button\\\" onClick={toggleDetails}>\\n        {expanded ? '收起详情' : '展开详情'}\\n      </button>\\n      {expanded && (\\n        <p>\\n          星栈组件仅用于合成验证。\\n          {badge}\\n        </p>\\n      )}\\n    </section>\\n  );\\n}\\n\"}]"
const lexicalVectorsJSON = "[{\"binding_id\":\"91a75f6ea4a7cf1f69a3be5f9649aff1e48f5df04d1cc5058a44d6f55fb26484\",\"index_row_id\":\"6aa92c4afbf211f6d263b6824aabe4388bac9df392e20137b9d4179011744d22\",\"chunk_plan_sha256\":\"c0c5d19e0d561d1488a43bcaffa0c82f9e781eaeb551a37b4d1cd3873aeec28b\",\"knowledge_ref\":{\"projection_version\":\"0.1.0-candidate\",\"source_ref\":{\"domain_id\":\"dddddddddddddddddddddddddddddddd\",\"source_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"source_revision_id\":\"11111111111111111111111111111111\",\"source_sha256\":\"54cdc84e2ba89f4191a23902f3f7393ad3b47fe9b44e4e56f132d1c614c76ef8\"},\"representation_ref\":{\"kind\":\"structure_view\",\"id\":\"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\",\"producer_sha256\":\"b11b15856149b9b678ec6327411bdb7d5e4ab433a2d4f80ec2f2c0eb07a01809\"},\"members\":[{\"node_ref\":{\"kind\":\"r1_node\",\"id\":\"0000000000000000000000000000000000000000000000000000000000000065\"},\"range\":{\"start_byte\":0,\"end_byte\":580,\"sha256\":\"54cdc84e2ba89f4191a23902f3f7393ad3b47fe9b44e4e56f132d1c614c76ef8\"}}]}},{\"binding_id\":\"5ad6a5f9ba6d6a3632fbad496ed87ac5332e2a696818e33a10fd5ffd938ec036\",\"index_row_id\":\"fcccbe1f6a43195936c58324508c5d3d6d34020a7987ed3da96f3aede0b34ddf\",\"chunk_plan_sha256\":\"c0c5d19e0d561d1488a43bcaffa0c82f9e781eaeb551a37b4d1cd3873aeec28b\",\"knowledge_ref\":{\"projection_version\":\"0.1.0-candidate\",\"source_ref\":{\"domain_id\":\"dddddddddddddddddddddddddddddddd\",\"source_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"source_revision_id\":\"22222222222222222222222222222222\",\"source_sha256\":\"d04ff0dd5fcb7e49ccff838cf4473f87a0a7cbdb4c4c22e6d8080aa225c70fe1\"},\"representation_ref\":{\"kind\":\"structure_view\",\"id\":\"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\",\"producer_sha256\":\"b11b15856149b9b678ec6327411bdb7d5e4ab433a2d4f80ec2f2c0eb07a01809\"},\"members\":[{\"node_ref\":{\"kind\":\"r1_node\",\"id\":\"0000000000000000000000000000000000000000000000000000000000000066\"},\"range\":{\"start_byte\":0,\"end_byte\":580,\"sha256\":\"d04ff0dd5fcb7e49ccff838cf4473f87a0a7cbdb4c4c22e6d8080aa225c70fe1\"}}]}},{\"binding_id\":\"c31848eb162b5b8274d71265a2d69ed16a0bcb495d6e36881d08fc090ea5e7d5\",\"index_row_id\":\"1789e1c1c48661b4d527d03d893665632f8b63d5d061f658a25888c71a12a915\",\"chunk_plan_sha256\":\"c0c5d19e0d561d1488a43bcaffa0c82f9e781eaeb551a37b4d1cd3873aeec28b\",\"knowledge_ref\":{\"projection_version\":\"0.1.0-candidate\",\"source_ref\":{\"domain_id\":\"dddddddddddddddddddddddddddddddd\",\"source_id\":\"cccccccccccccccccccccccccccccccc\",\"source_revision_id\":\"44444444444444444444444444444444\",\"source_sha256\":\"67af464ec94005887aaba2cbbfc20d6fe816b585c148465b48150e8fa45ebaff\"},\"representation_ref\":{\"kind\":\"structure_view\",\"id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"producer_sha256\":\"b11b15856149b9b678ec6327411bdb7d5e4ab433a2d4f80ec2f2c0eb07a01809\"},\"members\":[{\"node_ref\":{\"kind\":\"r1_node\",\"id\":\"0000000000000000000000000000000000000000000000000000000000000067\"},\"range\":{\"start_byte\":0,\"end_byte\":721,\"sha256\":\"67af464ec94005887aaba2cbbfc20d6fe816b585c148465b48150e8fa45ebaff\"}}]}},{\"binding_id\":\"6ae296823863cf9cf41456719eef6d82f1f481fa9e6ddb371853040c6769de81\",\"index_row_id\":\"968c53840a346cef55f2c85253def640896cf137657d55598ed2739fd83bbed1\",\"chunk_plan_sha256\":\"c286b36ad861fe8923e00de7a1ee43fb61141173b4db682be80f86ea859d3a5f\",\"knowledge_ref\":{\"projection_version\":\"0.1.0-candidate\",\"source_ref\":{\"domain_id\":\"dddddddddddddddddddddddddddddddd\",\"source_id\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"source_revision_id\":\"33333333333333333333333333333333\",\"source_sha256\":\"4666dd2979f23e1bf7bdb6199de1a0112c946eb1ae689d8b6e01acf93290d462\"},\"representation_ref\":{\"kind\":\"source_revision\",\"id\":\"33333333333333333333333333333333\",\"producer_sha256\":null},\"members\":[{\"node_ref\":{\"kind\":\"c1_node\",\"id\":\"0000000000000000000000000000000000000000000000000000000000000068\"},\"range\":{\"start_byte\":0,\"end_byte\":279,\"sha256\":\"4666dd2979f23e1bf7bdb6199de1a0112c946eb1ae689d8b6e01acf93290d462\"}}]}}]"

func lexicalFixtureSelection(t *testing.T, s *DatasetService, ctx context.Context, store storage.Storage) entity.KnowledgeLexicalSelection {
	t.Helper()
	var fixtures []struct {
		Path    string `json:"path"`
		Content string `json:"content_utf8"`
	}
	if err := json.Unmarshal([]byte(lexicalFixturesJSON), &fixtures); err != nil {
		t.Fatal(err)
	}
	selection := entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: []entity.KnowledgeLexicalTarget{}}
	for i, f := range fixtures {
		if f.Path == "field-guide-v2.md" {
			continue
		}
		docID := strings.Repeat(string(rune('2'+i)), 32)
		uploadKnowledgeTest(t, store, docID, f.Path, []byte(f.Content))
		snapshot, err := s.SnapshotKnowledgeSource(ctx, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: docID})
		if err != nil {
			t.Fatal(err)
		}
		target := entity.KnowledgeLexicalTarget{Source: entity.KnowledgeLexicalSourceRef{DomainID: knowledgeTestDomain, SourceID: docID, RevisionID: snapshot.Source.ID, SHA256: snapshot.Source.SHA256}, Representation: entity.KnowledgeLexicalRepresentationRef{Kind: "source_revision", ID: snapshot.Source.ID}}
		if strings.HasSuffix(f.Path, ".md") || strings.HasSuffix(f.Path, ".tsx") {
			target.Representation = lexicalPublishedR1Fixture(t, s, ctx, snapshot, f.Path, []byte(f.Content))
		}
		selection.Targets = append(selection.Targets, target)
	}
	return selection
}

func TestKnowledgeLocalLexicalJobCurrentnessAcrossStates(t *testing.T) {
	if _, err := os.Stat("/usr/share/infinity/resource/rag/huqie.trie"); err != nil {
		t.Skip("native lexical resources not prepared")
	}
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: "/usr/share/infinity/resource", MinSize: 1, MaxSize: 2}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tokenizer.Close)
	s, ctx := lexicalTestContext(t)
	all := lexicalFixtureSelection(t, s, ctx, storage.GetStorageFactory().GetStorage())
	producer, err := tokenizer.ProducerSHA256()
	if err != nil {
		t.Fatal(err)
	}
	one := entity.KnowledgeLexicalSelection{Kind: "fixed_revisions", Targets: all.Targets[:1]}
	selections := []entity.KnowledgeLexicalSelection{one, all}
	publications := []entity.KnowledgeLexicalPublicationRecord{}
	for _, selection := range selections {
		selection, sha := lexicalSelection(selection)
		units, coverage, _, err := s.lexicalUnits(ctx, knowledgeTestOwner, knowledgeTestDomain, "", producer, selection)
		if err != nil {
			t.Fatal(err)
		}
		m := lexicalManifest{Version: entity.KnowledgeLocalLexicalVersion, Owner: knowledgeTestOwner, Domain: knowledgeTestDomain, Selection: selection, SelectionSHA256: sha, PolicyVersion: 1, FoundationVersion: 1, Producer: producer, Language: "", Profile: lexicalProfile("", producer), Coverage: coverage, Rows: []lexicalManifestRow{}, PublishedAt: lexicalTime(time.Now())}
		for _, u := range units {
			m.Rows = append(m.Rows, u.Row)
		}
		payload := lexicalJSON(m)
		digest := lexicalSHA([]byte(payload))
		p := entity.KnowledgeLexicalPublicationRecord{ID: lexicalDigest("knowledge-local-index-publication/1", knowledgeTestOwner, knowledgeTestDomain, digest), OwnerID: knowledgeTestOwner, DomainID: knowledgeTestDomain, SelectionSHA256: sha, ManifestSHA256: digest, Payload: payload}
		if err := dao.DB.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
		publications = append(publications, p)
	}
	for phase := 0; phase < 2; phase++ {
		if phase == 1 {
			if err := dao.DB.Model(&entity.KnowledgeSourceRevision{}).Where("id = ?", one.Targets[0].Source.RevisionID).Update("is_current", false).Error; err != nil {
				t.Fatal(err)
			}
		}
		for i, selection := range selections {
			expected := "current"
			if phase == 1 {
				expected = "historical"
				if i == 1 {
					expected = "mixed"
				}
			}
			for _, state := range []string{"queued", "failed", "published"} {
				_, sha := lexicalSelection(selection)
				j := entity.KnowledgeLexicalJobRecord{ID: strings.Repeat("c", 32), OwnerID: knowledgeTestOwner, DomainID: knowledgeTestDomain, Key: "synthetic-currentness", RequestSHA256: strings.Repeat("d", 64), Selection: lexicalJSON(selection), SelectionSHA256: sha, PolicyVersion: 1, FoundationVersion: 1, State: state, Diagnostic: "index_failed", CreatedAt: time.Now(), UpdatedAt: time.Now()}
				if state == "published" {
					j.IndexID = publications[i].ID
				}
				view, err := s.lexicalJobView(ctx, j)
				if err != nil || view.Readiness.Currentness != expected {
					t.Fatalf("state=%s phase=%d selection=%d currentness=%+v err=%v", state, phase, i, view, err)
				}
				if state == "published" && view.Readiness.Kind != "ready" {
					t.Fatal("published result schema changed")
				}
			}
		}
	}
}
