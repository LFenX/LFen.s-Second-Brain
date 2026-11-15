//go:build integration && linux

package knowledgeaccess

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

const fixtureAPIELF = "f83b6d38482068fd48a8c46c3571b3cf3fcd0308ce5f619b09e4f2dafd7e4dc0"
const fixtureContentVersion = "0.2.0-candidate"
const fixtureLeaseLimit = 15 * time.Minute
const fixtureHTTPAttempts = 120
const fixtureJSONBudget = 16 << 20
const fixtureResponseLimit = 96 << 10

var errFixture = errors.New("fixture_scope_or_exchange_refused")
var errFixtureNotReady = errors.New("fixture_owner_chain_not_ready")

type fixtureSafety struct {
	RegistrationEnabled bool "json:\"registration_enabled\""
	SMTPEnabled         bool "json:\"smtp_enabled\""
	SMTPDisabledProven  bool "json:\"smtp_disabled_proven\""
	GlobalDefaultsEmpty bool "json:\"global_defaults_empty\""
	OCRDisabled         bool "json:\"ocr_disabled\""
	ModelsDisabled      bool "json:\"models_disabled\""
	SyncDisabled        bool "json:\"sync_disabled\""
	WritersBound        bool "json:\"all_authority_writers_bound\""
	SecretLoggingOff    bool "json:\"secret_logging_off\""
}

type fixtureLease struct {
	AdmissionSHA      string        `json:"admission_sha256"`
	SourceSetSHA      string        `json:"source_set_sha256"`
	OperatorRecipeSHA string        `json:"operator_recipe_sha256"`
	Operator          bool          `json:"allow_bounded_operator_stages"`
	OperatorAckPipe   string        `json:"operator_ack_ram_pipe"`
	Purpose           string        "json:\"purpose\""
	RunID             string        "json:\"run_id\""
	Origin            string        "json:\"origin\""
	NotBefore         time.Time     "json:\"not_before\""
	Expires           time.Time     "json:\"expires_at\""
	Synthetic         bool          "json:\"synthetic\""
	Prepare           bool          "json:\"allow_fixture_prepare\""
	Accounts          bool          "json:\"allow_account_creation\""
	Foundation        bool          "json:\"allow_owner_foundation\""
	Handoff           bool          "json:\"allow_ram_session_handoff\""
	APIELF            string        "json:\"api_elf_sha256\""
	RuntimeReceiptSHA string        "json:\"runtime_attestation_sha256\""
	MaterialsSHA      string        "json:\"materials_manifest_sha256\""
	OutputPipe        string        "json:\"output_ram_pipe\""
	Safety            fixtureSafety "json:\"safety\""
	admission         fixtureAdmission
	inputBytes        int
	scopeSHA          string
}

// The setup input has no password, session, token or credential field.
type fixtureInput struct {
	Scope     json.RawMessage "json:\"scope\""
	Admission json.RawMessage `json:"admission"`
}

func readFixtureLease(t *testing.T) fixtureLease {
	t.Helper()
	if os.Getenv("LFEN_ACCESS_FIXTURE_RUNTIME") != "root-approved-synthetic-fixture" {
		t.Skip("fixture_scope_not_granted")
	}
	expected := os.Getenv("LFEN_ACCESS_FIXTURE_SCOPE_SHA256")
	if !isHex(expected, 64) {
		t.Fatal("fixture_scope_refused")
	}
	expires, err := time.Parse(time.RFC3339Nano, os.Getenv("LFEN_ACCESS_FIXTURE_EXPIRES_AT"))
	if err != nil || !expires.After(time.Now()) || time.Until(expires) > fixtureLeaseLimit {
		t.Fatal("fixture_bootstrap_expiry_refused")
	}
	ctx, cancel := context.WithDeadline(context.Background(), expires)
	defer cancel()
	raw, err := fixtureReadRAM(ctx, os.Getenv("LFEN_ACCESS_FIXTURE_INPUT_PIPE"), 32768, nil)
	if err != nil {
		t.Fatal("fixture_input_refused")
	}
	var input fixtureInput
	var scope fixtureLease
	if !fixtureExactFields(raw, "scope admission") || decodeClosed(raw, &input) != nil || digest(input.Scope) != expected ||
		!fixtureExactFields(input.Scope, "admission_sha256 source_set_sha256 operator_recipe_sha256 allow_bounded_operator_stages operator_ack_ram_pipe purpose run_id origin not_before expires_at synthetic allow_fixture_prepare allow_account_creation allow_owner_foundation allow_ram_session_handoff api_elf_sha256 runtime_attestation_sha256 materials_manifest_sha256 output_ram_pipe safety") ||
		decodeClosed(input.Scope, &scope) != nil {
		t.Fatal("fixture_scope_refused")
	}
	now := time.Now()
	safe := scope.Safety
	_, originErr := localOrigin(scope.Origin)
	if originErr != nil || scope.Origin != os.Getenv("LFEN_ACCESS_FIXTURE_ORIGIN") ||
		scope.Purpose != "ROOT_ACCESS02_S1_SYNTHETIC_FIXTURE" ||
		!isHex(scope.RunID, 32) || !scope.Synthetic || !scope.Prepare || !scope.Accounts ||
		!scope.Foundation || !scope.Handoff || !scope.Operator || scope.APIELF != fixtureAPIELF ||
		!isHex(scope.RuntimeReceiptSHA, 64) || scope.MaterialsSHA != fixtureMaterialManifestSHA ||
		!scope.Expires.Equal(expires) || ctx.Err() != nil ||
		scope.NotBefore.IsZero() || now.Before(scope.NotBefore) || !scope.Expires.After(now) ||
		scope.Expires.Sub(scope.NotBefore) > fixtureLeaseLimit ||
		!safe.RegistrationEnabled || safe.SMTPEnabled || !safe.SMTPDisabledProven ||
		!safe.GlobalDefaultsEmpty || !safe.OCRDisabled || !safe.ModelsDisabled ||
		!safe.SyncDisabled || !safe.WritersBound || !safe.SecretLoggingOff {
		t.Fatal("fixture_scope_refused")
	}
	if _, err := fixtureFIFO(scope.OutputPipe); err != nil {
		t.Fatal("fixture_output_pipe_refused")
	}
	if _, err := fixtureFIFO(scope.OperatorAckPipe); err != nil {
		t.Fatal("fixture_operator_pipe_refused")
	}
	if !fixtureHash(scope.AdmissionSHA) || scope.AdmissionSHA != os.Getenv("LFEN_ACCESS_FIXTURE_ADMISSION_SHA256") || digest(input.Admission) != scope.AdmissionSHA ||
		!fixtureAdmissionFields(input.Admission) || decodeClosed(input.Admission, &scope.admission) != nil {
		t.Fatal("fixture_admission_refused")
	}
	scope.inputBytes = len(raw)
	scope.scopeSHA = expected
	return scope
}

