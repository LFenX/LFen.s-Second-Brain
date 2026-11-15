//go:build integration && linux

package knowledgeaccess

import (
	"context"
	"encoding/json"
	"syscall"
	"time"

	"ragflow/internal/entity"
)

type fixtureOperatorAck struct {
	CapabilityID  string              `json:"capability_id"`
	OperatorName  string              `json:"operator_name"`
	DriverSHA     string              `json:"driver_artifact_sha256"`
	AR2           *fixtureAR2CASProof `json:"ar2_cas"`
	RunID         string              "json:\"run_id\""
	Phase         string              "json:\"phase\""
	Nonce         string              "json:\"nonce\""
	Completed     bool                "json:\"completed\""
	ReceiptSHA    string              "json:\"receipt_sha256\""
	DomainID      string              "json:\"domain_id\""
	OwnerID       string              "json:\"owner_id\""
	SourceID      string              "json:\"source_id\""
	RootID        string              "json:\"root_id\""
	OriginalSHA   string              "json:\"original_sha256\""
	HTTPAttempts  int                 "json:\"http_attempts\""
	HTTPJSONBytes int                 "json:\"http_json_bytes\""
}

type fixtureParsedBinding struct {
	Source       entity.KnowledgeContentSourceRef `json:"source_ref"`
	NativeFileID string                           `json:"native_file_id"`
	Bytes        int64                            `json:"byte_length"`
}

// Only the exact newly bound pair is eligible for this fixture preparation.
// Location hashes are synthetic private facts; the actual old/new locations and
// object key remain in Root's operator lease and RAM driver, never the fixture.
type fixtureAR2CASProof struct {
	NativeFileID      string `json:"native_file_id"`
	LinkID            string `json:"file2document_id"`
	OldRevisionID     string `json:"old_revision_id"`
	OldSourceSHA      string `json:"old_source_sha256"`
	OldLocationSHA    string `json:"old_location_sha256"`
	NewLocationSHA    string `json:"new_location_sha256"`
	OldBytes          int64  `json:"old_bytes"`
	NewBytes          int64  `json:"new_bytes"`
	DocumentRows      int    `json:"document_rows_updated"`
	FileRows          int    `json:"file_rows_updated"`
	SharingPolicyRows int    `json:"sharing_policy_rows"`
	ModuleRows        int    `json:"module_rows"`
	GrantRows         int    `json:"grant_rows"`
	ViewRows          int    `json:"view_rows"`
	TransactionSHA    string `json:"transaction_receipt_sha256"`
}

