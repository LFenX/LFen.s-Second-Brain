//go:build linux

package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Credential failures intentionally never print a response or secret value.
func credentialTestCall(t *testing.T, f *accessTestFixture, ctx context.Context, actor, operation string, request any, want int) accessTestReply {
	t.Helper()
	status, raw := f.Service.KnowledgeAccess(ctx, actor, operation, common.GenerateUUID(), []byte(accessJSON(request)))
	var reply accessTestReply
	if status != want || json.Unmarshal(raw, &reply) != nil || entity.KnowledgeAccessDecode(raw, accessDefinition(operation), nil) != nil {
		t.Fatalf("credential operation failed: %s status=%d expected=%d", operation, status, want)
	}
	return reply
}

func credentialTestData[T any](t *testing.T, reply accessTestReply) T {
	t.Helper()
	var value T
	if json.Unmarshal(reply.Data, &value) != nil {
		t.Fatal("invalid credential operation data")
	}
	return value
}

func credentialTestFixture(t *testing.T) (*accessTestFixture, entity.KnowledgeAccessInspectData) {
	t.Helper()
	t.Setenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE", "http://localhost:29398/api/v1/knowledge/access/mcp")
	contentLexicalTokenizer(t)
	f := accessTestSetup(t, false)
	f.enable(t, f.Scope.Targets[:1], "read_search")
	view := credentialTestData[entity.KnowledgeAccessInspectData](t, credentialTestCall(t, f, f.Recipient, accessTestRecipient, "inspect", entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: "read_search", Delivery: "web_and_mcp"}, 200))
	return f, view
}

func credentialTestRequest(view string, key string) entity.KnowledgeAccessCreateCredentialRequest {
	return entity.KnowledgeAccessCreateCredentialRequest{ContractVersion: entity.KnowledgeAccessVersion, ViewID: view, Mode: "read", TTLSeconds: nil, ClientLabel: "Synthetic managed client", Confirmed: true, IdempotencyKey: key}
}

func credentialTestMint(t *testing.T, f *accessTestFixture, view string, key string) entity.KnowledgeAccessCredentialMintData {
	t.Helper()
	return credentialTestData[entity.KnowledgeAccessCredentialMintData](t, credentialTestCall(t, f, f.Recipient, accessTestRecipient, "createCredential", credentialTestRequest(view, key), 201))
}

func credentialTestAuth(t *testing.T, f *accessTestFixture, token string) (context.Context, string) {
	t.Helper()
	requestID := common.GenerateUUID()
	ctx, user, _, err := f.Service.AuthenticateKnowledgeAccessMCP(t.Context(), "Bearer "+token, requestID)
	if err != nil || ctx == nil || user != accessTestRecipient {
		t.Fatal("MCP credential authentication failed")
	}
	return ctx, requestID
}

func credentialTestConsume[T any](t *testing.T, f *accessTestFixture, token, operation string, request any, want int) T {
	t.Helper()
	ctx, requestID := credentialTestAuth(t, f, token)
	status, raw := f.Service.KnowledgeAccess(ctx, accessTestRecipient, operation, requestID, []byte(accessJSON(request)))
	var reply accessTestReply
	if status != want || json.Unmarshal(raw, &reply) != nil || entity.KnowledgeAccessDecode(raw, accessDefinition(operation), nil) != nil {
		t.Fatalf("scoped MCP operation failed: %s status=%d expected=%d", operation, status, want)
	}
	if f.Service.FinalizeKnowledgeAccessMCP(ctx) != nil {
		t.Fatal("MCP final authentication failed")
	}
	return credentialTestData[T](t, reply)
}

func credentialTestInspect(f *accessTestFixture, mode string) entity.KnowledgeAccessInspectRequest {
	return entity.KnowledgeAccessInspectRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, Mode: mode, Delivery: "web_and_mcp"}
}

