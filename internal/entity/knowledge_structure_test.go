package entity

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestKnowledgeStructureCapabilityRoundTrip(t *testing.T) {
	var capabilities KnowledgeStructureCapabilities
	if err := json.Unmarshal(KnowledgeStructureCapabilitiesJSON, &capabilities); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatal(err)
	}
	var before, after any
	if err = json.Unmarshal(KnowledgeStructureCapabilitiesJSON, &before); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("frozen capabilities changed through DTO roundtrip")
	}
}

func TestKnowledgeStructureFrozenDigests(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-structure-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Vectors map[string]struct {
			Projection any    `json:"projection"`
			SHA256     string `json:"sha256"`
		} `json:"digest_vectors"`
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures.Vectors) != 8 {
		t.Fatal("missing frozen vectors")
	}
	for name, vector := range fixtures.Vectors {
		t.Run(name, func(t *testing.T) {
			got, err := KnowledgeLifecycleDigest(vector.Projection)
			if err != nil || got != vector.SHA256 {
				t.Fatalf("got %s want %s: %v", got, vector.SHA256, err)
			}
		})
	}
	got, err := KnowledgeStructureProducerDigest([]KnowledgeStructureAlias{})
	if err != nil || got != fixtures.Vectors["producer"].SHA256 {
		t.Fatal("producer budgets differ from frozen vector", got, err)
	}
	one := []KnowledgeStructureAlias{{Prefix: "@a/", Target: "frontend/"}, {Prefix: "@b/", Target: "shared/"}}
	two := []KnowledgeStructureAlias{one[1], one[0]}
	a, _ := KnowledgeStructureProducerDigest(one)
	b, _ := KnowledgeStructureProducerDigest(two)
	if a != b || a == got {
		t.Fatal("alias identity/order failed")
	}
}