// Operator events contain synthetic IDs/material SHA and fixed action names;
// credentials are never fields in the request or acknowledgement.
func (c *fixtureHTTP) operatorStage(phase, domain, owner, source string) (fixtureOperatorAck, error) {
	var none fixtureOperatorAck
	if !c.active() || !c.scope.Operator || !isHex(domain, 32) || !isHex(owner, 32) ||
		(phase != "r1_root" && phase != "a_r2") {
		return none, c.refuse()
	}
	cap, err := c.operatorCapability(phase)
	if err != nil {
		return none, err
	}
	nonce, err := fixtureRandomID(16)
	if err != nil {
		return none, c.refuse()
	}
	material := "guide.md"
	action := "bind_exact_synthetic_root_and_place_guide"
	if phase == "a_r2" {
		material = "synthetic-A-r2.csv"
		action = "cas_exact_synthetic_document_and_bound_file_original"
		if !isHex(source, 32) {
			return none, c.refuse()
		}
	}
	var binding *fixtureParsedBinding
	if phase == "a_r2" {
		b, ok := c.parsedBindings[source]
		if !ok || b.Source.DomainID != domain || b.Bytes != 36 || !isHex(b.NativeFileID, 32) {
			return none, c.refuse()
		}
		binding = &b
	}
	raw, err := fixtureMaterial(material)
	if err != nil {
		return none, c.refuse()
	}
	event, err := json.Marshal(struct {
		CapabilityID string                `json:"capability_id"`
		OperatorName string                `json:"operator_name"`
		DriverSHA    string                `json:"driver_artifact_sha256"`
		ScopeSHA     string                `json:"scope_sha256"`
		Expires      time.Time             `json:"expires_at"`
		Binding      *fixtureParsedBinding `json:"expected_parsed_binding"`
		Kind         string                "json:\"kind\""
		RunID        string                "json:\"run_id\""
		Phase        string                "json:\"phase\""
		Nonce        string                "json:\"nonce\""
		DomainID     string                "json:\"domain_id\""
		OwnerID      string                "json:\"owner_id\""
		SourceID     string                "json:\"source_id\""
		Action       string                "json:\"action\""
		Material     string                "json:\"material\""
		SHA          string                "json:\"sha256\""
	}{cap.ID, cap.Name, cap.Driver.SHA, c.scope.scopeSHA, c.scope.Expires, binding, "operator_request", c.scope.RunID, phase, nonce, domain, owner, source, action, material, digest(raw)})
	if err != nil || len(event) > 32768 || c.reserveOperator() != nil || c.writeRAM(event) != nil {
		return none, c.refuse()
	}
	wire, err := c.readOperatorRAM(32768)
	if err != nil {
		return none, err
	}
	var ack fixtureOperatorAck
	if !fixtureExactFields(wire, "capability_id operator_name driver_artifact_sha256 ar2_cas run_id phase nonce completed receipt_sha256 domain_id owner_id source_id root_id original_sha256 http_attempts http_json_bytes") || decodeClosed(wire, &ack) != nil || ack.RunID != c.scope.RunID || ack.Phase != phase || ack.Nonce != nonce || !ack.Completed ||
		ack.CapabilityID != cap.ID || ack.OperatorName != cap.Name || ack.DriverSHA != cap.Driver.SHA ||
		!fixtureHash(ack.ReceiptSHA) || ack.DomainID != domain || ack.OwnerID != owner || ack.SourceID != source ||
		ack.OriginalSHA != digest(raw) || ack.HTTPAttempts < 0 || ack.HTTPAttempts > 4 || ack.HTTPJSONBytes < 0 || ack.HTTPJSONBytes > 1<<20 || !c.active() {
		return none, c.refuse()
	}
	// The external upper allocation is never refunded, including bad acks.
	if phase == "r1_root" && (!isHex(ack.RootID, 32) || ack.AR2 != nil) {
		return none, c.refuse()
	}
	if phase == "a_r2" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(wire, &fields) != nil || !fixtureExactFields(fields["ar2_cas"], "native_file_id file2document_id old_revision_id old_source_sha256 old_location_sha256 new_location_sha256 old_bytes new_bytes document_rows_updated file_rows_updated sharing_policy_rows module_rows grant_rows view_rows transaction_receipt_sha256") || ack.RootID != "" {
			return none, c.refuse()
		}
		p := ack.AR2
		if p == nil || p.NativeFileID != binding.NativeFileID || !isHex(p.LinkID, 32) || p.OldRevisionID != binding.Source.RevisionID || p.OldSourceSHA != binding.Source.SHA256 ||
			!fixtureHash(p.OldLocationSHA) || !fixtureHash(p.NewLocationSHA) || p.OldLocationSHA == p.NewLocationSHA || p.OldBytes != binding.Bytes || p.NewBytes != int64(len(raw)) ||
			p.DocumentRows != 1 || p.FileRows != 1 || p.SharingPolicyRows != 0 || p.ModuleRows != 0 || p.GrantRows != 0 || p.ViewRows != 0 || !fixtureHash(p.TransactionSHA) {
			return none, c.refuse()
		}
	}
	return ack, nil
}

