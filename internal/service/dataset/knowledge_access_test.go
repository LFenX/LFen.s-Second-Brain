//go:build linux

package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

const accessTestRecipient = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const accessTestOther = "cccccccccccccccccccccccccccccccc"

type accessTestReply struct {
	Data         json.RawMessage `json:"data"`
	Code         string          `json:"code"`
	OutcomeKnown bool            `json:"outcome_known"`
	RetrySameKey bool            `json:"retry_same_key"`
}

func accessTestCall(t *testing.T, s *DatasetService, ctx context.Context, actor, operation string, request any, status int) accessTestReply {
	t.Helper()
	actual, raw := s.KnowledgeAccess(ctx, actor, operation, common.GenerateUUID(), []byte(accessJSON(request)))
	var reply accessTestReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatal(err)
	}
	if actual != status {
		t.Fatalf("operation=%s status=%d want=%d body=%s", operation, actual, status, raw)
	}
	if err := entity.KnowledgeAccessDecode(raw, accessDefinition(operation), nil); err != nil {
		t.Fatalf("invalid output schema: %s %v", raw, err)
	}
	return reply
}
func accessTestData[T any](t *testing.T, reply accessTestReply) T {
	t.Helper()
	var value T
	if json.Unmarshal(reply.Data, &value) != nil {
		t.Fatalf("invalid test data %s", reply.Data)
	}
	return value
}
func accessTestSession(t *testing.T, user string) context.Context {
	t.Helper()
	ensureDatasetTestMembership(t, user, knowledgeTestOwner, "normal")
	token := "synthetic-access-session-" + user
	if err := dao.DB.Model(&entity.User{}).Where("id = ?", user).Update("access_token", token).Error; err != nil {
		t.Fatal(err)
	}
	return context.WithValue(t.Context(), lexicalSessionKey{}, lexicalSession{UserID: user, SHA256: lexicalSHA([]byte(token))})
}

type accessTestEngine struct {
	*lexicalTestEngine
	inject    func([]map[string]any) []map[string]any
	observed  *types.SearchRequest
	returnNil bool
}

func (e *accessTestEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.searches++
	e.observed = req
	if e.returnNil {
		return nil, nil
	}
	if !req.KnowledgeAccessStable || len(req.MatchExprs) != 1 || len(req.RankFeature) != 0 || req.Offset != 0 {
		return nil, fmt.Errorf("unsafe scoped request")
	}
	clauses, ok := req.Filter["or"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing paired scope")
	}
	rows := []map[string]any{}
	for _, row := range e.rows {
		allowed := false
		for _, clause := range clauses {
			ids, _ := clause["id"].([]string)
			allowed = allowed || clause["knowledge_binding_kwd"] == row["knowledge_binding_kwd"] && slices.Contains(ids, row["id"].(string))
		}
		if allowed {
			copy := map[string]any{}
			for k, v := range row {
				copy[k] = v
			}
			copy["_score"] = 9999
			rows = append(rows, copy)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["id"].(string) < rows[j]["id"].(string) })
	if len(rows) > req.Limit {
		rows = rows[:req.Limit]
	}
	if e.inject != nil {
		rows = e.inject(rows)
	}
	return &types.SearchResult{Chunks: rows, Total: 999999}, nil
}

type accessTestStorage struct {
	storage.Storage
	gets  map[string]int
	onGet func()
}

func (s *accessTestStorage) Get(ctx context.Context, bucket, key string, tenant ...string) ([]byte, error) {
	s.gets[key]++
	if s.onGet != nil {
		s.onGet()
	}
	return s.Storage.Get(ctx, bucket, key, tenant...)
}

type accessTestFixture struct {
	Service          *DatasetService
	Owner, Recipient context.Context
	Scope            entity.KnowledgeAccessModuleScope
	Module           entity.KnowledgeAccessModuleAdmin
	Grant            entity.KnowledgeAccessGrant
	Manifest         *contentLexicalManifest
	Engine           *accessTestEngine
	Store            *accessTestStorage
}

