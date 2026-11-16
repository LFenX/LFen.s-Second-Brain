package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKnowledgeAccessCredentialClosedWireAndOneTimeDelivery(t *testing.T) {
	valid := `{"contract_version":"0.3.0-candidate","view_id":"11111111111111111111111111111111","mode":"read","ttl_seconds":null,"client_label":"Synthetic Client","confirmed":true,"idempotency_key":"Synthetic_Credential_Key"}`
	for _, test := range []struct {
		name, raw string
		valid     bool
	}{
		{"default TTL", valid, true},
		{"minimum TTL", strings.Replace(valid, `"ttl_seconds":null`, `"ttl_seconds":60`, 1), true},
		{"maximum TTL", strings.Replace(valid, `"ttl_seconds":null`, `"ttl_seconds":28800`, 1), true},
		{"missing TTL", strings.Replace(valid, `"ttl_seconds":null,`, "", 1), false},
		{"short TTL", strings.Replace(valid, `"ttl_seconds":null`, `"ttl_seconds":59`, 1), false},
		{"long TTL", strings.Replace(valid, `"ttl_seconds":null`, `"ttl_seconds":28801`, 1), false},
		{"fraction TTL", strings.Replace(valid, `"ttl_seconds":null`, `"ttl_seconds":60.1`, 1), false},
		{"missing consent", strings.Replace(valid, `"confirmed":true`, `"confirmed":false`, 1), false},
		{"claimed subject", strings.Replace(valid, `"confirmed":true`, `"confirmed":true,"user_id":"22222222222222222222222222222222"`, 1), false},
		{"duplicate", strings.Replace(valid, `"mode":"read"`, `"mode":"read","mode":"read_search"`, 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if (KnowledgeAccessDecode([]byte(test.raw), "CreateCredentialRequest", nil) == nil) != test.valid {
				t.Fatal("credential request boundary failed")
			}
		})
	}
	id := strings.Repeat("a", 32)
	credential := KnowledgeAccessCredential{CredentialID: id, DomainID: id, ModuleID: id, ModuleVersion: 1, Mode: "read", ClientLabel: "Synthetic Client", IssuedAt: "2030-01-01T00:00:00Z", ExpiresAt: "2030-01-01T01:00:00Z"}
	token := "kac1_" + id + "." + strings.Repeat("a", 43)
	for _, test := range []struct {
		delivery string
		token    *string
		valid    bool
	}{{"issued_once", &token, true}, {"issued_once", nil, false}, {"unavailable_after_first_response", nil, true}, {"unavailable_after_first_response", &token, false}} {
		raw, _ := json.Marshal(KnowledgeAccessCredentialMintData{Credential: credential, Token: test.token, Delivery: test.delivery})
		if (KnowledgeAccessDecode(raw, "CredentialMintData", nil) == nil) != test.valid {
			t.Fatal("one-time delivery shape failed")
		}
	}
	raw, _ := json.Marshal(credential)
	unknown := strings.TrimSuffix(string(raw), "}") + `,"verifier_sha256":"private"}`
	if KnowledgeAccessDecode([]byte(unknown), "Credential", nil) == nil {
		t.Fatal("private verifier field entered public metadata")
	}
}

func TestKnowledgeAccessClosedRequests(t *testing.T) {
	valid := `{"contract_version":"0.3.0-candidate","domain_id":"11111111111111111111111111111111","expected_version":0,"enabled":true,"confirmed":true,"idempotency_key":"Synthetic_Key_0001"}`
	for _, test := range []struct {
		name, raw string
		valid     bool
	}{
		{"valid", valid, true},
		{"duplicate", strings.Replace(valid, `"enabled":true`, `"enabled":true,"enabled":false`, 1), false},
		{"unknown", strings.Replace(valid, `"enabled":true`, `"enabled":true,"tenant_id":"22222222222222222222222222222222"`, 1), false},
		{"missing", strings.Replace(valid, `"confirmed":true,`, "", 1), false},
		{"null", strings.Replace(valid, `"enabled":true`, `"enabled":null`, 1), false},
		{"unsafe", strings.Replace(valid, `"expected_version":0`, `"expected_version":9007199254740992`, 1), false},
		{"fraction", strings.Replace(valid, `"expected_version":0`, `"expected_version":0.0000000000000000000001`, 1), false},
		{"surrogate", strings.Replace(valid, `Synthetic_Key_0001`, `Synthetic_Key_\ud800`, 1), false},
		{"false consent", strings.Replace(valid, `"confirmed":true`, `"confirmed":false`, 1), false},
		{"trailing", valid + "{}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var req KnowledgeAccessSetPolicyRequest
			err := KnowledgeAccessDecode([]byte(test.raw), "SetPolicyRequest", &req)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
		})
	}
}

func TestKnowledgeAccessNullableRequiredAndReadUnion(t *testing.T) {
	valid := `{"contract_version":"0.3.0-candidate","tenant_id":"11111111111111111111111111111111","domain_id":null}`
	var req KnowledgeAccessListModulesRequest
	if err := KnowledgeAccessDecode([]byte(valid), "ListModulesRequest", &req); err != nil || req.DomainID != nil {
		t.Fatalf("explicit null: %v", err)
	}
	if KnowledgeAccessDecode([]byte(strings.Replace(valid, `,"domain_id":null`, "", 1)), "ListModulesRequest", &req) == nil {
		t.Fatal("missing nullable key accepted")
	}
	unit := `{"contract_version":"0.3.0-candidate","selector":{"kind":"unit","view_id":"11111111111111111111111111111111","target_index":0,"unit_ordinal":0}}`
	if err := KnowledgeAccessDecode([]byte(unit), "ReadRequest", nil); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(unit, `"target_index":0`, `"target_index":32`, 1), strings.Replace(unit, `"unit_ordinal":0`, `"unit_ordinal":-1`, 1), strings.Replace(unit, `"kind":"unit"`, `"kind":"unit","evidence_ref":null`, 1)} {
		if KnowledgeAccessDecode([]byte(bad), "ReadRequest", nil) == nil {
			t.Fatal("invalid selector accepted")
		}
	}
}
