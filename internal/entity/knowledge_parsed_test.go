package entity

import "testing"

func TestKnowledgeParsedDecodeFinitePositionsAndStrictFields(t *testing.T) {
	type sample struct {
		Positions [][]float64 `json:"positions"`
		Label     string      `json:"label"`
	}
	var got sample
	if err := KnowledgeParsedDecode([]byte(`{"positions":[[1.25,2.5]],"label":"合成"}`), &got); err != nil || got.Positions[0][0] != 1.25 {
		t.Fatal(got, err)
	}
	for _, raw := range []string{
		`{"positions":[],"positions":[],"label":""}`,
		`{"positions":[],"label":"","owner_id":"forged"}`,
		`{"positions":[[1e309]],"label":""}`,
		`{"positions":[],"label":"\ud800"}`,
		`{"positions":[],"label":""} {}`,
	} {
		if err := KnowledgeParsedDecode([]byte(raw), &got); err == nil {
			t.Fatal("accepted invalid parsed JSON", raw)
		}
	}
}
