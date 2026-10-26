package entity

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"unicode/utf8"
)

//go:embed knowledge_web_digests.json
var knowledgeWebDigestsJSON []byte

//go:embed knowledge_web_capabilities.json
var KnowledgeWebCapabilitiesJSON []byte

type webDigestProjection struct {
	Prefix string   `json:"prefix"`
	Fields []string `json:"fields"`
}

var webDigestProjections = func() map[string]webDigestProjection {
	var projections map[string]webDigestProjection
	if err := json.Unmarshal(knowledgeWebDigestsJSON, &projections); err != nil {
		panic(err)
	}
	return projections
}()

func webLengthPrefix(dst *bytes.Buffer, value string) {
	dst.WriteString(strconv.Itoa(len(value)))
	dst.WriteByte(':')
	dst.WriteString(value)
}

func webEncodeValue(dst *bytes.Buffer, value any, depth int) error {
	if depth > 64 {
		return fmt.Errorf("web digest nesting limit")
	}
	switch value := value.(type) {
	case nil:
		webLengthPrefix(dst, "n")
	case string:
		if !utf8.ValidString(value) {
			return fmt.Errorf("invalid digest UTF-8")
		}
		webLengthPrefix(dst, "s")
		webLengthPrefix(dst, value)
	case bool:
		webLengthPrefix(dst, "b")
		if value {
			webLengthPrefix(dst, "1")
		} else {
			webLengthPrefix(dst, "0")
		}
	case json.Number:
		n, err := value.Int64()
		if err != nil || n < -9007199254740991 || n > 9007199254740991 || strconv.FormatInt(n, 10) != string(value) {
			return fmt.Errorf("web digest requires canonical safe integer")
		}
		webLengthPrefix(dst, "i")
		webLengthPrefix(dst, string(value))
	case []any:
		webLengthPrefix(dst, "a")
		webLengthPrefix(dst, strconv.Itoa(len(value)))
		for _, child := range value {
			if err := webEncodeValue(dst, child, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			if !utf8.ValidString(key) {
				return fmt.Errorf("invalid digest key UTF-8")
			}
			keys = append(keys, key)
		}
		slices.Sort(keys)
		webLengthPrefix(dst, "o")
		webLengthPrefix(dst, strconv.Itoa(len(keys)))
		for _, key := range keys {
			webLengthPrefix(dst, key)
			if err := webEncodeValue(dst, value[key], depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported web digest value %T", value)
	}
	return nil
}

func webDigestValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// KnowledgeWebDigest hashes exactly the approved projection using typed UTF-8
// length prefixes. Callers canonicalize sets first; ordered arrays stay ordered.
func KnowledgeWebDigest(kind string, value any) (string, error) {
	projection, ok := webDigestProjections[kind]
	if !ok {
		return "", fmt.Errorf("unknown web digest projection")
	}
	decoded, err := webDigestValue(value)
	if err != nil {
		return "", err
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return "", fmt.Errorf("web digest projection requires an object")
	}
	selected := make(map[string]any, len(projection.Fields))
	for _, key := range projection.Fields {
		child, exists := object[key]
		if !exists {
			return "", fmt.Errorf("missing web digest field %s", key)
		}
		selected[key] = child
	}
	var encoded bytes.Buffer
	webLengthPrefix(&encoded, projection.Prefix)
	if err := webEncodeValue(&encoded, selected, 0); err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded.Bytes())
	return hex.EncodeToString(digest[:]), nil
}