func (c *fixtureHTTP) active() bool {
	return !c.stopped && c.ctx != nil && c.ctx.Err() == nil && !time.Now().Before(c.scope.NotBefore) && time.Now().Before(c.scope.Expires)
}
func (c *fixtureHTTP) chargeRAM(n int) bool {
	if n < 0 {
		return false
	}
	c.bodyBytes += n
	return c.bodyBytes <= fixtureJSONBudget
}
func (c *fixtureHTTP) reserveOperator() error {
	if !c.active() || c.attempts > fixtureHTTPAttempts-4 || c.bodyBytes > fixtureJSONBudget-(1<<20) {
		return c.refuse()
	}
	c.attempts += 4
	c.bodyBytes += 1 << 20
	return nil
}
func (c *fixtureHTTP) operatorCapability(phase string) (fixtureOperatorCapability, error) {
	if c.active() {
		for _, cap := range c.scope.admission.Operators {
			if cap.Phase == phase && !time.Now().Before(cap.NotBefore) && time.Now().Before(cap.Expires) {
				return cap, nil
			}
		}
	}
	return fixtureOperatorCapability{}, c.refuse()
}
func (c *fixtureHTTP) pause(duration time.Duration) error {
	if !c.active() {
		return c.refuse()
	}
	if fixturePause(c.ctx, duration) != nil || !c.active() {
		return c.refuse()
	}
	return nil
}
func fixturePause(ctx context.Context, duration time.Duration) error {
	if ctx.Err() != nil {
		return errFixture
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errFixture
	case <-timer.C:
		if ctx.Err() != nil {
			return errFixture
		}
		return nil
	}
}
func fixtureOpenRAM(ctx context.Context, name string, write bool) (int, error) {
	if ctx.Err() != nil {
		return -1, errFixture
	}
	before, err := fixtureFIFO(name)
	if err != nil {
		return -1, errFixture
	}
	mode := syscall.O_RDONLY
	if write {
		mode = syscall.O_WRONLY
	}
	fd, err := syscall.Open(name, mode|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, errFixture
	}
	var after syscall.Stat_t
	prior, ok := before.Sys().(*syscall.Stat_t)
	if syscall.Fstat(fd, &after) != nil || !ok || prior.Dev != after.Dev || prior.Ino != after.Ino || ctx.Err() != nil {
		syscall.Close(fd)
		return -1, errFixture
	}
	return fd, nil
}

// Nonblocking syscalls plus a child context bounded by the absolute Root expiry
// avoid blocked open/read goroutines. Every opened descriptor is closed here.
func fixtureReadRAM(parent context.Context, name string, limit int, charge func(int) bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if limit < 1 {
		return nil, errFixture
	}
	fd, err := fixtureOpenRAM(ctx, name, false)
	if err != nil {
		return nil, errFixture
	}
	defer syscall.Close(fd)
	raw := make([]byte, 0, min(limit, 4096))
	buffer := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return raw, errFixture
		}
		size := min(len(buffer), limit+1-len(raw))
		if size < 1 {
			return raw, errFixture
		}
		n, readErr := syscall.Read(fd, buffer[:size])
		if n > 0 {
			raw = append(raw, buffer[:n]...)
			if charge != nil && !charge(n) {
				return raw, errFixture
			}
			if len(raw) > limit {
				return raw, errFixture
			}
		}
		if ctx.Err() != nil {
			return raw, errFixture
		}
		if readErr != nil && readErr != syscall.EAGAIN && readErr != syscall.EWOULDBLOCK && readErr != syscall.EINTR {
			return raw, errFixture
		}
		if n == 0 && readErr == nil && len(raw) > 0 {
			return raw, nil
		}
		if n <= 0 && fixturePause(ctx, 5*time.Millisecond) != nil {
			return raw, errFixture
		}
	}
}
func (c *fixtureHTTP) readOperatorRAM(protocolLimit int) ([]byte, error) {
	if !c.active() {
		return nil, c.refuse()
	}
	limit := min(protocolLimit, fixtureJSONBudget-c.bodyBytes-1)
	if limit < 1 {
		return nil, c.refuse()
	}
	raw, err := fixtureReadRAM(c.ctx, c.scope.OperatorAckPipe, limit, c.chargeRAM)
	if err != nil || !c.active() {
		return nil, c.refuse()
	}
	return raw, nil
}
func (c *fixtureHTTP) writeRAM(wire []byte) error {
	if !c.active() || len(wire) > 131072 || c.bodyBytes+len(wire) > fixtureJSONBudget {
		return c.refuse()
	}
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	fd, err := fixtureOpenRAM(ctx, c.scope.OutputPipe, true)
	if err != nil {
		return c.refuse()
	}
	defer syscall.Close(fd)
	for len(wire) > 0 {
		if ctx.Err() != nil {
			return c.refuse()
		}
		n, writeErr := syscall.Write(fd, wire[:min(4096, len(wire))])
		if n > 0 {
			if !c.chargeRAM(n) {
				return c.refuse()
			}
			wire = wire[n:]
		}
		if ctx.Err() != nil {
			return c.refuse()
		}
		if writeErr != nil && writeErr != syscall.EAGAIN && writeErr != syscall.EWOULDBLOCK && writeErr != syscall.EINTR {
			return c.refuse()
		}
		if n <= 0 && fixturePause(ctx, 5*time.Millisecond) != nil {
			return c.refuse()
		}
	}
	if !c.active() {
		return c.refuse()
	}
	return nil
}

