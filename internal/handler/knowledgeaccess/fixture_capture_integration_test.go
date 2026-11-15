//go:build integration && linux

package knowledgeaccess

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
)

// These mirrors retain the owning manifest's exact JSON names and field order.
// They decode an immutable SQL publication; they never create a publication.
type fixtureCanonicalReference struct {
	Version             string                                    `json:"projection_version"`
	Address             entity.KnowledgeContentAddress            `json:"address"`
	CoordinateSpace     string                                    `json:"coordinate_space"`
	ReadingSHA          string                                    `json:"reading_text_sha256"`
	Range               entity.KnowledgeContentRange              `json:"range"`
	Locator             entity.KnowledgeContentLocator            `json:"locator"`
	OriginalPosition    *entity.KnowledgeDocumentOriginalPosition `json:"original_position"`
	CoordinatePrecision string                                    `json:"coordinate_precision"`
	Assembly            string                                    `json:"assembly"`
}
type fixtureCanonicalRow struct {
	ID           string                    `json:"row_id"`
	Binding      string                    `json:"binding_id"`
	Plan         string                    `json:"plan_sha256"`
	Reference    fixtureCanonicalReference `json:"reference"`
	ReferenceSHA string                    `json:"reference_sha256"`
	ByteLength   int64                     `json:"byte_length"`
}
type fixtureCapturedPolicies struct {
	FoundationVersion int64  `json:"foundation_policy_version"`
	LocalVersion      int64  `json:"local_policy_version"`
	ContentVersion    int64  `json:"content_policy_version"`
	Language          string `json:"language"`
	LocalEnabled      bool   `json:"local_enabled"`
	ContentEnabled    bool   `json:"content_enabled"`
}
type fixtureCapturedFence struct {
	SourceID string `json:"source_id"`
	SHA      string `json:"fence_sha256"`
}
type fixtureCanonicalManifest struct {
	Version        string                                   `json:"version"`
	Protocol       string                                   `json:"protocol"`
	Owner          string                                   `json:"owner"`
	Domain         string                                   `json:"domain"`
	Selection      entity.KnowledgeContentLexicalSelection  `json:"selection"`
	SelectionSHA   string                                   `json:"selection_sha256"`
	Policies       fixtureCapturedPolicies                  `json:"policies"`
	ProducerSHA    string                                   `json:"producer_sha256"`
	ProfileSHA     string                                   `json:"profile_sha256"`
	Coverage       []entity.KnowledgeContentLexicalCoverage `json:"coverage"`
	TokenlessBytes int64                                    `json:"tokenless_reading_bytes"`
	Rows           []fixtureCanonicalRow                    `json:"rows"`
	Sources        []fixtureCapturedFence                   `json:"sources"`
	PublishedAt    string                                   `json:"published_at"`
}
type fixtureSQLPublication struct {
	Protocol     string `json:"protocol"`
	ID           string `json:"id"`
	OwnerID      string `json:"owner_id"`
	DomainID     string `json:"domain_id"`
	SelectionSHA string `json:"selection_sha256"`
	ManifestSHA  string `json:"manifest_sha256"`
	Payload      string `json:"payload"`
}
type fixtureSQLCapture struct {
	CapabilityID     string                  `json:"capability_id"`
	OperatorName     string                  `json:"operator_name"`
	DriverSHA        string                  `json:"driver_artifact_sha256"`
	Snapshots        []fixtureTypedSnapshot  `json:"full_typed_snapshots"`
	RunID            string                  `json:"run_id"`
	Phase            string                  `json:"phase"`
	Nonce            string                  `json:"nonce"`
	OwnerID          string                  `json:"owner_id"`
	TenantID         string                  `json:"tenant_id"`
	DomainID         string                  `json:"domain_id"`
	ReceiptSHA       string                  `json:"receipt_sha256"`
	SQLStatements    int                     `json:"sql_statements"`
	SQLRows          int                     `json:"sql_rows"`
	HTTPAttempts     int                     `json:"http_attempts"`
	Publication      fixtureSQLPublication   `json:"publication"`
	Fences           []json.RawMessage       `json:"source_fence_shapes"`
	Readings         map[string]string       `json:"native_readings_by_source"`
	Policies         fixtureCapturedPolicies `json:"current_policies"`
	AccessPolicyRows int                     `json:"access_policy_rows"`
	ModuleRows       int                     `json:"module_rows"`
	GrantRows        int                     `json:"grant_rows"`
	NativePolicies   fixtureNativePolicies   `json:"native_policies"`
}

