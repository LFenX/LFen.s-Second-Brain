package knowledgeaccess

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const subjectSHA = "a6d98e012a735c3a2dd75d72f6492fa9928c98b5ee14ea8b98c800e79fdad0e3"
const maxRAMInput = 256 << 10

type actorFixture struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
}

type publicScope struct {
	Purpose         string    `json:"purpose"`
	RunID           string    `json:"run_id"`
	Origin          string    `json:"origin"`
	NotBefore       time.Time `json:"not_before"`
	ExpiresAt       time.Time `json:"expires_at"`
	Synthetic       bool      `json:"synthetic"`
	Mutations       bool      `json:"allow_s1_mutations"`
	NativeModels    bool      `json:"allow_native_models_patch"`
	SubjectSHA      string    `json:"subject_sha256"`
	FixtureSHA      string    `json:"fixture_sha256"`
	SetupReceiptSHA string    `json:"setup_receipt_sha256"`
}

type expectedUnit struct {
	Target          json.RawMessage `json:"target"`
	Citation        json.RawMessage `json:"citation"`
	Text            string          `json:"text"`
	TypedContext    json.RawMessage `json:"typed_context"`
	RawRowID        string          `json:"raw_row_id"`
	Binding         string          `json:"binding"`
	CanonicalRefSHA string          `json:"canonical_reference_sha256"`
}

type setupFixture struct {
	Prepared              bool              `json:"prepared"`
	DedicatedSynthetic    bool              `json:"dedicated_synthetic"`
	ExistingDataProtected bool              `json:"existing_data_protected"`
	Owner                 actorFixture      `json:"owner"`
	Recipient             actorFixture      `json:"recipient"`
	Foreign               actorFixture      `json:"foreign"`
	DomainID              string            `json:"domain_id"`
	ForeignDomainID       string            `json:"foreign_domain_id"`
	AbsentID              string            `json:"reserved_absent_id"`
	Scope                 json.RawMessage   `json:"module_scope"`
	Units                 []expectedUnit    `json:"units"`
	SearchQuery           string            `json:"search_query"`
	SearchRows            []string          `json:"search_row_ids"`
	DeniedRowIDs          []string          `json:"denied_matching_row_ids"`
	Forbidden             []string          `json:"forbidden_markers"`
	NativeModelValues     map[string]string `json:"native_model_values"`
}

type ramInput struct {
	Scope    json.RawMessage   `json:"scope"`
	Fixture  json.RawMessage   `json:"fixture"`
	Sessions map[string]string `json:"sessions"`
}

type preparedRun struct {
	scope   publicScope
	fixture setupFixture
	client  *accessClient
}

func decodeClosed(raw []byte, value any) error {
	if _, err := strictJSON(raw); err != nil {
		return errScope
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return errScope
	}
	if d.Decode(new(any)) != io.EOF {
		return errScope
	}
	return nil
}

// Authentication is accepted only through the caller's bounded RAM stream.
// The public Root scope fingerprint binds exact raw scope and fixture bytes;
// no filename, URL, argv argument, env value or test log carries a session.
func loadRun(input io.Reader, expectedScopeSHA string, contracts *contractSet) (*preparedRun, error) {
	if !isHex(expectedScopeSHA, 64) {
		return nil, errScope
	}
	raw, err := io.ReadAll(io.LimitReader(input, maxRAMInput+1))
	if err != nil || len(raw) > maxRAMInput {
		return nil, errScope
	}
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	var wire ramInput
	if decodeClosed(raw, &wire) != nil || digest(wire.Scope) != expectedScopeSHA {
		return nil, errScope
	}
	var scope publicScope
	var fixture setupFixture
	if decodeClosed(wire.Scope, &scope) != nil || decodeClosed(wire.Fixture, &fixture) != nil {
		return nil, errScope
	}
	if scope.Purpose != "ROOT_ACCESS02_S1_SYNTHETIC_ACCEPTANCE" || scope.SubjectSHA != subjectSHA || !scope.Synthetic || (!scope.Mutations && !scope.NativeModels) || !isHex(scope.SetupReceiptSHA, 64) || digest(wire.Fixture) != scope.FixtureSHA {
		return nil, errScope
	}
	if validateFixture(fixture, contracts) != nil {
		return nil, errScope
	}
	c, err := newAccessClient(runScope{id: scope.RunID, origin: scope.Origin, notBefore: scope.NotBefore, expires: scope.ExpiresAt, synthetic: scope.Synthetic, s1Mutations: scope.Mutations, nativeModels: scope.NativeModels}, scope.Origin, wire.Sessions, contracts.routes, nil)
	if err != nil {
		return nil, errScope
	}
	return &preparedRun{scope: scope, fixture: fixture, client: c}, nil
}

