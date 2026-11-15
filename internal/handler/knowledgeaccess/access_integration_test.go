//go:build integration

package knowledgeaccess

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var integrationInput struct {
	once      sync.Once
	run       *preparedRun
	contracts *contractSet
	err       error
}

func admittedRun(t *testing.T) (*preparedRun, *contractSet) {
	t.Helper()
	// This guard is secondary to the integration build tag. No live-service
	// test exists in the default unit selection.
	if os.Getenv("LFEN_ACCESS_RUNTIME") != "root-approved-synthetic" {
		t.Skip("runtime_scope_not_granted")
	}
	integrationInput.once.Do(func() {
		expectedSHA, origin := os.Getenv("LFEN_ACCESS_SCOPE_SHA256"), os.Getenv("LFEN_ACCESS_ORIGIN")
		if _, err := localOrigin(origin); err != nil || !isHex(expectedSHA, 64) {
			integrationInput.err = errScope
			return
		}
		root, err := repositoryRoot()
		if err != nil {
			integrationInput.err = errScope
			return
		}
		contracts, err := loadContracts(root)
		if err != nil {
			integrationInput.err = errContract
			return
		}
		stream, err := openRAMPipe(os.Getenv("LFEN_ACCESS_RAM_PIPE"))
		if err != nil {
			integrationInput.err = errScope
			return
		}
		run, err := readAdmittedRAM(stream, expectedSHA, contracts)
		if err != nil {
			integrationInput.err = errScope
			return
		}
		if run.scope.Origin != origin {
			integrationInput.err = errScope
			return
		}
		integrationInput.run, integrationInput.contracts = run, contracts
	})
	if integrationInput.err != nil || integrationInput.run == nil {
		t.Fatal("runtime_input_refused")
	}
	run := integrationInput.run
	t.Cleanup(func() {
		if t.Failed() {
			run.client.mu.Lock()
			if run.client.stopped == nil {
				run.client.stopped = errScope
			}
			run.client.mu.Unlock()
		}
	})
	return run, integrationInput.contracts
}

type blackBox struct {
	t         *testing.T
	run       *preparedRun
	contracts *contractSet
}

func (s blackBox) check(id string, condition bool) {
	s.t.Helper()
	if !condition {
		s.t.Fatalf("acceptance_assertion_failed:%s", id)
	}
}
func (s blackBox) key(label string) string {
	return "s1-" + digest([]byte(s.run.scope.RunID+":"+label))
}
func (s blackBox) encode(value any) []byte {
	s.t.Helper()
	raw, err := json.Marshal(value)
	s.check("LOCAL_JSON", err == nil)
	return raw
}

func (s blackBox) call(id, actor, op string, body any, status int, code string) envelope {
	s.t.Helper()
	raw := s.encode(body)
	s.check(id+"_REQUEST_SCHEMA", s.contracts.validateRequest(op, raw) == nil)
	return s.raw(id, actor, op, raw, status, code)
}

func (s blackBox) raw(id, actor, op string, body []byte, status int, code string) envelope {
	s.t.Helper()
	r, err := s.run.client.post(context.Background(), actor, op, s.run.fixture.DomainID, body)
	s.check(id+"_EXCHANGE", err == nil)
	e, err := s.contracts.validateResponse(op, r)
	s.check(id+"_RESPONSE_CONTRACT", err == nil)
	if status >= 0 {
		s.check(id+"_HTTP_STATUS", r.status == status)
	}
	if code != "" {
		s.check(id+"_ERROR_CODE", e.Code == code)
	}
	if status < 0 {
		s.check(id+"_DENIED", e.Code == "not_found" || e.Code == "scope_changed" || e.Code == "view_stale")
	}
	// Authorization, raw owning references and denied-source sentinels are
	// inspected in RAM only; failure output contains the static case label.
	decoded, _ := strictJSON(r.body)
	s.check(id+"_AUTH_NO_ECHO", noEcho(decoded, s.run.client.secretMarkers()) && noEcho(r.header, s.run.client.secretMarkers()))
	if op == "inspect" || op == "read" || op == "search" || op == "listModules" {
		value, err := strictJSON(r.body)
		s.check(id+"_CLOSED_PROJECTION", err == nil && noPrivateFields(value))
		s.check(id+"_DENIED_MARKER", noEcho(value, s.run.fixture.Forbidden) && noEcho(r.header, s.run.fixture.Forbidden))
	}
	return e
}

func decodeData[T any](s blackBox, id string, e envelope) T {
	s.t.Helper()
	var value T
	s.check(id, json.Unmarshal(e.Data, &value) == nil)
	return value
}

