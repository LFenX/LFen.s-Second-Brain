package entity

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestKnowledgeLifecycleFrozenTypedLPVectors(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-source-lifecycle-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Vectors []struct {
			Name   string          `json:"name"`
			Value  json.RawMessage `json:"value"`
			SHA256 string          `json:"sha256"`
		} `json:"digest_vectors"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Vectors) != 14 {
		t.Fatal("vector count changed")
	}
	for _, vector := range data.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			decoder := json.NewDecoder(bytes.NewReader(vector.Value))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			digest, err := KnowledgeLifecycleDigest(value)
			if err != nil || digest != vector.SHA256 {
				t.Fatalf("digest=%s err=%v want=%s", digest, err, vector.SHA256)
			}
		})
	}
}