func sameJSON(a, b []byte) bool {
	x, err := strictJSON(a)
	if err != nil {
		return false
	}
	y, err := strictJSON(b)
	return err == nil && reflect.DeepEqual(x, y)
}

type byteRange struct {
	Start int    `json:"start_byte"`
	End   int    `json:"end_byte"`
	SHA   string `json:"sha256"`
}
type citation struct {
	Target          json.RawMessage `json:"target"`
	NodeID          string          `json:"node_id"`
	CoordinateSpace string          `json:"coordinate_space"`
	Range           byteRange       `json:"range"`
	DisplayName     string          `json:"display_name"`
	PositionLabel   string          `json:"position_label"`
	RevisionBinding string          `json:"revision_binding"`
}
type typedMember struct {
	NodeID     string    `json:"node_id"`
	Kind       string    `json:"kind"`
	Range      byteRange `json:"range"`
	Status     string    `json:"text_status"`
	PayloadSHA string    `json:"payload_sha256"`
}
type typedContext struct {
	Members   []typedMember `json:"members"`
	Complete  bool          `json:"members_complete"`
	Omissions []string      `json:"omission_reasons"`
}
type evidenceRef struct {
	Version    string `json:"contract_version"`
	ViewID     string `json:"view_id"`
	EvidenceID string `json:"evidence_id"`
}
type unitLocator struct {
	ViewID  string `json:"view_id"`
	Target  int    `json:"target_index"`
	Ordinal int    `json:"unit_ordinal"`
	Count   int    `json:"target_unit_count"`
	Next    *int   `json:"next_unit_ordinal"`
}
type readData struct {
	Ref             evidenceRef     `json:"evidence_ref"`
	Citation        json.RawMessage `json:"citation"`
	Text            string          `json:"text"`
	ByteLength      int             `json:"byte_length"`
	Encoding        string          `json:"encoding"`
	Complete        bool            `json:"complete"`
	Typed           json.RawMessage `json:"typed_context"`
	Attachments     string          `json:"attachments"`
	ParentExpansion string          `json:"parent_expansion"`
	Locator         unitLocator     `json:"unit_locator"`
}