func accessTestPublish(t *testing.T, s *DatasetService, ctx context.Context, selection entity.KnowledgeContentLexicalSelection) (entity.KnowledgeAccessModuleScope, *contentLexicalManifest, *accessTestEngine) {
	t.Helper()
	prepared, err := s.prepareKnowledgeContentLexical(ctx, knowledgeTestOwner, knowledgeTestDomain, selection, true)
	if err != nil {
		t.Fatal(err)
	}
	var kb entity.Knowledgebase
	if err = dao.DB.Where("id = ?", knowledgeTestDomain).Take(&kb).Error; err != nil {
		t.Fatal(err)
	}
	fences, err := contentLexicalCaptureSources(dao.DB, &kb, prepared.Selection)
	if err != nil {
		t.Fatal(err)
	}
	rows, tokenless, err := lexicalTokenizeInputs(ctx, entity.KnowledgeLexicalJobRecord{DomainID: kb.ID, Language: prepared.Policies.Language, CreatedAt: time.Now()}, prepared.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest := &contentLexicalManifest{Version: entity.KnowledgeContentVersion, Protocol: entity.KnowledgeContentLexicalProtocol, Owner: knowledgeTestOwner, Domain: kb.ID, Selection: prepared.Selection, SelectionSHA256: prepared.SelectionSHA256, Policies: prepared.Policies, ProducerSHA256: prepared.ProducerSHA256, ProfileSHA256: prepared.ProfileSHA256, Coverage: prepared.Coverage, TokenlessBytes: tokenless, Rows: prepared.Rows, Sources: fences, PublishedAt: accessTime(time.Now())}
	payload := accessJSON(manifest)
	hash := lexicalSHA([]byte(payload))
	record := entity.KnowledgeLexicalPublicationRecord{ID: lexicalDigest("knowledge-content-index-publication/0.2.0", knowledgeTestOwner, kb.ID, hash), OwnerID: knowledgeTestOwner, DomainID: kb.ID, Protocol: entity.KnowledgeContentLexicalProtocol, SelectionSHA256: prepared.SelectionSHA256, ManifestSHA256: hash, Payload: payload}
	if _, err = contentLexicalPublication(record); err != nil {
		t.Fatal(err)
	}
	if err = dao.DB.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	native := &accessTestEngine{lexicalTestEngine: &lexicalTestEngine{rows: map[string]map[string]any{}}}
	for _, row := range rows {
		native.rows[row["id"].(string)] = row
	}
	s.docEngine = native
	scope := entity.KnowledgeAccessModuleScope{Kind: "fixed_published_targets", Publication: entity.KnowledgeAccessOwnerPublication{ID: record.ID, ManifestSHA256: hash}, Targets: []entity.KnowledgeAccessTarget{}}
	for _, target := range prepared.Selection.Targets {
		scope.Targets = append(scope.Targets, accessTarget(target))
	}
	return scope, manifest, native
}
func accessTestSetup(t *testing.T, mixed bool) *accessTestFixture {
	t.Helper()
	var s *DatasetService
	var owner context.Context
	var selection entity.KnowledgeContentLexicalSelection
	if mixed {
		s, owner, selection, _ = contentLexicalMixedFixture(t)
	} else {
		var store storage.Storage
		s, store = setupKnowledgeTest(t)
		registerKnowledgeTest(t, s, knowledgeTestDomain)
		owner = contentLexicalSession(t, s, true, true)
		selection = entity.KnowledgeContentLexicalSelection{Kind: "fixed_representations", Targets: []entity.KnowledgeContentTarget{}}
		for i, body := range []string{strings.Repeat("合成 A 固定原文\r\n", 800), "Denied synthetic B body\r\n"} {
			id := strings.Repeat(fmt.Sprint(i+2), 32)
			name := fmt.Sprintf("source-%d.txt", i)
			uploadKnowledgeTest(t, store, id, name, []byte(body))
			snapshot, err := s.SnapshotKnowledgeSource(owner, knowledgeTestOwner, knowledgeTestDomain, entity.SnapshotKnowledgeSourceRequest{DocumentID: id, RelativePath: name, ComponentSide: "non_code"})
			if err != nil {
				t.Fatal(err)
			}
			selection.Targets = append(selection.Targets, contentTarget(contentSource(&snapshot.Source), "source_revision", snapshot.Source.ID, nil))
		}
	}
	scope, manifest, native := accessTestPublish(t, s, owner, selection)
	recipient := accessTestSession(t, accessTestRecipient)
	base := storage.GetStorageFactory().GetStorage()
	counted := &accessTestStorage{Storage: base, gets: map[string]int{}}
	storage.GetStorageFactory().SetStorage(counted)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(base) })
	return &accessTestFixture{Service: s, Owner: owner, Recipient: recipient, Scope: scope, Manifest: manifest, Engine: native, Store: counted}
}
func (f *accessTestFixture) enable(t *testing.T, targets []entity.KnowledgeAccessTarget, mode string) {
	t.Helper()
	s := f.Service
	accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ExpectedVersion: 0, Enabled: true, Confirmed: true, IdempotencyKey: "Synthetic_Policy_0001"}, 200)
	scope := f.Scope
	scope.Targets = targets
	f.Module = accessTestData[entity.KnowledgeAccessModuleAdmin](t, accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setModule", entity.KnowledgeAccessSetModuleRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, Name: "Synthetic module", Enabled: true, Scope: scope, Confirmed: true, IdempotencyKey: "Synthetic_Module_0001"}, 200))
	f.Grant = accessTestData[entity.KnowledgeAccessGrant](t, accessTestCall(t, s, f.Owner, knowledgeTestOwner, "createGrant", entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, ExpectedModuleVersion: f.Module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: accessTestRecipient}, Mode: mode, Delivery: "web_and_mcp", ExpiresAt: accessTime(time.Now().Add(time.Hour)), Confirmed: true, IdempotencyKey: "Synthetic_Grant_0001"}, 201))
}
func (f *accessTestFixture) inspect(t *testing.T, mode string, ids *[]string) entity.KnowledgeAccessInspectData {
	t.Helper()
	return accessTestData[entity.KnowledgeAccessInspectData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: mode, Delivery: "web_only", SourceIDs: ids}, 200))
}
func accessTestReadRequest(view string, target, ordinal int64) entity.KnowledgeAccessReadRequest {
	return entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: json.RawMessage(accessJSON(entity.KnowledgeAccessUnitReadSelector{Kind: "unit", ViewID: view, TargetIndex: target, UnitOrdinal: ordinal}))}
}
func accessTestEpoch(t *testing.T) int64 {
	t.Helper()
	var row entity.KnowledgeAccessTenantStateRecord
	if err := dao.DB.Where("tenant_id = ?", knowledgeTestOwner).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.Epoch
}

