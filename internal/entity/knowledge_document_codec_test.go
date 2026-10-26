package entity

import (
	"encoding/json"
	"testing"
)

func TestKnowledgeDocumentCodecScalarAndUnionEquivalence(t *testing.T) {
	t.Run("scalar_equal_keeps_digest_normalization", func(t *testing.T) {
		type namedString string
		type namedBool bool
		var nilString *string
		values := []struct {
			name  string
			value any
		}{
			{"nil", nil},
			{"false", false},
			{"true", true},
			{"empty", ""},
			{"paragraph", "paragraph"},
			{"slide", "slide"},
			{"unicode", "节😀"},
			{"nfc", "é"},
			{"combining", "e\u0301"},
			{"nul", "\x00"},
			{"replacement", "\ufffd"},
			{"invalid_utf8", string([]byte{0xff})},
			{"named_string", namedString("paragraph")},
			{"named_bool", namedBool(true)},
			{"typed_nil", nilString},
			{"nil_slice", []any(nil)},
			{"nil_map", map[string]any(nil)},
			{"raw_null", json.RawMessage(`null`)},
			{"raw_string", json.RawMessage(`"paragraph"`)},
			{"raw_bool", json.RawMessage(`true`)},
			{"number", json.Number("1")},
			{"safe_max", json.Number("9007199254740991")},
			{"noncanonical_number", json.Number("01")},
			{"empty_slice", []any{}},
			{"empty_map", map[string]any{}},
			{"nested", map[string]any{"kind": "paragraph", "values": []any{true, nil, json.Number("1")}}},
		}
		canonicalEqual := func(a, b any) bool {
			left, err := KnowledgeLifecycleDigest(a)
			if err != nil {
				return false
			}
			right, err := KnowledgeLifecycleDigest(b)
			return err == nil && left == right
		}
		for _, left := range values {
			for _, right := range values {
				got := knowledgeDocumentEqual(left.value, right.value)
				want := canonicalEqual(left.value, right.value)
				if got != want {
					t.Fatalf("%s/%s equal=%t, canonical=%t", left.name, right.name, got, want)
				}
			}
		}
	})

	t.Run("object_constraints_and_oneof_cardinality", func(t *testing.T) {
		cases := []struct {
			name   string
			schema string
			value  string
			valid  bool
		}{
			{
				"existing_matching_const",
				`{"type":"object","required":["kind"],"properties":{"kind":{"const":"paragraph"}}}`,
				`{"kind":"paragraph"}`, true,
			},
			{
				"existing_wrong_const",
				`{"type":"object","properties":{"kind":{"const":"paragraph"}}}`,
				`{"kind":"slide"}`, false,
			},
			{
				"missing_optional_const",
				`{"type":"object","properties":{"kind":{"const":"paragraph"}}}`,
				`{}`, true,
			},
			{
				"missing_required_const",
				`{"type":"object","required":["kind"],"properties":{"kind":{"const":"paragraph"}}}`,
				`{}`, false,
			},
			{
				"nonobject_properties_are_inapplicable",
				`{"properties":{"kind":{"const":"paragraph"}}}`,
				`"text"`, true,
			},
			{
				"wrong_const_type",
				`{"type":"object","properties":{"kind":{"const":"paragraph"}}}`,
				`{"kind":true}`, false,
			},
			{
				"matching_kind_still_rejects_unknown",
				`{"type":"object","additionalProperties":false,"properties":{"kind":{"const":"paragraph"}}}`,
				`{"kind":"paragraph","extra":true}`, false,
			},
			{
				"matching_kind_still_checks_other_properties",
				`{"type":"object","properties":{"kind":{"const":"paragraph"},"name":{"type":"string","minLength":2}}}`,
				`{"kind":"paragraph","name":"a"}`, false,
			},
			{
				"matching_kind_still_checks_predicate",
				`{"type":"object","properties":{"kind":{"const":"paragraph"}},"x-reading-range":true}`,
				`{"kind":"paragraph","start_byte":2,"end_byte":1}`, false,
			},
			{
				"oneof_exactly_one",
				`{"oneOf":[{"properties":{"kind":{"const":"paragraph"}}},{"properties":{"kind":{"const":"slide"}}}]}`,
				`{"kind":"paragraph"}`, true,
			},
			{
				"oneof_zero",
				`{"oneOf":[{"properties":{"kind":{"const":"paragraph"}}},{"properties":{"kind":{"const":"slide"}}}]}`,
				`{"kind":"table"}`, false,
			},
			{
				"oneof_two_must_not_stop_at_first_success",
				`{"oneOf":[{"properties":{"kind":{"const":"paragraph"}}},{"properties":{"kind":{"const":"paragraph"}}}]}`,
				`{"kind":"paragraph"}`, false,
			},
			{
				"oneof_optional_const_missing_matches_both",
				`{"oneOf":[{"properties":{"kind":{"const":"paragraph"}}},{"properties":{"kind":{"const":"slide"}}}]}`,
				`{}`, false,
			},
			{
				"nonstring_const_uses_existing_validation",
				`{"type":"object","properties":{"enabled":{"const":false},"reason":{"const":null}}}`,
				`{"enabled":false,"reason":null}`, true,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rawSchema, err := knowledgeDocumentJSON([]byte(tc.schema))
				if err != nil {
					t.Fatal(err)
				}
				value, err := knowledgeDocumentJSON([]byte(tc.value))
				if err != nil {
					t.Fatal(err)
				}
				schema, ok := rawSchema.(map[string]any)
				if !ok {
					t.Fatal("test schema is not an object")
				}
				got := knowledgeDocumentValidateSchema(schema, value, 0) == nil
				if got != tc.valid {
					t.Fatalf("valid=%t, want %t", got, tc.valid)
				}
			})
		}
	})
}