func targetKey(raw []byte) (string, error) {
	var value struct {
		Source struct {
			Domain   string `json:"domain_id"`
			Source   string `json:"source_id"`
			Revision string `json:"source_revision_id"`
			SHA      string `json:"source_sha256"`
		} `json:"source"`
		Representation struct {
			Kind     string  `json:"kind"`
			ID       string  `json:"id"`
			Producer *string `json:"producer_sha256"`
		} `json:"representation"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return "", errScope
	}
	producer := ""
	if value.Representation.Producer != nil {
		producer = *value.Representation.Producer
	}
	return strings.Join([]string{value.Source.Domain, value.Source.Source, value.Source.Revision, value.Source.SHA, value.Representation.Kind, value.Representation.ID, producer}, "\x00"), nil
}

func validateFixture(f setupFixture, c *contractSet) error {
	if !f.Prepared || !f.DedicatedSynthetic || !f.ExistingDataProtected || len(f.Units) < 2 || len(f.Units) > 8 || f.SearchQuery == "" || len([]byte(f.SearchQuery)) > 4096 || len(f.Forbidden) == 0 || len(f.Forbidden) > 16 {
		return errScope
	}
	ids := []string{f.Owner.UserID, f.Recipient.UserID, f.Foreign.UserID, f.DomainID, f.ForeignDomainID, f.AbsentID}
	seen := make(map[string]bool)
	for _, id := range ids {
		if !isHex(id, 32) || seen[id] {
			return errScope
		}
		seen[id] = true
	}
	if !isHex(f.Owner.TenantID, 32) || f.Owner.TenantID != f.Recipient.TenantID || !isHex(f.Foreign.TenantID, 32) || f.Foreign.TenantID == f.Owner.TenantID {
		return errScope
	}
	for _, marker := range f.Forbidden {
		if len(marker) < 8 || len(marker) > 512 {
			return errScope
		}
	}
	value, err := strictJSON(f.Scope)
	if err != nil || c.definitions["ModuleScope"].Validate(value) != nil {
		return errScope
	}
	var scope struct {
		Targets []json.RawMessage `json:"targets"`
	}
	if json.Unmarshal(f.Scope, &scope) != nil {
		return errScope
	}
	if len(scope.Targets) < 2 || len(scope.Targets) > 4 {
		return errScope
	}
	byTarget := make(map[string]int)
	for _, target := range scope.Targets {
		key, err := targetKey(target)
		if err != nil {
			return errScope
		}
		if !strings.HasPrefix(key, f.DomainID+"\x00") {
			return errScope
		}
		if strings.Split(key, "\x00")[1] == f.AbsentID {
			return errScope
		}
		byTarget[key] = 0
	}
	rows := make(map[string]bool)
	lastTarget := ""
	lastStart, lastEnd := -1, -1
	lastNode, lastRow := "", ""
	hasUTF8, hasEmptyMember, hasAvailableMember := false, false, false
	for _, unit := range f.Units {
		key, err := targetKey(unit.Target)
		if err != nil {
			return errScope
		}
		if _, ok := byTarget[key]; !ok {
			return errScope
		}
		byTarget[key]++
		if !isHex(unit.RawRowID, 64) || rows[unit.RawRowID] || !isHex(unit.CanonicalRefSHA, 64) || unit.Binding == "" || len(unit.Binding) > 4096 {
			return errScope
		}
		rows[unit.RawRowID] = true
		for _, b := range []byte(unit.Binding) {
			if b < 0x20 || b > 0x7e {
				return errScope
			}
		}
		if !utf8.ValidString(unit.Text) || len([]byte(unit.Text)) < 1 || len([]byte(unit.Text)) > 8192 {
			return errScope
		}
		if len([]byte(unit.Text)) != utf8.RuneCountInString(unit.Text) {
			hasUTF8 = true
		}
		cv, err := strictJSON(unit.Citation)
		if err != nil || c.definitions["Citation"].Validate(cv) != nil {
			return errScope
		}
		var cite citation
		if json.Unmarshal(unit.Citation, &cite) != nil || !sameJSON(cite.Target, unit.Target) || cite.Range.End-cite.Range.Start != len([]byte(unit.Text)) || digest([]byte(unit.Text)) != cite.Range.SHA {
			return errScope
		}
		if key < lastTarget || (key == lastTarget && (cite.Range.Start < lastStart || (cite.Range.Start == lastStart && (cite.Range.End < lastEnd || (cite.Range.End == lastEnd && (cite.NodeID < lastNode || (cite.NodeID == lastNode && unit.RawRowID <= lastRow))))))) {
			return errScope
		}
		lastTarget, lastStart, lastEnd, lastNode, lastRow = key, cite.Range.Start, cite.Range.End, cite.NodeID, unit.RawRowID
		tv, err := strictJSON(unit.TypedContext)
		if err != nil || c.definitions["TypedContext"].Validate(tv) != nil || validateTyped(unit.Text, cite.Range, unit.TypedContext) != nil {
			return errScope
		}
		var typed typedContext
		_ = json.Unmarshal(unit.TypedContext, &typed)
		for _, member := range typed.Members {
			if member.Status == "empty" {
				hasEmptyMember = true
			}
			if member.Status == "available" {
				hasAvailableMember = true
			}
		}
	}
	if !hasUTF8 || !hasEmptyMember || !hasAvailableMember {
		return errScope
	}
	multiUnit := false
	for _, count := range byTarget {
		if count == 0 {
			return errScope
		}
		if count > 1 {
			multiUnit = true
		}
	}
	if !multiUnit {
		return errScope
	}
	if len(f.SearchRows) < 2 || len(f.SearchRows) > len(f.Units) || !sort.StringsAreSorted(f.SearchRows) {
		return errScope
	}
	for i, row := range f.SearchRows {
		if !rows[row] || (i > 0 && row == f.SearchRows[i-1]) {
			return errScope
		}
	}
	if len(f.DeniedRowIDs) < 1 || len(f.DeniedRowIDs) > 64 {
		return errScope
	}
	first := f.SearchRows[0]
	betterDenied := false
	for _, row := range f.DeniedRowIDs {
		if !isHex(row, 64) || rows[row] {
			return errScope
		}
		if row < first {
			betterDenied = true
		}
	}
	if !betterDenied {
		return errScope
	}
	allowed := map[string]bool{"asr_id": true, "embd_id": true, "img2txt_id": true, "llm_id": true, "rerank_id": true, "tts_id": true}
	if len(f.NativeModelValues) != 6 {
		return errScope
	}
	for key, value := range f.NativeModelValues {
		if !allowed[key] || value != "" {
			return errScope
		}
	}
	return nil
}

func validateTyped(text string, outer byteRange, raw []byte) error {
	var ctx typedContext
	if json.Unmarshal(raw, &ctx) != nil {
		return errContract
	}
	for _, member := range ctx.Members {
		start, end := member.Range.Start-outer.Start, member.Range.End-outer.Start
		if start < 0 || end < start || end > len(text) || !utf8.ValidString(text[:start]) || !utf8.ValidString(text[:end]) || digest([]byte(text[start:end])) != member.Range.SHA {
			return errContract
		}
		if (member.Status == "empty") != (start == end) {
			return errContract
		}
	}
	return nil
}

func validateRead(data readData, unit expectedUnit, view string, target, ordinal, count int) error {
	if data.Ref.Version != accessVersion || data.Ref.ViewID != view || data.Ref.EvidenceID != evidenceDigest(view, unit.RawRowID, unit.Binding, unit.CanonicalRefSHA) || !sameJSON(data.Citation, unit.Citation) || data.Text != unit.Text || data.ByteLength != len([]byte(unit.Text)) || data.Encoding != "utf-8" || !data.Complete || data.Attachments != "not_exposed" || data.ParentExpansion != "disabled" || !sameJSON(data.Typed, unit.TypedContext) {
		return errContract
	}
	loc := data.Locator
	if loc.ViewID != view || loc.Target != target || loc.Ordinal != ordinal || loc.Count != count {
		return errContract
	}
	if ordinal+1 < count {
		if loc.Next == nil || *loc.Next != ordinal+1 {
			return errContract
		}
	} else if loc.Next != nil {
		return errContract
	}
	var cite citation
	if json.Unmarshal(data.Citation, &cite) != nil || validateTyped(data.Text, cite.Range, data.Typed) != nil {
		return errContract
	}
	return nil
}

type policyData struct {
	Domain  string `json:"domain_id"`
	Version int64  `json:"version"`
	Enabled bool   `json:"enabled"`
}
type grantData struct {
	ID      string `json:"grant_id"`
	Domain  string `json:"domain_id"`
	Module  string `json:"module_id"`
	Version int64  `json:"module_version"`
	Subject struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"subject"`
	Mode     string     `json:"mode"`
	Delivery string     `json:"delivery"`
	Issued   time.Time  `json:"issued_at"`
	Expires  time.Time  `json:"expires_at"`
	Revoked  *time.Time `json:"revoked_at"`
	IssuedBy string     `json:"issued_by_user_id"`
}
type moduleData struct {
	ID      string          `json:"module_id"`
	Domain  string          `json:"domain_id"`
	Name    string          `json:"name"`
	Version int64           `json:"version"`
	Enabled bool            `json:"enabled"`
	Scope   json.RawMessage `json:"scope"`
	Grants  []grantData     `json:"grants"`
	Policy  policyData      `json:"policy"`
}
type moduleSummary struct {
	ID      string   `json:"module_id"`
	Domain  string   `json:"domain_id"`
	Name    string   `json:"name"`
	Version int64    `json:"version"`
	Enabled bool     `json:"enabled"`
	Targets int      `json:"target_count"`
	Modes   []string `json:"modes"`
	Owner   bool     `json:"administrative_owner"`
	MCP     bool     `json:"mcp_delivery_allowed"`
}

type moduleList struct {
	Items    []moduleSummary `json:"items"`
	Policies []policyData    `json:"owned_domain_policies"`
}

func moduleDefinitionMatches(actual, expected moduleData) bool {
	for _, grant := range actual.Grants {
		if grant.Domain != actual.Domain || grant.Module != actual.ID || grant.Version != actual.Version {
			return false
		}
	}
	return actual.ID == expected.ID && actual.Domain == expected.Domain && actual.Name == expected.Name && actual.Version == expected.Version && actual.Enabled == expected.Enabled && sameJSON(actual.Scope, expected.Scope)
}

func moduleSummaryMatches(actual moduleSummary, expected moduleData) bool {
	var scope struct {
		Targets []json.RawMessage `json:"targets"`
	}
	return json.Unmarshal(expected.Scope, &scope) == nil && actual.ID == expected.ID && actual.Domain == expected.Domain && actual.Name == expected.Name && actual.Version == expected.Version && actual.Enabled == expected.Enabled && actual.Targets == len(scope.Targets) && !actual.MCP
}

type revokedData struct {
	ID string    `json:"grant_id"`
	At time.Time `json:"revoked_at"`
}

func revokeMatches(actual revokedData, expected grantData) bool {
	return actual.ID == expected.ID && !actual.At.IsZero() && !actual.At.Before(expected.Issued)
}

func duplicateVersionBody(valid []byte) []byte {
	return append([]byte(`{"contract_version":"`+accessVersion+`",`), valid[1:]...)
}

func invalidQueryBody(valid []byte) []byte {
	return bytes.Replace(valid, []byte("synthetic utf8 control"), append([]byte("synthetic utf8 control"), 0xff), 1)
}
