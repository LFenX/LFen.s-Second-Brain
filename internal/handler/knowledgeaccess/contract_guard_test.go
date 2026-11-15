package knowledgeaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func unitContracts(t *testing.T) (string, *contractSet) {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal("locked contract root unavailable")
	}
	c, err := loadContracts(root)
	if err != nil {
		t.Fatal("locked contract load failed")
	}
	return root, c
}

func TestContractLoadsExactNineRoutesAndPinnedExamples(t *testing.T) {
	root, c := unitContracts(t)
	if len(c.routes) != 9 || len(c.request) != 9 || len(c.response) != 9 {
		t.Fatal("S1 route/schema set is incomplete")
	}
	raw, err := readContract(root, "examples.candidate.json")
	if err != nil {
		t.Fatal("locked examples unavailable")
	}
	var examples struct {
		Examples []struct {
			Schema string          `json:"schema"`
			Value  json.RawMessage `json:"value"`
		} `json:"examples"`
	}
	if json.Unmarshal(raw, &examples) != nil {
		t.Fatal("locked examples invalid")
	}
	for _, example := range examples.Examples {
		if c.definitions[example.Schema] == nil {
			t.Fatal("locked example schema missing")
		}
		value, err := strictJSON(example.Value)
		if err != nil || c.definitions[example.Schema].Validate(value) != nil {
			t.Fatal("locked example fails its schema")
		}
	}
	for op, path := range s1Paths() {
		if c.routes[op] != path {
			t.Fatal("operation path drift")
		}
	}
	if c.request["listDepartments"] != nil || c.request["createCredential"] != nil {
		t.Fatal("out-of-S1 operation became callable")
	}
}

func TestContractRejectsTamperedBytesAndExternalRefs(t *testing.T) {
	root, _ := unitContracts(t)
	raw, err := readContract(root, "access.schema.candidate.json")
	if err != nil {
		t.Fatal("locked schema unavailable")
	}
	temp := t.TempDir()
	dir := filepath.Join(temp, "docs/knowledge-access-20261108-02")
	if os.MkdirAll(dir, 0700) != nil {
		t.Fatal("unit fixture setup failed")
	}
	raw[0] = ' '
	if os.WriteFile(filepath.Join(dir, "access.schema.candidate.json"), raw, 0600) != nil {
		t.Fatal("unit fixture write failed")
	}
	if _, err := loadContracts(temp); err != errContract {
		t.Fatal("altered schema bytes were accepted")
	}
	for _, ref := range []string{"https://example.invalid/schema", "file:///private/schema", "../other.json#/$defs/X", "#"} {
		if localRefs(map[string]any{"$ref": ref}) {
			t.Fatal("external/unknown reference was admitted")
		}
	}
}