type catalogue struct {
	View     string    `json:"view_id"`
	Domain   string    `json:"domain_id"`
	Module   string    `json:"module_id"`
	Version  int64     `json:"module_version"`
	Mode     string    `json:"mode"`
	Delivery string    `json:"delivery"`
	Expires  time.Time `json:"expires_at"`
	Targets  []struct {
		Target json.RawMessage `json:"target"`
		Count  int             `json:"unit_count"`
	} `json:"targets"`
	SourceCount int `json:"source_count"`
	UnitCount   int `json:"unit_count"`
}
type searchData struct {
	View  string `json:"view_id"`
	Query string `json:"query"`
	Limit int    `json:"requested_limit"`
	Count int    `json:"returned_count"`
	Hits  []struct {
		Ref      evidenceRef     `json:"evidence_ref"`
		Citation json.RawMessage `json:"citation"`
		Text     string          `json:"text"`
		Bytes    int             `json:"byte_length"`
		Rank     int             `json:"rank_ordinal"`
	} `json:"hits"`
}

func (s blackBox) list(id, actor, tenant string) moduleList {
	e := s.call(id, actor, "listModules", map[string]any{"contract_version": accessVersion, "tenant_id": tenant, "domain_id": s.run.fixture.DomainID}, 200, "")
	value := decodeData[moduleList](s, id+"_DATA", e)
	for _, item := range value.Items {
		s.check(id+"_ITEM_DOMAIN", item.Domain == s.run.fixture.DomainID)
	}
	for _, policy := range value.Policies {
		s.check(id+"_POLICY_DOMAIN", policy.Domain == s.run.fixture.DomainID)
	}
	return value
}

func (s blackBox) policy(id string, version int64, enabled bool, label string) envelope {
	e := s.call(id, "owner", "setPolicy", map[string]any{"contract_version": accessVersion, "domain_id": s.run.fixture.DomainID, "expected_version": version, "enabled": enabled, "confirmed": true, "idempotency_key": s.key(label)}, 200, "")
	value := decodeData[policyData](s, id+"_DATA_BINDING", e)
	// These calls request a state change or replay that same known change;
	// a receipt is bound to its original expected version, not today's row.
	s.check(id+"_REQUEST_BINDING", value == (policyData{Domain: s.run.fixture.DomainID, Version: version + 1, Enabled: enabled}))
	return e
}

func (s blackBox) moduleBody(module *moduleData, enabled bool, label string) map[string]any {
	var id any
	version := int64(0)
	name := "Synthetic S1 acceptance"
	if module != nil {
		id = module.ID
		version = module.Version
		name = module.Name
	}
	return map[string]any{"contract_version": accessVersion, "domain_id": s.run.fixture.DomainID, "module_id": id, "expected_version": version, "name": name, "enabled": enabled, "scope": json.RawMessage(s.run.fixture.Scope), "confirmed": true, "idempotency_key": s.key(label)}
}

func (s blackBox) grantBody(module moduleData, subject, mode, label string) map[string]any {
	s.check("GRANT_KNOWN_SUBJECT", subject == s.run.fixture.Recipient.UserID || subject == s.run.fixture.Owner.UserID)
	return map[string]any{"contract_version": accessVersion, "domain_id": s.run.fixture.DomainID, "module_id": module.ID, "expected_module_version": module.Version, "subject": map[string]any{"kind": "user", "id": subject}, "mode": mode, "delivery": "web_only", "expires_at": time.Now().UTC().Truncate(time.Second).Add(20 * time.Minute).Format(time.RFC3339), "confirmed": true, "idempotency_key": s.key(label)}
}

func (s blackBox) grant(id string, module moduleData, subject, mode, label string) grantData {
	body := s.grantBody(module, subject, mode, label)
	g := decodeData[grantData](s, id+"_DATA", s.call(id, "owner", "createGrant", body, 201, ""))
	s.checkGrant(id, g, module, mode, body)
	return g
}

func (s blackBox) checkGrant(id string, g grantData, module moduleData, mode string, body map[string]any) {
	s.t.Helper()
	expiry, err := time.Parse(time.RFC3339Nano, body["expires_at"].(string))
	subject, ok := body["subject"].(map[string]any)
	subjectID, known := subject["id"].(string)
	s.check(id+"_BINDING", err == nil && ok && known && subject["kind"] == "user" && (subjectID == s.run.fixture.Recipient.UserID || subjectID == s.run.fixture.Owner.UserID) && g.Domain == s.run.fixture.DomainID && g.Module == module.ID && g.Version == module.Version && g.Subject.Kind == "user" && g.Subject.ID == subjectID && g.Mode == mode && g.Delivery == "web_only" && g.Revoked == nil && g.IssuedBy == s.run.fixture.Owner.UserID && g.Expires.Equal(expiry))
}

func (s blackBox) revoke(id string, grant grantData, label string) envelope {
	s.check(id+"_KNOWN_GRANT_DOMAIN", grant.Domain == s.run.fixture.DomainID)
	e := s.call(id, "owner", "revokeGrant", map[string]any{"contract_version": accessVersion, "domain_id": s.run.fixture.DomainID, "grant_id": grant.ID, "confirmed": true, "idempotency_key": s.key(label)}, 200, "")
	s.check(id+"_GRANT_BINDING", revokeMatches(decodeData[revokedData](s, id+"_DATA", e), grant))
	return e
}

