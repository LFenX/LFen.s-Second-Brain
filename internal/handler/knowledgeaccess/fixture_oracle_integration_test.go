//go:build integration && linux

package knowledgeaccess

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"ragflow/internal/entity"
)

// All fields below are public synthetic owner observations. There is no
// session/password field. The bridge must obtain these before S1 consumption.
type fixtureOwnerUnit struct {
	Name                                     string
	Source                                   entity.KnowledgeContentSourceRef
	Target                                   json.RawMessage
	RawRowID, Binding, CanonicalReferenceSHA string
	ReferenceRange                           byteRange
	NodeID, CoordinateSpace                  string
	FullReading                              string
	NativeReadingSHA                         string
	Typed                                    []entity.KnowledgeContentLexicalTypedMember
	OwnerMetadataReceiptSHA                  string
	FullTypedMetadataSHA                     string
}

type fixtureOwnerProof struct {
	Scope                                           json.RawMessage
	Units                                           []fixtureOwnerUnit
	MatchingAllowedRows, MatchingDeniedRows         []string
	Forbidden                                       []string
	PublicationID, ManifestSHA, CaptureReceiptSHA   string
	AR2CreatedBeforePublication, OldR1OwnerReadable bool
	StableHeadsLifecycleNativePolicies              bool
	FreshS1PolicyZeroModulesZeroGrants              bool
	ActualOwnerQuery                                string
}