// Root supplies this attestation only after independently verifying the actual
// referenced review/compiler receipts. Its exact RAM bytes are pinned outside
// the input, by the same Root launcher that supplies the private scope SHA.
// The fixture does not grant itself approval by generating these fields.
type fixtureArtifact struct {
	SHA   string `json:"sha256"`
	Bytes int    `json:"bytes"`
}
type fixtureEvidenceReceipt struct {
	Kind         string          `json:"kind"`
	Artifact     fixtureArtifact `json:"artifact"`
	Actor        string          `json:"actor"`
	SourceSetSHA string          `json:"source_set_sha256"`
	SourceFiles  int             `json:"source_files"`
	Finished     time.Time       `json:"finished_at"`
	Result       string          `json:"result"`
	GOOS         string          `json:"goos"`
	BuildTag     string          `json:"build_tag"`
	Exit         int             `json:"exit_code"`
	RuntimeCalls int             `json:"runtime_calls"`
}
type fixtureOperatorCapability struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Phase         string          `json:"phase"`
	Driver        fixtureArtifact `json:"reviewed_driver_artifact"`
	NotBefore     time.Time       `json:"not_before"`
	Expires       time.Time       `json:"expires_at"`
	MaxHTTP       int             `json:"max_http_attempts"`
	MaxHTTPBytes  int             `json:"max_http_bytes"`
	MaxSQLSelect  int             `json:"max_sql_select"`
	MaxSQLRows    int             `json:"max_sql_rows"`
	MaxCASRows    int             `json:"max_cas_rows"`
	MaxObjectPUT  int             `json:"max_object_put"`
	MaxFileWrites int             `json:"max_file_writes"`
}
type fixtureAdmission struct {
	Version      string                      `json:"version"`
	RootID       string                      `json:"root_id"`
	RunID        string                      `json:"run_id"`
	Origin       string                      `json:"origin"`
	NotBefore    time.Time                   `json:"not_before"`
	Expires      time.Time                   `json:"expires_at"`
	Issued       time.Time                   `json:"issued_at"`
	ReferenceSHA string                      `json:"reference_manifest_sha256"`
	SourceSetSHA string                      `json:"source_set_sha256"`
	RecipeSHA    string                      `json:"operator_recipe_sha256"`
	APIELF       string                      `json:"api_elf_sha256"`
	RootVerified bool                        `json:"root_verified_actual_artifacts"`
	Review       fixtureEvidenceReceipt      `json:"review"`
	Compile      fixtureEvidenceReceipt      `json:"compile"`
	Operators    []fixtureOperatorCapability `json:"operators"`
}

func fixtureHash(value string) bool { return isHex(value, 64) && value != strings.Repeat("0", 64) }