type fixtureNativePolicies struct {
	DomainID         string            `json:"domain_id"`
	OwnerID          string            `json:"owner_id"`
	TenantID         string            `json:"tenant_id"`
	Permission       string            `json:"permission"`
	KnowledgeEnabled bool              `json:"knowledge_enabled"`
	DocumentVersion  int64             `json:"document_policy_version"`
	DocumentEnabled  bool              `json:"document_enabled"`
	StructureVersion int64             `json:"structure_policy_version"`
	StructureEnabled bool              `json:"structure_enabled"`
	OwnerRole        string            `json:"owner_role"`
	RecipientID      string            `json:"recipient_id"`
	RecipientRole    string            `json:"recipient_role"`
	OwnerModels      map[string]string `json:"owner_model_values"`
}

func fixtureJSONBytes(value any) []byte { raw, _ := json.Marshal(value); return raw }
func fixtureExactFields(raw []byte, names string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	keys := strings.Fields(names)
	if len(fields) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	return true
}
func fixtureAccessTarget(target entity.KnowledgeContentTarget) json.RawMessage {
	return fixtureJSONBytes(map[string]any{"source": target.Source, "representation": target.Representation})
}

// The owning lexical digest is a tagged length-prefixed array, not JSON SHA.
func fixtureLexicalDigest(values ...any) string {
	var out bytes.Buffer
	var encode func(any) bool
	write := func(value uint64) { var raw [8]byte; binary.BigEndian.PutUint64(raw[:], value); out.Write(raw[:]) }
	encode = func(value any) bool {
		switch x := value.(type) {
		case string:
			if !utf8.ValidString(x) {
				return false
			}
			out.WriteByte('S')
			write(uint64(len(x)))
			out.WriteString(x)
		case int64:
			if x < 0 || x > 9007199254740991 {
				return false
			}
			out.WriteByte('U')
			write(uint64(x))
		case []any:
			out.WriteByte('A')
			write(uint64(len(x)))
			for _, child := range x {
				if !encode(child) {
					return false
				}
			}
		default:
			return false
		}
		return true
	}
	if !encode(values) {
		return ""
	}
	return digest(out.Bytes())
}

// A later Root lease must authorize the exact fixed SQL recipe, identities,
// targets and publication. This code only consumes that operator's RAM stream.
// No SQL client, credentials, statement text or write operation is accepted.
func (c *fixtureHTTP) captureOwner(phase, domain string, actor actorFixture, index entity.KnowledgeLexicalIndexRef, selection entity.KnowledgeContentLexicalSelection) (fixtureSQLCapture, string, error) {
	var result fixtureSQLCapture
	if !c.active() || !c.scope.Operator || (phase != "canonical_pre" && phase != "canonical_post") ||
		!isHex(domain, 32) || !isHex(actor.UserID, 32) || !isHex(actor.TenantID, 32) || !isHex(index.ID, 64) || !isHex(index.ManifestSHA256, 64) || len(selection.Targets) != 5 {
		return result, "", c.refuse()
	}
	cap, err := c.operatorCapability(phase)
	if err != nil {
		return result, "", err
	}
	nonce, err := fixtureRandomID(16)
	if err != nil {
		return result, "", c.refuse()
	}
	event := fixtureJSONBytes(map[string]any{"kind": "canonical_owner_capture", "run_id": c.scope.RunID, "phase": phase, "nonce": nonce,
		"capability_id": cap.ID, "operator_name": cap.Name, "driver_artifact_sha256": cap.Driver.SHA, "scope_sha256": c.scope.scopeSHA, "expires_at": c.scope.Expires,
		"owner_id": actor.UserID, "tenant_id": actor.TenantID, "domain_id": domain, "index_ref": index, "selection": selection,
		"action": "read_only_exact_fixture_publication_and_native_fences", "max_sql_statements": 16, "max_sql_rows": 128,
		"max_http_attempts": 0, "max_ack_bytes": 131072})
	if len(event) > 32768 || c.writeRAM(event) != nil {
		return result, "", c.refuse()
	}
	raw, err := c.readOperatorRAM(131072)
	if err != nil {
		return result, "", err
	}
	if !fixtureExactFields(raw, "capability_id operator_name driver_artifact_sha256 full_typed_snapshots run_id phase nonce owner_id tenant_id domain_id receipt_sha256 sql_statements sql_rows http_attempts publication source_fence_shapes native_readings_by_source current_policies access_policy_rows module_rows grant_rows native_policies") ||
		decodeClosed(raw, &result) != nil || c.bodyBytes > fixtureJSONBudget || !c.active() ||
		result.CapabilityID != cap.ID || result.OperatorName != cap.Name || result.DriverSHA != cap.Driver.SHA || len(result.Snapshots) != 2 ||
		result.RunID != c.scope.RunID || result.Phase != phase || result.Nonce != nonce || result.OwnerID != actor.UserID || result.TenantID != actor.TenantID || result.DomainID != domain ||
		!fixtureHash(result.ReceiptSHA) || result.HTTPAttempts != 0 || result.SQLStatements < 1 || result.SQLStatements > 16 || result.SQLRows < 1 || result.SQLRows > 128 ||
		result.AccessPolicyRows != 0 || result.ModuleRows != 0 || result.GrantRows != 0 || len(result.Fences) != 5 || len(result.Readings) != 5 ||
		result.Publication.ID != index.ID || result.Publication.ManifestSHA != index.ManifestSHA256 || result.Publication.OwnerID != actor.UserID || result.Publication.DomainID != domain {
		return result, "", c.refuse()
	}
	return result, digest(raw), nil
}