// The oracle uses original bytes/owner metadata, never a S1 inspect/read result.
// It validates the bounded whole units and emits an intentionally unprepared
// public candidate; only Root may review and admit a different prepared artifact.
func fixturePublicOracle(proof fixtureOwnerProof, actors fixtureActors, domains map[string]string, contracts *contractSet) (*setupFixture, error) {
	bad := func() (*setupFixture, error) { return nil, errFixtureNotReady }
	if !proof.AR2CreatedBeforePublication || !proof.OldR1OwnerReadable || !proof.StableHeadsLifecycleNativePolicies ||
		!proof.FreshS1PolicyZeroModulesZeroGrants || !isHex(proof.PublicationID, 64) || !isHex(proof.ManifestSHA, 64) ||
		!isHex(proof.CaptureReceiptSHA, 64) || proof.ActualOwnerQuery != "合成" || len(proof.Units) < 5 || len(proof.Units) > 8 {
		return bad()
	}
	scope, err := strictJSON(proof.Scope)
	if err != nil || contracts.definitions["ModuleScope"].Validate(scope) != nil {
		return bad()
	}
	var fixed struct {
		Publication struct {
			ID  string `json:"id"`
			SHA string `json:"manifest_sha256"`
		} `json:"publication"`
		Targets []json.RawMessage `json:"targets"`
	}
	if json.Unmarshal(proof.Scope, &fixed) != nil || fixed.Publication.ID != proof.PublicationID || fixed.Publication.SHA != proof.ManifestSHA || len(fixed.Targets) != 4 {
		return bad()
	}
	selected := map[string]bool{}
	for _, target := range fixed.Targets {
		key, e := targetKey(target)
		if e != nil || selected[key] {
			return bad()
		}
		selected[key] = true
	}
	units := make([]expectedUnit, 0, len(proof.Units))
	kinds := map[string]int{}
	rows := map[string]bool{}
	docxWitness := false
	c1 := 0
	for _, observed := range proof.Units {
		if observed.Source.DomainID != domains["owner"] || !isHex(observed.Source.SourceID, 32) ||
			!isHex(observed.Source.RevisionID, 32) || !isHex(observed.OwnerMetadataReceiptSHA, 64) ||
			!isHex(observed.RawRowID, 64) || rows[observed.RawRowID] || !isHex(observed.Binding, 64) ||
			!isHex(observed.CanonicalReferenceSHA, 64) || observed.NodeID == "" {
			return bad()
		}
		rows[observed.RawRowID] = true
		targetIdentity, e := targetKey(observed.Target)
		if e != nil || !selected[targetIdentity] {
			return bad()
		}
		var target struct {
			Source struct {
				Domain   string "json:\"domain_id\""
				ID       string "json:\"source_id\""
				Revision string "json:\"source_revision_id\""
				SHA      string "json:\"source_sha256\""
			} "json:\"source\""
			Representation struct {
				Kind string "json:\"kind\""
			} "json:\"representation\""
		}
		if json.Unmarshal(observed.Target, &target) != nil || target.Source.Domain != observed.Source.DomainID || target.Source.ID != observed.Source.SourceID || target.Source.Revision != observed.Source.RevisionID || target.Source.SHA != observed.Source.SHA256 {
			return bad()
		}
		kind := target.Representation.Kind
		kinds[kind]++
		full := ""
		switch kind {
		case "source_revision", "structure_view":
			name := "long-source.txt"
			if kind == "structure_view" {
				name = "guide.md"
			}
			raw, e := fixtureMaterial(name)
			if e != nil {
				return bad()
			}
			if observed.Source.SHA256 != digest(raw) || observed.Name != name {
				return bad()
			}
			full = string(raw)
			if kind == "source_revision" {
				c1++
			}
		case "parsed_representation":
			if !fixtureHash(observed.FullTypedMetadataSHA) {
				return bad()
			}
			raw, e := fixtureMaterial(observed.Name)
			if e != nil || observed.Source.SHA256 != digest(raw) {
				return bad()
			}
			if observed.Name != "synthetic-A.csv" {
				return bad()
			}
			full = "类别\t代号\n合成A\tA-001\n空值\t"
		case "document_representation":
			if observed.Name != "empty-cell.docx" {
				return bad()
			}
			if !fixtureHash(observed.FullTypedMetadataSHA) {
				return bad()
			}
			raw, e := fixtureMaterial("empty-cell.docx")
			if e != nil || observed.Source.SHA256 != digest(raw) {
				return bad()
			}
			// Full derived reading is independently captured from actual R2 metadata.
			full = observed.FullReading
			if full == "" || !strings.Contains(full, "合成 DOCX") || !strings.Contains(full, "A") || !strings.Contains(full, "B") {
				return bad()
			}
			if fixtureEmptyCellWitness(full, observed.Typed) != nil {
				return bad()
			}
			docxWitness = true
		default:
			return bad()
		}
		if !utf8.ValidString(full) || digest([]byte(full)) != observed.NativeReadingSHA {
			return bad()
		}
		r := observed.ReferenceRange
		if r.Start < 0 || r.End <= r.Start || r.End > len(full) || r.End-r.Start > 8192 ||
			!utf8.ValidString(full[:r.Start]) || !utf8.ValidString(full[:r.End]) || digest([]byte(full[r.Start:r.End])) != r.SHA {
			return bad()
		}
		text := full[r.Start:r.End]
		typed, err := fixtureSummarizeTyped(full, r, observed.Typed)
		if err != nil {
			return bad()
		}
		cite := citation{Target: observed.Target, NodeID: observed.NodeID, CoordinateSpace: observed.CoordinateSpace, Range: r,
			DisplayName: observed.Name, PositionLabel: fmt.Sprintf("UTF-8 bytes %d–%d", r.Start, r.End), RevisionBinding: "fixed"}
		citationRaw, _ := json.Marshal(cite)
		checked, e := strictJSON(citationRaw)
		if e != nil || contracts.definitions["Citation"].Validate(checked) != nil {
			return bad()
		}
		units = append(units, expectedUnit{Target: observed.Target, Citation: citationRaw, Text: text, TypedContext: typed,
			RawRowID: observed.RawRowID, Binding: observed.Binding, CanonicalRefSHA: observed.CanonicalReferenceSHA})
	}
	if len(kinds) != 4 || c1 != 2 || !docxWitness {
		return bad()
	}
	sort.Slice(units, func(i, j int) bool {
		a, _ := targetKey(units[i].Target)
		b, _ := targetKey(units[j].Target)
		if a != b {
			return a < b
		}
		var x, y citation
		_ = json.Unmarshal(units[i].Citation, &x)
		_ = json.Unmarshal(units[j].Citation, &y)
		if x.Range.Start != y.Range.Start {
			return x.Range.Start < y.Range.Start
		}
		if x.Range.End != y.Range.End {
			return x.Range.End < y.Range.End
		}
		if x.NodeID != y.NodeID {
			return x.NodeID < y.NodeID
		}
		return units[i].RawRowID < units[j].RawRowID
	})
	allowed := append([]string(nil), proof.MatchingAllowedRows...)
	sort.Strings(allowed)
	denied := append([]string(nil), proof.MatchingDeniedRows...)
	sort.Strings(denied)
	if len(allowed) < 2 || len(denied) < 1 || !isHex(denied[0], 64) || denied[0] >= allowed[0] {
		return bad()
	}
	for i, id := range allowed {
		if !rows[id] || (i > 0 && id == allowed[i-1]) {
			return bad()
		}
	}
	for _, id := range denied {
		if !isHex(id, 64) || rows[id] {
			return bad()
		}
	}
	if len(proof.Forbidden) < 1 || len(proof.Forbidden) > 16 {
		return bad()
	}
	for _, marker := range proof.Forbidden {
		if len(marker) < 8 || len(marker) > 512 {
			return bad()
		}
	}
	absent, err := fixtureRandomID(16)
	if err != nil {
		return bad()
	}
	models := map[string]string{}
	for _, key := range []string{"llm_id", "embd_id", "rerank_id", "asr_id", "img2txt_id", "tts_id"} {
		models[key] = ""
	}
	return &setupFixture{Prepared: false, DedicatedSynthetic: true, ExistingDataProtected: true,
		Owner: actors.Public["owner"], Recipient: actors.Public["recipient"], Foreign: actors.Public["foreign"],
		DomainID: domains["owner"], ForeignDomainID: domains["foreign"], AbsentID: absent, Scope: proof.Scope, Units: units,
		SearchQuery: proof.ActualOwnerQuery, SearchRows: allowed, DeniedRowIDs: denied, Forbidden: proof.Forbidden, NativeModelValues: models}, nil
}

