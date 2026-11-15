package entity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed knowledge_access_schema.json
var knowledgeAccessSchemaJSON []byte

var knowledgeAccessDefinitions = func() map[string]any {
	var schema struct {
		Definitions map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(knowledgeAccessSchemaJSON, &schema); err != nil {
		panic(err)
	}
	return schema.Definitions
}()

// KnowledgeAccessDecode checks the closed wire shape before decoding a DTO.
// Nullable fields remain required; duplicate keys and invalid Unicode never
// reach canonical command hashing.
func KnowledgeAccessDecode(raw []byte, definition string, target any) error {
	if len(raw) > 1<<20 || knowledgeDocumentUnicode(raw) != nil {
		return fmt.Errorf("invalid access JSON")
	}
	check := json.NewDecoder(bytes.NewReader(raw))
	check.UseNumber()
	if err := parsedJSONValue(check, 0); err != nil {
		return err
	}
	if _, err := check.Token(); err != io.EOF {
		return fmt.Errorf("trailing access JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := knowledgeAccessValidate(knowledgeAccessDefinitions[definition], value, 0); err != nil {
		return err
	}
	if target != nil {
		normalized, err := json.Marshal(knowledgeAccessIntegers(value))
		if err != nil {
			return err
		}
		return json.Unmarshal(normalized, target)
	}
	return nil
}

func knowledgeAccessValidate(raw, value any, depth int) error {
	fail := func() error { return fmt.Errorf("invalid access value") }
	if depth > 64 || raw == nil {
		return fail()
	}
	schema, ok := raw.(map[string]any)
	if !ok {
		return fail()
	}
	if ref, ok := schema["$ref"].(string); ok {
		return knowledgeAccessValidate(knowledgeAccessDefinitions[strings.TrimPrefix(ref, "#/$defs/")], value, depth+1)
	}
	if constant, ok := schema["const"]; ok && !reflect.DeepEqual(constant, value) {
		return fail()
	}
	if values, ok := schema["enum"].([]any); ok {
		found := false
		for _, candidate := range values {
			found = found || reflect.DeepEqual(candidate, value)
		}
		if !found {
			return fail()
		}
	}
	for _, union := range []string{"oneOf", "anyOf"} {
		if alternatives, ok := schema[union].([]any); ok {
			matches := 0
			for _, alternative := range alternatives {
				if knowledgeAccessValidate(alternative, value, depth+1) == nil {
					matches++
				}
			}
			if matches == 0 || union == "oneOf" && matches != 1 {
				return fail()
			}
		}
	}
	if all, ok := schema["allOf"].([]any); ok {
		for _, item := range all {
			if err := knowledgeAccessValidate(item, value, depth+1); err != nil {
				return err
			}
		}
	}
	if condition, ok := schema["if"]; ok {
		branch := "else"
		if knowledgeAccessValidate(condition, value, depth+1) == nil {
			branch = "then"
		}
		if next, ok := schema[branch]; ok {
			if err := knowledgeAccessValidate(next, value, depth+1); err != nil {
				return err
			}
		}
	}
	switch schema["type"] {
	case "null":
		if value != nil {
			return fail()
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fail()
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fail()
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fail()
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fail()
		}
	case "integer":
		n, ok := value.(json.Number)
		if !ok {
			return fail()
		}
		r, ok := new(big.Rat).SetString(string(n))
		if !ok || !r.IsInt() || !r.Num().IsInt64() {
			return fail()
		}
		integer := r.Num().Int64()
		if integer < -KnowledgeAccessMaxGeneration || integer > KnowledgeAccessMaxGeneration {
			return fail()
		}
		if min, ok := schema["minimum"].(float64); ok && integer < int64(min) {
			return fail()
		}
		if max, ok := schema["maximum"].(float64); ok && integer > int64(max) {
			return fail()
		}
	}
	if object, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				if _, exists := object[key.(string)]; !exists {
					return fail()
				}
			}
		}
		for key, item := range object {
			shape, exists := properties[key]
			if !exists {
				if schema["additionalProperties"] == false {
					return fail()
				}
				continue
			}
			if err := knowledgeAccessValidate(shape, item, depth+1); err != nil {
				return err
			}
		}
	}
	if text, ok := value.(string); ok {
		if !utf8.ValidString(text) {
			return fail()
		}
		length := utf8.RuneCountInString(text)
		if min, ok := schema["minLength"].(float64); ok && length < int(min) {
			return fail()
		}
		if max, ok := schema["maxLength"].(float64); ok && length > int(max) {
			return fail()
		}
		if pattern, ok := schema["pattern"].(string); ok {
			expression, err := regexp.Compile(pattern)
			if err != nil || !expression.MatchString(text) {
				return fail()
			}
		}
		if schema["format"] == "date-time" {
			if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
				return fail()
			}
		}
	}
	if values, ok := value.([]any); ok {
		if min, ok := schema["minItems"].(float64); ok && len(values) < int(min) {
			return fail()
		}
		if max, ok := schema["maxItems"].(float64); ok && len(values) > int(max) {
			return fail()
		}
		seen := map[string]bool{}
		for _, item := range values {
			if shape, ok := schema["items"]; ok {
				if err := knowledgeAccessValidate(shape, item, depth+1); err != nil {
					return err
				}
			}
			if schema["uniqueItems"] == true {
				canonical, err := json.Marshal(item)
				if err != nil || seen[string(canonical)] {
					return fail()
				}
				seen[string(canonical)] = true
			}
		}
	}
	return nil
}

func knowledgeAccessIntegers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if integer, ok := new(big.Rat).SetString(string(v)); ok && integer.IsInt() && integer.Num().IsInt64() {
			return integer.Num().Int64()
		}
	case map[string]any:
		for key, item := range v {
			v[key] = knowledgeAccessIntegers(item)
		}
	case []any:
		for i, item := range v {
			v[i] = knowledgeAccessIntegers(item)
		}
	}
	return value
}