func fixtureDecodePublication(capture fixtureSQLCapture, job entity.KnowledgeContentLexicalJob) (fixtureCanonicalManifest, error) {
	var m fixtureCanonicalManifest
	p := capture.Publication
	if job.Index == nil || p.Protocol != entity.KnowledgeContentLexicalProtocol || p.ID != job.Index.ID || p.ManifestSHA != job.Index.ManifestSHA256 ||
		len(p.Payload) > 65536 || digest([]byte(p.Payload)) != p.ManifestSHA || fixtureLexicalDigest("knowledge-content-index-publication/0.2.0", p.OwnerID, p.DomainID, p.ManifestSHA) != p.ID ||
		decodeClosed([]byte(p.Payload), &m) != nil || string(fixtureJSONBytes(m)) != p.Payload || m.Version != entity.KnowledgeContentVersion || m.Protocol != p.Protocol || m.Owner != p.OwnerID || m.Domain != p.DomainID ||
		m.SelectionSHA != p.SelectionSHA || m.SelectionSHA != digest(fixtureJSONBytes(m.Selection)) || !reflect.DeepEqual(m.Selection, job.Selection) || m.Policies != capture.Policies ||
		!m.Policies.LocalEnabled || !m.Policies.ContentEnabled || m.Policies.FoundationVersion != job.FoundationVersion || m.Policies.LocalVersion != job.LocalVersion || m.Policies.ContentVersion != job.ContentVersion ||
		m.ProfileSHA != job.ProfileSHA256 || m.ProducerSHA != job.ProducerSHA256 || len(m.Rows) < 6 || len(m.Rows) > 9 || len(m.Sources) != 5 || len(m.Coverage) != 5 || m.PublishedAt == "" {
		return m, errFixture
	}
	return m, nil
}