func TestKnowledgeAccessCommandsReplayCASAndRollback(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	s := f.Service
	list := accessTestData[entity.KnowledgeAccessListModulesData](t, accessTestCall(t, s, f.Owner, knowledgeTestOwner, "listModules", entity.KnowledgeAccessListModulesRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner, DomainID: &[]string{knowledgeTestDomain}[0]}, 200))
	if len(list.OwnedDomainPolicies) != 1 || list.OwnedDomainPolicies[0].Version != 0 || list.OwnedDomainPolicies[0].Enabled {
		t.Fatal(list)
	}
	first := entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, Enabled: true, Confirmed: true, IdempotencyKey: "Synthetic_Policy_REPLAY"}
	p1 := accessTestData[entity.KnowledgeAccessPolicy](t, accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", first, 200))
	second := first
	second.ExpectedVersion = 1
	second.Enabled = false
	second.IdempotencyKey = "Synthetic_Policy_SECOND"
	accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", second, 200)
	epoch := accessTestEpoch(t)
	replayed := accessTestData[entity.KnowledgeAccessPolicy](t, accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", first, 200))
	if replayed != p1 || accessTestEpoch(t) != epoch {
		t.Fatal("replay changed original result or epoch")
	}
	conflict := first
	conflict.Enabled = false
	if accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", conflict, 409).Code != "idempotency_conflict" {
		t.Fatal("different body reused key")
	}
	stale := second
	stale.IdempotencyKey = "Synthetic_CAS_REUSABLE"
	if accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", stale, 409).Code != "version_conflict" {
		t.Fatal("CAS accepted")
	}
	stale.ExpectedVersion = 2
	stale.Enabled = true
	accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", stale, 200)
	callback := "access:test:reject_command_audit"
	if err := dao.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "knowledge_access_audit" {
			tx.AddError(errors.New("synthetic audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failing := first
	failing.ExpectedVersion = 3
	failing.IdempotencyKey = "Synthetic_ATOMIC_FAILURE"
	epoch = accessTestEpoch(t)
	accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", failing, 503)
	dao.DB.Callback().Update().Remove(callback)
	var policy entity.KnowledgeAccessDomainPolicyRecord
	dao.DB.Where("domain_id = ?", knowledgeTestDomain).Take(&policy)
	receipt, err := accessReceipt(t.Context(), knowledgeTestOwner, "setPolicy", failing.IdempotencyKey)
	if err != nil || receipt != nil || policy.Version != 3 || accessTestEpoch(t) != epoch {
		t.Fatal("failed audit left partial state", receipt, policy, err)
	}
	accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", failing, 200)
	var row entity.KnowledgeAccessAuditRecord
	if err := dao.DB.Where("command_key = ?", first.IdempotencyKey).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.CommandResultJSON == nil || row.CommandState == nil || *row.CommandState != "completed" || len(row.CommandRequestSHA256) != 32 {
		t.Fatal("missing safe receipt")
	}
	if err := dao.DB.Model(&row).Updates(map[string]any{"command_compacted_at": time.Now(), "command_result_json": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if accessTestCall(t, s, f.Owner, knowledgeTestOwner, "setPolicy", first, 409).Code != "idempotency_conflict" {
		t.Fatal("compacted key executed")
	}
}

func TestKnowledgeAccessSubsetUnitReadAndIdentityIsolation(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read")
	f.Store.gets = map[string]int{}
	inspectReq := entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: "read", Delivery: "web_only"}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "inspect", inspectReq, 404)
	denied := inspectReq
	denied.SourceIDs = &[]string{f.Scope.Targets[1].Source.SourceID}
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", denied, 404)
	if len(f.Store.gets) != 0 {
		t.Fatal("denied/inspect loaded body")
	}
	view := f.inspect(t, "read", nil)
	if len(view.Targets) != 1 || view.SourceCount != 1 || view.Targets[0].UnitCount < 2 || len(f.Store.gets) != 0 {
		t.Fatal("catalog leaked body or used global source set", view, f.Store.gets)
	}
	first := accessTestData[entity.KnowledgeAccessReadData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 200))
	if first.Text == "" || first.UnitLocator.NextUnitOrdinal == nil || first.UnitLocator.TargetUnitCount != view.Targets[0].UnitCount || !first.Complete {
		t.Fatal(first)
	}
	byEvidence := entity.KnowledgeAccessReadRequest{ContractVersion: entity.KnowledgeAccessVersion, Selector: json.RawMessage(accessJSON(entity.KnowledgeAccessEvidenceReadSelector{Kind: "evidence", EvidenceRef: first.EvidenceRef}))}
	again := accessTestData[entity.KnowledgeAccessReadData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", byEvidence, 200))
	if again.Text != first.Text || accessJSON(again.UnitLocator) != accessJSON(first.UnitLocator) {
		t.Fatal("evidence did not bind whole unit")
	}
	for key := range f.Store.gets {
		if strings.Contains(key, f.Scope.Targets[1].Source.SourceSHA256) {
			t.Fatal("denied source body materialized")
		}
	}
	other := accessTestSession(t, accessTestOther)
	accessTestCall(t, f.Service, other, accessTestOther, "read", byEvidence, 404)
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 1, 0), 400)
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "search", entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "合成", Limit: 2}, 403)
	if f.Engine.searches != 0 {
		t.Fatal("read grant searched")
	}
	_, err := f.Service.GetKnowledgeContentRepresentation(f.Recipient, accessTestRecipient, knowledgeTestDomain, accessContentTarget(f.Scope.Targets[0]))
	assertKnowledgeStatus(t, err, 404)
	var audit []entity.KnowledgeAccessAuditRecord
	dao.DB.Where("action = ? AND outcome = ?", "read", "allowed").Find(&audit)
	if len(audit) != 2 {
		t.Fatal("allowed reads not durably audited", len(audit))
	}
	for _, row := range audit {
		if row.CommandOperation != nil || row.BeforeDefinitionJSON != nil || row.AfterDefinitionJSON != nil {
			t.Fatal("content audit contains command/body fields")
		}
	}
}

