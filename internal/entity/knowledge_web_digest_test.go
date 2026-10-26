package entity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestKnowledgeWebFrozenDigestVectors(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-web-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Digest []struct {
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Input  any    `json:"input"`
			SHA256 string `json:"sha256"`
		} `json:"digest_vectors"`
		Typed []struct {
			Name   string `json:"name"`
			Input  any    `json:"input"`
			Hex    string `json:"encoded_hex"`
			SHA256 string `json:"sha256"`
		} `json:"typed_encoding_vectors"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Digest) < 13 || len(vectors.Typed) < 10 {
		t.Fatal("frozen vectors missing")
	}
	for _, vector := range vectors.Digest {
		t.Run(vector.Name, func(t *testing.T) {
			got, err := KnowledgeWebDigest(vector.Kind, vector.Input)
			if err != nil || got != vector.SHA256 {
				t.Fatalf("digest=%s expected=%s err=%v", got, vector.SHA256, err)
			}
		})
	}
	for _, vector := range vectors.Typed {
		t.Run(vector.Name, func(t *testing.T) {
			var encoded bytes.Buffer
			if err := webEncodeValue(&encoded, vector.Input, 0); err != nil {
				t.Fatal(err)
			}
			got := sha256.Sum256(encoded.Bytes())
			if hex.EncodeToString(encoded.Bytes()) != vector.Hex || hex.EncodeToString(got[:]) != vector.SHA256 {
				t.Fatal("typed encoding differs from frozen independent vector")
			}
		})
	}
}

func TestKnowledgeWebFrozenProjectionRoundTrips(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-web-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples struct {
		Samples map[string]json.RawMessage `json:"samples"`
	}
	if err := json.Unmarshal(raw, &samples); err != nil {
		t.Fatal(err)
	}
	projections := map[string]any{
		"registration_request": &KnowledgeWebCreateRegistration{}, "registration": &KnowledgeWebRegistration{}, "target": &KnowledgeWebTarget{},
		"local_capture_request": &KnowledgeWebCreateLocalCapture{}, "fetch_capture_request": &KnowledgeWebCreateFetchCapture{}, "local_capture_ready": &KnowledgeWebCapture{}, "fetch_capture_ready": &KnowledgeWebCapture{},
		"acquire": &KnowledgeWebAcquire{}, "observe": &KnowledgeWebObserve{}, "observation": &KnowledgeWebObservation{}, "entry": &KnowledgeWebEntry{}, "reference": &KnowledgeWebReference{},
		"preview_request": &KnowledgeWebCreatePreview{}, "preview": &KnowledgeWebPreview{}, "preview_item": &KnowledgeWebPreviewItem{}, "confirm": &KnowledgeWebConfirm{}, "receipt": &KnowledgeWebReceipt{}, "receipt_item": &KnowledgeWebReceiptItem{}, "web_revision": &KnowledgeWebRevision{}, "window": &KnowledgeWebWindow{}, "capabilities": &KnowledgeWebCapabilities{},
	}
	for name, target := range projections {
		t.Run(name, func(t *testing.T) {
			sample, ok := samples.Samples[name]
			if !ok {
				t.Fatal("frozen sample missing")
			}
			decoder := json.NewDecoder(bytes.NewReader(sample))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(target); err != nil {
				t.Fatal(err)
			}
			actual, err := json.Marshal(target)
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if json.Unmarshal(sample, &a) != nil || json.Unmarshal(actual, &b) != nil || !reflect.DeepEqual(a, b) {
				t.Fatal("projection lost a field, null, scalar type, or ordered value")
			}
		})
	}
}

func TestKnowledgeWebCapabilitiesMatchFrozenLimits(t *testing.T) {
	raw, err := os.ReadFile("../../docs/knowledge-web-import-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples struct {
		Samples struct {
			Capabilities json.RawMessage `json:"capabilities"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	var actual, expected any
	if json.Unmarshal(KnowledgeWebCapabilitiesJSON, &actual) != nil || json.Unmarshal(examples.Samples.Capabilities, &expected) != nil {
		t.Fatal("invalid capability JSON")
	}
	// runtime_ready is a boolean, unlike the frozen constant limits. The
	// candidate sample was false before implementation; this runtime is ready.
	if actual.(map[string]any)["runtime_ready"] != true {
		t.Fatal("implemented runtime not ready")
	}
	expected.(map[string]any)["runtime_ready"] = true
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("runtime capabilities differ from frozen producer versions and limits")
	}
}

func TestKnowledgeWebDigestRejectsInvalidProjection(t *testing.T) {
	for _, test := range []struct {
		kind  string
		value any
	}{
		{"unknown", map[string]any{}}, {"entry_key", nil}, {"entry_key", map[string]any{"identity_basis": "page"}},
		{"entry_key", map[string]any{"identity_basis": "page", "identity_value": 1.2}},
		{"entry_key", map[string]any{"identity_basis": "page", "identity_value": json.Number("9007199254740992")}},
	} {
		if _, err := KnowledgeWebDigest(test.kind, test.value); err == nil {
			t.Fatal("invalid projection accepted", test.kind)
		}
	}
}
