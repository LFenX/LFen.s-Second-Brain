package structure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type fileBuilder struct {
	ctx          context.Context
	in           Input
	file         File
	lineStart    []int
	ends         []int
	counts       map[string]int
	dups         map[string][]int
	depth        map[string]int
	nodeIndex    map[string]int
	relationSeen map[string]bool
	ts           *tsFile
	err          error
}

// Build consumes only the exact supplied selection. No files, package metadata,
// network resources, compilers, or model runtimes are consulted. It sorts output
// files by source identity; input order is not a semantic input.
func Build(ctx context.Context, inputs []Input, aliases []Alias) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if len(inputs) == 0 || len(inputs) > MaxFiles {
		return Output{}, &Error{Code: "file_count_limit"}
	}
	if err := validateAliases(aliases); err != nil {
		return Output{}, err
	}
	selected := append([]Input(nil), inputs...)
	sort.Slice(selected, func(i, j int) bool { return selected[i].SourceID < selected[j].SourceID })
	builders := make([]*fileBuilder, 0, len(selected))
	bytesTotal, nodesTotal, pendingRelations := 0, 0, 0
	for i, in := range selected {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		if in.SourceID == "" || in.RevisionID == "" || !utf8.ValidString(in.SourceID) || !utf8.ValidString(in.RevisionID) || (i > 0 && selected[i-1].SourceID == in.SourceID) {
			return Output{}, &Error{Code: "invalid_source_identity", SourceID: in.SourceID}
		}
		if !isProjectPath(in.Path, false) {
			return Output{}, &Error{Code: "invalid_source_path", SourceID: in.SourceID}
		}
		if !matchesParser(in.Path, in.Parser) {
			return Output{}, &Error{Code: "unsupported_source", SourceID: in.SourceID}
		}
		if len(in.Content) > MaxFileBytes {
			return Output{}, &Error{Code: "file_bytes_limit", SourceID: in.SourceID}
		}
		bytesTotal += len(in.Content)
		if bytesTotal > MaxInputBytes {
			return Output{}, &Error{Code: "input_bytes_limit"}
		}
		if !utf8.Valid(in.Content) {
			return Output{}, &Error{Code: "invalid_utf8", SourceID: in.SourceID}
		}
	}
	for _, in := range selected {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		b := newFileBuilder(ctx, in)
		var err error
		switch in.Parser {
		case "go":
			err = b.parseGo()
		case "markdown":
			err = b.parseMarkdown()
		case "typescript", "tsx":
			err = b.parseTypeScript()
		}
		if err != nil {
			return Output{}, err
		}
		if b.err != nil {
			return Output{}, b.err
		}
		if err = b.addPassages(); err != nil {
			return Output{}, err
		}
		if err = b.validateTree(); err != nil {
			return Output{}, err
		}
		nodesTotal += len(b.file.Nodes)
		if nodesTotal > MaxNodes {
			return Output{}, &Error{Code: "view_nodes_limit"}
		}
		pendingRelations += len(b.file.Relations) + len(b.file.Nodes) - 1
		if b.ts != nil {
			pendingRelations += len(b.ts.imports) + len(b.ts.references) + len(b.ts.stars)
			for _, exports := range b.ts.exports {
				pendingRelations += len(exports)
			}
		}
		if pendingRelations > MaxRelations {
			return Output{}, &Error{Code: "relations_limit"}
		}
		builders = append(builders, b)
	}
	if err := resolveTypeScript(ctx, builders, aliases); err != nil {
		return Output{}, err
	}
	out := Output{Files: make([]File, 0, len(builders))}
	edgesTotal := 0
	for _, b := range builders {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		if b.err != nil {
			return Output{}, b.err
		}
		edgesTotal += len(b.file.Relations)
		// Contains is projected by the service, but must consume the same frozen
		// view relation budget as a materialized static relation.
		edgesTotal += len(b.file.Nodes) - 1
		if edgesTotal > MaxRelations {
			return Output{}, &Error{Code: "relations_limit"}
		}
		sort.Slice(b.file.Nodes, func(i, j int) bool {
			a, z := b.file.Nodes[i], b.file.Nodes[j]
			if a.StartByte != z.StartByte {
				return a.StartByte < z.StartByte
			}
			if a.EndByte != z.EndByte {
				return a.EndByte < z.EndByte
			}
			if a.Kind != z.Kind {
				return a.Kind < z.Kind
			}
			return a.Key < z.Key
		})
		sort.Slice(b.file.Relations, func(i, j int) bool {
			a, z := b.file.Relations[i], b.file.Relations[j]
			if a.Evidence.StartByte != z.Evidence.StartByte {
				return a.Evidence.StartByte < z.Evidence.StartByte
			}
			if a.Evidence.EndByte != z.Evidence.EndByte {
				return a.Evidence.EndByte < z.Evidence.EndByte
			}
			if a.Kind != z.Kind {
				return a.Kind < z.Kind
			}
			if a.From.Key != z.From.Key {
				return a.From.Key < z.From.Key
			}
			return relationTarget(a) < relationTarget(z)
		})
		out.Files = append(out.Files, b.file)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	return out, nil
}