func TestKnowledgeAccessSearchScopesBeforeLimitAndRejectsInjectedHits(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read_search")
	view := f.inspect(t, "read_search", nil)
	request := entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "合成", Limit: 2}
	result := accessTestData[entity.KnowledgeAccessSearchData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "search", request, 200))
	if len(result.Hits) != 2 || result.TotalMatches != nil || result.Hits[0].RankOrdinal != 1 || result.Hits[1].RankOrdinal != 2 {
		t.Fatal(result)
	}
	for _, hit := range result.Hits {
		if hit.Citation.Target.Source.SourceID != f.Scope.Targets[0].Source.SourceID {
			t.Fatal("scope postfilter exposed B")
		}
	}
	for _, kind := range []string{"foreign_id", "binding", "reference", "text", "domain", "source", "duplicate", "order"} {
		t.Run(kind, func(t *testing.T) {
			f.Engine.inject = func(rows []map[string]any) []map[string]any {
				switch kind {
				case "foreign_id":
					for _, row := range f.Engine.rows {
						if row["doc_id"] == f.Scope.Targets[1].Source.SourceID {
							rows[0] = row
							break
						}
					}
				case "binding":
					rows[0]["knowledge_binding_kwd"] = strings.Repeat("e", 64)
				case "reference":
					rows[0]["knowledge_ref"] = "{}"
				case "text":
					rows[0]["content_with_weight"] = "injected body"
				case "domain":
					rows[0]["kb_id"] = knowledgeTestOther
				case "source":
					rows[0]["doc_id"] = f.Scope.Targets[1].Source.SourceID
				case "duplicate":
					rows[1] = rows[0]
				case "order":
					rows[0], rows[1] = rows[1], rows[0]
				}
				return rows
			}
			reply := accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "search", request, 409)
			if reply.Code != "integrity_error" || string(reply.Data) != "null" {
				t.Fatal(reply)
			}
		})
	}
	f.Engine.inject = nil
}