func (s blackBox) inspectBody(module moduleData, mode string, sources any) map[string]any {
	return map[string]any{"contract_version": accessVersion, "domain_id": s.run.fixture.DomainID, "module_id": module.ID, "mode": mode, "delivery": "web_only", "source_ids": sources}
}

func (s blackBox) inspect(id string, module moduleData, mode string) catalogue {
	return s.inspectSources(id, module, mode, nil, s.run.fixture.Units)
}

func (s blackBox) inspectSources(id string, module moduleData, mode string, sources any, expectedUnits []expectedUnit) catalogue {
	started := time.Now()
	e := s.call(id, "recipient", "inspect", s.inspectBody(module, mode, sources), 200, "")
	view := decodeData[catalogue](s, id+"_DATA", e)
	s.check(id+"_BOUND_VIEW", view.Domain == s.run.fixture.DomainID && view.Module == module.ID && view.Version == module.Version && view.Mode == mode && view.Delivery == "web_only" && view.Expires.After(started) && !view.Expires.After(time.Now().Add(301*time.Second)))
	keys := []string{}
	total := 0
	for _, target := range view.Targets {
		key, err := targetKey(target.Target)
		s.check(id+"_TARGET", err == nil)
		keys = append(keys, key)
		expected := 0
		for _, unit := range expectedUnits {
			candidate, _ := targetKey(unit.Target)
			if candidate == key {
				expected++
			}
		}
		s.check(id+"_TARGET_COUNT", expected > 0 && target.Count == expected)
		total += target.Count
	}
	expectedKeys := map[string]bool{}
	for _, unit := range expectedUnits {
		key, _ := targetKey(unit.Target)
		expectedKeys[key] = true
	}
	s.check(id+"_CATALOGUE", sort.StringsAreSorted(keys) && len(keys) == len(expectedKeys) && view.SourceCount == len(keys) && view.UnitCount == total && total == len(expectedUnits))
	return view
}

func unitRequest(view string, target, ordinal int) map[string]any {
	return map[string]any{"contract_version": accessVersion, "selector": map[string]any{"kind": "unit", "view_id": view, "target_index": target, "unit_ordinal": ordinal}}
}
func refRequest(ref evidenceRef) map[string]any {
	return map[string]any{"contract_version": accessVersion, "selector": map[string]any{"kind": "evidence", "evidence_ref": ref}}
}

func (s blackBox) readUnit(id string, view catalogue, target, ordinal int) (readData, expectedUnit) {
	key, _ := targetKey(view.Targets[target].Target)
	units := []expectedUnit{}
	for _, unit := range s.run.fixture.Units {
		candidate, _ := targetKey(unit.Target)
		if candidate == key {
			units = append(units, unit)
		}
	}
	s.check(id+"_FIXTURE_ORDINAL", ordinal < len(units))
	data := decodeData[readData](s, id+"_DATA", s.call(id, "recipient", "read", unitRequest(view.View, target, ordinal), 200, ""))
	s.check(id+"_WHOLE_UNIT_REF_UTF8_TYPED", validateRead(data, units[ordinal], view.View, target, ordinal, len(units)) == nil)
	return data, units[ordinal]
}