func TestKnowledgeAccessCredentialMintPersistsOnlyVerifierAndSafeReceipt(t *testing.T) {
	f, view := credentialTestFixture(t)
	epoch := accessTestEpoch(t)
	minted := credentialTestMint(t, f, view.ViewID, "Synthetic_Credential_0001")
	if minted.Token == nil || len(*minted.Token) != 81 || minted.Delivery != "issued_once" || minted.Credential.Mode != "read" || minted.Credential.RevokedAt != nil || minted.Credential.LastUsedAt != nil || accessTestEpoch(t) != epoch {
		t.Fatal("invalid first credential delivery")
	}
	var row entity.KnowledgeAccessCredentialRecord
	if dao.DB.Where("id = ?", minted.Credential.CredentialID).Take(&row).Error != nil || len(row.VerifierSHA256) != 32 {
		t.Fatal("missing bounded credential verifier")
	}
	parts := strings.Split(*minted.Token, ".")
	secret, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	defer clear(secret)
	if err != nil || len(secret) != 32 {
		t.Fatal("invalid random credential secret")
	}
	hash := sha256.New()
	for _, part := range [][]byte{[]byte("knowledge-access-credential/1"), []byte(row.ID), secret} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		hash.Write(length[:])
		hash.Write(part)
	}
	if string(hash.Sum(nil)) != string(row.VerifierSHA256) || !row.ExpiresAt.After(mustCredentialTime(t, view.ExpiresAt)) || row.ExpiresAt.After(mustCredentialTime(t, f.Grant.ExpiresAt)) {
		t.Fatal("verifier or independent TTL incorrect")
	}
	var receipt entity.KnowledgeAccessAuditRecord
	if dao.DB.Where("id = ?", row.CreateCommandID).Take(&receipt).Error != nil || receipt.CommandResultJSON == nil || receipt.AfterDefinitionJSON == nil || receipt.CredentialID == nil || *receipt.CredentialID != row.ID {
		t.Fatal("credential receipt missing")
	}
	for _, text := range []string{row.CaptureJSON, *receipt.CommandResultJSON, *receipt.AfterDefinitionJSON, accessJSON(row)} {
		if strings.Contains(text, *minted.Token) || strings.Contains(text, parts[1]) || strings.Contains(text, base64.StdEncoding.EncodeToString(row.VerifierSHA256)) || strings.Contains(text, "合成 A 固定原文") {
			t.Fatal("credential or source material persisted in safe metadata")
		}
	}
	var safe entity.KnowledgeAccessCredentialMintData
	if json.Unmarshal([]byte(*receipt.CommandResultJSON), &safe) != nil || safe.Token != nil || safe.Delivery != "unavailable_after_first_response" {
		t.Fatal("journal retained first secret delivery")
	}
}

func mustCredentialTime(t *testing.T, value string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal("invalid credential timestamp")
	}
	return instant
}

func TestKnowledgeAccessCredentialReplaySurvivesViewCollectionAndGrantLoss(t *testing.T) {
	f, view := credentialTestFixture(t)
	request := credentialTestRequest(view.ViewID, "Synthetic_Credential_Replay01")
	first := credentialTestData[entity.KnowledgeAccessCredentialMintData](t, credentialTestCall(t, f, f.Recipient, accessTestRecipient, "createCredential", request, 201))
	epoch := accessTestEpoch(t)
	if dao.DB.Where("id = ?", view.ViewID).Delete(&entity.KnowledgeAccessViewRecord{}).Error != nil {
		t.Fatal("synthetic view collection failed")
	}
	replay := credentialTestData[entity.KnowledgeAccessCredentialMintData](t, credentialTestCall(t, f, f.Recipient, accessTestRecipient, "createCredential", request, 200))
	if replay.Token != nil || replay.Delivery != "unavailable_after_first_response" || accessJSON(replay.Credential) != accessJSON(first.Credential) || accessTestEpoch(t) != epoch {
		t.Fatal("mint replay changed metadata, secret or epoch")
	}
	changed := request
	changed.ClientLabel = "Changed synthetic client"
	if reply := credentialTestCall(t, f, f.Recipient, accessTestRecipient, "createCredential", changed, 409); reply.Code != "idempotency_conflict" {
		t.Fatal("changed-body replay not rejected")
	}
	credentialTestCall(t, f, f.Owner, knowledgeTestOwner, "revokeGrant", entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: "Synthetic_Revoke_Parent01"}, 200)
	if dao.DB.Where("user_id = ? AND tenant_id = ?", accessTestRecipient, knowledgeTestOwner).Delete(&entity.UserTenant{}).Error != nil {
		t.Fatal("synthetic membership removal failed")
	}
	listed := credentialTestData[entity.KnowledgeAccessListCredentialsData](t, credentialTestCall(t, f, f.Recipient, accessTestRecipient, "listCredentials", entity.KnowledgeAccessListCredentialsRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, IdempotencyKey: &request.IdempotencyKey}, 200))
	if len(listed.Items) != 1 || listed.Items[0].CredentialID != first.Credential.CredentialID {
		t.Fatal("self metadata unavailable after content authority loss")
	}
	revoke := entity.KnowledgeAccessRevokeCredentialRequest{ContractVersion: entity.KnowledgeAccessVersion, CredentialID: first.Credential.CredentialID, Confirmed: true, IdempotencyKey: "Synthetic_Revoke_Credential01"}
	epoch = accessTestEpoch(t)
	credentialTestCall(t, f, f.Recipient, accessTestRecipient, "revokeCredential", revoke, 200)
	credentialTestCall(t, f, f.Recipient, accessTestRecipient, "revokeCredential", revoke, 200)
	if accessTestEpoch(t) != epoch {
		t.Fatal("self credential revoke bumped tenant epoch")
	}
}

