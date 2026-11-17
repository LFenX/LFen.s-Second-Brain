package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKnowledgeAnswerUsageUsesApprovedWireFields(t *testing.T) {
	usage := KnowledgeAnswerUsage{Input: 804, Output: 59, Total: 863}
	raw, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	var decoded KnowledgeAnswerUsage
	if err := KnowledgeAnswerDecode(raw, "Usage", &decoded); err != nil || decoded != usage {
		t.Fatal("usage must match the approved input/output token contract", err)
	}
	if KnowledgeAnswerDecode([]byte(`{"prompt_tokens":804,"completion_tokens":59,"total_tokens":863}`), "Usage", nil) == nil {
		t.Fatal("provider field names escaped into the public answer contract")
	}
}

func TestKnowledgeAnswerRejectsMalformedWireAndUnsupportedCitations(t *testing.T) {
	valid := `{"answer":"47","insufficient_evidence":false,"evidence_ids":["E1"]}`
	if err := KnowledgeAnswerDecode([]byte(valid), "ModelOutput", nil); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"answer":"47","answer":"999","insufficient_evidence":false,"evidence_ids":["E1"]}`,
		`{"answer":"47","insufficient_evidence":false,"evidence_ids":[]}`,
		`{"answer":"unknown","insufficient_evidence":true,"evidence_ids":["E1"]}`,
		`{"answer":"47","insufficient_evidence":false,"evidence_ids":["E1","E1"]}`,
		`{"answer":"47","insufficient_evidence":false,"evidence_ids":["E6"]}`,
		`{"answer":"47","insufficient_evidence":false,"evidence_ids":["E1"],"api_key":"synthetic"}`,
		`{"answer":"\ud800","insufficient_evidence":false,"evidence_ids":["E1"]}`,
		strings.Replace(valid, "47", strings.Repeat("中", 2731), 1),
		valid + `{}`,
	} {
		if KnowledgeAnswerDecode([]byte(raw), "ModelOutput", nil) == nil {
			t.Fatalf("accepted malformed model output: length=%d", len(raw))
		}
	}
	if KnowledgeAnswerDecode([]byte(`{"answer":"资料不足","insufficient_evidence":true,"evidence_ids":[]}`), "ModelOutput", nil) != nil {
		t.Fatal("refusal must be valid")
	}
	for _, raw := range []string{
		`{"contract_version":"0.1.0-candidate","expected_version":0,"enabled":true,"scope":null,"model_ref":null,"confirmed":true,"external_send_confirmed":true,"idempotency_key":"enable"}`,
		`{"contract_version":"0.1.0-candidate","expected_version":0,"enabled":false,"scope":null,"model_ref":null,"confirmed":true,"external_send_confirmed":true,"idempotency_key":"disable"}`,
		`{"contract_version":"0.1.0-candidate","expected_version":9007199254740992,"enabled":false,"scope":null,"model_ref":null,"confirmed":true,"external_send_confirmed":false,"idempotency_key":"disable"}`,
	} {
		if KnowledgeAnswerDecode([]byte(raw), "PolicySetRequest", nil) == nil {
			t.Fatal("accepted invalid consent/version")
		}
	}
}
