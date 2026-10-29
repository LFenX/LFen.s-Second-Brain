package entity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"unicode/utf8"
)

// KnowledgeParsedDecode permits finite producer positions while retaining exact
// key/Unicode checks. It does not widen the frozen R2 integer-only JSON codec.
func KnowledgeParsedDecode(raw []byte, target any) error {
	if len(raw) > 32<<20 {
		return fmt.Errorf("parsed JSON capacity exceeded")
	}
	if err := knowledgeDocumentUnicode(raw); err != nil {
		return err
	}
	check := json.NewDecoder(bytes.NewReader(raw))
	check.UseNumber()
	if err := parsedJSONValue(check, 0); err != nil {
		return err
	}
	if _, err := check.Token(); err != io.EOF {
		return fmt.Errorf("trailing parsed JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing parsed JSON")
	}
	return nil
}

func parsedJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("parsed JSON depth exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if number, ok := token.(json.Number); ok {
		value, err := number.Float64()
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
			return fmt.Errorf("invalid parsed number")
		}
		return nil
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate parsed JSON key")
			}
			seen[name] = true
			if err := parsedJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("invalid parsed object")
		}
	case '[':
		for decoder.More() {
			if err := parsedJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("invalid parsed array")
		}
	default:
		return fmt.Errorf("unexpected parsed delimiter")
	}
	return nil
}

func parsedRangeMatches(r KnowledgeContentRange, text string) bool {
	if r.Validate() != nil || r.EndByte > int64(len(text)) || r.SHA256 == nil {
		return false
	}
	if r.StartByte < int64(len(text)) && !utf8.RuneStart(text[r.StartByte]) || r.EndByte < int64(len(text)) && !utf8.RuneStart(text[r.EndByte]) {
		return false
	}
	digest := sha256.Sum256([]byte(text[r.StartByte:r.EndByte]))
	return hex.EncodeToString(digest[:]) == *r.SHA256
}

func KnowledgeParsedValidateBound(rep KnowledgeParsedRepresentation, reading string, nodes []KnowledgeParsedNode, manifest []KnowledgeParsedManifestEntry) error {
	fail := func() error { return fmt.Errorf("parsed_integrity") }
	if !utf8.ValidString(reading) || rep.ReadingByteLength != int64(len(reading)) || rep.NodeCount != int64(len(nodes)) || len(nodes) != len(manifest) || len(nodes) < 1 || rep.Ref.ID == nil {
		return fail()
	}
	digest := sha256.Sum256([]byte(reading))
	if hex.EncodeToString(digest[:]) != rep.ReadingTextSHA256 {
		return fail()
	}
	manifestSHA, err := KnowledgeParsedManifestDigest(manifest)
	if err != nil || manifestSHA != rep.ManifestSHA256 {
		return fail()
	}
	repID, err := KnowledgeParsedRepresentationDigest(rep)
	if err != nil || repID != *rep.Ref.ID {
		return fail()
	}
	if rep.RootRef.Kind != "parsed_node" || rep.RootRef.ID == nil || rep.RootRef.ItemIndex != nil || *rep.RootRef.ID != nodes[0].NodeID {
		return fail()
	}
	var unsupported, unavailable, cursor int64
	seen := map[string]bool{}
	for i, node := range nodes {
		p := node.Payload
		payloadSHA, err := KnowledgeParsedPayloadDigest(p)
		if err != nil || payloadSHA != node.PayloadSHA256 || manifest[i].PayloadSHA256 != payloadSHA || p.LocalIndex != int64(i) || node.RepresentationID != repID {
			return fail()
		}
		id, err := KnowledgeParsedNodeDigest(repID, int64(i), payloadSHA)
		if err != nil || id != node.NodeID || seen[id] {
			return fail()
		}
		seen[id] = true
		if i == 0 {
			if p.Kind != "document" || p.ParentLocalIndex != nil || p.Text.Range == nil || p.Text.Range.StartByte != 0 || p.Text.Range.EndByte != int64(len(reading)) {
				return fail()
			}
		} else {
			if p.ParentLocalIndex == nil || *p.ParentLocalIndex != 0 || manifest[i].ParentLocalIndex == nil || *manifest[i].ParentLocalIndex != 0 {
				return fail()
			}
		}
		if p.Text.Value != nil || p.Text.Validate("derived_utf8") != nil || p.Text.Range == nil || !parsedRangeMatches(*p.Text.Range, reading) {
			return fail()
		}
		if p.Text.OwnRange != nil && !parsedRangeMatches(*p.Text.OwnRange, reading) {
			return fail()
		}
		if p.Text.Status == "unsupported" || p.Text.Status == "unsupported_or_requires_ocr" {
			unsupported++
		}
		if p.Text.Status == "unavailable" {
			unavailable++
		}
		if p.Kind != "passage" {
			continue
		}
		r := p.Text.Range
		if r.StartByte != cursor || r.EndByte <= r.StartByte || r.EndByte-r.StartByte > rep.MaxUnitBytes {
			return fail()
		}
		memberCursor := r.StartByte
		for _, member := range p.Members {
			if member.LocalIndex < 0 || member.LocalIndex >= int64(i) || member.Range.StartByte != memberCursor || member.Range.EndByte > r.EndByte || !parsedRangeMatches(member.Range, reading) {
				return fail()
			}
			owner := nodes[member.LocalIndex].Payload.Text.Range
			if owner == nil || member.Range.StartByte < owner.StartByte || member.Range.EndByte > owner.EndByte {
				return fail()
			}
			if member.Kind == "assembly_separator" {
				if member.LocalIndex != 0 || reading[member.Range.StartByte:member.Range.EndByte] != "\n" {
					return fail()
				}
			} else if member.Kind != "content" || member.LocalIndex == 0 {
				return fail()
			}
			memberCursor = member.Range.EndByte
		}
		if memberCursor != r.EndByte {
			return fail()
		}
		cursor = r.EndByte
	}
	if cursor != int64(len(reading)) || unsupported != rep.UnsupportedNodeCount || unavailable != rep.UnavailableNodeCount {
		return fail()
	}
	return nil
}