type fixtureStoredTypedNode struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Ordinal  int64  `json:"ordinal"`
	Payload  string `json:"payload"`
}

// Exact native representation and complete immutable manifest/node rows, never
// the owner0.2 filtered context. SQL recipe statement8 joins the bounded nodes
// into the existing representation SELECT; no extra GET or SELECT is needed.
type fixtureTypedSnapshot struct {
	OwnerID  string                        `json:"owner_id"`
	DomainID string                        `json:"domain_id"`
	Protocol string                        `json:"protocol"`
	Target   entity.KnowledgeContentTarget `json:"target"`
	Payload  string                        `json:"payload"`
	Manifest string                        `json:"manifest"`
	Nodes    []fixtureStoredTypedNode      `json:"nodes"`
}

func fixtureRangeMatches(body string, r entity.KnowledgeContentRange) bool {
	return r.SHA256 != nil && r.StartByte >= 0 && r.EndByte >= r.StartByte && r.EndByte <= int64(len(body)) &&
		utf8.ValidString(body[:r.StartByte]) && utf8.ValidString(body[:r.EndByte]) && digest([]byte(body[r.StartByte:r.EndByte])) == *r.SHA256
}

func fixtureFullTyped(capture fixtureSQLCapture, m fixtureCanonicalManifest, names map[string]string) (map[string][]entity.KnowledgeContentLexicalTypedMember, map[string]string, error) {
	members := map[string][]entity.KnowledgeContentLexicalTypedMember{}
	receipts := map[string]string{}
	if len(capture.Snapshots) != 2 {
		return nil, nil, errFixture
	}
	for _, snap := range capture.Snapshots {
		t := snap.Target
		id := t.Source.SourceID
		name := names[id]
		if snap.OwnerID != m.Owner || snap.DomainID != m.Domain || t.Validate(m.Domain) != nil || (name != "synthetic-A.csv" && name != "empty-cell.docx") || members[id] != nil || len(snap.Nodes) < 1 || len(snap.Nodes) > 64 {
			return nil, nil, errFixture
		}
		selected := false
		for _, target := range m.Selection.Targets {
			if reflect.DeepEqual(t, target) {
				selected = true
			}
		}
		if !selected {
			return nil, nil, errFixture
		}
		bound := false
		for _, raw := range capture.Fences {
			var shape struct {
				Target      entity.KnowledgeContentTarget `json:"target"`
				PayloadSHA  string                        `json:"representation_payload_sha256"`
				ManifestSHA string                        `json:"representation_manifest_sha256"`
			}
			if json.Unmarshal(raw, &shape) == nil && reflect.DeepEqual(shape.Target, t) && shape.PayloadSHA == digest([]byte(snap.Payload)) && shape.ManifestSHA == digest([]byte(snap.Manifest)) {
				bound = true
			}
		}
		if !bound {
			return nil, nil, errFixture
		}
		body := capture.Readings[id]
		var actual []entity.KnowledgeContentLexicalTypedMember
		var err error
		if name == "synthetic-A.csv" {
			actual, err = fixtureParsedTyped(snap, body)
		} else {
			actual, err = fixtureDOCXTyped(snap, body)
		}
		if err != nil {
			return nil, nil, err
		}
		members[id] = actual
		receipts[id] = digest(fixtureJSONBytes(snap))
	}
	if len(members) != 2 {
		return nil, nil, errFixture
	}
	return members, receipts, nil
}