func TestKnowledgeAccessS1(t *testing.T) {
	run, contracts := admittedRun(t)
	if !run.scope.Mutations {
		t.Skip("s1_scope_not_granted")
	}
	s := blackBox{t: t, run: run, contracts: contracts}
	f := run.fixture
	initial := s.list("DB04_DEFAULT_OFF", "owner", f.Owner.TenantID)
	s.check("DB04_FRESH_DOMAIN", len(initial.Items) == 0 && len(initial.Policies) == 1 && initial.Policies[0].Domain == f.DomainID && initial.Policies[0].Version == 0 && !initial.Policies[0].Enabled)
	s.check("ID02_NO_GRANT_LIST", len(s.list("ID02_NO_GRANT_LIST", "recipient", f.Recipient.TenantID).Items) == 0)
	s.call("ID07_FOREIGN_TENANT", "foreign", "listModules", map[string]any{"contract_version": accessVersion, "tenant_id": f.Owner.TenantID, "domain_id": f.DomainID}, 404, "not_found")
	s.call("ID05_RECIPIENT_CANNOT_MANAGE", "recipient", "setPolicy", map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "expected_version": 0, "enabled": true, "confirmed": true, "idempotency_key": s.key("denied-policy")}, 404, "not_found")
	createdBody := s.moduleBody(nil, true, "module-create")
	created := s.call("DB04_MODULE_WITH_POLICY_OFF", "owner", "setModule", createdBody, 200, "")
	module := decodeData[moduleData](s, "MODULE_CREATED", created)
	s.check("DB04_POLICY_STAYS_OFF", moduleDefinitionMatches(module, moduleData{ID: module.ID, Domain: f.DomainID, Name: "Synthetic S1 acceptance", Version: 1, Enabled: true, Scope: f.Scope}) && len(module.Grants) == 0 && module.Policy == (policyData{Domain: f.DomainID, Version: 0, Enabled: false}))
	replay := s.call("CR05_MODULE_CREATE_REPLAY", "owner", "setModule", createdBody, 200, "")
	s.check("CR05_ONE_MODULE_ID", sameJSON(created.Data, replay.Data) && created.RequestID != replay.RequestID)
	var foreignScope map[string]any
	s.check("SC12_LOCAL_FIXTURE", json.Unmarshal(f.Scope, &foreignScope) == nil)
	foreignScope["targets"].([]any)[0].(map[string]any)["source"].(map[string]any)["domain_id"] = f.ForeignDomainID
	foreignBody := s.moduleBody(nil, true, "foreign-target-denied")
	foreignBody["scope"] = foreignScope
	foreignResult := s.call("SC12_FOREIGN_TARGET_PUBLICATION_MEMBERSHIP", "owner", "setModule", foreignBody, 409, "")
	s.check("SC12_FIXED_ERROR", foreignResult.Code == "scope_not_supported" || foreignResult.Code == "integrity_error")
	s.call("DEFAULT_OFF_RECIPIENT_DENIED", "recipient", "inspect", s.inspectBody(module, "read", nil), 404, "not_found")
	enabled := s.policy("POLICY_ENABLE", 0, true, "policy-enable")
	policy := decodeData[policyData](s, "POLICY_DATA", enabled)
	s.check("POLICY_CAS_ONE", policy.Version == 1 && policy.Enabled)
	replay = s.policy("CR05_POLICY_REPLAY", 0, true, "policy-enable")
	s.check("CR05_POLICY_ORIGINAL_DATA", sameJSON(enabled.Data, replay.Data))
	s.call("CR05_DIFFERENT_BODY_CONFLICT", "owner", "setPolicy", map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "expected_version": 0, "enabled": false, "confirmed": true, "idempotency_key": s.key("policy-enable")}, 409, "idempotency_conflict")
	s.call("POLICY_STALE_CAS", "owner", "setPolicy", map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "expected_version": 0, "enabled": true, "confirmed": true, "idempotency_key": s.key("stale-policy")}, 409, "version_conflict")
	grantBody := s.grantBody(module, f.Recipient.UserID, "read", "grant-read")
	invalidGrant := make(map[string]any)
	for key, value := range grantBody {
		invalidGrant[key] = value
	}
	invalidGrant["expected_module_version"] = 0
	s.raw("CR05_PREVALIDATION_DOES_NOT_CONSUME_KEY", "owner", "createGrant", s.encode(invalidGrant), 400, "invalid_request")
	firstGrant := s.call("ID02_DIRECT_READ_GRANT", "owner", "createGrant", grantBody, 201, "")
	readGrant := decodeData[grantData](s, "READ_GRANT_DATA", firstGrant)
	s.checkGrant("ID02_DIRECT_READ_GRANT", readGrant, module, "read", grantBody)
	replay = s.call("CR05_GRANT_REPLAY", "owner", "createGrant", grantBody, 200, "")
	s.check("CR05_GRANT_ONE_ID", sameJSON(firstGrant.Data, replay.Data) && firstGrant.RequestID != replay.RequestID)
	changedGrant := make(map[string]any)
	for k, v := range grantBody {
		changedGrant[k] = v
	}
	changedGrant["mode"] = "read_search"
	s.call("CR05_GRANT_BODY_CONFLICT", "owner", "createGrant", changedGrant, 409, "idempotency_conflict")
	admin := decodeData[moduleData](s, "ADMIN_DATA", s.call("ADMIN_OWN_METADATA", "owner", "getModuleAdmin", map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "module_id": module.ID}, 200, ""))
	s.check("CR05_ONE_LIVE_GRANT", moduleDefinitionMatches(admin, module) && admin.Policy == policy && len(admin.Grants) == 1 && reflect.DeepEqual(admin.Grants[0], readGrant))
	ownerList := s.list("SC13_OWNER_METADATA_ONLY", "owner", f.Owner.TenantID)
	s.check("SC13_OWNER_NO_IMPLICIT_CONSUME", len(ownerList.Items) == 1 && moduleSummaryMatches(ownerList.Items[0], module) && ownerList.Items[0].Owner && len(ownerList.Items[0].Modes) == 0 && len(ownerList.Policies) == 1 && ownerList.Policies[0] == policy)
	list := s.list("ID02_FILTERED_MODULE_LIST", "recipient", f.Recipient.TenantID)
	s.check("ID02_ONLY_GRANTED_MODULE", len(list.Items) == 1 && moduleSummaryMatches(list.Items[0], module) && !list.Items[0].Owner && reflect.DeepEqual(list.Items[0].Modes, []string{"read"}) && len(list.Policies) == 0)
	s.call("ADMIN_RECIPIENT_DENIED", "recipient", "getModuleAdmin", map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "module_id": module.ID}, 404, "not_found")
	s.call("ID07_FOREIGN_INSPECT", "foreign", "inspect", s.inspectBody(module, "read", nil), 404, "not_found")
	view := s.inspect("READONLY_FIRST_CATALOGUE", module, "read")
	var first readData
	var firstUnit expectedUnit
	for target, entry := range view.Targets {
		for ordinal := 0; ordinal < entry.Count; ordinal++ {
			data, unit := s.readUnit("READONLY_TRAVERSE", view, target, ordinal)
			if target == 0 && ordinal == 0 {
				first, firstUnit = data, unit
			}
		}
	}
	s.check("READONLY_CLIENT_NO_SEARCH_REQUEST", run.client.operations["search"] == 0)
	reread := decodeData[readData](s, "REF_REREAD_DATA", s.call("SC03_EXACT_REF_REREAD", "recipient", "read", refRequest(first.Ref), 200, ""))
	s.check("SC03_EXACT_REF_BINDING", validateRead(reread, firstUnit, view.View, 0, 0, view.Targets[0].Count) == nil)
	s.call("SC13_OWNER_HAS_NO_SELF_GRANT", "owner", "inspect", s.inspectBody(module, "read", nil), 404, "not_found")
	s.call("SC13_OWNER_CANNOT_READ_RECIPIENT_REF", "owner", "read", refRequest(first.Ref), 404, "not_found")
	s.call("UNIT_TARGET_BOUNDS", "recipient", "read", unitRequest(view.View, len(view.Targets), 0), 400, "invalid_request")
	s.call("UNIT_ORDINAL_BOUNDS", "recipient", "read", unitRequest(view.View, 0, view.Targets[0].Count), 400, "invalid_request")
	s.call("SC03_FOREIGN_REF", "foreign", "read", refRequest(first.Ref), 404, "not_found")
	tampered := first.Ref
	tampered.EvidenceID = strings.Repeat("0", 64)
	s.call("SC03_TAMPERED_REF", "recipient", "read", refRequest(tampered), 404, "not_found")
	s.call("READONLY_SEARCH_DENIED", "recipient", "search", map[string]any{"contract_version": accessVersion, "view_id": view.View, "query": f.SearchQuery, "limit": 1}, 403, "action_denied")
	firstTargetKey, _ := targetKey(view.Targets[0].Target)
	firstSource := strings.Split(firstTargetKey, "\x00")[1]
	subsetUnits := []expectedUnit{}
	for _, unit := range f.Units {
		key, _ := targetKey(unit.Target)
		if strings.Split(key, "\x00")[1] == firstSource {
			subsetUnits = append(subsetUnits, unit)
		}
	}
	subsetView := s.inspectSources("SC08_POSITIVE_EXACT_SUBSET", module, "read", []string{firstSource}, subsetUnits)
	s.check("SC08_SUBSET_DOES_NOT_EXPAND", len(subsetView.Targets) == 1 && subsetView.SourceCount < view.SourceCount && subsetView.UnitCount < view.UnitCount && sameJSON(subsetView.Targets[0].Target, view.Targets[0].Target))
	s.check("SC03_DIFFERENT_COVERING_VIEWS_HAVE_TIME", subsetView.View != view.View && time.Until(view.Expires) > time.Minute && time.Until(subsetView.Expires) > time.Minute)
	subsetRead, subsetUnit := s.readUnit("SC08_SUBSET_COMMON_UNIT", subsetView, 0, 0)
	s.check("SC03_COMMON_UNIT_IN_BOTH_SCOPES", reflect.DeepEqual(subsetUnit, firstUnit))
	subsetReread := decodeData[readData](s, "SC08_SUBSET_REF_DATA", s.call("SC08_SUBSET_COMMON_REF_VALID", "recipient", "read", refRequest(subsetRead.Ref), 200, ""))
	s.check("SC08_SUBSET_REF_BINDING", validateRead(subsetReread, firstUnit, subsetView.View, 0, 0, subsetView.Targets[0].Count) == nil)
	fullReread := decodeData[readData](s, "SC03_FULL_REF_STILL_VALID_DATA", s.call("SC03_FULL_REF_STILL_VALID", "recipient", "read", refRequest(first.Ref), 200, ""))
	s.check("SC03_FULL_REF_STILL_BOUND", validateRead(fullReread, firstUnit, view.View, 0, 0, view.Targets[0].Count) == nil && time.Until(view.Expires) > 30*time.Second && time.Until(subsetView.Expires) > 30*time.Second)
	transposed := first.Ref
	transposed.ViewID = subsetView.View
	s.call("SC03_REF_CANNOT_CHANGE_VIEW", "recipient", "read", refRequest(transposed), 404, "not_found")
	outside := s.inspectBody(module, "read", []string{firstSource, f.AbsentID})
	s.call("SC08_UNKNOWN_SOURCE_REJECTS_WHOLE", "recipient", "inspect", outside, 404, "not_found")
	unknown := s.inspectBody(module, "read", nil)
	unknown["owner_user_id"] = f.Owner.UserID
	s.raw("SC04_UNKNOWN_FIELD", "recipient", "inspect", s.encode(unknown), 400, "invalid_request")
	wrongVersion := s.inspectBody(module, "read", nil)
	wrongVersion["contract_version"] = "0.2.0-candidate"
	s.raw("VERSION_NO_FALLBACK", "recipient", "inspect", s.encode(wrongVersion), 412, "contract_mismatch")
	validInspect := s.encode(s.inspectBody(module, "read", nil))
	s.check("STRICT_INSPECT_CONTROL", contracts.validateRequest("inspect", validInspect) == nil)
	s.raw("STRICT_DUPLICATE_JSON", "recipient", "inspect", duplicateVersionBody(validInspect), 400, "invalid_request")
	// The owner has no implicit consumption right, as checked above. This
	// explicit temporary self grant changes another subject's authorization.
	epochView := s.inspect("RV05_BASELINE_CATALOGUE", module, "read")
	epochRead, epochUnit := s.readUnit("RV05_BASELINE_UNIT", epochView, 0, 0)
	epochReread := decodeData[readData](s, "RV05_BASELINE_REF_DATA", s.call("RV05_BASELINE_REF_VALID", "recipient", "read", refRequest(epochRead.Ref), 200, ""))
	s.check("RV05_BASELINE_REF_BINDING_AND_TIME", validateRead(epochReread, epochUnit, epochView.View, 0, 0, epochView.Targets[0].Count) == nil && time.Until(epochView.Expires) > 2*time.Minute && time.Until(readGrant.Expires) > 2*time.Minute)
	epochGrant := s.grant("RV05_UNRELATED_OWNER_SELF_GRANT", module, f.Owner.UserID, "read", "grant-epoch-owner")
	unchanged := decodeData[moduleData](s, "RV05_CURRENT_ADMIN_DATA", s.call("RV05_CURRENT_ADMIN", "owner", "getModuleAdmin", map[string]any{"contract_version": accessVersion, "domain_id": f.DomainID, "module_id": module.ID}, 200, ""))
	s.check("RV05_MODULE_POLICY_SCOPE_UNCHANGED", moduleDefinitionMatches(unchanged, module) && unchanged.Policy == policy && len(unchanged.Grants) == 2)
	readUnchanged, ownerGrantBound := false, false
	for _, grant := range unchanged.Grants {
		if grant.ID == readGrant.ID {
			readUnchanged = reflect.DeepEqual(grant, readGrant)
		}
		if grant.ID == epochGrant.ID {
			ownerGrantBound = reflect.DeepEqual(grant, epochGrant)
		}
	}
	s.check("RV05_RECIPIENT_GRANT_UNCHANGED", readUnchanged && ownerGrantBound && epochGrant.Subject.ID != readGrant.Subject.ID && time.Until(epochView.Expires) > time.Minute)
	s.raw("RV05_COARSE_EPOCH_INVALIDATES_OLD_VIEW", "recipient", "read", s.encode(refRequest(epochRead.Ref)), -1, "")
	s.check("RV05_NOT_EXPIRY_INVALIDATION", time.Until(epochView.Expires) > 30*time.Second && time.Until(readGrant.Expires) > time.Minute)
	freshEpochView := s.inspect("RV05_SAME_GRANT_FRESH_CATALOGUE", module, "read")
	freshEpochRead, freshEpochUnit := s.readUnit("RV05_SAME_GRANT_FRESH_UNIT", freshEpochView, 0, 0)
	s.check("RV05_SAME_SCOPE_AND_UNIT_STILL_READABLE", freshEpochView.View != epochView.View && reflect.DeepEqual(freshEpochView.Targets, epochView.Targets) && freshEpochView.SourceCount == epochView.SourceCount && freshEpochView.UnitCount == epochView.UnitCount && reflect.DeepEqual(freshEpochUnit, epochUnit) && freshEpochRead.Text == epochRead.Text)
	s.revoke("RV05_CLEANUP_OWNER_SELF_GRANT", epochGrant, "revoke-epoch-owner")
	s.revoke("REVOKE_READ_GRANT", readGrant, "revoke-read")
	searchGrant := s.grant("SEARCH_GRANT", module, f.Recipient.UserID, "read_search", "grant-search")
	searchView := s.inspect("SEARCH_SCOPED_CATALOGUE", module, "read_search")
	for _, limit := range []int{1, 2, 2} {
		data := decodeData[searchData](s, "SCOPED_SEARCH_DATA", s.call("SE08_SE09_SCOPED_BEFORE_LIMIT", "recipient", "search", map[string]any{"contract_version": accessVersion, "view_id": searchView.View, "query": f.SearchQuery, "limit": limit}, 200, ""))
		s.check("SE08_NO_GLOBAL_COUNT", data.View == searchView.View && data.Query == f.SearchQuery && data.Limit == limit && data.Count == limit && len(data.Hits) == limit)
		for i, hit := range data.Hits {
			var expected expectedUnit
			for _, unit := range f.Units {
				if unit.RawRowID == f.SearchRows[i] {
					expected = unit
				}
			}
			s.check("SE09_STABLE_SCOPED_ORDER", hit.Ref.ViewID == searchView.View && hit.Ref.EvidenceID == evidenceDigest(searchView.View, expected.RawRowID, expected.Binding, expected.CanonicalRefSHA) && hit.Rank == i+1 && hit.Text == expected.Text && hit.Bytes == len([]byte(expected.Text)) && sameJSON(hit.Citation, expected.Citation))
		}
	}
	validSearch := s.encode(map[string]any{"contract_version": accessVersion, "view_id": searchView.View, "query": "synthetic utf8 control", "limit": 1})
	s.check("STRICT_SEARCH_CONTROL", contracts.validateRequest("search", validSearch) == nil)
	s.raw("STRICT_UTF8_JSON", "recipient", "search", invalidQueryBody(validSearch), 400, "invalid_request")
	badSearch := map[string]any{"contract_version": accessVersion, "view_id": searchView.View, "query": f.SearchQuery, "limit": 1, "embedding": true}
	s.raw("SE05_MODEL_FIELD_REJECTED", "recipient", "search", s.encode(badSearch), 400, "invalid_request")
	s.raw("QUERY_UTF8_BYTE_LIMIT", "recipient", "search", s.encode(map[string]any{"contract_version": accessVersion, "view_id": searchView.View, "query": strings.Repeat("合", 2000), "limit": 1}), 400, "invalid_request")
	revoked := s.revoke("RV_REVOKE_SEARCH_GRANT", searchGrant, "revoke-search")
	replay = s.revoke("CR05_REVOKE_REPLAY", searchGrant, "revoke-search")
	s.check("CR05_REVOKE_ORIGINAL_DATA", sameJSON(revoked.Data, replay.Data))
	s.raw("RV_REVOKED_VIEW_HAS_NO_BODY", "recipient", "read", s.encode(unitRequest(searchView.View, 0, 0)), -1, "")
	s.check("REVOKED_MODULE_DISAPPEARS", len(s.list("REVOKED_LIST", "recipient", f.Recipient.TenantID).Items) == 0)
	currentGrant := s.grant("MODULE_TEST_GRANT", module, f.Recipient.UserID, "read_search", "grant-module-test")
	oldView := s.inspect("MODULE_OLD_VIEW", module, "read")
	updateBody := s.moduleBody(&module, true, "module-update")
	updateBody["name"] = "Synthetic S1 acceptance updated"
	expectedModule := module
	expectedModule.Name, expectedModule.Version = "Synthetic S1 acceptance updated", 2
	module = decodeData[moduleData](s, "MODULE_UPDATED", s.call("RV10_MODULE_VERSION_CHANGE", "owner", "setModule", updateBody, 200, ""))
	s.check("RV10_VERSION_TWO", moduleDefinitionMatches(module, expectedModule) && len(module.Grants) == 0 && module.Policy == policy)
	s.raw("RV10_OLD_VIEW_DENIED", "recipient", "read", s.encode(unitRequest(oldView.View, 0, 0)), -1, "")
	s.check("RV10_GRANT_NOT_AUTO_UPGRADED", len(s.list("MODULE_CHANGED_LIST", "recipient", f.Recipient.TenantID).Items) == 0)
	s.revoke("REVOKE_OLD_VERSION_GRANT", currentGrant, "revoke-old-version")
	currentGrant = s.grant("MODULE_NEW_VERSION_GRANT", module, f.Recipient.UserID, "read_search", "grant-version-two")
	oldView = s.inspect("MODULE_ENABLED_VIEW", module, "read")
	expectedModule = module
	expectedModule.Version, expectedModule.Enabled = 3, false
	module = decodeData[moduleData](s, "MODULE_DISABLED", s.call("MODULE_DISABLE", "owner", "setModule", s.moduleBody(&module, false, "module-disable"), 200, ""))
	s.check("MODULE_DISABLE_VERSION", moduleDefinitionMatches(module, expectedModule) && len(module.Grants) == 0 && module.Policy == policy)
	s.raw("MODULE_DISABLED_VIEW_DENIED", "recipient", "read", s.encode(unitRequest(oldView.View, 0, 0)), -1, "")
	s.revoke("REVOKE_DISABLED_VERSION_GRANT", currentGrant, "revoke-disabled-version")
	expectedModule = module
	expectedModule.Version, expectedModule.Enabled = 4, true
	module = decodeData[moduleData](s, "MODULE_REENABLED", s.call("MODULE_ENABLE", "owner", "setModule", s.moduleBody(&module, true, "module-enable"), 200, ""))
	s.check("MODULE_REENABLE_NEW_VERSION", moduleDefinitionMatches(module, expectedModule) && len(module.Grants) == 0 && module.Policy == policy)
	currentGrant = s.grant("POLICY_TEST_GRANT", module, f.Recipient.UserID, "read_search", "grant-final")
	oldView = s.inspect("POLICY_ENABLED_VIEW", module, "read")
	policy = decodeData[policyData](s, "POLICY_DISABLED_DATA", s.policy("POLICY_DISABLE", policy.Version, false, "policy-disable"))
	s.check("POLICY_DISABLED", !policy.Enabled && policy.Version == 2)
	s.raw("POLICY_DISABLED_VIEW_DENIED", "recipient", "read", s.encode(unitRequest(oldView.View, 0, 0)), -1, "")
	replay = s.policy("CR05_STORED_RECEIPT_NOT_CURRENT_STATE", 0, true, "policy-enable")
	s.check("CR05_ORIGINAL_SAFE_RESULT", sameJSON(enabled.Data, replay.Data))
	moduleReplay := s.call("CR05_STORED_MODULE_RECEIPT", "owner", "setModule", createdBody, 200, "")
	s.check("CR05_ORIGINAL_MODULE_NOT_LATEST", sameJSON(created.Data, moduleReplay.Data))
	current := s.list("CR05_RELOAD_CURRENT_POLICY", "owner", f.Owner.TenantID)
	s.check("CR05_REPLAY_NO_SECOND_MUTATION", len(current.Policies) == 1 && current.Policies[0] == policy && len(current.Items) == 1 && moduleSummaryMatches(current.Items[0], module))
	policy = decodeData[policyData](s, "POLICY_REENABLED_DATA", s.policy("POLICY_ENABLE_FOR_EXPIRY", policy.Version, true, "policy-enable-expiry"))
	expiring := s.inspect("RV01_VIEW_EXPIRY_CAPTURE", module, "read")
	s.check("RV01_WAIT_FITS_LEASE", expiring.Expires.Add(time.Second).Before(run.scope.ExpiresAt) && time.Until(expiring.Expires) <= 301*time.Second)
	timer := time.NewTimer(time.Until(expiring.Expires.Add(25 * time.Millisecond)))
	defer timer.Stop()
	<-timer.C
	s.call("RV08_OWN_EXPIRED_VIEW", "recipient", "read", unitRequest(expiring.View, 0, 0), 409, "view_stale")
	fresh := s.inspect("EXPLICIT_REOPEN_AFTER_EXPIRY", module, "read")
	s.check("EXPIRED_VIEW_NOT_REVIVED", fresh.View != expiring.View)
	// Success-only cleanup uses the same approved mutations. A failed assertion
	// leaves first evidence/state for Root; no deferred network rollback runs.
	s.revoke("SUCCESS_CLEANUP_GRANT", currentGrant, "cleanup-grant")
	s.policy("SUCCESS_CLEANUP_SHARING_OFF", policy.Version, false, "cleanup-policy")
	t.Log("S1_HTTP_ASSERTIONS_COMPLETED_NOT_SQL_ATOMICITY_OR_FULL70_ACCEPTANCE")
}