func TestKnowledgeAccessFinalPermitRevocationAndEpochNeverRevive(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read")
	view := f.inspect(t, "read", nil)
	requestID := common.GenerateUUID()
	verified, err := f.Service.accessOpenView(f.Recipient, accessTestRecipient, view.ViewID, "read", requestID)
	if err != nil {
		t.Fatal(err)
	}
	revoke := entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: "Synthetic_Revoke_0001"}
	original := accessTestData[entity.KnowledgeAccessRevoked](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "revokeGrant", revoke, 200))
	epoch := accessTestEpoch(t)
	if err = f.Service.accessFinalPermit(f.Recipient, verified, "read", requestID); err == nil {
		t.Fatal("permit committed after revocation")
	}
	var audit entity.KnowledgeAccessAuditRecord
	dao.DB.Where("id = ?", verified.AuditID).Take(&audit)
	if audit.Outcome != "denied" {
		t.Fatal("revoked response was audited allowed")
	}
	replayed := accessTestData[entity.KnowledgeAccessRevoked](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "revokeGrant", revoke, 200))
	if original != replayed || accessTestEpoch(t) != epoch {
		t.Fatal("revoke replay changed receipt or epoch")
	}
	for _, flag := range []string{"0", "1"} {
		if err = dao.WithKnowledgeAccessNativeMutation(t.Context(), dao.DB, dao.KnowledgeAccessNativeMutation{Action: "native_user_flags", UserIDs: []string{accessTestRecipient}}, func(tx *gorm.DB) error {
			return tx.Model(&entity.User{}).Where("id = ?", accessTestRecipient).Update("is_active", flag).Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	if accessTestEpoch(t) != epoch+2 {
		t.Fatal("disable/restore did not advance generation")
	}
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 409)
	// Existing owner-only fixed content still has its own explicit native check.
	row := f.Manifest.Rows[0]
	oldRef := contentLexicalEvidence(entity.KnowledgeLexicalIndexRef{ID: f.Scope.Publication.ID, ManifestSHA256: f.Scope.Publication.ManifestSHA256}, row)
	if _, err = f.Service.ReadKnowledgeContentLexical(f.Owner, knowledgeTestOwner, knowledgeTestDomain, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: oldRef}); err != nil {
		t.Fatal("sharing revoke changed old owner read", err)
	}
}

