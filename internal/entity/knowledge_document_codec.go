package entity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The private schema set is generated from the frozen document contract.
// It is not a dynamic schema loader or a new public contract.
//
//go:embed knowledge_document_schemas.json
var knowledgeDocumentSchemasJSON []byte

var knowledgeDocumentXMLLocalName = regexp.MustCompile("^[_\\p{L}][_\\p{L}\\p{N}\\p{M}.\\-\\x{00b7}]*$")

var knowledgeDocumentKeyPattern = regexp.MustCompile("^[A-Za-z0-9_-]{1,128}$")

var knowledgeDocumentSchemas = func() map[string]map[string]any {
	decoder := json.NewDecoder(bytes.NewReader(knowledgeDocumentSchemasJSON))
	decoder.UseNumber()
	var schemas map[string]map[string]any
	if err := decoder.Decode(&schemas); err != nil {
		panic(err)
	}
	return schemas
}()

func knowledgeDocumentHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			if r < 'a' || r > 'f' {
				return false
			}
		}
	}
	return true
}

// knowledgeDocumentUnicode rejects escaped unpaired surrogates before the
// standard decoder can replace them, and rejects literal invalid UTF-8.
func knowledgeDocumentUnicode(raw []byte) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid document JSON UTF-8")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		i++
		for i < len(raw) && raw[i] != '"' {
			if raw[i] != '\\' {
				i++
				continue
			}
			if i+1 >= len(raw) {
				return fmt.Errorf("invalid document JSON escape")
			}
			if raw[i+1] != 'u' {
				i += 2
				continue
			}
			if i+6 > len(raw) {
				return fmt.Errorf("invalid document Unicode escape")
			}
			first, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
			if err != nil {
				return err
			}
			switch {
			case first >= 0xd800 && first <= 0xdbff:
				if i+12 > len(raw) || string(raw[i+6:i+8]) != "\\u" {
					return fmt.Errorf("unpaired document surrogate")
				}
				second, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
				if err != nil || second < 0xdc00 || second > 0xdfff {
					return fmt.Errorf("unpaired document surrogate")
				}
				i += 12
			case first >= 0xdc00 && first <= 0xdfff:
				return fmt.Errorf("unpaired document surrogate")
			default:
				i += 6
			}
		}
	}
	return nil
}

func knowledgeDocumentJSON(raw []byte) (any, error) {
	if len(raw) > 16*1024*1024 {
		return nil, fmt.Errorf("document JSON exceeds pack budget")
	}
	if err := knowledgeDocumentUnicode(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := knowledgeDocumentReadJSON(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("document JSON has trailing data")
	}
	return value, nil
}

func knowledgeDocumentReadJSON(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("document JSON depth exceeds 64")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			result := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("invalid document object key")
				}
				if _, exists := result[key]; exists {
					return nil, fmt.Errorf("duplicate document JSON key")
				}
				child, err := knowledgeDocumentReadJSON(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				result[key] = child
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, fmt.Errorf("invalid document object")
			}
			return result, nil
		case '[':
			result := make([]any, 0)
			for decoder.More() {
				child, err := knowledgeDocumentReadJSON(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				result = append(result, child)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, fmt.Errorf("invalid document array")
			}
			return result, nil
		}
		return nil, fmt.Errorf("invalid document delimiter")
	case json.Number:
		n, err := value.Int64()
		if err != nil || n < -9007199254740991 || n > 9007199254740991 || strconv.FormatInt(n, 10) != string(value) {
			return nil, fmt.Errorf("document JSON requires canonical safe integers")
		}
		return value, nil
	default:
		return value, nil
	}
}

func KnowledgeDocumentValidateJSON(schemaName string, raw []byte) error {
	schema, ok := knowledgeDocumentSchemas[schemaName]
	if !ok {
		return fmt.Errorf("unknown document schema")
	}
	value, err := knowledgeDocumentJSON(raw)
	if err != nil {
		return err
	}
	return knowledgeDocumentValidateSchema(schema, value, 0)
}