func TestKnowledgeAccessNativeModels(t *testing.T) {
	run, _ := admittedRun(t)
	if !run.scope.NativeModels {
		t.Skip("native_models_scope_not_granted")
	}
	s := blackBox{t: t, run: run}
	f := run.fixture
	for _, test := range []struct {
		id, actor, tenant string
		unknown           bool
		code              int
	}{
		{"NATIVE_OWNER_SAME_VALUES", "owner", f.Owner.TenantID, false, 0},
		{"NATIVE_FOREIGN_TENANT_DENIED", "foreign", f.Owner.TenantID, false, 403},
		{"NATIVE_OWNER_FOREIGN_TARGET_DENIED", "owner", f.Foreign.TenantID, false, 403},
		{"NATIVE_NONOWNER_DENIED", "recipient", f.Owner.TenantID, false, 403},
		{"NATIVE_UNKNOWN_FIELD_DENIED", "owner", f.Owner.TenantID, true, 101},
	} {
		body := make(map[string]any)
		for key, value := range f.NativeModelValues {
			body[key] = value
		}
		body["tenant_id"] = test.tenant
		if test.unknown {
			body["acceptance_unknown_field"] = "SYNTHETIC_MUST_NOT_PERSIST"
		}
		r, err := run.client.patchModels(context.Background(), test.actor, s.encode(body))
		s.check(test.id+"_EXCHANGE", err == nil)
		s.check(test.id+"_HTTP", r.status == 200)
		value, err := strictJSON(r.body)
		s.check(test.id+"_JSON", err == nil)
		object, ok := value.(map[string]any)
		_, hasData := object["data"]
		s.check(test.id+"_ENVELOPE", ok && hasData && len(object) == 3)
		code, ok := object["code"].(float64)
		s.check(test.id+"_CODE", ok && code == float64(test.code))
		_, message := object["message"].(string)
		s.check(test.id+"_MESSAGE_TYPE", message)
		if test.code == 0 {
			s.check(test.id+"_SUCCESS", object["data"] == true)
		} else {
			s.check(test.id+"_NO_DATA", object["data"] == nil)
		}
		s.check(test.id+"_AUTH_NO_ECHO", noEcho(value, run.client.secretMarkers()) && noEcho(r.header, run.client.secretMarkers()))
	}
	t.Log("NATIVE_PATCH_HTTP_GUARDS_COMPLETED_DB_UNCHANGEDNESS_REQUIRES_OWNING_TEST")
}
