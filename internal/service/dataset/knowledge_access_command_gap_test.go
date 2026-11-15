//go:build linux

package dataset

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

func prepareAccessCommandGap(t *testing.T, operation string) (*accessTestFixture, any, int) {
	t.Helper()
	f := accessTestSetup(t, false)
	key := "Synthetic_Gap_" + operation + "_0001"
	switch operation {
	case "setPolicy":
		return f, entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, Enabled: true, Confirmed: true, IdempotencyKey: key}, 200
	case "setModule":
		return f, entity.KnowledgeAccessSetModuleRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, Name: "Synthetic null-ID module", Enabled: true, Scope: f.Scope, Confirmed: true, IdempotencyKey: key}, 200
	case "createGrant":
		f.enable(t, f.Scope.Targets[:1], "read")
		accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "revokeGrant", entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: key + "_prepare"}, 200)
		return f, entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, ExpectedModuleVersion: f.Module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: accessTestRecipient}, Mode: "read_search", Delivery: "web_only", ExpiresAt: accessTime(time.Now().Add(time.Hour)), Confirmed: true, IdempotencyKey: key}, 201
	case "revokeGrant":
		f.enable(t, f.Scope.Targets[:1], "read")
		return f, entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: key}, 200
	default:
		t.Fatal("unknown command case")
		return nil, nil, 0
	}
}

func snapshotAccessCommandGap(t *testing.T) []byte {
	t.Helper()
	var policies []entity.KnowledgeAccessDomainPolicyRecord
	var modules []entity.KnowledgeAccessModuleRecord
	var grants []entity.KnowledgeAccessGrantRecord
	var audits []entity.KnowledgeAccessAuditRecord
	var epochs []entity.KnowledgeAccessTenantStateRecord
	for _, rows := range []any{&policies, &modules, &grants, &audits, &epochs} {
		if err := dao.DB.Order("1 ASC").Find(rows).Error; err != nil {
			t.Fatal("snapshot query failed")
		}
	}
	raw, err := json.Marshal([]any{policies, modules, grants, audits, epochs})
	if err != nil {
		t.Fatal("snapshot encoding failed")
	}
	return raw
}

func checkAccessCommandGapReceipt(t *testing.T, operation, key string) entity.KnowledgeAccessAuditRecord {
	t.Helper()
	var rows []entity.KnowledgeAccessAuditRecord
	if err := dao.DB.Where("actor_user_id = ? AND command_operation = ? AND command_key = ?", knowledgeTestOwner, operation, key).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatal("command receipt count")
	}
	row := rows[0]
	if row.CommandState == nil || *row.CommandState != "completed" || row.CommandResultJSON == nil || row.CommandResourceID == nil || row.CommandCommittedAt == nil || len(row.CommandRequestSHA256) != 32 {
		t.Fatal("incomplete or pending receipt")
	}
	return row
}