func TestKnowledgeAccessFourOwningFamiliesAndSafeTypedProjection(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, true)
	f.enable(t, f.Scope.Targets, "read")
	view := f.inspect(t, "read", nil)
	kinds := map[string]bool{}
	for i, target := range view.Targets {
		if target.UnitCount < 1 {
			t.Fatal(target)
		}
		read := accessTestData[entity.KnowledgeAccessReadData](t, accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, int64(i), 0), 200))
		kinds[target.Target.Representation.Kind] = true
		if accessJSON(read.Citation.Target) != accessJSON(target.Target) || read.ByteLength != int64(len(read.Text)) || read.ByteLength > 8192 || read.Citation.Range.SHA256 != lexicalSHA([]byte(read.Text)) {
			t.Fatal("fixed unit proof changed")
		}
		for _, member := range read.TypedContext.Members {
			if member.Range.StartByte < read.Citation.Range.StartByte || member.Range.EndByte > read.Citation.Range.EndByte {
				t.Fatal("typed member expanded evidence")
			}
		}
		raw := accessJSON(read)
		for _, forbidden := range []string{"attributes", "original_position", "attachment_ref", "folder_id", "commit_id", "parent_ref"} {
			if strings.Contains(raw, `"`+forbidden+`"`) {
				t.Fatal("raw owning payload exposed", forbidden)
			}
		}
	}
	if len(kinds) != 4 {
		t.Fatal(kinds)
	}
	var private entity.KnowledgeAccessViewRecord
	dao.DB.Where("id = ?", view.ViewID).Take(&private)
	if len(private.PrivateManifestJSON)+len(private.CaptureJSON) > 65536 || strings.Contains(private.PrivateManifestJSON, f.Manifest.Rows[0].ID) {
		t.Fatal("view stored whole row map")
	}
}

func TestKnowledgeAccessDependencyClosureUsesFinalExactSet(t *testing.T) {
	makeTarget := func(id string, kind string) entity.KnowledgeAccessTarget {
		return entity.KnowledgeAccessTarget{Source: entity.KnowledgeAccessSource{DomainID: knowledgeTestDomain, SourceID: strings.Repeat(id, 32), SourceRevisionID: strings.Repeat(id, 32), SourceSHA256: strings.Repeat(id, 64)}, Representation: entity.KnowledgeAccessRepresentation{Kind: kind, ID: strings.Repeat(id, 64)}}
	}
	a, b, c := makeTarget("2", "parsed_representation"), makeTarget("3", "source_revision"), makeTarget("4", "source_revision")
	proof := func(target entity.KnowledgeAccessTarget) accessDependency {
		return accessDependency{Source: target.Source, Target: &target, Complete: true}
	}
	base := map[string][]accessDependency{accessJSON(a): {proof(a), proof(b)}, accessJSON(b): {proof(b), proof(c)}, accessJSON(c): {proof(c)}}
	for _, test := range []struct {
		name     string
		selected []entity.KnowledgeAccessTarget
		valid    bool
	}{{"missing direct", []entity.KnowledgeAccessTarget{a}, false}, {"missing transitive", []entity.KnowledgeAccessTarget{a, b}, false}, {"complete", []entity.KnowledgeAccessTarget{a, b, c}, true}} {
		t.Run(test.name, func(t *testing.T) {
			if (accessVerifyClosure(test.selected, base) == nil) != test.valid {
				t.Fatal("incorrect final transitive closure")
			}
		})
	}
	base[accessJSON(a)] = []accessDependency{{Source: b.Source, Complete: true}}
	base[accessJSON(b)] = []accessDependency{proof(b)}
	if err := accessVerifyClosure([]entity.KnowledgeAccessTarget{a, b}, base); err != nil {
		t.Fatal(err)
	}
	derived := b
	derived.Representation.Kind = "parsed_representation"
	base[accessJSON(derived)] = []accessDependency{proof(derived)}
	if accessVerifyClosure([]entity.KnowledgeAccessTarget{a, derived}, base) == nil {
		t.Fatal("derived target covered source-only dependency")
	}
	base[accessJSON(a)] = []accessDependency{{Source: a.Source, Target: &a, Complete: false}}
	if accessVerifyClosure([]entity.KnowledgeAccessTarget{a}, base) == nil {
		t.Fatal("unknown footprint accepted")
	}
}