func fixtureParsedTyped(snap fixtureTypedSnapshot, body string) ([]entity.KnowledgeContentLexicalTypedMember, error) {
	var rep entity.KnowledgeParsedRepresentation
	var manifest []entity.KnowledgeParsedManifestEntry
	if snap.Protocol != entity.KnowledgeParsedProtocol || decodeClosed([]byte(snap.Payload), &rep) != nil || decodeClosed([]byte(snap.Manifest), &manifest) != nil ||
		!reflect.DeepEqual(snap.Target, entity.KnowledgeContentTarget{Source: rep.Source, Representation: rep.Ref}) || len(manifest) != len(snap.Nodes) || len(manifest) > 64 {
		return nil, errFixture
	}
	nodes := make([]entity.KnowledgeParsedNode, len(manifest))
	seen := map[int64]bool{}
	for _, row := range snap.Nodes {
		var node entity.KnowledgeParsedNode
		if decodeClosed([]byte(row.Payload), &node) != nil || node.Payload.LocalIndex < 0 || node.Payload.LocalIndex >= int64(len(nodes)) || seen[node.Payload.LocalIndex] ||
			row.ID != node.NodeID || row.Ordinal != node.Payload.Ordinal {
			return nil, errFixture
		}
		parent := ""
		if node.Payload.ParentLocalIndex != nil {
			if rep.RootRef.ID == nil {
				return nil, errFixture
			}
			parent = *rep.RootRef.ID
		}
		if row.ParentID != parent {
			return nil, errFixture
		}
		seen[node.Payload.LocalIndex] = true
		nodes[node.Payload.LocalIndex] = node
	}
	if entity.KnowledgeParsedValidateBound(rep, body, nodes, manifest) != nil {
		return nil, errFixture
	}
	result := make([]entity.KnowledgeContentLexicalTypedMember, 0, len(nodes))
	for _, node := range nodes {
		p := node.Payload
		id, ordinal := node.NodeID, p.Ordinal
		n := entity.KnowledgeContentNode{Ref: entity.KnowledgeContentNodeRef{Kind: "parsed_node", ID: &id}, Kind: p.Kind, Name: p.Name, Ordinal: &ordinal, Locator: p.Locator,
			OriginalPosition: p.OriginalPosition, CoordinatePrecision: p.CoordinatePrecision, Text: p.Text, Attributes: p.Attributes}
		if p.ParentLocalIndex != nil {
			parent := rep.RootRef
			n.ParentRef = &parent
		}
		if n.Validate(&snap.Target) != nil {
			return nil, errFixture
		}
		result = append(result, entity.KnowledgeContentLexicalTypedMember{Node: n, PayloadSHA256: node.PayloadSHA256, TextAssembly: "existing_reading_range"})
	}
	return result, nil
}