func newFileBuilder(ctx context.Context, in Input) *fileBuilder {
	b := &fileBuilder{ctx: ctx, in: in, counts: map[string]int{}, dups: map[string][]int{}, depth: map[string]int{"file": 1}, nodeIndex: map[string]int{"file": 0}, relationSeen: map[string]bool{}, lineStart: []int{0}}
	for i, ch := range in.Content {
		if ch == '\n' {
			b.lineStart = append(b.lineStart, i+1)
		}
	}
	b.file = File{SourceID: in.SourceID, RevisionID: in.RevisionID, Path: in.Path, Parser: in.Parser, SHA256: digest(in.Content), ByteLength: len(in.Content), Nodes: []Node{}, Relations: []Relation{}, Diagnostics: []Diagnostic{}}
	b.file.Nodes = append(b.file.Nodes, Node{Key: "file", Kind: "file", Name: truncateUTF8(path.Base(in.Path), 256), LogicalKey: "file", IdentityStability: "file", StartByte: 0, EndByte: len(in.Content), StartLine: 1, EndLine: b.endLine(0, len(in.Content))})
	return b
}

func matchesParser(filename, parser string) bool {
	switch parser {
	case "typescript":
		return strings.HasSuffix(filename, ".ts")
	case "tsx":
		return strings.HasSuffix(filename, ".tsx")
	case "go":
		return strings.HasSuffix(filename, ".go")
	case "markdown":
		return strings.HasSuffix(filename, ".md") || strings.HasSuffix(filename, ".markdown")
	}
	return false
}

