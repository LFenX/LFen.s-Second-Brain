package entity

import (
	"strings"
	"testing"
)

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