func knowledgeDocumentDecode(schemaName string, raw []byte, target any) error {
	if err := KnowledgeDocumentValidateJSON(schemaName, raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func KnowledgeDocumentDecode(schemaName string, raw []byte, target any) error {
	return knowledgeDocumentDecode(schemaName, raw, target)
}

func knowledgeDocumentMarshal(schemaName string, value any) ([]byte, error) {
	if err := knowledgeDocumentGoUnicode(reflect.ValueOf(value), 0); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if err := KnowledgeDocumentValidateJSON(schemaName, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func knowledgeDocumentGoUnicode(value reflect.Value, depth int) error {
	if depth > 64 {
		return fmt.Errorf("document Go value depth exceeds 64")
	}
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("invalid document Go string UTF-8")
		}
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			return knowledgeDocumentGoUnicode(value.Elem(), depth+1)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanInterface() {
				if err := knowledgeDocumentGoUnicode(value.Field(i), depth+1); err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if err := knowledgeDocumentGoUnicode(iter.Key(), depth+1); err != nil {
				return err
			}
			if err := knowledgeDocumentGoUnicode(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := knowledgeDocumentGoUnicode(value.Index(i), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func knowledgeDocumentNumber(value any) float64 {
	switch number := value.(type) {
	case json.Number:
		n, _ := number.Float64()
		return n
	case int64:
		return float64(number)
	case int:
		return float64(number)
	default:
		return math.NaN()
	}
}

func knowledgeDocumentEqual(a, b any) bool {
	// These exact Go scalar types already have canonical typed-LP values.
	// Other representations retain the digest normalization path below.
	switch left := a.(type) {
	case string:
		if right, ok := b.(string); ok && utf8.ValidString(left) && utf8.ValidString(right) {
			return left == right
		}
	case bool:
		if right, ok := b.(bool); ok {
			return left == right
		}
	case nil:
		if b == nil {
			return true
		}
	}
	left, err := KnowledgeLifecycleDigest(a)
	if err != nil {
		return false
	}
	right, err := KnowledgeLifecycleDigest(b)
	return err == nil && left == right
}

func knowledgeDocumentSchemaMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func knowledgeDocumentValidateSchema(schema map[string]any, value any, depth int) error {
	if depth > 128 {
		return fmt.Errorf("document schema depth exceeds bound")
	}
	fail := func() error { return fmt.Errorf("document value does not match the frozen schema") }
	// Existing discriminator consts are necessary object constraints. Reject
	// impossible branches before nested work without changing oneOf counting.
	if object, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		for _, key := range [...]string{"kind", "scheme"} {
			child, exists := object[key]
			if !exists {
				continue
			}
			property := knowledgeDocumentSchemaMap(properties[key])
			if constant, ok := property["const"].(string); ok && !knowledgeDocumentEqual(child, constant) {
				return fail()
			}
		}
	}
	if ref, ok := schema["$ref"].(string); ok {
		target, exists := knowledgeDocumentSchemas[strings.TrimPrefix(ref, "#/components/schemas/")]
		if !exists {
			return fail()
		}
		if err := knowledgeDocumentValidateSchema(target, value, depth+1); err != nil {
			return err
		}
	}
	if constant, ok := schema["const"]; ok && !knowledgeDocumentEqual(value, constant) {
		return fail()
	}
	if values, ok := schema["enum"].([]any); ok {
		found := false
		for _, candidate := range values {
			found = found || knowledgeDocumentEqual(value, candidate)
		}
		if !found {
			return fail()
		}
	}
	if branches, ok := schema["allOf"].([]any); ok {
		for _, branch := range branches {
			if err := knowledgeDocumentValidateSchema(knowledgeDocumentSchemaMap(branch), value, depth+1); err != nil {
				return err
			}
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if branches, ok := schema[keyword].([]any); ok {
			matched := 0
			for _, branch := range branches {
				if knowledgeDocumentValidateSchema(knowledgeDocumentSchemaMap(branch), value, depth+1) == nil {
					matched++
				}
			}
			if matched < 1 || keyword == "oneOf" && matched != 1 {
				return fail()
			}
		}
	}
	if conditional, ok := schema["if"].(map[string]any); ok {
		branch := "else"
		if knowledgeDocumentValidateSchema(conditional, value, depth+1) == nil {
			branch = "then"
		}
		if constraint, ok := schema[branch].(map[string]any); ok {
			if err := knowledgeDocumentValidateSchema(constraint, value, depth+1); err != nil {
				return err
			}
		}
	}
	if kind, ok := schema["type"].(string); ok {
		valid := false
		switch kind {
		case "null":
			valid = value == nil
		case "object":
			_, valid = value.(map[string]any)
		case "array":
			_, valid = value.([]any)
		case "string":
			_, valid = value.(string)
		case "boolean":
			_, valid = value.(bool)
		case "integer":
			number, ok := value.(json.Number)
			if ok {
				n, err := number.Int64()
				valid = err == nil && n >= -9007199254740991 && n <= 9007199254740991
			}
		}
		if !valid {
			return fail()
		}
	}
	if number, ok := value.(json.Number); ok {
		if minimum, exists := schema["minimum"]; exists && knowledgeDocumentNumber(number) < knowledgeDocumentNumber(minimum) {
			return fail()
		}
		if maximum, exists := schema["maximum"]; exists && knowledgeDocumentNumber(number) > knowledgeDocumentNumber(maximum) {
			return fail()
		}
	}
	if text, ok := value.(string); ok {
		length := utf8.RuneCountInString(text)
		if limit, exists := schema["minLength"]; exists && float64(length) < knowledgeDocumentNumber(limit) {
			return fail()
		}
		if limit, exists := schema["maxLength"]; exists && float64(length) > knowledgeDocumentNumber(limit) {
			return fail()
		}
		if limit, exists := schema["x-max-utf8-bytes"]; exists && float64(len(text)) > knowledgeDocumentNumber(limit) {
			return fail()
		}
		if pattern, ok := schema["pattern"].(string); ok && !knowledgeDocumentPattern(pattern, text) {
			return fail()
		}
		if schema["format"] == "date-time" {
			if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
				return fail()
			}
		}
		if schema["x-xml-local-name"] == true {
			if !knowledgeDocumentXMLLocalName.MatchString(text) {
				return fail()
			}
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
		// Reject an existing string const before unrelated nested properties.
		// Successful branches keep full validation; oneOf still counts every branch.
		for key, raw := range properties {
			property := knowledgeDocumentSchemaMap(raw)
			constant, ok := property["const"].(string)
			if !ok {
				continue
			}
			if child, exists := object[key]; exists && !knowledgeDocumentEqual(child, constant) {
				return fail()
			}
		}
		for key, child := range object {
			property, exists := properties[key]
			if !exists {
				if schema["additionalProperties"] == false {
					return fail()
				}
				continue
			}
			if err := knowledgeDocumentValidateSchema(knowledgeDocumentSchemaMap(property), child, depth+1); err != nil {
				return err
			}
		}
		if dependencies, ok := schema["dependentRequired"].(map[string]any); ok {
			for key, raw := range dependencies {
				if _, exists := object[key]; !exists {
					continue
				}
				for _, target := range raw.([]any) {
					if _, exists := object[target.(string)]; !exists {
						return fail()
					}
				}
			}
		}
	}
	if values, ok := value.([]any); ok {
		if limit, exists := schema["minItems"]; exists && float64(len(values)) < knowledgeDocumentNumber(limit) {
			return fail()
		}
		if limit, exists := schema["maxItems"]; exists && float64(len(values)) > knowledgeDocumentNumber(limit) {
			return fail()
		}
		prefixes, _ := schema["prefixItems"].([]any)
		for i, child := range values {
			if i < len(prefixes) {
				if err := knowledgeDocumentValidateSchema(knowledgeDocumentSchemaMap(prefixes[i]), child, depth+1); err != nil {
					return err
				}
			}
			if item, ok := schema["items"].(map[string]any); ok {
				if err := knowledgeDocumentValidateSchema(item, child, depth+1); err != nil {
					return err
				}
			}
		}
		if contains, ok := schema["contains"].(map[string]any); ok {
			count, minimum := 0, 1.0
			if raw, exists := schema["minContains"]; exists {
				minimum = knowledgeDocumentNumber(raw)
			}
			for _, child := range values {
				if knowledgeDocumentValidateSchema(contains, child, depth+1) == nil {
					count++
				}
			}
			if float64(count) < minimum {
				return fail()
			}
			if maximum, exists := schema["maxContains"]; exists && float64(count) > knowledgeDocumentNumber(maximum) {
				return fail()
			}
		}
		if schema["uniqueItems"] == true {
			seen := make(map[string]bool, len(values))
			for _, child := range values {
				key, err := KnowledgeLifecycleDigest(child)
				if err != nil || seen[key] {
					return fail()
				}
				seen[key] = true
			}
		}
	}
	if limit, exists := schema["x-max-json-bytes"]; exists {
		raw, err := json.Marshal(value)
		if err != nil || float64(len(raw)) > knowledgeDocumentNumber(limit) {
			return fail()
		}
	}
	return knowledgeDocumentValidatePredicates(schema, value)
}

func knowledgeDocumentPattern(pattern, value string) bool {
	if strings.HasPrefix(pattern, "^(?!") {
		if value == "" || strings.ContainsAny(value, "\\:") {
			return false
		}
		for _, r := range value {
			if r < 32 || r == 127 {
				return false
			}
		}
		if strings.Contains(pattern, ".*//") {
			for _, part := range strings.Split(value, "/") {
				if part == "" || part == "." || part == ".." {
					return false
				}
			}
			return true
		}
		return value != "." && value != ".." && !strings.Contains(value, "/")
	}
	switch pattern {
	case "^[0-9a-f]{32}$":
		return knowledgeDocumentHex(value, 32)
	case "^[0-9a-f]{64}$":
		return knowledgeDocumentHex(value, 64)
	case "^[A-Za-z0-9_-]{1,128}$":
		return knowledgeDocumentKeyPattern.MatchString(value)
	}
	matched, err := regexp.MatchString(pattern, value)
	return err == nil && matched
}

func (value KnowledgeDocumentLocator) MarshalJSON() ([]byte, error) {
	var object map[string]any
	switch value.Scheme {
	case "document_v1":
		if value.PageNumber != 0 || value.Part != "" || len(value.Segments) != 0 || value.ParentLocator != nil || value.ChunkIndex != 0 {
			return nil, fmt.Errorf("non-document locator fields")
		}
		object = map[string]any{"scheme": value.Scheme}
	case "pdf_page_v1":
		if value.Part != "" || len(value.Segments) != 0 || value.ParentLocator != nil || value.ChunkIndex != 0 {
			return nil, fmt.Errorf("non-PDF locator fields")
		}
		object = map[string]any{"scheme": value.Scheme, "page_number": value.PageNumber}
	case "office_package_v1":
		if value.PageNumber != 0 || value.ParentLocator != nil || value.ChunkIndex != 0 {
			return nil, fmt.Errorf("non-Office locator fields")
		}
		object = map[string]any{"scheme": value.Scheme, "part": value.Part, "segments": value.Segments}
	case "derived_passage_v1":
		if value.PageNumber != 0 || value.Part != "" || len(value.Segments) != 0 || value.ParentLocator == nil || value.ParentLocator.Scheme == "derived_passage_v1" {
			return nil, fmt.Errorf("invalid derived locator fields")
		}
		object = map[string]any{"scheme": value.Scheme, "parent_locator": value.ParentLocator, "chunk_index": value.ChunkIndex}
	default:
		return nil, fmt.Errorf("unknown document locator scheme")
	}
	return knowledgeDocumentMarshal("Locator", object)
}

func (value *KnowledgeDocumentLocator) UnmarshalJSON(raw []byte) error {
	if err := KnowledgeDocumentValidateJSON("Locator", raw); err != nil {
		return err
	}
	decoded, err := knowledgeDocumentJSON(raw)
	if err != nil {
		return err
	}
	object := decoded.(map[string]any)
	result := KnowledgeDocumentLocator{Scheme: object["scheme"].(string)}
	switch result.Scheme {
	case "pdf_page_v1":
		result.PageNumber, _ = object["page_number"].(json.Number).Int64()
	case "office_package_v1":
		result.Part = object["part"].(string)
		encoded, _ := json.Marshal(object["segments"])
		if err := json.Unmarshal(encoded, &result.Segments); err != nil {
			return err
		}
	case "derived_passage_v1":
		result.ChunkIndex, _ = object["chunk_index"].(json.Number).Int64()
		encoded, _ := json.Marshal(object["parent_locator"])
		if err := json.Unmarshal(encoded, &result.ParentLocator); err != nil {
			return err
		}
	}
	*value = result
	return nil
}

func (value KnowledgeDocumentOriginalPosition) MarshalJSON() ([]byte, error) {
	object := map[string]any{"kind": value.Kind}
	switch value.Kind {
	case "document_only":
		if value.PageNumber != 0 || value.SlideNumber != 0 {
			return nil, fmt.Errorf("non-document position fields")
		}
	case "pdf_page":
		if value.SlideNumber != 0 {
			return nil, fmt.Errorf("non-PDF position fields")
		}
		object["page_number"] = value.PageNumber
	case "pptx_slide":
		if value.PageNumber != 0 {
			return nil, fmt.Errorf("non-PPTX position fields")
		}
		object["slide_number"] = value.SlideNumber
	default:
		return nil, fmt.Errorf("unknown document original position")
	}
	return knowledgeDocumentMarshal("OriginalPosition", object)
}

func (value *KnowledgeDocumentOriginalPosition) UnmarshalJSON(raw []byte) error {
	if err := KnowledgeDocumentValidateJSON("OriginalPosition", raw); err != nil {
		return err
	}
	decoded, err := knowledgeDocumentJSON(raw)
	if err != nil {
		return err
	}
	object := decoded.(map[string]any)
	result := KnowledgeDocumentOriginalPosition{Kind: object["kind"].(string)}
	if number, ok := object["page_number"].(json.Number); ok {
		result.PageNumber, _ = number.Int64()
	}
	if number, ok := object["slide_number"].(json.Number); ok {
		result.SlideNumber, _ = number.Int64()
	}
	*value = result
	return nil
}

func (value KnowledgeDocumentOperationRecord) MarshalJSON() ([]byte, error) {
	if err := KnowledgeDocumentValidateJSON("OperationRecord", value.Raw); err != nil {
		return nil, err
	}
	return bytes.Clone(value.Raw), nil
}

func (value *KnowledgeDocumentOperationRecord) UnmarshalJSON(raw []byte) error {
	if err := KnowledgeDocumentValidateJSON("OperationRecord", raw); err != nil {
		return err
	}
	value.Raw = bytes.Clone(raw)
	return nil
}