func fixtureAdmissionFields(raw []byte) bool {
	if !fixtureExactFields(raw, "version root_id run_id origin not_before expires_at issued_at reference_manifest_sha256 source_set_sha256 operator_recipe_sha256 api_elf_sha256 root_verified_actual_artifacts review compile operators") {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	for _, key := range []string{"review", "compile"} {
		if !fixtureExactFields(fields[key], "kind artifact actor source_set_sha256 source_files finished_at result goos build_tag exit_code runtime_calls") {
			return false
		}
		var receipt map[string]json.RawMessage
		if json.Unmarshal(fields[key], &receipt) != nil || !fixtureExactFields(receipt["artifact"], "sha256 bytes") {
			return false
		}
	}
	var operators []map[string]json.RawMessage
	if json.Unmarshal(fields["operators"], &operators) != nil {
		return false
	}
	for _, cap := range operators {
		if !fixtureExactFields(fixtureJSONBytes(cap), "id name phase reviewed_driver_artifact not_before expires_at max_http_attempts max_http_bytes max_sql_select max_sql_rows max_cas_rows max_object_put max_file_writes") ||
			!fixtureExactFields(cap["reviewed_driver_artifact"], "sha256 bytes") {
			return false
		}
	}
	return true
}

func fixturePreparationGaps(scope fixtureLease, expectedSourceSHA string, now time.Time) []string {
	a := scope.admission
	if !fixtureHash(expectedSourceSHA) || scope.SourceSetSHA != expectedSourceSHA || a.SourceSetSHA != expectedSourceSHA ||
		a.Version != "root-fixture-admission/2" || a.RootID != "01a1a9a5-c724-7f52-8cbb-16d19d982ba4" || !a.RootVerified ||
		a.RunID != scope.RunID || a.Origin != scope.Origin || !a.NotBefore.Equal(scope.NotBefore) || !a.Expires.Equal(scope.Expires) ||
		a.Issued.IsZero() || a.Issued.After(now) || now.Before(a.NotBefore) || !now.Before(a.Expires) || a.Expires.Sub(a.NotBefore) > fixtureLeaseLimit ||
		a.ReferenceSHA != "ab12875dfc1e0385eb1df1c6b474b4439d8d0e811f57ccc1259497055ff1001e" || !fixtureHash(scope.OperatorRecipeSHA) || a.RecipeSHA != scope.OperatorRecipeSHA || a.APIELF != fixtureAPIELF {
		return []string{"root_admission_binding_missing"}
	}
	for _, r := range []fixtureEvidenceReceipt{a.Review, a.Compile} {
		if !fixtureHash(r.Artifact.SHA) || r.Artifact.Bytes <= 0 || len(r.Actor) < 1 || len(r.Actor) > 128 || r.SourceSetSHA != expectedSourceSHA || r.SourceFiles != 21 ||
			r.Finished.IsZero() || r.Finished.After(a.Issued) || r.Exit != 0 || r.RuntimeCalls != 0 {
			return []string{"independent_artifact_receipt_missing"}
		}
	}
	if a.Review.Kind != "independent_source_review" || a.Review.Result != "approved" || a.Compile.Kind != "linux_integration_compile" || a.Compile.Result != "passed" ||
		a.Compile.GOOS != "linux" || a.Compile.BuildTag != "integration" || a.Review.Actor == a.Compile.Actor || a.Review.Artifact.SHA == a.Compile.Artifact.SHA {
		return []string{"review_or_linux_compile_receipt_missing"}
	}
	if len(a.Operators) != 4 {
		return []string{"named_operator_capability_missing"}
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, cap := range a.Operators {
		if !isHex(cap.ID, 32) || ids[cap.ID] || cap.Name != "fixture_"+cap.Phase || seen[cap.Phase] || !fixtureHash(cap.Driver.SHA) || cap.Driver.Bytes <= 0 ||
			!cap.NotBefore.Equal(scope.NotBefore) || !cap.Expires.Equal(scope.Expires) {
			return []string{"operator_capability_binding_invalid"}
		}
		seen[cap.Phase] = true
		ids[cap.ID] = true
		switch cap.Phase {
		case "r1_root":
			if cap.MaxHTTP != 4 || cap.MaxHTTPBytes != 1<<20 || cap.MaxSQLSelect != 0 || cap.MaxSQLRows != 0 || cap.MaxCASRows != 0 || cap.MaxObjectPUT != 0 || cap.MaxFileWrites != 2 {
				return []string{"r1_operator_bounds_invalid"}
			}
		case "a_r2":
			if cap.MaxHTTP != 4 || cap.MaxHTTPBytes != 1<<20 || cap.MaxSQLSelect != 1 || cap.MaxSQLRows != 2 || cap.MaxCASRows != 2 || cap.MaxObjectPUT != 1 || cap.MaxFileWrites != 0 {
				return []string{"ar2_operator_bounds_invalid"}
			}
		case "canonical_pre", "canonical_post":
			if cap.MaxHTTP != 0 || cap.MaxHTTPBytes != 0 || cap.MaxSQLSelect != 16 || cap.MaxSQLRows != 128 || cap.MaxCASRows != 0 || cap.MaxObjectPUT != 0 || cap.MaxFileWrites != 0 {
				return []string{"capture_operator_bounds_invalid"}
			}
		default:
			return []string{"operator_phase_unknown"}
		}
	}
	return nil
}

type fixtureRoute struct {
	method, path, family, operation string
	status                          int
}

// This client owns its finite setup budget. It never calls or widens accessClient.
var fixtureRoutes = map[string]fixtureRoute{
	"localPolicy":          {"GET", "/api/v1/datasets/{id}/knowledge/local-lexical-policy", "local", "getLocalPolicy", 200},
	"localPolicySet":       {"POST", "/api/v1/datasets/{id}/knowledge/local-lexical-policy/changes", "local", "changeLocalPolicy", 200},
	"directoryRoots":       {"GET", "/api/v1/datasets/{id}/knowledge/directory-roots", "directory", "", 200},
	"project":              {"POST", "/api/v1/datasets/{id}/knowledge/groups", "foundation", "", 201},
	"directoryRegister":    {"POST", "/api/v1/datasets/{id}/knowledge/directory-registrations", "directory", "", 201},
	"directoryObserve":     {"POST", "/api/v1/datasets/{id}/knowledge/directory-registrations/{id}/observations", "directory", "", 201},
	"batchPreview":         {"POST", "/api/v1/datasets/{id}/knowledge/directory-registrations/{id}/batch-previews", "batch", "", 201},
	"batchAccept":          {"POST", "/api/v1/datasets/{id}/knowledge/directory-registrations/{id}/batch-tasks", "batch", "", 202},
	"batchGet":             {"GET", "/api/v1/datasets/{id}/knowledge/directory-registrations/{id}/batch-tasks/{id}", "batch", "", 200},
	"manifestPreview":      {"POST", "/api/v1/datasets/{id}/knowledge/project-manifest-previews", "batch", "", 201},
	"manifestPublish":      {"POST", "/api/v1/datasets/{id}/knowledge/project-manifest-publications", "batch", "", 201},
	"structurePolicy":      {"GET", "/api/v1/datasets/{id}/knowledge/structure-policy", "structure", "", 200},
	"structurePolicySet":   {"POST", "/api/v1/datasets/{id}/knowledge/structure-policy/changes", "structure", "", 201},
	"structureTask":        {"POST", "/api/v1/datasets/{id}/knowledge/structure-tasks", "structure", "", 201},
	"structureTaskGet":     {"GET", "/api/v1/datasets/{id}/knowledge/structure-tasks/{id}", "structure", "", 200},
	"originalRegister":     {"POST", "/api/v1/datasets/{id}/knowledge/original-registrations", "original", "", 201},
	"originalUpload":       {"POST", "/api/v1/datasets/{id}/knowledge/original-registrations/{id}/uploads", "original", "", 201},
	"originalBytes":        {"PUT", "/api/v1/datasets/{id}/knowledge/original-registrations/{id}/uploads/{id}/content", "original", "", 200},
	"originalConfirm":      {"POST", "/api/v1/datasets/{id}/knowledge/original-registrations/{id}/imports", "original", "", 201},
	"documentCapabilities": {"GET", "/api/v1/knowledge/document-structure-contract", "document", "", 200},
	"documentPolicy":       {"GET", "/api/v1/datasets/{id}/knowledge/document-structure-policy", "document", "", 200},
	"documentPolicySet":    {"POST", "/api/v1/datasets/{id}/knowledge/document-structure-policy/changes", "document", "", 201},
	"documentTask":         {"POST", "/api/v1/datasets/{id}/knowledge/document-tasks", "document", "", 201},
	"documentTaskGet":      {"GET", "/api/v1/datasets/{id}/knowledge/document-tasks/{id}", "document", "", 200},
	"lifecycle":            {"GET", "/api/v1/datasets/{id}/knowledge/source-lifecycles/{id}", "lifecycle", "", 200},
	"register":             {"POST", "/api/v1/users", "native", "", 200},
	"models":               {"GET", "/api/v1/users/me/models", "native", "", 200},
	"invite":               {"POST", "/api/v1/tenants/{id}/users", "native", "", 200},
	"accept":               {"PATCH", "/api/v1/tenants/{id}", "native", "", 200},
	"members":              {"GET", "/api/v1/tenants/{id}/users", "native", "", 200},
	"dataset":              {"POST", "/api/v1/datasets", "native", "", 200},
	"domain":               {"PUT", "/api/v1/datasets/{id}/knowledge/domain", "foundation", "registerKnowledgeDomain", 200},
	"nativeUpload":         {"POST", "/api/v1/datasets/{id}/documents", "native", "", 200},
	"sourceSnapshot":       {"POST", "/api/v1/datasets/{id}/knowledge/sources", "foundation", "", 201},
	"parsedPolicy":         {"GET", "/api/v1/datasets/{id}/knowledge/content/policy", "content", "content-policy", 200},
	"parsedPolicySet":      {"POST", "/api/v1/datasets/{id}/knowledge/content/policy", "content", "content-policy-change", 200},
	"parsedUpload":         {"POST", "/api/v1/datasets/{id}/knowledge/content/parsed-source/upload", "content", "parsed-source-upload", 200},
	"parsedSnapshot":       {"POST", "/api/v1/datasets/{id}/knowledge/content/parsed-source/snapshot", "content", "parsed-source-snapshot", 200},
	"parsedSource":         {"POST", "/api/v1/datasets/{id}/knowledge/content/parsed-source", "content", "parsed-source", 200},
	"parsedTask":           {"POST", "/api/v1/datasets/{id}/knowledge/content/parsed-task", "content", "parsed-task-create", 201},
	"parsedTaskGet":        {"POST", "/api/v1/datasets/{id}/knowledge/content/parsed-task/get", "content", "parsed-task-get", 200},
	"ownerInspect":         {"POST", "/api/v1/datasets/{id}/knowledge/content/lexical/inspect", "lexical", "inspectIndex", 200},
	"ownerIndex":           {"POST", "/api/v1/datasets/{id}/knowledge/content/lexical/indexes", "lexical", "createIndex", 202},
	"ownerJob":             {"POST", "/api/v1/datasets/{id}/knowledge/content/lexical/jobs/get", "lexical", "getIndexJob", 200},
	"ownerSearch":          {"POST", "/api/v1/datasets/{id}/knowledge/content/lexical/search", "lexical", "search", 200},
	"ownerRead":            {"POST", "/api/v1/datasets/{id}/knowledge/content/lexical/evidence/read", "lexical", "read", 200},
}

type fixtureReply struct {
	data                     json.RawMessage
	authorization, requestID string
}
type fixtureHTTP struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	scope               fixtureLease
	client              *http.Client
	mu                  sync.Mutex
	attempts, bodyBytes int
	stopped             bool
	sessions            map[string]string
	ids                 map[string]bool
	parsedBindings      map[string]fixtureParsedBinding
}

func newFixtureHTTP(scope fixtureLease) *fixtureHTTP {
	tr := &http.Transport{
		Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		MaxResponseHeaderBytes: 16384, MaxConnsPerHost: 1,
		DisableKeepAlives: true, DisableCompression: true,
	}
	ctx, cancel := context.WithDeadline(context.Background(), scope.Expires)
	return &fixtureHTTP{scope: scope, ctx: ctx, cancel: cancel, bodyBytes: scope.inputBytes, parsedBindings: map[string]fixtureParsedBinding{}, sessions: map[string]string{}, ids: map[string]bool{},
		client: &http.Client{Transport: tr, Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *fixtureHTTP) refuse() error {
	c.stopped = true
	if c.cancel != nil {
		c.cancel()
	}
	return errFixture
}
func (c *fixtureHTTP) close() {
	c.cancel()
	for key := range c.sessions {
		delete(c.sessions, key)
	}
	c.client.CloseIdleConnections()
}

// Error bodies, Authorization headers and registration bodies never leave this method as diagnostics.
func (c *fixtureHTTP) exchange(op, actor string, ids []string, body []byte, mediaType string) (fixtureReply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var none fixtureReply
	route, ok := fixtureRoutes[op]
	if !ok || !c.active() ||
		c.attempts >= fixtureHTTPAttempts || len(body) > 32768 || c.bodyBytes+len(body)+fixtureResponseLimit+1 > fixtureJSONBudget {
		return none, c.refuse()
	}
	path := route.path
	for _, id := range ids {
		if !isHex(id, 32) || !strings.Contains(path, "{id}") {
			return none, c.refuse()
		}
		path = strings.Replace(path, "{id}", id, 1)
	}
	if strings.Contains(path, "{id}") || strings.ContainsAny(path, "?#") {
		return none, c.refuse()
	}
	if mediaType == "application/json" && len(body) > 0 {
		if _, err := strictJSON(body); err != nil {
			return none, c.refuse()
		}
	}
	if (op == "register") != (actor == "anonymous") {
		return none, c.refuse()
	}
	if actor != "anonymous" && c.sessions[actor] == "" {
		return none, c.refuse()
	}
	requestID, err := fixtureRandomID(16)
	if err != nil {
		return none, c.refuse()
	}
	ctx, cancel := context.WithDeadline(context.Background(), c.scope.Expires)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, route.method, c.scope.Origin+path, bytes.NewReader(body))
	if err != nil {
		return none, c.refuse()
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	if route.family == "batch" || route.family == "lifecycle" || route.family == "directory" {
		v := "0.4.0"
		if route.family == "directory" {
			v = "0.3.0"
		}
		if route.family == "lifecycle" {
			v = "0.7.0"
		}
		req.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
		req.Header.Set("X-Knowledge-Source-Contract-Version", v)
	}
	if route.family == "structure" {
		req.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
		req.Header.Set("X-Knowledge-Structure-Contract-Version", "0.1.0")
	}
	if route.family == "original" {
		req.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
		req.Header.Set("X-Knowledge-Source-Contract-Version", "0.5.0")
	}
	if route.family == "document" {
		req.Header.Set("X-Knowledge-Contract-Version", "1.0.0")
		req.Header.Set("X-Knowledge-Document-Structure-Contract-Version", "0.1.0")
	}
	if mediaType != "" {
		req.Header.Set("Content-Type", mediaType)
	}
	if actor != "anonymous" {
		req.Header.Set("Authorization", c.sessions[actor])
	}
	c.attempts++
	c.bodyBytes += len(body)
	res, err := c.client.Do(req)
	if err != nil {
		return none, c.refuse()
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, fixtureResponseLimit+1))
	c.bodyBytes += len(raw)
	if err != nil || len(raw) > fixtureResponseLimit || c.bodyBytes > fixtureJSONBudget ||
		res.StatusCode != route.status || !c.active() || !utf8.Valid(raw) {
		return none, c.refuse()
	}
	mt, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return none, c.refuse()
	}
	value, err := fixturePreparationReplyJSON(op, raw)
	object, ok := value.(map[string]any)
	if err != nil || !ok {
		return none, c.refuse()
	}
	data, exists := object["data"]
	if !exists || data == nil {
		return none, c.refuse()
	}
	if route.family == "native" {
		code, ok := object["code"].(float64)
		if !ok || code != 0 {
			return none, c.refuse()
		}
		// Native code/data/message has no request_id contract. Do not invent an echo.
		for name := range object {
			if name != "code" && name != "data" && name != "message" && name != "total" {
				return none, c.refuse()
			}
		}
	} else {
		rid, ok := object["request_id"].(string)
		if !ok || !isHex(rid, 32) || res.Header.Get("X-Request-ID") != rid || c.ids[rid] {
			return none, c.refuse()
		}
		c.ids[rid] = true
		if route.family == "lexical" && (object["contract_version"] != fixtureContentVersion || object["operation"] != route.operation ||
			res.Header.Get("X-Knowledge-Content-Version") != fixtureContentVersion ||
			res.Header.Get("X-Knowledge-Content-Lexical-Protocol") != "content_lexical/0.2.0") {
			return none, c.refuse()
		}
		if route.operation != "" && route.family != "foundation" && object["operation"] != route.operation {
			return none, c.refuse()
		}
		if route.family == "original" || route.family == "document" || route.family == "batch" || route.family == "structure" || route.family == "lifecycle" || route.family == "directory" {
			if object["code"] != float64(0) || object["contract_version"] != "1.0.0" || !strings.Contains(strings.ToLower(res.Header.Get("Cache-Control")), "no-store") {
				return none, c.refuse()
			}
			if route.family == "directory" && (object["source_contract_version"] != "0.3.0" || res.Header.Get("X-Knowledge-Source-Contract-Version") != "0.3.0") {
				return none, c.refuse()
			}
			if route.family == "batch" && (object["source_contract_version"] != "0.4.0" || res.Header.Get("X-Knowledge-Source-Contract-Version") != "0.4.0") {
				return none, c.refuse()
			}
			if route.family == "lifecycle" && (object["source_contract_version"] != "0.7.0" || res.Header.Get("X-Knowledge-Source-Contract-Version") != "0.7.0") {
				return none, c.refuse()
			}
			if route.family == "structure" && (object["structure_contract_version"] != "0.1.0" || res.Header.Get("X-Knowledge-Structure-Contract-Version") != "0.1.0") {
				return none, c.refuse()
			}
			if route.family == "original" && (object["source_contract_version"] != "0.5.0" || res.Header.Get("X-Knowledge-Source-Contract-Version") != "0.5.0") {
				return none, c.refuse()
			}
			if route.family == "document" && (object["document_structure_contract_version"] != "0.1.0" || res.Header.Get("X-Knowledge-Document-Structure-Contract-Version") != "0.1.0") {
				return none, c.refuse()
			}
		} else if route.family == "local" {
			if object["contract_version"] != "0.1.0-candidate" || res.Header.Get("X-Knowledge-Local-Lexical-Version") != "0.1.0-candidate" || !strings.Contains(strings.ToLower(res.Header.Get("Cache-Control")), "no-store") {
				return none, c.refuse()
			}
		} else if route.family == "foundation" {
			if object["code"] != float64(0) || object["contract_version"] != "1.0.0" {
				return none, c.refuse()
			}
		} else {
			if code, present := object["code"]; present && code != "" {
				return none, c.refuse()
			}
			if object["contract_version"] != fixtureContentVersion || !strings.Contains(strings.ToLower(res.Header.Get("Cache-Control")), "no-store") {
				return none, c.refuse()
			}
		}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return none, c.refuse()
	}
	auth := ""
	if op == "register" {
		auth = res.Header.Get("Authorization")
		if auth == "" || len(auth) > 4096 || strings.TrimSpace(auth) != auth || strings.ContainsAny(auth, "\r\n") {
			return none, c.refuse()
		}
	}
	return fixtureReply{data: encoded, authorization: auth, requestID: requestID}, nil
}

// fixturePreparationReplyJSON validates ordinary preparation replies. Only the
// listed numeric DTO fields use finite float64; all other tokens stay exact.
func fixturePreparationReplyJSON(op string, raw []byte) (any, error) {
	return fixturePreparationJSON(op, raw, nil)
}

// fixtureDecodeReply retains closed DTO decoding after the same reply checks.
// raw is the data slice, so its numeric paths start at the envelope's data key.
func fixtureDecodeReply(op string, raw []byte, out any) error {
	if _, err := fixturePreparationJSON(op, raw, []any{"data"}); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errContract
	}
	if _, err := d.Token(); err != io.EOF {
		return errContract
	}
	return nil
}