func TestKnowledgeAccessS1FourCommandsReplayOriginalAndRecheckOwner(t *testing.T) {
	contentLexicalTokenizer(t)
	for _, operation := range []string{"setPolicy", "setModule", "createGrant", "revokeGrant"} {
		t.Run(operation, func(t *testing.T) {
			f, request, status := prepareAccessCommandGap(t, operation)
			first := accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, operation, request, status)
			var body map[string]any
			if json.Unmarshal([]byte(accessJSON(request)), &body) != nil {
				t.Fatal("invalid request fixture")
			}
			key := body["idempotency_key"].(string)
			// Advance current state, then reconstruct the service without the
			// first response. This is not an OS-process restart claim.
			switch operation {
			case "setPolicy":
				accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, operation, entity.KnowledgeAccessSetPolicyRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ExpectedVersion: 1, Enabled: false, Confirmed: true, IdempotencyKey: key + "_advance"}, 200)
			case "setModule":
				module := accessTestData[entity.KnowledgeAccessModuleAdmin](t, first)
				accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, operation, entity.KnowledgeAccessSetModuleRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: &module.ModuleID, ExpectedVersion: 1, Name: "Synthetic later module", Enabled: true, Scope: f.Scope, Confirmed: true, IdempotencyKey: key + "_advance"}, 200)
			case "createGrant":
				grant := accessTestData[entity.KnowledgeAccessGrant](t, first)
				accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "revokeGrant", entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: grant.GrantID, Confirmed: true, IdempotencyKey: key + "_advance"}, 200)
			case "revokeGrant":
				accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "createGrant", entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, ExpectedModuleVersion: f.Module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: accessTestRecipient}, Mode: "read_search", Delivery: "web_only", ExpiresAt: accessTime(time.Now().Add(time.Hour)), Confirmed: true, IdempotencyKey: key + "_advance"}, 201)
			}
			before := snapshotAccessCommandGap(t)
			restarted := NewDatasetService()
			replay := accessTestCall(t, restarted, f.Owner, knowledgeTestOwner, operation, request, 200)
			if !bytes.Equal(first.Data, replay.Data) || !bytes.Equal(before, snapshotAccessCommandGap(t)) {
				t.Fatal("replay changed original result or persistent state")
			}
			switch operation {
			case "setPolicy":
				body["enabled"] = false
			case "setModule":
				body["name"] = "Synthetic conflicting module"
			case "createGrant":
				body["mode"] = "read"
			case "revokeGrant":
				body["grant_id"] = common.GenerateUUID()
			}
			if accessTestCall(t, restarted, f.Owner, knowledgeTestOwner, operation, body, 409).Code != "idempotency_conflict" || !bytes.Equal(before, snapshotAccessCommandGap(t)) {
				t.Fatal("changed body replaced winner")
			}
			if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestDomain).Update("created_by", accessTestRecipient).Error; err != nil {
				t.Fatal("synthetic owner transfer failed")
			}
			actual, raw := restarted.KnowledgeAccess(f.Owner, knowledgeTestOwner, operation, common.GenerateUUID(), []byte(accessJSON(request)))
			var denied accessTestReply
			if json.Unmarshal(raw, &denied) != nil || entity.KnowledgeAccessDecode(raw, accessDefinition(operation), nil) != nil || (actual != 403 && actual != 404) || !bytes.Equal(denied.Data, []byte("null")) {
				t.Fatal("receipt bypassed current owner")
			}
			if err := dao.DB.Model(&entity.Knowledgebase{}).Where("id = ?", knowledgeTestDomain).Update("created_by", knowledgeTestOwner).Error; err != nil {
				t.Fatal("synthetic owner restore failed")
			}
			row := checkAccessCommandGapReceipt(t, operation, key)
			now := time.Now().UTC()
			if err := dao.DB.Model(&row).Updates(map[string]any{"occurred_at": now.Add(-91 * 24 * time.Hour), "command_compacted_at": now, "command_result_json": nil}).Error; err != nil {
				t.Fatal("synthetic tombstone failed")
			}
			before = snapshotAccessCommandGap(t)
			if accessTestCall(t, NewDatasetService(), f.Owner, knowledgeTestOwner, operation, request, 409).Code != "idempotency_conflict" || !bytes.Equal(before, snapshotAccessCommandGap(t)) {
				t.Fatal("compacted key executed again")
			}
		})
	}
}