func fixtureDOCXTyped(snap fixtureTypedSnapshot, body string) ([]entity.KnowledgeContentLexicalTypedMember, error) {
	var rep entity.KnowledgeDocumentRepresentation
	var manifest entity.KnowledgeDocumentStructureManifest
	original, materialErr := fixtureMaterial("empty-cell.docx")
	if snap.Protocol != "document_structure/0.1.0" || decodeClosed([]byte(snap.Payload), &rep) != nil || decodeClosed([]byte(snap.Manifest), &manifest) != nil ||
		materialErr != nil || snap.Target.Source.SHA256 != digest(original) || rep.Original.ByteLength != int64(len(original)) || rep.Original.OriginalFilename != "empty-cell.docx" ||
		rep.OwnerID != snap.OwnerID || rep.DomainID != snap.DomainID || snap.Target.Representation.ID == nil || rep.RepresentationID != *snap.Target.Representation.ID ||
		snap.Target.Representation.ProducerSHA256 == nil || rep.ProducerSHA256 != *snap.Target.Representation.ProducerSHA256 || rep.ProfileID != entity.KnowledgeDocumentDOCXProfile ||
		rep.Original.SourceID != snap.Target.Source.SourceID || rep.Original.SourceRevisionID != snap.Target.Source.RevisionID || rep.Original.SHA256 != snap.Target.Source.SHA256 ||
		rep.ReadingTextSHA256 != digest([]byte(body)) || rep.ReadingByteLength != int64(len(body)) || rep.NodeCount != int64(len(snap.Nodes)) || len(manifest.Nodes) != len(snap.Nodes) ||
		rep.UnsupportedBlockCount != 0 || rep.ModelCalls != 0 || rep.LocalInference != 0 || rep.ExternalFetches != 0 || rep.Configuration.OCR || rep.Configuration.Models || rep.Configuration.RemoteResources {
		return nil, errFixture
	}
	repID, err := entity.KnowledgeDocumentRepresentationDigest(rep)
	if err != nil || repID != rep.RepresentationID {
		return nil, errFixture
	}
	producerSHA, err := entity.KnowledgeDocumentProducerDigest(rep.Producer)
	configurationSHA, configurationErr := entity.KnowledgeDocumentConfigurationDigest(rep.Configuration)
	if err != nil || configurationErr != nil || producerSHA != rep.ProducerSHA256 || configurationSHA != rep.Producer.ConfigurationSHA256 {
		return nil, errFixture
	}
	manifestSHA, err := entity.KnowledgeDocumentManifestDigest(manifest.Nodes)
	if err != nil || manifestSHA != rep.StructureManifestSHA256 {
		return nil, errFixture
	}
	byID := map[string]fixtureStoredTypedNode{}
	for _, row := range snap.Nodes {
		if _, ok := byID[row.ID]; ok {
			return nil, errFixture
		}
		byID[row.ID] = row
	}
	parents := map[string]string{}
	result := make([]entity.KnowledgeContentLexicalTypedMember, 0, len(snap.Nodes))
	for i, entry := range manifest.Nodes {
		id, err := entity.KnowledgeDocumentNodeDigest(repID, rep.Original.SourceRevisionID, entry.NodePayloadSHA256)
		if err != nil {
			return nil, errFixture
		}
		row, ok := byID[id]
		if !ok {
			return nil, errFixture
		}
		var node entity.KnowledgeDocumentNode
		if decodeClosed([]byte(row.Payload), &node) != nil || node.DocumentNodeID != id || node.RepresentationID != repID || node.SourceID != rep.Original.SourceID ||
			node.SourceRevisionID != rep.Original.SourceRevisionID || node.OriginalSHA256 != rep.Original.SHA256 || node.ProducerSHA256 != rep.ProducerSHA256 ||
			node.NodePayloadSHA256 != entry.NodePayloadSHA256 || row.Ordinal != node.Payload.Ordinal ||
			!reflect.DeepEqual(entry.NaturalRef, entity.KnowledgeDocumentNaturalRef{Kind: node.Payload.Kind, Locator: node.Payload.Locator}) {
			return nil, errFixture
		}
		p := node.Payload
		hash, err := entity.KnowledgeDocumentPayloadDigest(p)
		if err != nil || hash != entry.NodePayloadSHA256 {
			return nil, errFixture
		}
		parent := ""
		if p.ParentRef != nil {
			key, err := entity.KnowledgeLifecycleDigest(*p.ParentRef)
			if err != nil {
				return nil, errFixture
			}
			parent = parents[key]
			if parent == "" {
				return nil, errFixture
			}
		}
		if row.ParentID != parent || (node.ParentID == nil) != (parent == "") || (node.ParentID != nil && *node.ParentID != parent) ||
			(i == 0 && (id != rep.RootDocumentNodeID || p.Kind != "document" || parent != "")) || (i > 0 && parent == "") {
			return nil, errFixture
		}
		key, err := entity.KnowledgeLifecycleDigest(entry.NaturalRef)
		if err != nil || parents[key] != "" {
			return nil, errFixture
		}
		parents[key] = id
		sha := p.ReadingRange.TextSHA256
		r := entity.KnowledgeContentRange{StartByte: p.ReadingRange.StartByte, EndByte: p.ReadingRange.EndByte, SHA256: &sha}
		if !fixtureRangeMatches(body, r) {
			return nil, errFixture
		}
		attrs := &entity.KnowledgeContentDocumentAttributes{}
		var attr any
		switch p.Kind {
		case "document", "section", "paragraph", "text_block", "text_box":
			attrs.Empty = &entity.KnowledgeDocumentEmptyAttributes{}
			attr = attrs.Empty
		case "table":
			attrs.Table = &entity.KnowledgeDocumentTableAttributes{}
			attr = attrs.Table
		case "row":
			attrs.Row = &entity.KnowledgeDocumentRowAttributes{}
			attr = attrs.Row
		case "cell":
			attrs.Cell = &entity.KnowledgeDocumentCellAttributes{}
			attr = attrs.Cell
		case "passage":
			attrs.Passage = &entity.KnowledgeDocumentPassageAttributes{}
			attr = attrs.Passage
		default:
			return nil, errFixture
		}
		if decodeClosed(p.Attributes, attr) != nil {
			return nil, errFixture
		}
		ordinal := p.Ordinal
		n := entity.KnowledgeContentNode{Ref: entity.KnowledgeContentNodeRef{Kind: "r2_node", ID: &id}, Kind: p.Kind, Name: p.Name, NameTruncated: p.NameTruncated, Ordinal: &ordinal,
			Locator: entity.KnowledgeContentLocator{Kind: "document", Document: &p.Locator}, OriginalPosition: &p.OriginalPosition, CoordinatePrecision: p.CoordinatePrecision,
			Text: entity.KnowledgeContentText{CoordinateSpace: "derived_utf8", Range: &r, Status: p.TextStatus, Reason: p.TextReason}, Attributes: entity.KnowledgeContentAttributes{Document: attrs}}
		if node.ParentID != nil {
			n.ParentRef = &entity.KnowledgeContentNodeRef{Kind: "r2_node", ID: node.ParentID}
		}
		if p.OwnReadingRange != nil {
			ownSHA := p.OwnReadingRange.TextSHA256
			own := entity.KnowledgeContentRange{StartByte: p.OwnReadingRange.StartByte, EndByte: p.OwnReadingRange.EndByte, SHA256: &ownSHA}
			if !fixtureRangeMatches(body, own) || own.StartByte < r.StartByte || own.EndByte > r.EndByte {
				return nil, errFixture
			}
			n.Text.OwnRange = &own
		}
		if n.Validate(&snap.Target) != nil {
			return nil, errFixture
		}
		result = append(result, entity.KnowledgeContentLexicalTypedMember{Node: n, PayloadSHA256: hash, TextAssembly: "existing_reading_range"})
	}
	if fixtureEmptyCellWitness(body, result) != nil {
		return nil, errFixture
	}
	return result, nil
}