// The acknowledgement cannot provide or override a revision. The existing owner
// must snapshot the changed Document/File pair and return the checked tuple.
func (c *fixtureHTTP) snapshotAR2(domain, owner string, old entity.KnowledgeContentTarget) (entity.KnowledgeContentSourceRef, error) {
	var none entity.KnowledgeContentSourceRef
	bound, ok := c.parsedBindings[old.Source.SourceID]
	if old.Validate(domain) != nil || old.Source.DomainID != domain || old.Representation.Kind != "parsed_representation" || !ok || bound.Source != old.Source {
		return none, c.refuse()
	}
	if _, err := c.operatorStage("a_r2", domain, owner, old.Source.SourceID); err != nil {
		return none, err
	}
	reply, err := fixtureJSON(c, "parsedSnapshot", "owner", []string{domain}, map[string]any{
		"contract_version": entity.KnowledgeContentVersion, "request": entity.KnowledgeParsedSnapshotRequest{
			DocumentID: old.Source.SourceID, ExpectedRevisionID: old.Source.RevisionID, ExpectedPolicyVersion: 1,
			RelativePath: "synthetic-A.csv", Classification: entity.KnowledgeDirectoryClassification{ComponentSide: "non_code"}}})
	if err != nil {
		return none, err
	}
	var actual struct {
		Source entity.KnowledgeContentSourceRef "json:\"source_ref\""
	}
	raw, err := fixtureMaterial("synthetic-A-r2.csv")
	if err != nil || json.Unmarshal(reply.data, &actual) != nil || actual.Source.DomainID != domain ||
		actual.Source.SourceID != old.Source.SourceID || actual.Source.RevisionID == old.Source.RevisionID ||
		!isHex(actual.Source.RevisionID, 32) || actual.Source.SHA256 != digest(raw) {
		return none, c.refuse()
	}
	reply, err = fixtureJSON(c, "parsedSource", "owner", []string{domain}, map[string]any{
		"contract_version": entity.KnowledgeContentVersion, "source_id": old.Source.SourceID})
	if err != nil {
		return none, err
	}
	var state entity.KnowledgeParsedSourceState
	if json.Unmarshal(reply.data, &state) != nil || state.Source != actual.Source || state.ByteLength != int64(len(raw)) ||
		state.LifecycleVersion < 1 {
		return none, c.refuse()
	}
	return actual.Source, nil
}
