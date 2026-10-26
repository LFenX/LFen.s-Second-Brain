package entity

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func knowledgeDocumentFrozenExamples(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../../docs/knowledge-document-structure-v1.examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples map[string]json.RawMessage
	if err := json.Unmarshal(raw, &examples); err != nil {
		t.Fatal(err)
	}
	return examples
}

func knowledgeDocumentTestObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func knowledgeDocumentTestString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func knowledgeDocumentTestDecode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func knowledgeDocumentTestRequest(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	object := knowledgeDocumentTestObject(t, raw)
	object["idempotency_key"] = json.RawMessage("\"vector_key\"")
	result, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestKnowledgeDocumentFrozenDigestVectors(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	vectors := knowledgeDocumentTestObject(t, examples["digest_vectors"])
	if len(vectors) != 28 {
		t.Fatalf("frozen vector count=%d, want 28", len(vectors))
	}
	for name, raw := range vectors {
		t.Run(name, func(t *testing.T) {
			vector := knowledgeDocumentTestObject(t, raw)
			projection, err := knowledgeDocumentJSON(vector["projection"])
			if err != nil {
				t.Fatal(err)
			}
			got, err := KnowledgeLifecycleDigest(projection)
			if err != nil {
				t.Fatal(err)
			}
			want := knowledgeDocumentTestString(t, vector["sha256"])
			if got != want {
				t.Fatalf("typed-LP digest=%s, want %s", got, want)
			}
		})
	}
}

func TestKnowledgeDocumentFrozenDigestHelpers(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	vectors := knowledgeDocumentTestObject(t, examples["digest_vectors"])
	fixtures := knowledgeDocumentTestObject(t, examples["fixtures"])
	trees := knowledgeDocumentTestObject(t, fixtures["trees"])
	for name, raw := range vectors {
		t.Run(name, func(t *testing.T) {
			vector := knowledgeDocumentTestObject(t, raw)
			projection := knowledgeDocumentTestObject(t, vector["projection"])
			kind := ""
			if projection["kind"] != nil {
				kind = knowledgeDocumentTestString(t, projection["kind"])
			}
			var got string
			var err error
			switch kind {
			case "knowledge-document-configuration/0.1.0":
				got, err = KnowledgeDocumentConfigurationDigest(knowledgeDocumentTestDecode[KnowledgeDocumentConfiguration](t, projection["configuration"]))
			case "knowledge-document-producer/0.1.0":
				got, err = KnowledgeDocumentProducerDigest(knowledgeDocumentTestDecode[KnowledgeDocumentProducer](t, projection["producer"]))
			case "knowledge-document-task/0.1.0":
				got, err = KnowledgeDocumentTaskDigest(
					knowledgeDocumentTestString(t, projection["owner_id"]), knowledgeDocumentTestString(t, projection["domain_id"]),
					knowledgeDocumentTestDecode[KnowledgeDocumentTaskRequest](t, knowledgeDocumentTestRequest(t, projection["request"])))
			case "knowledge-document-policy/0.1.0":
				got, err = KnowledgeDocumentPolicyDigest(
					knowledgeDocumentTestString(t, projection["owner_id"]), knowledgeDocumentTestString(t, projection["domain_id"]),
					knowledgeDocumentTestDecode[KnowledgeDocumentPolicyChangeRequest](t, knowledgeDocumentTestRequest(t, projection["request"])))
			case "knowledge-document-cancel/0.1.0":
				got, err = KnowledgeDocumentCancelDigest(
					knowledgeDocumentTestString(t, projection["owner_id"]), knowledgeDocumentTestString(t, projection["domain_id"]),
					knowledgeDocumentTestString(t, projection["task_id"]),
					knowledgeDocumentTestDecode[KnowledgeDocumentCancelRequest](t, knowledgeDocumentTestRequest(t, projection["request"])))
			case "knowledge-document-node-payload/0.1.0":
				got, err = KnowledgeDocumentPayloadDigest(knowledgeDocumentTestDecode[KnowledgeDocumentNodePayload](t, projection["payload"]))
			case "knowledge-document-structure-manifest/0.1.0":
				got, err = KnowledgeDocumentManifestDigest(knowledgeDocumentTestDecode[[]KnowledgeDocumentManifestEntry](t, projection["nodes"]))
			case "knowledge-document-representation/0.1.0":
				format := knowledgeDocumentTestString(t, projection["format"])
				tree := knowledgeDocumentTestObject(t, trees[format])
				got, err = KnowledgeDocumentRepresentationDigest(knowledgeDocumentTestDecode[KnowledgeDocumentRepresentation](t, tree["representation"]))
			case "knowledge-document-node/0.1.0":
				got, err = KnowledgeDocumentNodeDigest(
					knowledgeDocumentTestString(t, projection["representation_id"]), knowledgeDocumentTestString(t, projection["source_revision_id"]),
					knowledgeDocumentTestString(t, projection["node_payload_sha256"]))
			default:
				value, decodeErr := knowledgeDocumentJSON(vector["projection"])
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				got, err = KnowledgeLifecycleDigest(value)
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := knowledgeDocumentTestString(t, vector["sha256"]); got != want {
				t.Fatalf("helper digest=%s, want %s", got, want)
			}
		})
	}
}

func TestKnowledgeDocumentFrozenSchemaExamples(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	for _, group := range []struct {
		Name      string
		WantCount int
		WantError bool
	}{{"positive", 140, false}, {"negative", 148, true}} {
		var values []json.RawMessage
		if err := json.Unmarshal(examples[group.Name], &values); err != nil {
			t.Fatal(err)
		}
		if len(values) != group.WantCount {
			t.Fatalf("%s count=%d, want %d", group.Name, len(values), group.WantCount)
		}
		for _, raw := range values {
			item := knowledgeDocumentTestObject(t, raw)
			name := knowledgeDocumentTestString(t, item["name"])
			schema := knowledgeDocumentTestString(t, item["schema"])
			t.Run(group.Name+"/"+name, func(t *testing.T) {
				err := KnowledgeDocumentValidateJSON(schema, item["value"])
				if (err != nil) != group.WantError {
					t.Fatalf("schema %s error=%v, want error=%t", schema, err, group.WantError)
				}
			})
		}
	}
}

func TestKnowledgeDocumentFrozenReadingVectors(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	var vectors []json.RawMessage
	if err := json.Unmarshal(examples["reading_vectors"], &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 10 {
		t.Fatalf("reading vector count=%d, want 10", len(vectors))
	}
	for _, raw := range vectors {
		vector := knowledgeDocumentTestObject(t, raw)
		t.Run(knowledgeDocumentTestString(t, vector["name"]), func(t *testing.T) {
			members := knowledgeDocumentTestDecode[[]string](t, vector["members"])
			separator := knowledgeDocumentTestString(t, vector["separator"])
			want := knowledgeDocumentTestString(t, vector["text"])
			if got := strings.Join(members, separator); got != want {
				t.Fatalf("reading text differs: %q != %q", got, want)
			}
			if hex.EncodeToString([]byte(want)) != knowledgeDocumentTestString(t, vector["utf8_hex"]) {
				t.Fatal("UTF-8 vector differs")
			}
			if knowledgeDocumentSHA(want) != knowledgeDocumentTestString(t, vector["sha256"]) {
				t.Fatal("reading SHA differs")
			}
			var length int
			if err := json.Unmarshal(vector["utf8_byte_length"], &length); err != nil || len(want) != length {
				t.Fatal("reading byte length differs")
			}
		})
	}
}

func TestKnowledgeDocumentStrictRawJSON(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	var cases []json.RawMessage
	if err := json.Unmarshal(examples["raw_json_cases"], &cases); err != nil {
		t.Fatal(err)
	}
	for _, raw := range cases {
		item := knowledgeDocumentTestObject(t, raw)
		t.Run(knowledgeDocumentTestString(t, item["name"]), func(t *testing.T) {
			var accepted bool
			if err := json.Unmarshal(item["accepted"], &accepted); err != nil {
				t.Fatal(err)
			}
			_, err := knowledgeDocumentJSON([]byte(knowledgeDocumentTestString(t, item["text"])))
			if (err == nil) != accepted {
				t.Fatalf("raw acceptance error=%v, want accepted=%t", err, accepted)
			}
		})
	}
}

func TestKnowledgeDocumentTypedRoundTrip(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	var cases []json.RawMessage
	if err := json.Unmarshal(examples["positive"], &cases); err != nil {
		t.Fatal(err)
	}
	for _, raw := range cases {
		item := knowledgeDocumentTestObject(t, raw)
		schema := knowledgeDocumentTestString(t, item["schema"])
		target := knowledgeDocumentDTOForSchema(schema)
		if target == nil {
			continue
		}
		t.Run(knowledgeDocumentTestString(t, item["name"]), func(t *testing.T) {
			if err := json.Unmarshal(item["value"], target); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := KnowledgeDocumentValidateJSON(schema, encoded); err != nil {
				t.Fatal(err)
			}
			before, err := knowledgeDocumentJSON(item["value"])
			if err != nil {
				t.Fatal(err)
			}
			after, err := knowledgeDocumentJSON(encoded)
			if err != nil || !knowledgeDocumentEqual(before, after) {
				t.Fatal("typed JSON lost required null/zero or added a union field")
			}
		})
	}
}

func TestKnowledgeDocumentProducerRequiresExplicitBuildInputs(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	fixtures := knowledgeDocumentTestObject(t, examples["fixtures"])
	profiles := knowledgeDocumentTestDecode[[]KnowledgeDocumentProfile](t, fixtures["profiles"])
	for _, expected := range profiles {
		t.Run(expected.ProfileID, func(t *testing.T) {
			got, err := KnowledgeDocumentBuildProfile(expected.ProfileID, expected.Producer.EngineVersion,
				expected.Producer.EngineBuildSHA256, expected.Producer.AdapterVersion, false)
			if err != nil {
				t.Fatal(err)
			}
			if got.ProducerSHA256 != expected.ProducerSHA256 {
				t.Fatal("explicit vector inputs do not reproduce producer SHA")
			}
			if _, err := KnowledgeDocumentBuildProducer(expected.ProfileID, "", "", ""); err == nil {
				t.Fatal("missing actual build inputs accepted")
			}
		})
	}
	if _, err := KnowledgeDocumentBuildCapabilities(profiles, false); err != nil {
		t.Fatal(err)
	}
	if _, err := KnowledgeDocumentBuildCapabilities(profiles, true); err == nil {
		t.Fatal("global ready accepted while profiles are not ready")
	}
}

func TestKnowledgeDocumentNoRepresentationRootCycle(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	fixtures := knowledgeDocumentTestObject(t, examples["fixtures"])
	trees := knowledgeDocumentTestObject(t, fixtures["trees"])
	tree := knowledgeDocumentTestObject(t, trees["pdf"])
	value := knowledgeDocumentTestDecode[KnowledgeDocumentRepresentation](t, tree["representation"])
	before, err := KnowledgeDocumentRepresentationDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	value.RootDocumentNodeID = strings.Repeat("f", 64)
	value.PublishedAt = "2026-10-27T00:00:00Z"
	after, err := KnowledgeDocumentRepresentationDigest(value)
	if err != nil || before != after {
		t.Fatal("derived root or publication time entered representation identity")
	}
	request := knowledgeDocumentTestDecode[KnowledgeDocumentTaskRequest](t, tree["task_request"])
	first, err := KnowledgeDocumentTaskDigest(value.OwnerID, value.DomainID, request)
	if err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = "different_key"
	second, err := KnowledgeDocumentTaskDigest(value.OwnerID, value.DomainID, request)
	if err != nil || first != second {
		t.Fatal("idempotency key entered content request digest")
	}
}

func TestKnowledgeDocumentLocatorAndPayloadRequiredNulls(t *testing.T) {
	examples := knowledgeDocumentFrozenExamples(t)
	fixtures := knowledgeDocumentTestObject(t, examples["fixtures"])
	trees := knowledgeDocumentTestObject(t, fixtures["trees"])
	tree := knowledgeDocumentTestObject(t, trees["pdf"])
	nodes := knowledgeDocumentTestDecode[[]KnowledgeDocumentNode](t, tree["nodes"])
	raw, err := json.Marshal(nodes[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"parent_ref", "text_reason", "own_reading_range"} {
		needle := []byte("\"" + key + "\":null")
		if !bytes.Contains(raw, needle) {
			t.Fatalf("required nullable field %s was omitted", key)
		}
	}
	nodes[0].Payload.Name = string([]byte{0xff})
	if _, err := json.Marshal(nodes[0].Payload); err == nil {
		t.Fatal("invalid Go UTF-8 was replaced instead of rejected")
	}
	locator := KnowledgeDocumentLocator{Scheme: "derived_passage_v1", ParentLocator: &KnowledgeDocumentLocator{Scheme: "document_v1"}, ChunkIndex: 0}
	encoded, err := json.Marshal(locator)
	if err != nil || !bytes.Contains(encoded, []byte("\"chunk_index\":0")) {
		t.Fatal("zero chunk index was omitted")
	}
	locator.Part = "word/document.xml"
	if _, err := json.Marshal(locator); err == nil {
		t.Fatal("a nonselected union field was silently discarded")
	}
}