func TestKnowledgeAccessPrivateConsumerLogsAndAuditAreContentFree(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read_search")
	if err := common.InitLogger("debug", common.FileOutput{}, "access-private-consumer"); err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.DebugLevel)
	previous, previousSugar := common.Logger, common.Sugar
	common.Logger = zap.New(core)
	common.Sugar = common.Logger.Sugar()
	t.Cleanup(func() { common.Logger, common.Sugar = previous, previousSugar })
	view := f.inspect(t, "read_search", nil)
	_ = accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(view.ViewID, 0, 0), 200)
	query := "合成 PrivateQuerySentinelMustNotBeLogged"
	_ = accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "search", entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: query, Limit: 2}, 200)
	if logs.Len() != 0 {
		t.Fatalf("private consumer emitted %d log entries", logs.Len())
	}
	var audit []entity.KnowledgeAccessAuditRecord
	if err := dao.DB.Where("action IN ?", []string{"inspect", "read", "search"}).Find(&audit).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range audit {
		raw := accessJSON(row)
		for _, marker := range []string{"PrivateQuerySentinelMustNotBeLogged", "固定原文", "synthetic-access-session"} {
			if strings.Contains(raw, marker) {
				t.Fatal("private content/session in audit")
			}
		}
	}
}

func TestKnowledgeAccessMissingEngineResultIsClosed(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read_search")
	view := f.inspect(t, "read_search", nil)
	f.Engine.returnNil = true
	accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "search", entity.KnowledgeAccessSearchRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view.ViewID, Query: "合成", Limit: 1}, 409)
}
func TestKnowledgeAccessTypedEmptyBoundaryAndBudgets(t *testing.T) {
	makeMember := func(id string, a, b int64) entity.KnowledgeContentLexicalTypedMember {
		hash := lexicalSHA([]byte("AB"[a:b]))
		return entity.KnowledgeContentLexicalTypedMember{Node: entity.KnowledgeContentNode{Ref: entity.KnowledgeContentNodeRef{Kind: "parsed_node", ID: &id}, Kind: "cell", Text: entity.KnowledgeContentText{Status: "available", CoordinateSpace: "derived_utf8", Range: &entity.KnowledgeContentRange{StartByte: a, EndByte: b, SHA256: &hash}}}, PayloadSHA256: strings.Repeat("a", 64)}
	}
	snapshot := &contentLexicalSnapshot{Body: "AB", TypedMembers: []entity.KnowledgeContentLexicalTypedMember{makeMember(strings.Repeat("1", 64), 0, 1), makeMember(strings.Repeat("2", 64), 1, 1), makeMember(strings.Repeat("3", 64), 0, 2)}}
	row := contentLexicalManifestRow{Reference: contentLexicalReference{Range: entity.KnowledgeContentRange{StartByte: 0, EndByte: 1}}}
	value, err := accessTypedContext(snapshot, row)
	if err != nil || len(value.Members) != 2 || value.Members[1].TextStatus != "empty" || value.Members[1].Range.SHA256 != lexicalSHA(nil) || !slices.Contains(value.OmissionReasons, "whole_member_outside_evidence") {
		t.Fatal("typed boundary/empty cell changed", value, err)
	}
	snapshot.TypedMembers = nil
	for i := 0; i < 35; i++ {
		snapshot.TypedMembers = append(snapshot.TypedMembers, makeMember(fmt.Sprintf("%064x", i+1), 0, 1))
	}
	value, err = accessTypedContext(snapshot, row)
	if err != nil || len(value.Members) != 32 || value.MembersComplete || !slices.Contains(value.OmissionReasons, "member_budget") {
		t.Fatal("typed member budget overrun", value, err)
	}
	snapshot.Body = "中"
	snapshot.TypedMembers = []entity.KnowledgeContentLexicalTypedMember{makeMember(strings.Repeat("1", 64), 0, 1)}
	if _, err = accessTypedContext(snapshot, row); err == nil {
		t.Fatal("invalid UTF8 boundary projected")
	}
}
func TestKnowledgeAccessConcurrentCommandRetryHasOneReceipt(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	epoch := accessTestEpoch(t)
	req := entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ExpectedVersion: 0, Enabled: true, Confirmed: true, IdempotencyKey: "Synthetic_Concurrent_0001"}
	start := make(chan struct{})
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			status, _ := f.Service.KnowledgeAccess(f.Owner, knowledgeTestOwner, "setPolicy", common.GenerateUUID(), []byte(accessJSON(req)))
			results <- status
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		status := <-results
		if status != 200 && status != 503 {
			t.Fatal("unexpected concurrent command status", status)
		}
	}
	reply := accessTestData[entity.KnowledgeAccessPolicy](t, accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "setPolicy", req, 200))
	if reply.Version != 1 || accessTestEpoch(t) != epoch+1 {
		t.Fatal("concurrent retry repeated mutation")
	}
	var count int64
	if err := dao.DB.Model(&entity.KnowledgeAccessAuditRecord{}).Where("actor_user_id = ? AND command_operation = ? AND command_key = ?", knowledgeTestOwner, "setPolicy", req.IdempotencyKey).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("concurrent duplicate receipt", count)
	}
}