func TestKnowledgeAccessCredentialBindsViewsAndRevokesOneBearerOnly(t *testing.T) {
	f, view := credentialTestFixture(t)
	first := credentialTestMint(t, f, view.ViewID, "Synthetic_Credential_Binding01")
	second := credentialTestMint(t, f, view.ViewID, "Synthetic_Credential_Binding02")
	one := credentialTestConsume[entity.KnowledgeAccessInspectData](t, f, *first.Token, "inspect", credentialTestInspect(f, "read"), 200)
	two := credentialTestConsume[entity.KnowledgeAccessInspectData](t, f, *second.Token, "inspect", credentialTestInspect(f, "read"), 200)
	credentialTestConsume[any](t, f, *second.Token, "read", accessTestReadRequest(one.ViewID, 0, 0), 404)
	credentialTestCall(t, f, f.Recipient, accessTestRecipient, "read", accessTestReadRequest(one.ViewID, 0, 0), 404)
	epoch := accessTestEpoch(t)
	credentialTestCall(t, f, f.Recipient, accessTestRecipient, "revokeCredential", entity.KnowledgeAccessRevokeCredentialRequest{ContractVersion: entity.KnowledgeAccessVersion, CredentialID: first.Credential.CredentialID, Confirmed: true, IdempotencyKey: "Synthetic_Revoke_Binding01"}, 200)
	if _, _, _, err := f.Service.AuthenticateKnowledgeAccessMCP(t.Context(), "Bearer "+*first.Token, common.GenerateUUID()); err == nil {
		t.Fatal("revoked bearer authenticated")
	}
	read := credentialTestConsume[entity.KnowledgeAccessReadData](t, f, *second.Token, "read", accessTestReadRequest(two.ViewID, 0, 0), 200)
	if read.Text == "" || accessTestEpoch(t) != epoch {
		t.Fatal("another credential was revoked or lost its view")
	}
}

func TestKnowledgeAccessCredentialNeverBorrowsNativeSessionOrOwnerToken(t *testing.T) {
	f, view := credentialTestFixture(t)
	minted := credentialTestMint(t, f, view.ViewID, "Synthetic_Credential_Native01")
	if dao.DB.Model(&entity.User{}).Where("id = ?", accessTestRecipient).Update("access_token", "synthetic-logged-out-session").Error != nil {
		t.Fatal("synthetic logout failed")
	}
	const callback = "synthetic:reject-native-secret-query"
	if err := dao.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user" && (len(tx.Statement.Selects) == 0 || strings.Contains(strings.Join(tx.Statement.Selects, ","), "access_token")) {
			tx.AddError(errors.New("native secret query refused"))
		}
	}); err != nil {
		t.Fatal("failed to install nonsecret query guard")
	}
	t.Cleanup(func() { dao.DB.Callback().Query().Remove(callback) })
	scoped := credentialTestConsume[entity.KnowledgeAccessInspectData](t, f, *minted.Token, "inspect", credentialTestInspect(f, "read"), 200)
	credentialTestConsume[entity.KnowledgeAccessReadData](t, f, *minted.Token, "read", accessTestReadRequest(scoped.ViewID, 0, 0), 200)
	ctx, requestID := credentialTestAuth(t, f, *minted.Token)
	if status, _ := f.Service.KnowledgeAccess(ctx, accessTestRecipient, "createCredential", requestID, []byte(accessJSON(credentialTestRequest(view.ViewID, "Synthetic_Forbidden_Delegation01")))); status != 403 {
		t.Fatal("restricted bearer delegated itself")
	}
}