func fixturePreparationJSON(op string, raw []byte, path []any) (any, error) {
	if len(raw) > fixtureResponseLimit || !utf8.Valid(raw) || !validEscapes(raw) {
		return nil, errContract
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := fixturePreparationValue(d, 0, op, path)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errContract
	}
	return value, nil
}

func fixturePreparationFloatField(op string, path []any) bool {
	route, ok := fixtureRoutes[op]
	if !ok {
		return false
	}
	if op == "members" && route.method == http.MethodGet && route.path == "/api/v1/tenants/{id}/users" && route.family == "native" &&
		len(path) == 3 && path[0] == "data" && path[2] == "delta_seconds" {
		index, ok := path[1].(int)
		return ok && index >= 0
	}
	if op == "ownerSearch" && route.method == http.MethodPost && route.path == "/api/v1/datasets/{id}/knowledge/content/lexical/search" && route.family == "lexical" &&
		len(path) == 4 && path[0] == "data" && path[1] == "hits" && path[3] == "score" {
		index, ok := path[2].(int)
		return ok && index >= 0
	}

	if op == "dataset" && route.method == http.MethodPost && route.path == "/api/v1/datasets" && route.family == "native" &&
		((len(path) == 2 && path[0] == "data" && (path[1] == "similarity_threshold" || path[1] == "keywords_similarity_weight")) ||
			(len(path) == 4 && path[0] == "data" && path[1] == "parser_config" && path[2] == "Tokenizer:LegalReadersDecide" && path[3] == "filename_embd_weight")) {
		return true
	}
	return false
}