func fixtureEmptyCellWitness(body string, members []entity.KnowledgeContentLexicalTypedMember) error {
	empty := 0
	available := map[int64]bool{}
	for _, member := range members {
		n := member.Node
		if n.Kind != "cell" {
			continue
		}
		if n.Attributes.Document == nil || n.Attributes.Document.Cell == nil || n.Locator.Kind != "document" || n.Locator.Document == nil || n.Locator.Document.Scheme != "office_package_v1" || n.Locator.Document.Part != "word/document.xml" ||
			n.Text.Range == nil || !fixtureRangeMatches(body, *n.Text.Range) || n.Attributes.Document.Cell.RowIndex != 0 {
			return errFixture
		}
		cell := n.Attributes.Document.Cell
		r := n.Text.Range
		if cell.CellIndex == 1 {
			if n.Text.Status != "empty" || r.StartByte != r.EndByte || r.SHA256 == nil || *r.SHA256 != digest(nil) {
				return errFixture
			}
			empty++
		} else if cell.CellIndex == 0 || cell.CellIndex == 2 {
			want := "A"
			if cell.CellIndex == 2 {
				want = "B"
			}
			if n.Text.Status != "available" || body[r.StartByte:r.EndByte] != want || available[cell.CellIndex] {
				return errFixture
			}
			available[cell.CellIndex] = true
		} else {
			return errFixture
		}
	}
	if empty != 1 || !available[0] || !available[2] {
		return errFixture
	}
	return nil
}

