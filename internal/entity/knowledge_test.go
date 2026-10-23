package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestKnowledgeIdentityMatchesSharedFixture(t *testing.T) {
	data, err := os.ReadFile("../../docs/knowledge-foundation-v1.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version string `json:"contract_version"`
		Inputs  struct {
			Domain    string `json:"domain_id"`
			Source    string `json:"source_id"`
			Revision  string `json:"source_revision_id"`
			Kind      string `json:"kind"`
			Key       string `json:"logical_key"`
			Structure string `json:"structure_version"`
		} `json:"identity_inputs"`
		Expected struct {
			Logical string `json:"logical_node_id"`
			Node    string `json:"node_id"`
			SHA     string `json:"source_sha256"`
			Bytes   int    `json:"byte_length"`
		} `json:"expected"`
		Text string `json:"source_text"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	i := fixture.Inputs
	logical := KnowledgeID("knowledge-logical/1", i.Domain, i.Source, i.Kind, i.Key)
	node := KnowledgeID("knowledge-node/1", logical, i.Revision, i.Structure)
	digest := sha256.Sum256([]byte(fixture.Text))
	if fixture.Version != KnowledgeContractVersion || logical != fixture.Expected.Logical || node != fixture.Expected.Node || hex.EncodeToString(digest[:]) != fixture.Expected.SHA || len(fixture.Text) != fixture.Expected.Bytes {
		t.Fatal("shared fixture and entity identity disagree")
	}
	if KnowledgeID("ab", "c") == KnowledgeID("a", "bc") {
		t.Fatal("ambiguous component boundaries")
	}
	if node == KnowledgeID("knowledge-node/1", logical, "another-revision", i.Structure) || node == KnowledgeID("knowledge-node/1", logical, i.Revision, "another-structure") {
		t.Fatal("revision or structure absent from identity")
	}
	if logical == KnowledgeID("knowledge-logical/1", "another-domain", i.Source, i.Kind, i.Key) {
		t.Fatal("domain absent from logical identity")
	}
}

func TestKnowledgeLocalPolicyHasExplicitEmptyModelUses(t *testing.T) {
	data, err := json.Marshal(KnowledgeLocalPolicy(1))
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]interface{}
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	uses, ok := policy["allowed_model_uses"].([]interface{})
	if !ok || len(uses) != 0 || policy["external_models"] != false || policy["automatic_ingestion"] != false || policy["access"] != "owner_only" {
		t.Fatalf("unsafe or ambiguous policy: %s", data)
	}
}