func fixturePreparationNumber(number json.Number, op string, path []any) (float64, error) {
	if !fixturePreparationFloatField(op, path) {
		return exactIntegerToken(number.String())
	}
	value, err := number.Float64()
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 9007199254740991 {
		return 0, errContract
	}
	mantissa := number.String()
	if at := strings.IndexAny(mantissa, "eE"); at >= 0 {
		mantissa = mantissa[:at]
	}
	if value == 0 && strings.ContainsAny(mantissa, "123456789") {
		return 0, errContract
	}
	return value, nil
}

func fixturePreparationValue(d *json.Decoder, depth int, op string, path []any) (any, error) {
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
				if _, exists := value[name]; exists {
					return nil, errContract
				}
				child, err := fixturePreparationValue(d, depth+1, op, append(path, name))
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
				child, err := fixturePreparationValue(d, depth+1, op, append(path, len(value)))
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
		return fixturePreparationNumber(number, op, path)
	}
	return token, nil
}

func fixtureRandomID(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", errFixture
	}
	return hex.EncodeToString(raw), nil
}
func fixtureEncode(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 32768 {
		return nil, errFixture
	}
	return raw, nil
}
func fixtureJSON(c *fixtureHTTP, op, actor string, ids []string, value any) (fixtureReply, error) {
	raw, err := fixtureEncode(value)
	if err != nil {
		return fixtureReply{}, c.refuse()
	}
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	return c.exchange(op, actor, ids, raw, "application/json")
}