func fixtureSummarizeTyped(full string, r byteRange, members []entity.KnowledgeContentLexicalTypedMember) (json.RawMessage, error) {
	if r.Start < 0 || r.End < r.Start || r.End > len(full) || !utf8.ValidString(full[:r.Start]) || !utf8.ValidString(full[:r.End]) {
		return nil, errFixture
	}
	text := full[r.Start:r.End]
	ctx := typedContext{Members: []typedMember{}, Complete: true, Omissions: []string{}}
	reasons := map[string]bool{}
	for _, member := range members {
		n := member.Node
		rangeValue := n.Text.Range
		if n.Text.Status == "unsupported" || n.Text.Status == "unsupported_or_requires_ocr" || n.Text.Status == "unavailable" {
			reasons["unsearchable_member"] = true
			continue
		}
		if rangeValue == nil || rangeValue.SHA256 == nil || n.Ref.ID == nil ||
			rangeValue.StartByte < 0 || rangeValue.EndByte < rangeValue.StartByte || rangeValue.EndByte > int64(len(full)) {
			return nil, errFixture
		}
		start, end := int(rangeValue.StartByte), int(rangeValue.EndByte)
		if start < r.Start || end > r.End {
			reasons["whole_member_outside_evidence"] = true
			continue
		}
		if !utf8.ValidString(full[:start]) || !utf8.ValidString(full[:end]) ||
			digest([]byte(full[start:end])) != *rangeValue.SHA256 || !isHex(member.PayloadSHA256, 64) {
			return nil, errFixture
		}
		if len(ctx.Members) >= 32 {
			reasons["member_budget"] = true
			continue
		}
		status := "available"
		if start == end {
			status = "empty"
		}
		item := typedMember{NodeID: *n.Ref.ID, Kind: n.Kind, Range: byteRange{Start: start, End: end, SHA: *rangeValue.SHA256}, Status: status, PayloadSHA: member.PayloadSHA256}
		candidate := append(append([]typedMember(nil), ctx.Members...), item)
		if len(fixtureJSONBytes(candidate)) > 65536 {
			reasons["metadata_budget"] = true
			continue
		}
		ctx.Members = candidate
		reasons["typed_payload_not_exposed_in_access_v03"] = true
	}
	for reason := range reasons {
		ctx.Omissions = append(ctx.Omissions, reason)
	}
	sort.Strings(ctx.Omissions)
	ctx.Complete = len(ctx.Omissions) == 0
	typed, _ := json.Marshal(ctx)
	if len(typed) > 65536 || validateTyped(text, r, typed) != nil {
		return nil, errFixture
	}
	return typed, nil
}