func fixtureValidateFences(capture fixtureSQLCapture, m fixtureCanonicalManifest, actor actorFixture, ar2 entity.KnowledgeContentSourceRef) error {
	seen := map[string]bool{}
	base := strings.Fields("kind target head_revision_id head_input_sha256 lifecycle_version lifecycle_state fixed_input_sha256 fixed_kind fixed_file_id fixed_commit_id fixed_folder_id fixed_byte_length native_file_id native_parent_id native_tenant_id native_name native_location native_size native_source_type")
	for _, raw := range capture.Fences {
		var shape map[string]any
		if json.Unmarshal(raw, &shape) != nil {
			return errFixture
		}
		allowed := map[string]bool{}
		for _, key := range base {
			allowed[key] = true
			if _, ok := shape[key]; !ok {
				return errFixture
			}
		}
		var target entity.KnowledgeContentTarget
		if decodeClosed(fixtureJSONBytes(shape["target"]), &target) != nil || target.Source.DomainID != capture.DomainID || seen[target.Source.SourceID] {
			return errFixture
		}
		seen[target.Source.SourceID] = true
		switch target.Representation.Kind {
		case "document_representation", "parsed_representation":
			for _, key := range []string{"representation_payload_sha256", "representation_manifest_sha256", "reading_text_sha256"} {
				allowed[key] = true
				value, _ := shape[key].(string)
				if !isHex(value, 64) {
					return errFixture
				}
			}
		case "structure_view":
			allowed["view_payload_sha256"] = true
			value, _ := shape["view_payload_sha256"].(string)
			if !isHex(value, 64) {
				return errFixture
			}
		case "source_revision":
		default:
			return errFixture
		}
		for key := range shape {
			if !allowed[key] {
				return errFixture
			}
		}
		expectedHead := target.Source.RevisionID
		if target.Source.SourceID == ar2.SourceID {
			expectedHead = ar2.RevisionID
		}
		if shape["kind"] != "content-lexical-source-fence/0.2.0" || shape["head_revision_id"] != expectedHead || shape["native_tenant_id"] != actor.TenantID || shape["native_source_type"] != "knowledgebase" || shape["lifecycle_state"] != "active" {
			return errFixture
		}
		version, ok := shape["lifecycle_version"].(float64)
		if !ok || version < 1 || version != float64(int64(version)) {
			return errFixture
		}
		for _, key := range []string{"head_input_sha256", "fixed_input_sha256"} {
			value, _ := shape[key].(string)
			if !isHex(value, 64) {
				return errFixture
			}
		}
		actual, err := entity.KnowledgeLifecycleDigest(shape)
		if err != nil {
			return errFixture
		}
		matched := false
		for i, fence := range m.Sources {
			if fence.SourceID == target.Source.SourceID && fence.SHA == actual && reflect.DeepEqual(m.Selection.Targets[i], target) {
				matched = true
			}
		}
		if !matched {
			return errFixture
		}
		reading, ok := capture.Readings[target.Source.SourceID]
		if !ok || !utf8.ValidString(reading) || len(reading) > 16384 || reading == "" {
			return errFixture
		}
		if target.Representation.Kind == "parsed_representation" || target.Representation.Kind == "document_representation" {
			if shape["reading_text_sha256"] != digest([]byte(reading)) {
				return errFixture
			}
		}
	}
	return nil
}

func fixtureEvidence(index entity.KnowledgeLexicalIndexRef, row fixtureCanonicalRow) entity.KnowledgeContentLexicalEvidence {
	r := row.Reference
	return entity.KnowledgeContentLexicalEvidence{Version: entity.KnowledgeContentVersion, Index: index, RowID: row.ID, Address: r.Address, CoordinateSpace: r.CoordinateSpace, ReadingSHA256: r.ReadingSHA,
		Range: r.Range, Locator: r.Locator, OriginalPosition: r.OriginalPosition, CoordinatePrecision: r.CoordinatePrecision, Assembly: r.Assembly}
}