func TestKnowledgeAccessCredentialViewExpiryIsBusinessFailureAndFinalRevokeWins(t *testing.T) {
	f, view := credentialTestFixture(t)
	minted := credentialTestMint(t, f, view.ViewID, "Synthetic_Credential_Final01")
	scoped := credentialTestConsume[entity.KnowledgeAccessInspectData](t, f, *minted.Token, "inspect", credentialTestInspect(f, "read"), 200)
	if dao.DB.Model(&entity.KnowledgeAccessViewRecord{}).Where("id = ?", scoped.ViewID).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error != nil {
		t.Fatal("synthetic view expiry failed")
	}
	credentialTestConsume[any](t, f, *minted.Token, "read", accessTestReadRequest(scoped.ViewID, 0, 0), 409)
	scoped = credentialTestConsume[entity.KnowledgeAccessInspectData](t, f, *minted.Token, "inspect", credentialTestInspect(f, "read"), 200)
	ctx, requestID := credentialTestAuth(t, f, *minted.Token)
	verified, err := f.Service.accessOpenView(ctx, accessTestRecipient, scoped.ViewID, "read", requestID)
	if err != nil {
		t.Fatal("fresh credential view unavailable")
	}
	credentialTestCall(t, f, f.Recipient, accessTestRecipient, "revokeCredential", entity.KnowledgeAccessRevokeCredentialRequest{ContractVersion: entity.KnowledgeAccessVersion, CredentialID: minted.Credential.CredentialID, Confirmed: true, IdempotencyKey: "Synthetic_Revoke_Final01"}, 200)
	if f.Service.accessFinalPermit(ctx, verified, "read", requestID) == nil || f.Service.FinalizeKnowledgeAccessMCP(ctx) == nil {
		t.Fatal("revocation lost to a previously opened view")
	}
}

func TestKnowledgeAccessCredentialMintGuardsAndRateLimit(t *testing.T) {
	f, view := credentialTestFixture(t)
	web := f.inspect(t, "read", nil)
	credentialTestCall(t, f, f.Recipient, accessTestRecipient, "createCredential", credentialTestRequest(web.ViewID, "Synthetic_WebOnly_Mint01"), 403)
	other := accessTestSession(t, accessTestOther)
	credentialTestCall(t, f, other, accessTestOther, "createCredential", credentialTestRequest(view.ViewID, "Synthetic_Other_View01"), 404)
	for i, key := range []string{"Synthetic_Mint_Rate01", "Synthetic_Mint_Rate02", "Synthetic_Mint_Rate03", "Synthetic_Mint_Rate04", "Synthetic_Mint_Rate05", "Synthetic_Mint_Rate06"} {
		want := 201
		if i == 5 {
			want = 429
		}
		credentialTestCall(t, f, f.Recipient, accessTestRecipient, "createCredential", credentialTestRequest(view.ViewID, key), want)
	}
	var count int64
	if dao.DB.Model(&entity.KnowledgeAccessCredentialRecord{}).Count(&count).Error != nil || count != 5 {
		t.Fatal("mint rate limit created extra credential")
	}
}