func TestKnowledgeAccessS1FourCommandsAuditFailureIsAtomic(t *testing.T) {
	contentLexicalTokenizer(t)
	for _, operation := range []string{"setPolicy", "setModule", "createGrant", "revokeGrant"} {
		for _, stage := range []string{"reservation_insert", "completion_update"} {
			t.Run(operation+"/"+stage, func(t *testing.T) {
				f, request, status := prepareAccessCommandGap(t, operation)
				before := snapshotAccessCommandGap(t)
				writes := 0
				name := "access:gap:" + operation + ":" + stage
				fault := func(tx *gorm.DB) {
					if tx.Statement.Table == "knowledge_access_audit" {
						tx.AddError(errors.New("synthetic command audit failure"))
					}
				}
				observe := func(tx *gorm.DB) {
					if tx.Error != nil || tx.RowsAffected < 1 {
						return
					}
					switch tx.Statement.Table {
					case "knowledge_access_domain_policy", "knowledge_access_module", "knowledge_access_grant":
						writes++
					}
				}
				if stage == "reservation_insert" {
					if err := dao.DB.Callback().Create().Before("gorm:create").Register(name, fault); err != nil {
						t.Fatal(err)
					}
				} else if err := dao.DB.Callback().Update().Before("gorm:update").Register(name, fault); err != nil {
					t.Fatal(err)
				}
				if err := dao.DB.Callback().Create().After("gorm:create").Register(name+"_writes", observe); err != nil {
					t.Fatal(err)
				}
				if err := dao.DB.Callback().Update().After("gorm:update").Register(name+"_writes", observe); err != nil {
					t.Fatal(err)
				}
				remove := func() {
					dao.DB.Callback().Create().Remove(name)
					dao.DB.Callback().Update().Remove(name)
					dao.DB.Callback().Create().Remove(name + "_writes")
					dao.DB.Callback().Update().Remove(name + "_writes")
				}
				t.Cleanup(remove)
				reply := accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, operation, request, 503)
				remove()
				if !bytes.Equal(reply.Data, []byte("null")) || !bytes.Equal(before, snapshotAccessCommandGap(t)) {
					t.Fatal("audit failure left privilege, epoch or command state")
				}
				if stage == "completion_update" && writes == 0 {
					t.Fatal("fault preceded actual privilege write")
				}
				accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, operation, request, status)
			})
		}
	}
}

func TestKnowledgeAccessS1FourCommandsConcurrentSameKey(t *testing.T) {
	contentLexicalTokenizer(t)
	for _, operation := range []string{"setPolicy", "setModule", "createGrant", "revokeGrant"} {
		t.Run(operation, func(t *testing.T) {
			f, request, firstStatus := prepareAccessCommandGap(t, operation)
			accessTestCall(t, f.Service, f.Owner, knowledgeTestOwner, "listModules", entity.KnowledgeAccessListModulesRequest{ContractVersion: entity.KnowledgeAccessVersion, TenantID: knowledgeTestOwner}, 200)
			epoch := accessTestEpoch(t)
			type concurrentReply struct {
				status int
				raw    []byte
			}
			start := make(chan struct{})
			results := make(chan concurrentReply, 2)
			var workers sync.WaitGroup
			for i := 0; i < 2; i++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					status, raw := f.Service.KnowledgeAccess(f.Owner, knowledgeTestOwner, operation, common.GenerateUUID(), []byte(accessJSON(request)))
					results <- concurrentReply{status: status, raw: raw}
				}()
			}
			close(start)
			workers.Wait()
			close(results)
			var successes []accessTestReply
			for result := range results {
				var reply accessTestReply
				if entity.KnowledgeAccessDecode(result.raw, accessDefinition(operation), nil) != nil || json.Unmarshal(result.raw, &reply) != nil {
					t.Fatal("concurrent result did not match the closed response schema")
				}
				if result.status == 503 {
					if reply.Code != "unavailable" || !bytes.Equal(reply.Data, []byte("null")) {
						t.Fatal("concurrent loser returned unsafe error data")
					}
					continue
				}
				if (result.status != 200 && result.status != firstStatus) || reply.Code != "" || bytes.Equal(reply.Data, []byte("null")) {
					t.Fatal("unexpected concurrent result")
				}
				successes = append(successes, reply)
			}
			if len(successes) == 0 {
				t.Fatal("concurrent phase had no successful response")
			}
			var body map[string]any
			if json.Unmarshal([]byte(accessJSON(request)), &body) != nil {
				t.Fatal("invalid request fixture")
			}
			// These reads precede every serial replay: a later first mutation
			// cannot supply a missing concurrent winner.
			row := checkAccessCommandGapReceipt(t, operation, body["idempotency_key"].(string))
			if accessTestEpoch(t) != epoch+1 {
				t.Fatal("concurrent phase did not commit exactly one epoch step")
			}
			winner := []byte(*row.CommandResultJSON)
			for _, reply := range successes {
				if !bytes.Equal(reply.Data, winner) {
					t.Fatal("initial success did not return the stored winner")
				}
			}
			switch operation {
			case "setPolicy":
				var policy entity.KnowledgeAccessDomainPolicyRecord
				if *row.CommandResourceID != knowledgeTestDomain || dao.DB.Where("domain_id = ?", *row.CommandResourceID).Take(&policy).Error != nil || policy.Version != 1 || !policy.Enabled {
					t.Fatal("concurrent policy resource binding")
				}
			case "setModule":
				var count int64
				if err := dao.DB.Model(&entity.KnowledgeAccessModuleRecord{}).Count(&count).Error; err != nil || count != 1 {
					t.Fatal("null-ID create made multiple modules")
				}
				var module entity.KnowledgeAccessModuleAdmin
				var stored entity.KnowledgeAccessModuleRecord
				if json.Unmarshal(winner, &module) != nil || module.ModuleID != *row.CommandResourceID || dao.DB.Where("id = ?", *row.CommandResourceID).Take(&stored).Error != nil || stored.Version != 1 {
					t.Fatal("concurrent module resource binding")
				}
			case "createGrant":
				var count int64
				if err := dao.DB.Model(&entity.KnowledgeAccessGrantRecord{}).Where("create_command_id = ? AND id = ?", row.ID, *row.CommandResourceID).Count(&count).Error; err != nil || count != 1 {
					t.Fatal("grant create did not have exactly one matching command resource")
				}
				var grant entity.KnowledgeAccessGrant
				if json.Unmarshal(winner, &grant) != nil || grant.GrantID != *row.CommandResourceID {
					t.Fatal("concurrent grant result binding")
				}
			case "revokeGrant":
				var grant entity.KnowledgeAccessGrantRecord
				var revoked entity.KnowledgeAccessRevoked
				if *row.CommandResourceID != f.Grant.GrantID || dao.DB.Where("id = ?", *row.CommandResourceID).Take(&grant).Error != nil || grant.RevokedAt == nil || json.Unmarshal(winner, &revoked) != nil || revoked.ID != grant.ID || revoked.RevokedAt != accessTime(*grant.RevokedAt) {
					t.Fatal("concurrent revoke resource binding")
				}
			}
			beforeReplay := snapshotAccessCommandGap(t)
			replay := accessTestCall(t, NewDatasetService(), f.Owner, knowledgeTestOwner, operation, request, 200)
			if !bytes.Equal(replay.Data, winner) || !bytes.Equal(beforeReplay, snapshotAccessCommandGap(t)) {
				t.Fatal("serial replay changed the concurrent winner or persistent state")
			}
			for _, reply := range successes {
				if !bytes.Equal(reply.Data, replay.Data) {
					t.Fatal("initial success and serial replay differed")
				}
			}
		})
	}
}

