package knowledgeaccess

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
)

const accessVersion = "0.3.0-candidate"

var errContract = errors.New("acceptance_contract_refused")

var contractPins = map[string]string{
	"access.schema.candidate.json":      "805eed9e6b9bdb9472bd18ad627b8ae619b34c6757b4b454162c1a5687618d5a",
	"access.openapi.candidate.json":     "cf9d31417af65ae85814ef3427d0b49718442d0ed2fae3508785058bcd1afe4f",
	"protocol.candidate.json":           "04191b440e165aa38e9d35ef2e800111516410de5efa025edccc258f6713f060",
	"examples.candidate.json":           "bbe3b2011389afe8c149c85602ccfcd54fc03ed1ed025865a873f723fc891939",
	"synthetic-fixtures.candidate.json": "d913e459d269f9fdbf79c9103d26b99ca2386fc9b9630732184b86446f2306ee",
}

type contractSet struct {
	routes      map[string]string
	request     map[string]*jsonschema.Resolved
	response    map[string]*jsonschema.Resolved
	definitions map[string]*jsonschema.Resolved
	statuses    map[string]map[int]bool
	errorCodes  map[int]map[string]bool
}

type schemaRef struct {
	Ref string `json:"$ref"`
}
type mediaSchema struct {
	Schema schemaRef `json:"schema"`
}
type operationDoc struct {
	OperationID string `json:"operationId"`
	RequestBody struct {
		Content map[string]mediaSchema `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]mediaSchema `json:"content"`
	} `json:"responses"`
}

func repositoryRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", errContract
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(wd, "docs/knowledge-access-20261108-02/access.schema.candidate.json")); err == nil {
				return wd, nil
			}
		}
		next := filepath.Dir(wd)
		if next == wd {
			return "", errContract
		}
		wd = next
	}
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func readContract(root, name string) ([]byte, error) {
	pin, ok := contractPins[name]
	if !ok {
		return nil, errContract
	}
	raw, err := os.ReadFile(filepath.Join(root, "docs/knowledge-access-20261108-02", name))
	if err != nil || len(raw) > 1<<20 || digest(raw) != pin {
		return nil, errContract
	}
	return raw, nil
}

// Every reference is local to the exact pinned definitions. Resolve cannot
// discover a network/file schema, even if the library default loader changes.
func localRefs(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#/$defs/") {
					return false
				}
			}
			if !localRefs(child) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !localRefs(child) {
				return false
			}
		}
	}
	return true
}

func loadContracts(root string) (*contractSet, error) {
	raw, err := readContract(root, "access.schema.candidate.json")
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil || !localRefs(document) {
		return nil, errContract
	}
	defs, ok := document["$defs"].(map[string]any)
	if !ok {
		return nil, errContract
	}
	c := &contractSet{routes: s1Paths(), request: make(map[string]*jsonschema.Resolved), response: make(map[string]*jsonschema.Resolved), definitions: make(map[string]*jsonschema.Resolved), statuses: make(map[string]map[int]bool), errorCodes: make(map[int]map[string]bool)}
	for name := range defs {
		wrapper := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": defs, "$ref": "#/$defs/" + name}
		data, err := json.Marshal(wrapper)
		if err != nil {
			return nil, errContract
		}
		var schema jsonschema.Schema
		if json.Unmarshal(data, &schema) != nil {
			return nil, errContract
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			return nil, errContract
		}
		c.definitions[name] = resolved
	}
	var api struct {
		Paths map[string]struct {
			Post *operationDoc `json:"post"`
		} `json:"paths"`
	}
	raw, err = readContract(root, "access.openapi.candidate.json")
	if err != nil || json.Unmarshal(raw, &api) != nil {
		return nil, errContract
	}
	seen := make(map[string]bool)
	for route, item := range api.Paths {
		if item.Post == nil || !allowedOperation(item.Post.OperationID) {
			continue
		}
		op := item.Post.OperationID
		if seen[op] || route != c.routes[op] {
			return nil, errContract
		}
		seen[op] = true
		ref := item.Post.RequestBody.Content["application/json"].Schema.Ref
		if !strings.HasPrefix(ref, "./access.schema.candidate.json#/$defs/") {
			return nil, errContract
		}
		c.request[op] = c.definitions[strings.TrimPrefix(ref, "./access.schema.candidate.json#/$defs/")]
		c.statuses[op] = make(map[int]bool)
		for status, entry := range item.Post.Responses {
			n, err := strconv.Atoi(status)
			if err != nil {
				return nil, errContract
			}
			c.statuses[op][n] = true
			if n >= 200 && n < 300 {
				ref := entry.Content["application/json"].Schema.Ref
				if !strings.HasPrefix(ref, "./access.schema.candidate.json#/$defs/") {
					return nil, errContract
				}
				resolved := c.definitions[strings.TrimPrefix(ref, "./access.schema.candidate.json#/$defs/")]
				if resolved == nil {
					return nil, errContract
				}
				if prior := c.response[op]; prior != nil && prior != resolved {
					return nil, errContract
				}
				c.response[op] = resolved
			}
		}
		if c.request[op] == nil || c.response[op] == nil {
			return nil, errContract
		}
	}
	if len(seen) != 9 {
		return nil, errContract
	}
	var protocol struct {
		Version    string                                                             `json:"contract_version"`
		Operations []struct{ Operation, Path, Method, Stage, Input, Response string } `json:"operations"`
		Errors     map[string][]string                                                `json:"errors"`
	}
	raw, err = readContract(root, "protocol.candidate.json")
	if err != nil || json.Unmarshal(raw, &protocol) != nil || protocol.Version != accessVersion {
		return nil, errContract
	}
	count := 0
	for _, op := range protocol.Operations {
		if op.Stage != "S1_user_module_Web" {
			continue
		}
		count++
		if !allowedOperation(op.Operation) || op.Method != "POST" || op.Path != c.routes[op.Operation] || c.request[op.Operation] != c.definitions[op.Input] || c.response[op.Operation] != c.definitions[op.Response] {
			return nil, errContract
		}
	}
	if count != 9 {
		return nil, errContract
	}
	for status, codes := range protocol.Errors {
		n, err := strconv.Atoi(status)
		if err != nil {
			return nil, errContract
		}
		c.errorCodes[n] = make(map[string]bool)
		for _, code := range codes {
			c.errorCodes[n][code] = true
		}
	}
	return c, nil
}

func validEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		i++
		for i < len(raw) && raw[i] != '"' {
			if raw[i] == '\\' {
				i++
				if i >= len(raw) {
					return false
				}
				if raw[i] == 'u' {
					if i+4 >= len(raw) {
						return false
					}
					n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
					if err != nil {
						return false
					}
					i += 4
					if n >= 0xdc00 && n <= 0xdfff {
						return false
					}
					if n >= 0xd800 && n <= 0xdbff {
						if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
							return false
						}
						low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
						if err != nil || low < 0xdc00 || low > 0xdfff {
							return false
						}
						i += 6
					}
				}
			}
			i++
		}
		if i >= len(raw) {
			return false
		}
	}
	return true
}

// Check decimal digits before converting: a rounded float cannot establish
// that the original JSON token was an exact integer. Exponents saturate at an
// input-sized bound, so short huge exponents never allocate a huge power of ten.
func exactIntegerToken(raw string) (float64, error) {
	mantissa, exponentText := raw, ""
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		mantissa, exponentText = raw[:i], raw[i+1:]
	}
	digits, fraction, first, last := 0, 0, -1, -1
	afterPoint := false
	for i := 0; i < len(mantissa); i++ {
		b := mantissa[i]
		if b == '-' {
			continue
		}
		if b == '.' {
			afterPoint = true
			continue
		}
		if b != '0' {
			if first < 0 {
				first = digits
			}
			last = digits
		}
		digits++
		if afterPoint {
			fraction++
		}
	}
	if first < 0 {
		return 0, nil
	}
	exponent, sign, bound := 0, 1, len(raw)+16
	for i := 0; i < len(exponentText); i++ {
		b := exponentText[i]
		if i == 0 && (b == '+' || b == '-') {
			if b == '-' {
				sign = -1
			}
			continue
		}
		n := int(b - '0')
		if exponent > (bound-n)/10 {
			exponent = bound
			break
		}
		exponent = exponent*10 + n
	}
	scale := sign*exponent - fraction + digits - last - 1
	if scale < 0 || last-first+1+scale > 16 {
		return 0, errContract
	}
	var integer int64
	position := 0
	for i := 0; i < len(mantissa); i++ {
		b := mantissa[i]
		if b == '-' || b == '.' {
			continue
		}
		if position >= first && position <= last {
			integer = integer*10 + int64(b-'0')
		}
		position++
	}
	for ; scale > 0; scale-- {
		integer *= 10
	}
	if integer > 9007199254740991 {
		return 0, errContract
	}
	if raw[0] == '-' {
		integer = -integer
	}
	return float64(integer), nil
}

func strictValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errContract
	}
	token, err := d.Token()
	if err != nil {
		return nil, errContract
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			value := make(map[string]any)
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok {
					return nil, errContract
				}
				if _, ok := value[name]; ok {
					return nil, errContract
				}
				child, err := strictValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				value[name] = child
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return nil, errContract
			}
			return value, nil
		case '[':
			value := []any{}
			for d.More() {
				child, err := strictValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				value = append(value, child)
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return nil, errContract
			}
			return value, nil
		default:
			return nil, errContract
		}
	}
	if number, ok := token.(json.Number); ok {
		return exactIntegerToken(number.String())
	}
	return token, nil
}

func strictJSON(raw []byte) (any, error) {
	if len(raw) > maxResponseBytes || !utf8.Valid(raw) || !validEscapes(raw) {
		return nil, errContract
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := strictValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errContract
	}
	return value, nil
}

func (c *contractSet) validateRequest(op string, raw []byte) error {
	value, err := strictJSON(raw)
	if err != nil || c.request[op] == nil {
		return errContract
	}
	if c.request[op].Validate(value) != nil {
		return errContract
	}
	return nil
}

type envelope struct {
	Operation    string          `json:"operation"`
	Version      string          `json:"contract_version"`
	RequestID    string          `json:"request_id"`
	Data         json.RawMessage `json:"data"`
	Code         string          `json:"code"`
	Message      string          `json:"message"`
	OutcomeKnown bool            `json:"outcome_known"`
	RetrySameKey bool            `json:"retry_same_key"`
}

func isHex(value string, n int) bool {
	if len(value) != n || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (c *contractSet) validateResponse(op string, r response) (envelope, error) {
	var empty envelope
	for _, key := range []string{"Content-Type", "Cache-Control", "X-Knowledge-Access-Version", "X-Request-ID"} {
		if len(r.header.Values(key)) != 1 {
			return empty, errContract
		}
	}
	media, parameters, err := mime.ParseMediaType(r.header.Get("Content-Type"))
	if charset := parameters["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
		return empty, errContract
	}
	if err != nil || media != "application/json" || r.header.Get("Cache-Control") != "no-store" || r.header.Get("X-Knowledge-Access-Version") != accessVersion || len(r.header.Values("ETag")) != 0 || len(r.header.Values("Last-Modified")) != 0 || len(r.header.Values("Location")) != 0 || len(r.body) > maxResponseBytes || !c.statuses[op][r.status] {
		return empty, errContract
	}
	value, err := strictJSON(r.body)
	if err != nil {
		return empty, errContract
	}
	if c.response[op] == nil || c.response[op].Validate(value) != nil {
		return empty, errContract
	}
	var e envelope
	if json.Unmarshal(r.body, &e) != nil || e.Operation != op || e.Version != accessVersion || !isHex(e.RequestID, 32) || r.header.Get("X-Request-ID") != e.RequestID {
		return empty, errContract
	}
	if r.status >= 200 && r.status < 300 {
		if e.Code != "" || bytes.Equal(e.Data, []byte("null")) {
			return empty, errContract
		}
	} else {
		if !c.errorCodes[r.status][e.Code] || !bytes.Equal(e.Data, []byte("null")) || e.Message != "Knowledge access operation could not be completed" {
			return empty, errContract
		}
		if !e.OutcomeKnown && (r.status != 503 || !e.RetrySameKey || (op != "setPolicy" && op != "setModule" && op != "createGrant" && op != "revokeGrant")) {
			return empty, errContract
		}
		if e.OutcomeKnown && e.RetrySameKey {
			return empty, errContract
		}
	}
	return e, nil
}

// Refs bind the new view to private, frozen owning-row proof inputs. The raw
// row/binding/owner ref never becomes an HTTP selector or report diagnostic.
func evidenceDigest(view, row, binding, canonicalRefSHA string) string {
	parts := []string{"knowledge-access-evidence/0.3.0", view, row, binding, canonicalRefSHA}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(parts)
	return digest(bytes.TrimSuffix(encoded.Bytes(), []byte("\n")))
}

func noPrivateFields(value any) bool {
	forbidden := map[string]bool{"publication": true, "manifest_sha256": true, "selection": true, "owner_user_id": true, "storage_owner_id": true, "tenant_epoch": true, "grant_id": true, "credential_id": true, "token": true, "verifier": true, "attributes": true, "Attributes": true, "payload": true, "TableData": true, "relationships": true, "attachment_id": true, "storage_path": true, "presigned_url": true, "score": true, "bm25": true, "cursor": true, "total_hits": true}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if forbidden[key] || !noPrivateFields(child) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !noPrivateFields(child) {
				return false
			}
		}
	}
	return true
}

func responseHeaders(id string) http.Header {
	return http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}, "X-Knowledge-Access-Version": {accessVersion}, "X-Request-Id": {id}}
}

func noEcho(value any, markers []string) bool {
	switch v := value.(type) {
	case string:
		for _, marker := range markers {
			if marker != "" && strings.Contains(v, marker) {
				return false
			}
		}
	case map[string]any:
		for key, child := range v {
			if !noEcho(key, markers) || !noEcho(child, markers) {
				return false
			}
		}
	case []any:
		for _, child := range v {
			if !noEcho(child, markers) {
				return false
			}
		}
	case http.Header:
		for _, values := range v {
			for _, child := range values {
				if !noEcho(child, markers) {
					return false
				}
			}
		}
	}
	return true
}
