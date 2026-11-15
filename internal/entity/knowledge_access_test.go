package entity

import (
	"strings"
	"testing"
)

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