func isProjectPath(v string, allowEmpty bool) bool {
	if v == "" {
		return allowEmpty
	}
	if len(v) > 512 || !utf8.ValidString(v) || strings.ContainsAny(v, "\\:") || strings.HasPrefix(v, "/") || strings.HasSuffix(v, "/") {
		return false
	}
	for _, ch := range v {
		if ch < 32 || ch == 127 {
			return false
		}
	}
	for _, part := range strings.Split(v, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validateAliases(aliases []Alias) error {
	if len(aliases) > 8 {
		return &Error{Code: "alias_limit"}
	}
	for i, alias := range aliases {
		if len(alias.Prefix) < 2 || len(alias.Prefix) > 64 || !strings.HasSuffix(alias.Prefix, "/") || !isAliasTarget(alias.Target) {
			return &Error{Code: "invalid_alias"}
		}
		for j, ch := range []byte(alias.Prefix) {
			allowed := (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '@' || ch == '~' || ch == '_'
			if j > 0 {
				allowed = allowed || (ch >= '0' && ch <= '9') || ch == '.' || ch == '/' || ch == '-'
			}
			if !allowed {
				return &Error{Code: "invalid_alias"}
			}
		}
		for j := 0; j < i; j++ {
			if strings.HasPrefix(alias.Prefix, aliases[j].Prefix) || strings.HasPrefix(aliases[j].Prefix, alias.Prefix) {
				return &Error{Code: "overlapping_aliases"}
			}
		}
	}
	return nil
}

func isAliasTarget(value string) bool {
	return value == "" || (len(value) <= 512 && strings.HasSuffix(value, "/") && isProjectPath(strings.TrimSuffix(value, "/"), false))
}

func (b *fileBuilder) fail(code string) error { return &Error{Code: code, SourceID: b.in.SourceID} }

func (b *fileBuilder) lineAt(offset int) int {
	return sort.Search(len(b.lineStart), func(i int) bool { return b.lineStart[i] > offset })
}

func (b *fileBuilder) endLine(start, end int) int {
	if end > start {
		return b.lineAt(end - 1)
	}
	return b.lineAt(start)
}

func (b *fileBuilder) addNode(parent, kind, declaration, name string, start, end int) string {
	if b.err != nil {
		return ""
	}
	if err := b.ctx.Err(); err != nil {
		b.err = err
		return ""
	}
	if start < 0 || end < start || end > len(b.in.Content) || !isBoundary(b.in.Content, start) || !isBoundary(b.in.Content, end) {
		b.err = b.fail("invalid_producer_range")
		return ""
	}
	if len(b.file.Nodes) >= MaxNodesPerFile {
		b.err = b.fail("file_nodes_limit")
		return ""
	}
	if b.depth[parent] == 0 || b.depth[parent] >= MaxDepth {
		b.err = b.fail("structure_depth_limit")
		return ""
	}
	parentLogical := b.file.Nodes[b.nodeIndex[parent]].LogicalKey
	logicalBase := parentLogical + "/" + declaration + "/" + url.PathEscape(name)
	ordinal := b.counts[logicalBase]
	b.counts[logicalBase]++
	logical := logicalBase + "#" + strconv.Itoa(ordinal)
	stability := "name_scope"
	if name == "" || len(logical) > 512 {
		logical = "file/position/" + declaration + "/" + strconv.Itoa(start) + "#" + strconv.Itoa(ordinal)
		stability = "position_based"
	}
	key := "n:" + strconv.Itoa(len(b.file.Nodes))
	node := Node{Key: key, ParentKey: parent, Kind: kind, Name: truncateUTF8(name, 256), LogicalKey: logical, IdentityStability: stability, StartByte: start, EndByte: end, StartLine: b.lineAt(start), EndLine: b.endLine(start, end)}
	b.file.Nodes = append(b.file.Nodes, node)
	b.nodeIndex[key] = len(b.file.Nodes) - 1
	b.depth[key] = b.depth[parent] + 1
	b.dups[logicalBase] = append(b.dups[logicalBase], len(b.file.Nodes)-1)
	if len(b.dups[logicalBase]) > 1 {
		for _, index := range b.dups[logicalBase] {
			b.file.Nodes[index].IdentityStability = "position_based"
		}
	}
	return key
}

func (b *fileBuilder) diagnostic(code, message string, start, end int) {
	if len(b.file.Diagnostics) >= 8 {
		return
	}
	for _, d := range b.file.Diagnostics {
		if d.Code == code {
			return
		}
	}
	d := Diagnostic{Code: truncateUTF8(code, 64), Message: truncateUTF8(message, 128)}
	if start >= 0 && end >= start {
		s, e := start, end
		d.StartByte, d.EndByte = &s, &e
	}
	b.file.Diagnostics = append(b.file.Diagnostics, d)
}

func (b *fileBuilder) addRelation(kind, from string, to *Ref, reason string, start, end int) {
	if b.err != nil {
		return
	}
	if len(b.file.Relations) >= MaxRelations {
		b.err = b.fail("relations_limit")
		return
	}
	if start < 0 || end < start || end > len(b.in.Content) || !isBoundary(b.in.Content, start) || !isBoundary(b.in.Content, end) {
		b.err = b.fail("invalid_relation_range")
		return
	}
	raw := b.in.Content[start:end]
	ref := truncateUTF8(string(raw), 512)
	relation := Relation{Kind: kind, From: Ref{SourceID: b.in.SourceID, Key: from}, To: to, Resolution: "unresolved", Reason: reason, Reference: ref, ReferenceTruncated: len(ref) < len(raw), Evidence: Evidence{SourceID: b.in.SourceID, RevisionID: b.in.RevisionID, StartByte: start, EndByte: end, FragmentSHA256: digest(raw)}}
	if to != nil {
		relation.Resolution = "resolved"
	}
	// One syntax reference can be reached by two AST context paths; suppress
	// only that identical fact. Repeated source statements have different ranges.
	identity := kind + "\x00" + from + "\x00" + strconv.Itoa(start) + "\x00" + strconv.Itoa(end) + "\x00" + reason + "\x00" + relationTarget(relation)
	if b.relationSeen[identity] {
		return
	}
	b.relationSeen[identity] = true
	b.file.Relations = append(b.file.Relations, relation)
}

func relationTarget(r Relation) string {
	if r.To == nil {
		return ""
	}
	return r.To.SourceID + "/" + r.To.Key
}

func (b *fileBuilder) owner(start, end int) string {
	key, width := "file", len(b.in.Content)+1
	for _, n := range b.file.Nodes {
		if n.Kind == "passage" || n.Kind == "file" || n.StartByte > start || n.EndByte < end {
			continue
		}
		w := n.EndByte - n.StartByte
		if w < width || (w == width && n.Key < key) {
			key, width = n.Key, w
		}
	}
	return key
}

func (b *fileBuilder) addPassages() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	sort.Ints(b.ends)
	ends := compactInts(b.ends)
	lineEnds := make([]int, 0, len(b.lineStart))
	for _, offset := range b.lineStart {
		if offset > 0 {
			lineEnds = append(lineEnds, offset)
		}
	}
	for start := 0; start < len(b.in.Content); {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		end := len(b.in.Content)
		if end-start > PassageMax {
			end = nearestCut(ends, start+PassageTarget/2, start+PassageMax, start+PassageTarget)
			if end < 0 {
				end = nearestCut(lineEnds, start+PassageTarget/2, start+PassageMax, start+PassageTarget)
			}
			if end < 0 {
				end = start + PassageTarget
				for !isBoundary(b.in.Content, end) {
					end--
				}
			}
		}
		parent := b.owner(start, end)
		if b.depth[parent] >= MaxDepth {
			return b.fail("structure_depth_limit")
		}
		if len(b.file.Nodes) >= MaxNodesPerFile {
			return b.fail("file_nodes_limit")
		}
		key := "p:" + strconv.Itoa(start)
		b.file.Nodes = append(b.file.Nodes, Node{Key: key, ParentKey: parent, Kind: "passage", Name: "", LogicalKey: "file/passage/" + strconv.Itoa(start) + "-" + strconv.Itoa(end), IdentityStability: "range_based", StartByte: start, EndByte: end, StartLine: b.lineAt(start), EndLine: b.endLine(start, end)})
		b.nodeIndex[key] = len(b.file.Nodes) - 1
		b.depth[key] = b.depth[parent] + 1
		start = end
	}
	return nil
}

func nearestCut(candidates []int, min, max, target int) int {
	first := sort.SearchInts(candidates, min)
	best := -1
	for i := first; i < len(candidates) && candidates[i] <= max; i++ {
		point := candidates[i]
		if best < 0 || abs(point-target) < abs(best-target) {
			best = point
		}
	}
	return best
}

func (b *fileBuilder) validateTree() error {
	nodes := make(map[string]Node, len(b.file.Nodes))
	for _, n := range b.file.Nodes {
		if _, duplicate := nodes[n.Key]; duplicate {
			return b.fail("duplicate_local_key")
		}
		nodes[n.Key] = n
	}
	for _, n := range b.file.Nodes {
		if n.Kind == "file" {
			continue
		}
		parent, found := nodes[n.ParentKey]
		if !found || parent.StartByte > n.StartByte || parent.EndByte < n.EndByte || parent.Kind == "passage" {
			return b.fail("invalid_parent_range")
		}
		depth, key := 1, n.ParentKey
		for key != "" {
			p, ok := nodes[key]
			if !ok {
				return b.fail("invalid_parent")
			}
			depth++
			if depth > MaxDepth {
				return b.fail("structure_depth_limit")
			}
			key = p.ParentKey
		}
	}
	return nil
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func isBoundary(raw []byte, i int) bool {
	return i >= 0 && i <= len(raw) && (i == len(raw) || utf8.RuneStart(raw[i]))
}
func truncateUTF8(v string, max int) string {
	if len(v) <= max {
		return v
	}
	n := max
	for n > 0 && !utf8.RuneStart(v[n]) {
		n--
	}
	return v[:n]
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func compactInts(v []int) []int {
	out := v[:0]
	for _, n := range v {
		if len(out) == 0 || out[len(out)-1] != n {
			out = append(out, n)
		}
	}
	return out
}
func hasBOM(v []byte) bool { return bytes.HasPrefix(v, []byte{0xef, 0xbb, 0xbf}) }