func TestKnowledgeAccessCredentialRejectsMalformedSecretAudienceAndScopeWidening(t *testing.T) {
	f, view := credentialTestFixture(t)
	minted := credentialTestMint(t, f, view.ViewID, "Synthetic_Credential_Reject01")
	for _, authorization := range []string{"", "Bearer native-session", *minted.Token, "Bearer " + *minted.Token + " ", "Bearer\t" + *minted.Token, "Bearer kac1_" + strings.Repeat("f", 32) + "." + strings.Repeat("A", 43)} {
		_, _, _, err := f.Service.AuthenticateKnowledgeAccessMCP(t.Context(), authorization, common.GenerateUUID())
		var public *KnowledgeAccessError
		if !errors.As(err, &public) || public.Status != 401 || public.Code != "unauthenticated" || strings.Contains(err.Error(), *minted.Token) {
			t.Fatal("malformed or unknown bearer not uniformly refused")
		}
	}
	request := credentialTestInspect(f, "read")
	request.Delivery = "web_only"
	credentialTestConsume[any](t, f, *minted.Token, "inspect", request, 400)
	request.Delivery = "web_and_mcp"
	ids := []string{f.Scope.Targets[1].Source.SourceID}
	request.SourceIDs = &ids
	credentialTestConsume[any](t, f, *minted.Token, "inspect", request, 404)
	credentialTestConsume[any](t, f, *minted.Token, "inspect", credentialTestInspect(f, "read_search"), 403)
	t.Setenv("KNOWLEDGE_ACCESS_MCP_AUDIENCE", "https://other.example.invalid/api/v1/knowledge/access/mcp")
	if _, _, _, err := f.Service.AuthenticateKnowledgeAccessMCP(t.Context(), "Bearer "+*minted.Token, common.GenerateUUID()); err == nil {
		t.Fatal("credential used at a different service audience")
	}
}

func TestKnowledgeAccessCredentialFinalWriteExpiryRollsBack(t *testing.T) {
	for _, table := range []string{"knowledge_access_credential", "knowledge_access_audit"} {
		t.Run(table, func(t *testing.T) {
			f, _ := credentialTestFixture(t)
			credentialTestCall(t, f, f.Owner, knowledgeTestOwner, "revokeGrant", entity.KnowledgeAccessRevokeGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, GrantID: f.Grant.GrantID, Confirmed: true, IdempotencyKey: "Synthetic_FinalExpiry_Revoke"}, 200)
			f.Grant = credentialTestData[entity.KnowledgeAccessGrant](t, credentialTestCall(t, f, f.Owner, knowledgeTestOwner, "createGrant", entity.KnowledgeAccessCreateGrantRequest{ContractVersion: entity.KnowledgeAccessVersion, DomainID: knowledgeTestDomain, ModuleID: f.Module.ModuleID, ExpectedModuleVersion: f.Module.Version, Subject: entity.KnowledgeAccessSubject{Kind: "user", ID: accessTestRecipient}, Mode: "read", Delivery: "web_and_mcp", ExpiresAt: accessTime(time.Now().Add(3 * time.Second)), Confirmed: true, IdempotencyKey: "Synthetic_FinalExpiry_ShortGrant"}, 201))
			view := credentialTestData[entity.KnowledgeAccessInspectData](t, credentialTestCall(t, f, f.Recipient, accessTestRecipient, "inspect", credentialTestInspect(f, "read"), 200))
			minted := credentialTestMint(t, f, view.ViewID, "Synthetic_FinalExpiry_Mint")
			expiry := mustCredentialTime(t, minted.Credential.ExpiresAt)
			if expiry.After(time.Now().Add(3*time.Second)) || !expiry.Equal(mustCredentialTime(t, f.Grant.ExpiresAt)) {
				t.Fatal("short parent grant did not cap credential expiry")
			}
			ctx, _ := credentialTestAuth(t, f, *minted.Token)
			const callback = "synthetic:credential-final-write-expiry"
			delayed := false
			if err := dao.DB.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table && !delayed {
					delayed = true
					if wait := time.Until(expiry.Add(20 * time.Millisecond)); wait > 0 {
						time.Sleep(wait)
					}
				}
			}); err != nil {
				t.Fatal("final write delay installation failed")
			}
			t.Cleanup(func() { dao.DB.Callback().Update().Remove(callback) })
			err := f.Service.FinalizeKnowledgeAccessMCP(ctx)
			var public *KnowledgeAccessError
			if !delayed || !errors.As(err, &public) || public.Status != 401 || public.Code != "unauthenticated" {
				t.Fatal("credential expiry during final durable writes did not deny release")
			}
			var credential entity.KnowledgeAccessCredentialRecord
			var audit entity.KnowledgeAccessAuditRecord
			identity := accessCredential(ctx)
			if dao.DB.Where("id = ?", minted.Credential.CredentialID).Take(&credential).Error != nil || credential.LastUsedAt != nil || dao.DB.Where("id = ?", identity.ReservationID).Take(&audit).Error != nil || audit.Outcome != "denied" || audit.Action != "mcpTransport" {
				t.Fatal("expired final permit left committed use or allowed audit")
			}
		})
	}
}