func TestKnowledgeAccessHiddenResourcesAndDeniedActions(t *testing.T) {
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read")
	other := accessTestSession(t, accessTestOther)
	f.Store.gets = map[string]int{}
	inspect := entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: "read", Delivery: "web_only"}
	admin := entity.KnowledgeAccessGetModuleAdminRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID}
	hidden := func(ctx context.Context, actor, operation string, request any) {
		t.Helper()
		reply := accessTestCall(t, f.Service, ctx, actor, operation, request, 404)
		if reply.Code != "not_found" || string(reply.Data) != "null" {
			t.Fatal("hidden resource disclosed a different error", reply)
		}
	}
	hidden(f.Recipient, accessTestRecipient, "getModuleAdmin", admin)
	hidden(other, accessTestOther, "getModuleAdmin", admin)
	hidden(other, accessTestOther, "inspect", inspect)
	hidden(f.Owner, knowledgeTestOwner, "inspect", inspect)
	missingAdmin := admin
	missingAdmin.DomainID = strings.Repeat("e", 32)
	hidden(other, accessTestOther, "getModuleAdmin", missingAdmin)
	if err := dao.DB.Where("user_id = ? AND tenant_id = ?", accessTestOther, knowledgeTestOwner).Delete(&entity.UserTenant{}).Error; err != nil {
		t.Fatal(err)
	}
	hidden(other, accessTestOther, "getModuleAdmin", admin)
	escalation := inspect
	escalation.Mode = "read_search"
	if reply := accessTestCall(t, f.Service, f.Recipient, accessTestRecipient, "inspect", escalation, 403); reply.Code != "action_denied" || string(reply.Data) != "null" {
		t.Fatal("existing read grant acquired search mode", reply)
	}
	accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "revokeGrant", entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: "Synthetic_Hidden_Revoke"}, 200)
	hidden(f.Recipient, accessTestRecipient, "inspect", inspect)
	if len(f.Store.gets) != 0 || f.Engine.searches != 0 {
		t.Fatal("denied access loaded source bytes or searched", f.Store.gets, f.Engine.searches)
	}
}