func TestKnowledgeAccessProductionScopeRejectsCrossPairs(t *testing.T) {
	idA, idB := strings.Repeat("1", 64), strings.Repeat("2", 64)
	bindingA, bindingB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	scope := lexicalSearchScope([]lexicalSearchRowKey{{ID: idA, Binding: bindingA}, {ID: idB, Binding: bindingB}})
	engine := &accessTestEngine{lexicalTestEngine: &lexicalTestEngine{rows: map[string]map[string]any{
		"allowed-A": {"id": idA, "knowledge_binding_kwd": bindingA, "content_with_weight": "allowed-A"},
		"allowed-B": {"id": idB, "knowledge_binding_kwd": bindingB, "content_with_weight": "allowed-B"},
		"cross-A":   {"id": idA, "knowledge_binding_kwd": bindingB, "content_with_weight": "cross-pair"},
		"cross-B":   {"id": idB, "knowledge_binding_kwd": bindingA, "content_with_weight": "cross-pair"},
	}}}
	result, err := engine.Search(t.Context(), &types.SearchRequest{KnowledgeAccessStable: true, Filter: scope, MatchExprs: []any{nil}, Limit: 4})
	if err != nil || result == nil || len(result.Chunks) != 2 {
		t.Fatal("production scope permitted a binding/ID cross-product")
	}
	for _, row := range result.Chunks {
		if row["content_with_weight"] == "cross-pair" {
			t.Fatal("cross-pair reached engine result")
		}
	}
}