// Consume only actual owner0.2 reads/search. Raw SQL and native locations remain
// in RAM; the public oracle receives only synthetic IDs/hashes/reading metadata.
func (c *fixtureHTTP) collectOwnerProof(domain string, actors fixtureActors, job entity.KnowledgeContentLexicalJob, ar2 entity.KnowledgeContentSourceRef, names map[string]string) (fixtureOwnerProof, error) {
	var proof fixtureOwnerProof
	if job.Index == nil || len(names) != 5 {
		return proof, c.refuse()
	}
	pre, preSHA, err := c.captureOwner("canonical_pre", domain, actors.Public["owner"], *job.Index, job.Selection)
	if err != nil {
		return proof, err
	}
	m, err := fixtureDecodePublication(pre, job)
	if err != nil || fixtureValidateFences(pre, m, actors.Public["owner"], ar2) != nil {
		return proof, c.refuse()
	}
	fullTyped, typedReceipts, err := fixtureFullTyped(pre, m, names)
	if err != nil {
		return proof, c.refuse()
	}
	allowed := []json.RawMessage{}
	native := pre.NativePolicies
	if native.DomainID != domain || native.OwnerID != actors.Public["owner"].UserID || native.TenantID != actors.Public["owner"].TenantID ||
		native.Permission != "me" || !native.KnowledgeEnabled || !native.DocumentEnabled || !native.StructureEnabled || native.DocumentVersion < 1 || native.StructureVersion < 1 ||
		native.OwnerRole != "owner" || native.RecipientID != actors.Public["recipient"].UserID || native.RecipientRole != "normal" || len(native.OwnerModels) != 6 {
		return proof, c.refuse()
	}
	for _, key := range []string{"llm_id", "embd_id", "rerank_id", "asr_id", "img2txt_id", "tts_id"} {
		value, ok := native.OwnerModels[key]
		if !ok || value != "" {
			return proof, c.refuse()
		}
	}
	byRow := map[string]fixtureCanonicalRow{}
	seen := map[string]bool{}
	offsets := map[string]int64{}
	oldReadable := false
	for _, target := range job.Selection.Targets {
		if names[target.Source.SourceID] != "synthetic-B.csv" {
			allowed = append(allowed, fixtureAccessTarget(target))
		}
	}
	if len(allowed) != 4 {
		return proof, c.refuse()
	}
	proof.Scope = fixtureJSONBytes(map[string]any{"kind": "fixed_published_targets", "publication": map[string]string{"id": job.Index.ID, "manifest_sha256": job.Index.ManifestSHA256}, "targets": allowed})
	// Manifest rows are ID sorted; whole-unit traversal is range sorted per source.
	rows := append([]fixtureCanonicalRow(nil), m.Rows...)
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Reference, rows[j].Reference
		if a.Address.Target.Source.SourceID != b.Address.Target.Source.SourceID {
			return a.Address.Target.Source.SourceID < b.Address.Target.Source.SourceID
		}
		return a.Range.StartByte < b.Range.StartByte
	})
	for _, row := range rows {
		r := row.Reference
		source := r.Address.Target.Source
		reading, ok := pre.Readings[source.SourceID]
		name := names[source.SourceID]
		if !ok || name == "" || seen[row.ID] || !isHex(row.ID, 64) || !isHex(row.Binding, 64) || r.Version != entity.KnowledgeContentVersion || r.Address.Node.ID == nil ||
			r.Range.SHA256 == nil || r.Range.StartByte != offsets[source.SourceID] || r.Range.EndByte <= r.Range.StartByte || r.Range.EndByte > int64(len(reading)) || r.Range.EndByte-r.Range.StartByte > 8192 ||
			!utf8.ValidString(reading[:r.Range.StartByte]) || !utf8.ValidString(reading[:r.Range.EndByte]) || digest([]byte(reading)) != r.ReadingSHA ||
			digest([]byte(reading[r.Range.StartByte:r.Range.EndByte])) != *r.Range.SHA256 || row.ByteLength != r.Range.EndByte-r.Range.StartByte || row.Plan != m.ProfileSHA ||
			row.ReferenceSHA != digest(fixtureJSONBytes(r)) || row.Binding != fixtureLexicalDigest("knowledge-index-binding/0.2.0", digest(fixtureJSONBytes(r.Address.Target)), m.ProfileSHA, r.CoordinateSpace, r.ReadingSHA) ||
			row.ID != fixtureLexicalDigest("knowledge-index-row/0.2.0", row.Binding, row.ReferenceSHA, row.ByteLength, *r.Range.SHA256) {
			return proof, c.refuse()
		}
		seen[row.ID] = true
		byRow[row.ID] = row
		offsets[source.SourceID] = r.Range.EndByte
		evidence := fixtureEvidence(*job.Index, row)
		if evidence.Validate(domain) != nil {
			return proof, c.refuse()
		}
		if name == "synthetic-B.csv" {
			raw, err := fixtureMaterial(name)
			if err != nil || source.SHA256 != digest(raw) || reading != "类别\t代号\n合成B\tB_ONLY_NOT_AUTHORIZED" {
				return proof, c.refuse()
			}
			continue
		}
		if len(proof.Units) >= 8 {
			return proof, c.refuse()
		}
		// B is checked by the actual owner search below. At most eight owner
		// reads independently verify every authorized complete publication unit.
		reply, err := fixtureJSON(c, "ownerRead", "owner", []string{domain}, entity.KnowledgeContentLexicalReadRequest{Version: entity.KnowledgeContentVersion, Evidence: evidence})
		if err != nil {
			return proof, err
		}
		var actual entity.KnowledgeContentLexicalReadData
		if json.Unmarshal(reply.data, &actual) != nil || !reflect.DeepEqual(actual.Evidence, evidence) || actual.Encoding != "utf-8" || !actual.Complete || actual.ByteLength != int(row.ByteLength) ||
			actual.Text != reading[r.Range.StartByte:r.Range.EndByte] || actual.Context.ParentExpansion != "disabled" {
			return proof, c.refuse()
		}
		if source.SourceID == ar2.SourceID {
			if actual.Currentness != "historical" {
				return proof, c.refuse()
			}
			oldReadable = true
		} else if actual.Currentness != "current" {
			return proof, c.refuse()
		}
		proof.Units = append(proof.Units, fixtureOwnerUnit{Name: name, Source: source, Target: fixtureAccessTarget(r.Address.Target), RawRowID: row.ID, Binding: row.Binding, CanonicalReferenceSHA: row.ReferenceSHA,
			ReferenceRange: byteRange{Start: int(r.Range.StartByte), End: int(r.Range.EndByte), SHA: *r.Range.SHA256}, NodeID: *r.Address.Node.ID, CoordinateSpace: r.CoordinateSpace,
			FullReading: reading, NativeReadingSHA: r.ReadingSHA, Typed: fullTyped[source.SourceID], FullTypedMetadataSHA: typedReceipts[source.SourceID], OwnerMetadataReceiptSHA: digest(reply.data)})
	}
	for _, coverage := range m.Coverage {
		if !coverage.ReadingTextComplete || coverage.CoveredBytes != coverage.ExpectedBytes || offsets[coverage.Target.Source.SourceID] != coverage.ExpectedBytes ||
			coverage.ReadingSHA256 != digest([]byte(pre.Readings[coverage.Target.Source.SourceID])) {
			return proof, c.refuse()
		}
	}
	query, err := fixtureJSON(c, "ownerSearch", "owner", []string{domain}, entity.KnowledgeContentLexicalSearchRequest{Version: entity.KnowledgeContentVersion, Index: *job.Index, Selection: job.Selection, Query: "合成", Limit: 20})
	if err != nil {
		return proof, err
	}
	var matches entity.KnowledgeContentLexicalSearchData
	if json.Unmarshal(query.data, &matches) != nil || matches.Query != "合成" || matches.RequestedLimit != 20 || matches.ReturnedCount != len(matches.Hits) || matches.TotalMatches != nil || matches.Pagination != "none" || matches.Ranking != "native_lexical_only" ||
		!reflect.DeepEqual(matches.Binding.Index, *job.Index) || !reflect.DeepEqual(matches.Binding.Selection, job.Selection) || matches.ParentExpansion != "disabled" {
		return proof, c.refuse()
	}
	matched := map[string]bool{}
	for _, hit := range matches.Hits {
		row, ok := byRow[hit.Evidence.RowID]
		if !ok || matched[row.ID] || !reflect.DeepEqual(hit.Evidence, fixtureEvidence(*job.Index, row)) ||
			hit.Text != pre.Readings[row.Reference.Address.Target.Source.SourceID][row.Reference.Range.StartByte:row.Reference.Range.EndByte] {
			return proof, c.refuse()
		}
		matched[row.ID] = true
		if names[row.Reference.Address.Target.Source.SourceID] == "synthetic-B.csv" {
			proof.MatchingDeniedRows = append(proof.MatchingDeniedRows, row.ID)
		} else {
			proof.MatchingAllowedRows = append(proof.MatchingAllowedRows, row.ID)
		}
	}
	post, postSHA, err := c.captureOwner("canonical_post", domain, actors.Public["owner"], *job.Index, job.Selection)
	if err != nil || !reflect.DeepEqual(pre.Publication, post.Publication) || !reflect.DeepEqual(pre.Fences, post.Fences) || !reflect.DeepEqual(pre.Readings, post.Readings) || !reflect.DeepEqual(pre.NativePolicies, post.NativePolicies) || !reflect.DeepEqual(pre.Snapshots, post.Snapshots) || pre.Policies != post.Policies || fixtureValidateFences(post, m, actors.Public["owner"], ar2) != nil {
		return proof, c.refuse()
	}
	proof.PublicationID, proof.ManifestSHA, proof.CaptureReceiptSHA = job.Index.ID, job.Index.ManifestSHA256, digest([]byte(preSHA+postSHA))
	proof.AR2CreatedBeforePublication = true
	proof.OldR1OwnerReadable = oldReadable
	proof.StableHeadsLifecycleNativePolicies = true
	proof.FreshS1PolicyZeroModulesZeroGrants = true
	proof.ActualOwnerQuery = "合成"
	proof.Forbidden = []string{"B_ONLY_NOT_AUTHORIZED", "A_NEW_REVISION_NOT_GRANTED"}
	return proof, nil
}