func TestStrictJSONRejectsAmbiguousOrLossyInput(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"x":1,"x":2}`), []byte(`{"x":{"y":1,"y":2}}`), []byte(`{"x":9007199254740992}`), []byte(`{"x":1.5}`), []byte(`{"x":"\ud800"}`), []byte(`{"x":"\udc00"}`), []byte(`{"x":true} {}`), {'"', 0xff, '"'}} {
		if _, err := strictJSON(raw); err != errContract {
			t.Fatal("ambiguous JSON was accepted")
		}
	}
	if _, err := strictJSON([]byte(`{"x":"\ud83d\ude00","n":9007199254740991}`)); err != nil {
		t.Fatal("valid JSON was refused")
	}
	for _, token := range []string{"1.0000000000000001", "35.0000000000000001", "9007199254740991.1", "1e1000000000", "1e-1000000000"} {
		if _, err := strictJSON([]byte(`{"n":` + token + `}`)); err != errContract {
			t.Fatal("noninteger or unsafe numeric token was accepted")
		}
	}
	for _, test := range []struct {
		token string
		want  float64
	}{{"1.0", 1}, {"10e-1", 1}, {"1000000000000000000000e-21", 1}, {"-9007199254740991", -9007199254740991}, {"0e1000000000", 0}} {
		value, err := strictJSON([]byte(`{"n":` + test.token + `}`))
		if err != nil || value.(map[string]any)["n"] != test.want {
			t.Fatal("exact safe integer token was refused")
		}
	}
}

func TestResponseBindsStatusVersionOperationAndHeaders(t *testing.T) {
	_, c := unitContracts(t)
	id := strings.Repeat("a", 32)
	value := map[string]any{"operation": "read", "contract_version": accessVersion, "request_id": id, "data": nil, "code": "not_found", "message": "Knowledge access operation could not be completed", "outcome_known": true, "retry_same_key": false}
	raw, _ := json.Marshal(value)
	good := response{status: 404, header: responseHeaders(id), body: raw}
	if _, err := c.validateResponse("read", good); err != nil {
		t.Fatal("valid fixed error was refused")
	}
	for _, mutate := range []func(*response){func(r *response) { r.status = 200 }, func(r *response) { r.header.Set("Cache-Control", "public") }, func(r *response) { r.header.Set("ETag", "unit") }, func(r *response) { r.header.Set("X-Request-ID", strings.Repeat("b", 32)) }, func(r *response) { r.header.Set("X-Knowledge-Access-Version", "0.2") }, func(r *response) { r.status = 403 }, func(r *response) { r.body = bytes.Replace(r.body, []byte(`"read"`), []byte(`"search"`), 1) }} {
		bad := response{status: good.status, header: good.header.Clone(), body: append([]byte(nil), good.body...)}
		mutate(&bad)
		if _, err := c.validateResponse("read", bad); err != errContract {
			t.Fatal("response binding mismatch was accepted")
		}
	}
	value["source_body"] = "SYNTHETIC_DENIED"
	raw, _ = json.Marshal(value)
	good.body = raw
	if _, err := c.validateResponse("read", good); err != errContract {
		t.Fatal("error envelope leaked extra business data")
	}
	root, c := unitContracts(t)
	f := unitFixture(t, c, root)
	want := moduleData{ID: strings.Repeat("a", 32), Domain: f.DomainID, Name: "Synthetic binding", Version: 1, Enabled: true, Scope: f.Scope, Grants: []grantData{}, Policy: policyData{Domain: f.DomainID, Version: 1, Enabled: true}}
	if !moduleDefinitionMatches(want, want) {
		t.Fatal("valid module definition was refused")
	}
	for _, mutate := range []func(*moduleData){
		func(v *moduleData) { v.Domain = strings.Repeat("f", 32) },
		func(v *moduleData) { v.ID = strings.Repeat("e", 32) },
		func(v *moduleData) { v.Name += " changed" },
		func(v *moduleData) { v.Version++ },
		func(v *moduleData) { v.Enabled = false },
		func(v *moduleData) {
			var scope map[string]any
			_ = json.Unmarshal(v.Scope, &scope)
			scope["publication"].(map[string]any)["id"] = strings.Repeat("f", 64)
			v.Scope, _ = json.Marshal(scope)
		},
	} {
		bad := want
		mutate(&bad)
		raw, _ := json.Marshal(bad)
		value, err := strictJSON(raw)
		if err != nil || c.definitions["ModuleAdmin"].Validate(value) != nil || moduleDefinitionMatches(bad, want) {
			t.Fatal("schema-valid wrong module metadata was not isolated")
		}
	}
	summary := moduleSummary{ID: want.ID, Domain: want.Domain, Name: want.Name, Version: want.Version, Enabled: true, Targets: 2}
	if !moduleSummaryMatches(summary, want) {
		t.Fatal("valid list metadata was refused")
	}
	summary.Domain = strings.Repeat("f", 32)
	if moduleSummaryMatches(summary, want) {
		t.Fatal("wrong-domain list metadata was accepted")
	}
	grant := grantData{ID: strings.Repeat("b", 32), Issued: time.Now().UTC()}
	revoked := revokedData{ID: grant.ID, At: grant.Issued.Add(time.Second)}
	if !revokeMatches(revoked, grant) {
		t.Fatal("valid revoke binding was refused")
	}
	revoked.ID = strings.Repeat("c", 32)
	if revokeMatches(revoked, grant) {
		t.Fatal("wrong grant revoke metadata was accepted")
	}
	inspect := map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "module_id": want.ID, "mode": "read", "delivery": "web_only", "source_ids": nil}
	validInspect, _ := json.Marshal(inspect)
	duplicate := duplicateVersionBody(validInspect)
	var lenient any
	if c.validateRequest("inspect", validInspect) != nil || json.Unmarshal(duplicate, &lenient) != nil {
		t.Fatal("duplicate probe control is invalid")
	}
	repaired, _ := json.Marshal(lenient)
	if c.validateRequest("inspect", duplicate) != errContract || c.validateRequest("inspect", repaired) != nil {
		t.Fatal("duplicate probe has an independent schema fault")
	}
	validSearch, _ := json.Marshal(map[string]any{"contract_version": accessVersion, "view_id": strings.Repeat("d", 32), "query": "synthetic utf8 control", "limit": 1})
	badUTF8 := invalidQueryBody(validSearch)
	if c.validateRequest("search", validSearch) != nil || c.validateRequest("search", badUTF8) != errContract || c.validateRequest("search", bytes.ToValidUTF8(badUTF8, []byte("\ufffd"))) != nil {
		t.Fatal("UTF8 probe has an independent schema fault")
	}
}

func unitFixture(t *testing.T, c *contractSet, root string) setupFixture {
	t.Helper()
	raw, err := readContract(root, "examples.candidate.json")
	if err != nil {
		t.Fatal("locked examples unavailable")
	}
	var examples struct {
		Examples []struct {
			ID    string          `json:"id"`
			Value json.RawMessage `json:"value"`
		} `json:"examples"`
	}
	if json.Unmarshal(raw, &examples) != nil {
		t.Fatal("unit examples invalid")
	}
	var scope json.RawMessage
	var original readData
	for _, e := range examples.Examples {
		if e.ID == "EX01" {
			var v struct {
				Scope json.RawMessage `json:"scope"`
			}
			_ = json.Unmarshal(e.Value, &v)
			scope = v.Scope
		}
		if e.ID == "EX06" {
			var v struct {
				Data readData `json:"data"`
			}
			_ = json.Unmarshal(e.Value, &v)
			original = v.Data
		}
	}
	var cite citation
	_ = json.Unmarshal(original.Citation, &cite)
	units := make([]expectedUnit, 2)
	for i := range units {
		next := cite
		next.Range.Start = i * len(original.Text)
		next.Range.End = (i + 1) * len(original.Text)
		citationRaw, _ := json.Marshal(next)
		typed := original.Typed
		if i > 0 {
			typed = []byte(`{"members":[],"members_complete":false,"omission_reasons":["typed_payload_not_exposed_in_access_v03"]}`)
		}
		units[i] = expectedUnit{Target: cite.Target, Citation: citationRaw, Text: original.Text, TypedContext: typed, RawRowID: strings.Repeat(string(rune('a'+i)), 64), Binding: "unit-binding", CanonicalRefSHA: strings.Repeat("c", 64)}
	}
	var secondTarget map[string]any
	_ = json.Unmarshal(cite.Target, &secondTarget)
	secondSource := secondTarget["source"].(map[string]any)
	secondSource["source_id"] = strings.Repeat("8", 32)
	secondSource["source_revision_id"] = strings.Repeat("9", 32)
	secondRaw, _ := json.Marshal(secondTarget)
	var moduleScope map[string]any
	_ = json.Unmarshal(scope, &moduleScope)
	moduleScope["targets"] = append(moduleScope["targets"].([]any), secondTarget)
	scope, _ = json.Marshal(moduleScope)
	secondCitation := cite
	secondCitation.Target = secondRaw
	secondCitationRaw, _ := json.Marshal(secondCitation)
	units = append(units, expectedUnit{Target: secondRaw, Citation: secondCitationRaw, Text: original.Text, TypedContext: original.Typed, RawRowID: strings.Repeat("c", 64), Binding: "unit-second-binding", CanonicalRefSHA: strings.Repeat("d", 64)})
	f := setupFixture{Prepared: true, DedicatedSynthetic: true, ExistingDataProtected: true, Owner: actorFixture{UserID: strings.Repeat("1", 32), TenantID: strings.Repeat("2", 32)}, Recipient: actorFixture{UserID: strings.Repeat("3", 32), TenantID: strings.Repeat("2", 32)}, Foreign: actorFixture{UserID: strings.Repeat("4", 32), TenantID: strings.Repeat("5", 32)}, DomainID: "00000000000000000000000000000064", ForeignDomainID: strings.Repeat("6", 32), AbsentID: strings.Repeat("7", 32), Scope: scope, Units: units, SearchQuery: "合成A", SearchRows: []string{units[0].RawRowID, units[1].RawRowID}, DeniedRowIDs: []string{strings.Repeat("0", 64)}, Forbidden: []string{"SYNTHETIC_NEVER_RETURN"}, NativeModelValues: map[string]string{"asr_id": "", "embd_id": "", "img2txt_id": "", "llm_id": "", "rerank_id": "", "tts_id": ""}}
	if validateFixture(f, c) != nil {
		t.Fatal("valid inert unit fixture rejected")
	}
	return f
}

func TestRAMInputRequiresExactPublicBindingAndThreeDistinctSessions(t *testing.T) {
	root, c := unitContracts(t)
	f := unitFixture(t, c, root)
	fixture, _ := json.Marshal(f)
	scope := publicScope{Purpose: "ROOT_ACCESS02_S1_SYNTHETIC_ACCEPTANCE", RunID: "unit-only-not-runtime", Origin: "http://127.0.0.1:19001", NotBefore: time.Now().Add(-time.Second), ExpiresAt: time.Now().Add(time.Minute), Synthetic: true, Mutations: true, SubjectSHA: subjectSHA, FixtureSHA: digest(fixture), SetupReceiptSHA: strings.Repeat("d", 64)}
	scopeRaw, _ := json.Marshal(scope)
	wire := ramInput{Scope: scopeRaw, Fixture: fixture, Sessions: guardAuth()}
	input, _ := json.Marshal(wire)
	if _, err := loadRun(bytes.NewReader(input), digest(scopeRaw), c); err != nil {
		t.Fatal("inert RAM input failed")
	}
	if _, err := loadRun(bytes.NewReader(input), strings.Repeat("f", 64), c); err != errScope {
		t.Fatal("unbound public scope accepted")
	}
	wire.Sessions["recipient"] = wire.Sessions["owner"]
	input, _ = json.Marshal(wire)
	if _, err := loadRun(bytes.NewReader(input), digest(scopeRaw), c); err != errScope {
		t.Fatal("reused native session accepted")
	}
	wire.Sessions = guardAuth()
	scope.Mutations, scope.NativeModels = false, true
	scopeRaw, _ = json.Marshal(scope)
	wire.Scope = scopeRaw
	input, _ = json.Marshal(wire)
	run, err := loadRun(bytes.NewReader(input), digest(scopeRaw), c)
	if err != nil || run.client.scope.s1Mutations || !run.client.scope.nativeModels {
		t.Fatal("native-only RAM permission was not preserved")
	}
	calls := 0
	run.client.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errExchange })
	if _, err := run.client.post(context.Background(), "owner", "inspect", f.DomainID, []byte(`{}`)); err != errScope || calls != 0 {
		t.Fatal("native-only RAM admission reached an S1 transport")
	}
	scope.NativeModels = false
	scopeRaw, _ = json.Marshal(scope)
	wire.Scope = scopeRaw
	input, _ = json.Marshal(wire)
	if _, err := loadRun(bytes.NewReader(input), digest(scopeRaw), c); err != errScope {
		t.Fatal("an input with no action permission was accepted")
	}
	scope.Mutations = true
	f.Prepared = false
	wire.Fixture, _ = json.Marshal(f)
	scope.FixtureSHA = digest(wire.Fixture)
	scopeRaw, _ = json.Marshal(scope)
	wire.Scope = scopeRaw
	input, _ = json.Marshal(wire)
	if _, err := loadRun(bytes.NewReader(input), digest(scopeRaw), c); err != errScope {
		t.Fatal("unprepared fixture accepted")
	}
}

func TestFixtureAndReadChecksDetectRangeRefAndUTF8Tampering(t *testing.T) {
	root, c := unitContracts(t)
	f := unitFixture(t, c, root)
	unit := f.Units[0]
	view := strings.Repeat("d", 32)
	next := 1
	data := readData{Ref: evidenceRef{Version: accessVersion, ViewID: view, EvidenceID: evidenceDigest(view, unit.RawRowID, unit.Binding, unit.CanonicalRefSHA)}, Citation: unit.Citation, Text: unit.Text, ByteLength: len(unit.Text), Encoding: "utf-8", Complete: true, Typed: unit.TypedContext, Attachments: "not_exposed", ParentExpansion: "disabled", Locator: unitLocator{ViewID: view, Target: 0, Ordinal: 0, Count: 2, Next: &next}}
	if validateRead(data, unit, view, 0, 0, 2) != nil {
		t.Fatal("valid unit/ref binding refused")
	}
	var cite citation
	if json.Unmarshal(unit.Citation, &cite) != nil {
		t.Fatal("unit citation unavailable")
	}
	needle := []byte(`"end_byte":` + strconv.Itoa(cite.Range.End))
	badCitation := bytes.Replace(unit.Citation, needle, append(append([]byte(nil), needle...), []byte(".0000000000000001")...), 1)
	if bytes.Equal(badCitation, unit.Citation) || sameJSON(badCitation, unit.Citation) {
		t.Fatal("fractional citation range was rounded to the expected integer")
	}
	for _, change := range []func(*readData){func(d *readData) { d.ByteLength-- }, func(d *readData) { d.Ref.ViewID = strings.Repeat("e", 32) }, func(d *readData) { d.Ref.EvidenceID = strings.Repeat("e", 64) }, func(d *readData) { d.Text += "x" }, func(d *readData) { d.Locator.Next = nil }, func(d *readData) { d.Complete = false }} {
		bad := data
		change(&bad)
		if validateRead(bad, unit, view, 0, 0, 2) != errContract {
			t.Fatal("tampered read accepted")
		}
	}
	if noPrivateFields(map[string]any{"typed_context": map[string]any{"Attributes": "forbidden"}}) {
		t.Fatal("private projection field accepted")
	}
	bad := f
	bad.SearchRows = []string{f.SearchRows[1], f.SearchRows[0]}
	if validateFixture(bad, c) != errScope {
		t.Fatal("unordered expected search rows accepted")
	}
}
