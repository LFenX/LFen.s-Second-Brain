package dataset

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"ragflow/internal/entity"
	"sort"
	"unicode/utf8"
)

func lexicalSHA(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func lexicalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// lexicalEncode implements the approved tagged, uint64be length encoding.
func lexicalEncode(v any) []byte {
	var b bytes.Buffer
	length := func(n uint64) { var p [8]byte; binary.BigEndian.PutUint64(p[:], n); b.Write(p[:]) }
	switch x := v.(type) {
	case nil:
		b.WriteByte(0x4e)
	case string:
		if !utf8.ValidString(x) {
			panic("invalid UTF-8 identity")
		}
		b.WriteByte(0x53)
		length(uint64(len(x)))
		b.WriteString(x)
	case int64:
		if x < 0 || x > 9007199254740991 {
			panic("invalid identity integer")
		}
		b.WriteByte(0x55)
		length(uint64(x))
	case []any:
		b.WriteByte(0x41)
		length(uint64(len(x)))
		for _, e := range x {
			b.Write(lexicalEncode(e))
		}
	default:
		panic(fmt.Sprintf("unsupported identity type %T", v))
	}
	return b.Bytes()
}
func lexicalDigest(v ...any) string { return lexicalSHA(lexicalEncode(v)) }
func lexicalAssembly() string {
	return lexicalDigest("knowledge-first-slice-assembly/0.1.0-candidate", "exactly_one_member", "original_utf8_bytes_direct", "no_prefix", "no_suffix", "no_trim", "preserve_line_endings", "no_unicode_normalization", "no_split", "no_merge", "no_parent_or_mom")
}
func lexicalPlan(strategy, language, producer string) string {
	return lexicalDigest("knowledge-first-slice-plan/0.1.0-candidate", strategy, int64(8192), lexicalAssembly(), language, producer, "full_text")
}
func lexicalProfile(language, producer string) string {
	return lexicalDigest("knowledge-first-slice-profile-set/0.1.0-candidate", []any{[]any{"c1_complete_root", lexicalPlan("c1_complete_root", language, producer)}, []any{"r1_existing_passage", lexicalPlan("r1_existing_passage", language, producer)}})
}
func lexicalSelection(s entity.KnowledgeLexicalSelection) (entity.KnowledgeLexicalSelection, string) {
	s.Targets = append([]entity.KnowledgeLexicalTarget{}, s.Targets...)
	sort.Slice(s.Targets, func(i, j int) bool { return s.Targets[i].Source.SourceID < s.Targets[j].Source.SourceID })
	return s, lexicalSHA([]byte(lexicalJSON(s)))
}
func lexicalBinding(t entity.KnowledgeLexicalTarget, plan string) string {
	var p any
	if t.Representation.ProducerSHA256 != nil {
		p = *t.Representation.ProducerSHA256
	}
	return lexicalDigest("knowledge-index-binding/0.1.0-candidate", []any{t.Source.DomainID, t.Source.SourceID, t.Source.RevisionID, t.Source.SHA256}, []any{t.Representation.Kind, t.Representation.ID, p}, plan, "source_utf8", t.Source.SHA256)
}
func lexicalRowID(binding string, n entity.KnowledgeLexicalNodeRef, r entity.KnowledgeLexicalRange, sha string) string {
	return lexicalDigest("knowledge-index-row/0.1.0-candidate", binding, "chunk", lexicalAssembly(), []any{[]any{n.Kind, n.ID, r.Start, r.End, sha}}, r.End-r.Start, sha, nil)
}

type lexicalStoredRange struct {
	Start  int64  `json:"start_byte"`
	End    int64  `json:"end_byte"`
	SHA256 string `json:"sha256"`
}
type lexicalStoredMember struct {
	Node  entity.KnowledgeLexicalNodeRef `json:"node_ref"`
	Range lexicalStoredRange             `json:"range"`
}
type lexicalStoredReference struct {
	Version        string                                   `json:"projection_version"`
	Source         entity.KnowledgeLexicalSourceRef         `json:"source_ref"`
	Representation entity.KnowledgeLexicalRepresentationRef `json:"representation_ref"`
	Members        []lexicalStoredMember                    `json:"members"`
}
type lexicalManifestRow struct {
	ID              string                        `json:"row_id"`
	Binding         string                        `json:"binding_id"`
	Plan            string                        `json:"plan_sha256"`
	Reference       string                        `json:"reference"`
	ReferenceSHA256 string                        `json:"reference_sha256"`
	TextSHA256      string                        `json:"text_sha256"`
	ByteLength      int64                         `json:"byte_length"`
	Member          lexicalStoredMember           `json:"member"`
	Target          entity.KnowledgeLexicalTarget `json:"target"`
}
type lexicalManifest struct {
	Version           string                           `json:"version"`
	Owner             string                           `json:"owner"`
	Domain            string                           `json:"domain"`
	Selection         entity.KnowledgeLexicalSelection `json:"selection"`
	SelectionSHA256   string                           `json:"selection_sha256"`
	PolicyVersion     int64                            `json:"policy_version"`
	FoundationVersion int64                            `json:"foundation_version"`
	Producer          string                           `json:"producer_sha256"`
	Language          string                           `json:"language"`
	Profile           string                           `json:"profile_sha256"`
	Coverage          entity.KnowledgeLexicalCoverage  `json:"coverage"`
	Rows              []lexicalManifestRow             `json:"rows"`
	PublishedAt       string                           `json:"published_at"`
}