func fixtureFIFO(path string) (os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errFixture
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeNamedPipe == 0 || info.Mode().Perm() != 0600 {
		return nil, errFixture
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, errFixture
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || parent.Mode()&os.ModeSymlink != 0 || !parent.IsDir() || parent.Mode().Perm() != 0700 {
		return nil, errFixture
	}
	pstat, ok := parent.Sys().(*syscall.Stat_t)
	if !ok || int(pstat.Uid) != os.Geteuid() {
		return nil, errFixture
	}
	return info, nil
}

// Only signed session headers are handed to Root's private RAM receiver.
// This does not issue an S1 scope and cannot set the public fixture prepared=true.
func (c *fixtureHTTP) handoffSessions(actors map[string]actorFixture) error {
	if len(actors) != 3 || len(c.sessions) != 3 {
		return c.refuse()
	}
	for _, actor := range []string{"owner", "recipient", "foreign"} {
		if !isHex(actors[actor].UserID, 32) || !isHex(actors[actor].TenantID, 32) || c.sessions[actor] == "" {
			return c.refuse()
		}
	}
	if !c.scope.Handoff || !c.active() {
		return c.refuse()
	}
	wire, err := json.Marshal(struct {
		RunID    string                  "json:\"run_id\""
		Actors   map[string]actorFixture "json:\"actors\""
		Sessions map[string]string       "json:\"sessions\""
	}{c.scope.RunID, actors, c.sessions})
	if err != nil || len(wire) > 32768 {
		return c.refuse()
	}
	defer func() {
		for i := range wire {
			wire[i] = 0
		}
	}()
	if c.writeRAM(wire) != nil {
		return c.refuse()
	}
	return nil
}

func TestFixturePreparationReplyNumbers(t *testing.T) {
	good := []struct{ op, body string }{
		{"members", "{\"code\":0,\"data\":[{\"delta_seconds\":-0.125,\"epoch\":1}]}"},
		{"ownerSearch", "{\"data\":{\"hits\":[{\"score\":1.25e1}],\"returned_count\":1}}"},
		{"dataset", "{\"code\":0,\"data\":{\"similarity_threshold\":0.2,\"keywords_similarity_weight\":0.7,\"parser_config\":{\"Tokenizer:LegalReadersDecide\":{\"filename_embd_weight\":0.1}}}}"},
	}
	for _, test := range good {
		t.Run(test.op, func(t *testing.T) {
			raw := []byte(test.body)
			if _, err := strictJSON(raw); err != errContract {
				t.Fatal("original strict contract must reject these reply fractions")
			}
			if _, err := fixturePreparationReplyJSON(test.op, raw); err != nil {
				t.Fatal("ordinary preparation DTO fraction rejected")
			}
		})
	}
	bad := []struct{ op, body string }{
		{"members", "{\"code\":0.125,\"data\":[]}"},
		{"members", "{\"data\":[{\"version\":0.125}]}"},
		{"members", "{\"data\":{\"0\":{\"delta_seconds\":0.125}}}"},
		{"members", "{\"data\":[{\"nested\":{\"delta_seconds\":0.125}}]}"},
		{"ownerSearch", "{\"data\":{\"score\":0.125}}"},
		{"ownerSearch", "{\"data\":{\"hits\":{\"0\":{\"score\":0.125}}}}"},
		{"ownerSearch", "{\"data\":{\"hits\":[{\"version\":0.125}]}}"},
		{"dataset", "{\"data\":{\"vector_similarity_weight\":0.3}}"},
		{"dataset", "{\"data\":{\"nested\":{\"similarity_threshold\":0.2}}}"},
		{"dataset", "{\"data\":{\"parser_config\":{\"Tokenizer:other\":{\"filename_embd_weight\":0.1}}}}"},
		{"models", "{\"data\":[{\"delta_seconds\":0.125}]}"},
		{"ownerRead", "{\"data\":{\"hits\":[{\"score\":0.125}]}}"},
		{"members", "{\"data\":[{\"epoch\":1.0000000000000001}]}"},
		{"members", "{\"data\":[{\"epoch\":9007199254740993}]}"},
		{"members", "{\"data\":[{\"delta_seconds\":0.125,\"delta\\u005fseconds\":0.25}]}"},
		{"members", "{\"data\":[{\"delta_seconds\":0.125}]} trailing"},
	}
	for i, test := range bad {
		if _, err := fixturePreparationReplyJSON(test.op, []byte(test.body)); err != errContract {
			t.Errorf("invalid preparation numeric path/token accepted: %d", i)
		}
	}
	for _, token := range []string{"1e999", "-1e999", "1e-999", "9007199254740992"} {
		raw := []byte("{\"data\":[{\"delta_seconds\":" + token + "}]}")
		if _, err := fixturePreparationReplyJSON("members", raw); err != errContract {
			t.Fatal("nonfinite, unsafe or underflowed reply number accepted")
		}
	}
	if _, err := fixturePreparationReplyJSON("members", []byte(strings.Repeat(" ", fixtureResponseLimit+1))); err != errContract {
		t.Fatal("reply size limit changed")
	}
	if _, err := fixturePreparationReplyJSON("members", []byte{0xff}); err != errContract {
		t.Fatal("invalid UTF8 accepted")
	}
}

func TestFixturePreparationReplyClosedDTO(t *testing.T) {
	var members []struct {
		Delta float64 "json:\"delta_seconds\""
		Epoch int64   "json:\"epoch\""
	}
	raw := []byte("[{\"delta_seconds\":0.125,\"epoch\":1}]")
	if decodeClosed(raw, &members) == nil {
		t.Fatal("integer-only DTO decoder must stay unchanged")
	}
	if err := fixtureDecodeReply("members", raw, &members); err != nil || len(members) != 1 || members[0].Delta != 0.125 || members[0].Epoch != 1 {
		t.Fatal("ordinary reply DTO was rejected by a second integer parse")
	}
	var search struct {
		Hits []struct {
			Score float64 "json:\"score\""
		} "json:\"hits\""
	}
	if err := fixtureDecodeReply("ownerSearch", []byte("{\"hits\":[{\"score\":0.125}]}"), &search); err != nil || len(search.Hits) != 1 || search.Hits[0].Score != 0.125 {
		t.Fatal("score DTO rejected")
	}
	if err := fixtureDecodeReply("ownerSearch", []byte("{\"hits\":[{\"score\":0.125,\"unexpected\":1}]}"), &search); err == nil {
		t.Fatal("closed DTO unknown-field check changed")
	}
	if err := fixtureDecodeReply("ownerRead", []byte("{\"hits\":[{\"score\":0.125}]}"), &search); err == nil {
		t.Fatal("reply exception escaped its operation")
	}
}

func TestFixturePreparationReplyExchange(t *testing.T) {
	for _, op := range []string{"members", "ownerSearch", "dataset"} {
		t.Run(op, func(t *testing.T) {
			now := time.Now()
			client := newFixtureHTTP(fixtureLease{Origin: "http://127.0.0.1:1", NotBefore: now.Add(-time.Second), Expires: now.Add(time.Minute)})
			defer client.close()
			client.sessions["owner"] = "SYNTHETIC_TEST_ONLY"
			requests := 0
			client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests++
				header := make(http.Header)
				header.Set("Content-Type", "application/json")
				body := "{\"code\":0,\"data\":[{\"user_id\":\"" + strings.Repeat("2", 32) + "\",\"role\":\"normal\",\"delta_seconds\":0.125}]}"
				if op == "dataset" {
					body = "{\"code\":0,\"data\":{\"id\":\"" + strings.Repeat("3", 32) + "\",\"similarity_threshold\":0.2,\"keywords_similarity_weight\":0.7}}"
				}
				if op == "ownerSearch" {
					header.Set("X-Request-ID", request.Header.Get("X-Request-ID"))
					header.Set("Cache-Control", "no-store")
					header.Set("X-Knowledge-Content-Version", fixtureContentVersion)
					header.Set("X-Knowledge-Content-Lexical-Protocol", "content_lexical/0.2.0")
					body = "{\"request_id\":\"" + request.Header.Get("X-Request-ID") + "\",\"contract_version\":\"" + fixtureContentVersion + "\",\"operation\":\"search\",\"data\":{\"hits\":[{\"score\":0.125}],\"returned_count\":1}}"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var ids []string
			if op != "dataset" {
				ids = []string{strings.Repeat("1", 32)}
			}
			if _, err := client.exchange(op, "owner", ids, nil, ""); err != nil || requests != 1 || client.stopped {
				t.Fatal("preparation exchange did not reach and accept one ordinary reply")
			}
		})
	}
}
