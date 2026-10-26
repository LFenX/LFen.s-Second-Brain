package structure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSyntheticFacts(t *testing.T) {
	attempts := 0
	previous := http.DefaultTransport
	http.DefaultTransport = denyTransport{attempts: &attempts}
	defer func() { http.DefaultTransport = previous }()
	output, err := Build(context.Background(), fixtureInputs, []Alias{{Prefix: "@/", Target: "frontend/"}})
	if err != nil {
		t.Fatal(err)
	}
	files, inputs := map[string]File{}, map[string]Input{}
	for _, file := range output.Files {
		files[file.Path] = file
	}
	for _, input := range fixtureInputs {
		inputs[input.Path] = input
	}
	for _, fact := range fixtureFacts {
		t.Run(fact.ID+"_"+fact.Kind, func(t *testing.T) {
			file, input := files[fact.Path], inputs[fact.Path]
			if file.SHA256 != fixtureSHA[fact.Path] || digest(input.Content) != file.SHA256 {
				t.Fatal("fixed source SHA changed")
			}
			if string(input.Content[fact.Start:fact.End]) != fact.Anchor || digest(input.Content[fact.Start:fact.End]) != fact.SHA256 {
				t.Fatal("frozen fact anchor changed")
			}
			if bytes.Count(input.Content[:fact.Start], []byte{'\n'})+1 != fact.Line {
				t.Fatal("physical fact line changed")
			}
			symbol := func(name string) Node {
				t.Helper()
				for _, n := range file.Nodes {
					if n.Kind == "symbol" && n.Name == name && n.StartByte <= fact.Start && n.EndByte >= fact.End {
						return n
					}
				}
				t.Fatalf("symbol %q does not cover frozen fact", name)
				return Node{}
			}
			section := func() Node {
				t.Helper()
				for _, n := range file.Nodes {
					if n.Kind == "section" && n.StartByte == fact.Start && n.EndByte >= fact.End {
						return n
					}
				}
				t.Fatal("section does not cover frozen fact")
				return Node{}
			}
			resolved := func(kind, targetPath, targetName string) Relation {
				t.Helper()
				for _, r := range file.Relations {
					if r.Kind != kind || r.To == nil || !overlaps(r.Evidence.StartByte, r.Evidence.EndByte, fact.Start, fact.End) {
						continue
					}
					target := files[targetPath]
					if r.To.SourceID != target.SourceID {
						continue
					}
					for _, node := range target.Nodes {
						if node.Key == r.To.Key && (targetName == "" || node.Name == targetName) {
							return r
						}
					}
				}
				t.Fatalf("missing %s to %s:%s", kind, targetPath, targetName)
				return Relation{}
			}
			unresolved := func(kind, reason string) {
				t.Helper()
				for _, r := range file.Relations {
					if r.Kind == kind && r.To == nil && (reason == "" || r.Reason == reason) && overlaps(r.Evidence.StartByte, r.Evidence.EndByte, fact.Start, fact.End) {
						return
					}
				}
				t.Fatalf("missing unresolved %s (%s)", kind, reason)
			}
			noRelation := func(kind string) {
				t.Helper()
				for _, r := range file.Relations {
					if r.Kind == kind && overlaps(r.Evidence.StartByte, r.Evidence.EndByte, fact.Start, fact.End) {
						t.Fatalf("false %s from non-syntax source", kind)
					}
				}
			}
			switch fact.ID {
			case "F001":
				symbol("DataTable")
			case "F002":
				resolved("imports_file", "frontend/types.ts", "")
			case "F003":
				resolved("imports_file", "frontend/utils.ts", "")
			case "F004":
				resolved("references_type", "frontend/types.ts", "Row")
			case "F005":
				resolved("calls_local", "frontend/utils.ts", "normalize")
			case "F006":
				symbol("identity")
			case "F007":
				for _, r := range file.Relations {
					if r.Kind == "references_type" && r.To != nil && overlaps(r.Evidence.StartByte, r.Evidence.EndByte, fact.Start, fact.End) {
						t.Fatal("generic Row shadow was bound to imported Row")
					}
				}
				unresolved("references_type", "unsupported_target")
			case "F008", "F009", "F034":
				noRelation("imports_file")
			case "F010":
				unresolved("calls_local", "")
			case "F011":
				if !hasBOM(input.Content) || !bytes.Contains(input.Content, []byte("\r\n")) || !bytes.Contains(input.Content, []byte("😀")) {
					t.Fatal("BOM/CRLF/emoji missing")
				}
				if rootNode(t, file).StartByte != 0 {
					t.Fatal("file root dropped BOM")
				}
			case "F012":
				symbol("RowId")
			case "F013":
				symbol("Row")
			case "F014":
				symbol("Column")
			case "F015":
				resolved("references_type", "frontend/types.ts", "RowId")
			case "F016":
				resolved("references_type", "frontend/types.ts", "Row")
			case "F017":
				symbol("normalize")
			case "F018":
				symbol("lower")
			case "F019":
				unresolved("imports_file", "dynamic_import")
			case "F020":
				symbol("duplicated")
			case "F021":
				symbol("otherDuplicated")
			case "F022":
				for _, n := range file.Nodes {
					if n.Kind == "symbol" && n.Name == "Ghost" {
						t.Fatal("string produced Ghost declaration")
					}
				}
			case "F023":
				resolved("exports_symbol", "frontend/data-table.tsx", "DataTable")
			case "F024":
				resolved("exports_symbol", "frontend/types.ts", "Row")
				resolved("exports_symbol", "frontend/types.ts", "Column")
			case "F025":
				unresolved("exports_symbol", "wildcard_export")
			case "F026":
				symbol("Row")
				if file.SourceID == files["frontend/types.ts"].SourceID {
					t.Fatal("different sources collapsed")
				}
			case "F027":
				for _, n := range file.Nodes {
					if n.StartLine > bytes.Count(input.Content, []byte{'\n'})+1 || n.StartLine >= 900 {
						t.Fatal("logical //line escaped into physical coordinates")
					}
				}
			case "F028":
				if symbol("Display").ParentKey != "file" {
					t.Fatal("Go method pretends to be inside receiver struct bytes")
				}
			case "F029":
				resolved("references_type", "backend/table.go", "Row")
			case "F030":
				unresolved("imports_file", "go_package_unresolved")
			case "F031":
				resolved("calls_local", "backend/table.go", "clean")
			case "F032":
				resolved("references_type", "backend/table.go", "Row")
			case "F033":
				unresolved("calls_local", "go_package_unresolved")
			case "F035":
				symbol("RemoteRow")
			case "F036":
				symbol("CrossFile")
			case "F037":
				resolved("references_type", "backend/types.go", "RemoteRow")
			case "F038", "F039", "F040", "F043", "F044", "F047":
				section()
			case "F041":
				if attempts != 0 {
					t.Fatal("remote image fetched")
				}
			case "F042":
				if len(passages(file)) != 1 {
					t.Fatal("small file was force-split")
				}
			case "F045":
				for _, n := range file.Nodes {
					if n.Kind == "section" && n.StartByte >= fact.Start && n.StartByte < fact.End {
						t.Fatal("fence created false section")
					}
				}
			case "F046", "F050":
				assertCoverage(t, file, input.Content)
			case "F048":
				assertCoverage(t, file, input.Content)
				found := false
				for _, d := range file.Diagnostics {
					if d.Code == "block_navigation_not_materialized" {
						found = true
					}
				}
				if !found {
					t.Fatal("unmaterialized table navigation not explained")
				}
			case "F049":
				// This producer never renders HTML. Browser safe-reading behavior
				// remains an independent consumer acceptance, not proved here.
				assertCoverage(t, file, input.Content)
				if attempts != 0 {
					t.Fatal("embedded HTML caused a fetch during syntax production")
				}
			default:
				t.Fatalf("unhandled frozen fact %s", fact.ID)
			}
		})
	}
	for _, file := range output.Files {
		assertCoverage(t, file, inputs[file.Path].Content)
		assertRelations(t, file, files, inputs[file.Path].Content)
	}
	if attempts != 0 {
		t.Fatal("unexpected HTTP attempt")
	}
}

type denyTransport struct{ attempts *int }

func (d denyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*d.attempts++
	return nil, errors.New("network forbidden in deterministic producer test")
}

func TestMarkdownContainerHeadingsDoNotEscapeTheirSyntaxParent(t *testing.T) {
	inputs := mapInputs(map[string]string{"nested.md": "# Root\n\n- item\n\n  ## List heading\n\n  list text\n\n> ## Quoted heading\n> quote\n\n# Final\nend\n"})
	out, err := Build(context.Background(), inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	sections := []string{}
	for _, node := range out.Files[0].Nodes {
		if node.Kind == "section" {
			sections = append(sections, node.Name)
		}
	}
	if !reflect.DeepEqual(sections, []string{"Root", "Final"}) {
		t.Fatalf("container heading promoted to document section: %v", sections)
	}
	assertCoverage(t, out.Files[0], inputs[0].Content)
}

func TestDeterministicSelectionAndAliases(t *testing.T) {
	first, err := Build(context.Background(), fixtureInputs, []Alias{{Prefix: "@/", Target: "frontend/"}})
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]Input(nil), fixtureInputs...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	second, err := Build(context.Background(), reversed, []Alias{{Prefix: "@/", Target: "frontend/"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("input ordering changed deterministic facts")
	}
	for _, in := range fixtureInputs {
		if digest(in.Content) != fixtureSHA[in.Path] {
			t.Fatal("producer mutated caller source bytes")
		}
	}
}

func TestConservativeTypeScriptResolution(t *testing.T) {
	cases := []struct {
		name                       string
		sources                    map[string]string
		from, kind, reason, target string
		aliases                    []Alias
	}{
		{name: "ambiguous path", sources: map[string]string{"main.ts": "import {x} from './x'; x();", "x.ts": "export function x() {}", "x.tsx": "export function x() {}"}, from: "main.ts", kind: "imports_file", reason: "ambiguous"},
		{name: "out of exact selection", sources: map[string]string{"main.ts": "import {x} from './x'; x();"}, from: "main.ts", kind: "imports_file", reason: "outside_selection"},
		{name: "external package", sources: map[string]string{"main.ts": "import {x} from 'a-package'; x();"}, from: "main.ts", kind: "imports_file", reason: "external_package"},
		{name: "named export cycle", sources: map[string]string{"a.ts": "export {x} from './b';", "b.ts": "export {x} from './a';"}, from: "a.ts", kind: "exports_symbol", reason: "cycle"},
		{name: "parameter shadow", sources: map[string]string{"main.ts": "function run() {} function caller(run:()=>void) { run(); }"}, from: "main.ts", kind: "calls_local", reason: "unsupported_target"},
		{name: "destructuring shadow", sources: map[string]string{"main.ts": "function run() {} function caller({run}:{run:()=>void}) { run(); }"}, from: "main.ts", kind: "calls_local", reason: "unsupported_target"},
		{name: "generic type shadow", sources: map[string]string{"main.ts": "type Row = string; function identity<Row>(x:Row):Row { return x; }"}, from: "main.ts", kind: "references_type", reason: "unsupported_target"},
		{name: "member calls", sources: map[string]string{"main.ts": "const x={run() {}}; x.run();"}, from: "main.ts", kind: "calls_local", reason: "dynamic_call"},
		{name: "namespace type", sources: map[string]string{"main.ts": "import * as api from './x'; let v:api.Row;", "x.ts": "export interface Row {}"}, from: "main.ts", kind: "references_type", reason: "unsupported_target"},
		{name: "type only import cannot call", sources: map[string]string{"main.ts": "import type {run} from './x'; run();", "x.ts": "export function run() {}"}, from: "main.ts", kind: "calls_local", reason: "unsupported_target"},
		{name: "var shadows function scope", sources: map[string]string{"main.ts": "function run() {} function caller(){ if (true){var run=()=>{};} run();}"}, from: "main.ts", kind: "calls_local", reason: "lexical_binding", target: "main.ts"},
		{name: "named function expression scope", sources: map[string]string{"main.ts": "const x = function hidden() {}; hidden();"}, from: "main.ts", kind: "calls_local", reason: "unsupported_target"},
		{name: "class expression scope", sources: map[string]string{"main.ts": "const x = class Hidden {}; new Hidden();"}, from: "main.ts", kind: "calls_local", reason: "unsupported_target"},
		{name: "aliases target root", sources: map[string]string{"main.ts": "import {run} from '@/run'; run();", "run.ts": "export function run() {}"}, from: "main.ts", kind: "imports_file", reason: "explicit_alias", target: "run.ts", aliases: []Alias{{Prefix: "@/", Target: ""}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			inputs := mapInputs(test.sources)
			output, err := Build(context.Background(), inputs, test.aliases)
			if err != nil {
				t.Fatal(err)
			}
			ids := map[string]string{}
			for _, file := range output.Files {
				ids[file.Path] = file.SourceID
			}
			found := false
			for _, file := range output.Files {
				if file.Path != test.from {
					continue
				}
				for _, relation := range file.Relations {
					if relation.Kind == test.kind && relation.Reason == test.reason && ((test.target == "" && relation.To == nil) || (test.target != "" && relation.To != nil && relation.To.SourceID == ids[test.target])) {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("missing %s %s -> %s", test.kind, test.reason, test.target)
			}
		})
	}
}

func TestPassagePartitionAndBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, body string
		parser     string
	}{
		{"emoji-long-line", strings.Repeat("你好😀", 1800), "markdown"},
		{"crlf-lines", strings.Repeat("一行😀\r\n", 2000), "markdown"},
		{"large-function", "export function longFunction() {\r\n" + strings.Repeat("  const item = '中文😀';\r\n", 600) + "}\r\n", "typescript"},
		{"bom-md", "\xef\xbb\xbf# 标题\r\n\r\n一句话。\r\n", "markdown"},
		{"empty", "", "markdown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ext := ".md"
			if test.parser == "typescript" {
				ext = ".ts"
			}
			input := Input{SourceID: "a", RevisionID: "r", Path: "sample" + ext, Parser: test.parser, Content: []byte(test.body)}
			output, err := Build(context.Background(), []Input{input}, nil)
			if err != nil {
				t.Fatal(err)
			}
			file := output.Files[0]
			assertCoverage(t, file, input.Content)
			if len(input.Content) > PassageMax && len(passages(file)) < 2 {
				t.Fatal("large source was not partitioned")
			}
		})
	}
}

func TestControlledFailures(t *testing.T) {
	valid := Input{SourceID: "a", RevisionID: "r", Path: "good.md", Parser: "markdown", Content: []byte("one sentence")}
	cases := []struct {
		name    string
		inputs  []Input
		aliases []Alias
		code    string
	}{
		{"empty selection", nil, nil, "file_count_limit"},
		{"invalid utf8", []Input{{SourceID: "a", RevisionID: "r", Path: "bad.tsx", Parser: "tsx", Content: []byte{0xff, 0xfe}}}, nil, "invalid_utf8"},
		{"invalid tsx", []Input{{SourceID: "a", RevisionID: "r", Path: "bad.tsx", Parser: "tsx", Content: []byte("export function bad( { return <div>;")}}, nil, "syntax_error"},
		{"empty go", []Input{{SourceID: "a", RevisionID: "r", Path: "bad.go", Parser: "go"}}, nil, "syntax_error"},
		{"case sensitive suffix", []Input{{SourceID: "a", RevisionID: "r", Path: "bad.TS", Parser: "typescript", Content: []byte("const x=1;")}}, nil, "unsupported_source"},
		{"no parser fallback", []Input{{SourceID: "a", RevisionID: "r", Path: "bad.tsx", Parser: "typescript", Content: []byte("const x=1;")}}, nil, "unsupported_source"},
		{"escape source", []Input{{SourceID: "a", RevisionID: "r", Path: "../bad.md", Parser: "markdown"}}, nil, "invalid_source_path"},
		{"path control char", []Input{{SourceID: "a", RevisionID: "r", Path: "bad\t.md", Parser: "markdown"}}, nil, "invalid_source_path"},
		{"duplicate identity", []Input{valid, valid}, nil, "invalid_source_identity"},
		{"overlapping aliases", []Input{valid}, []Alias{{Prefix: "@/", Target: "frontend/"}, {Prefix: "@/x/", Target: "x/"}}, "overlapping_aliases"},
		{"alias target requires slash", []Input{valid}, []Alias{{Prefix: "@/", Target: "frontend"}}, "invalid_alias"},
		{"alias escape", []Input{valid}, []Alias{{Prefix: "@/", Target: "../x/"}}, "invalid_alias"},
		{"file bytes", []Input{{SourceID: "a", RevisionID: "r", Path: "big.md", Parser: "markdown", Content: bytes.Repeat([]byte("x"), MaxFileBytes+1)}}, nil, "file_bytes_limit"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			output, err := Build(context.Background(), test.inputs, test.aliases)
			var controlled *Error
			if !errors.As(err, &controlled) || controlled.Code != test.code {
				t.Fatalf("got %v; want %s", err, test.code)
			}
			if len(output.Files) != 0 {
				t.Fatal("failed build returned publishable partial files")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output, err := Build(ctx, fixtureInputs, nil)
	if !errors.Is(err, context.Canceled) || len(output.Files) != 0 {
		t.Fatal("pre-cancelled build proceeded")
	}
}

func TestDuplicateAndScopeIdentity(t *testing.T) {
	output, err := Build(context.Background(), mapInputs(map[string]string{"same.ts": "function same() {} function same() {} function parent(){function same(){}}", "same.md": "# Same\n\n## Child\n\n# Same\n"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range output.Files {
		keys := map[string]bool{}
		for _, node := range file.Nodes {
			if keys[node.Key] {
				t.Fatal("local key collision")
			}
			keys[node.Key] = true
			if node.Name == "same" && node.ParentKey == "file" && node.IdentityStability != "position_based" {
				t.Fatal("ambiguous duplicate promised name stability")
			}
			if file.Path == "same.md" && node.Name == "Same" && node.IdentityStability != "position_based" {
				t.Fatal("duplicate heading promised unique name stability")
			}
		}
	}
}

func TestOutputBudgets(t *testing.T) {
	t.Run("file count", func(t *testing.T) {
		sources := map[string]string{}
		for i := 0; i < MaxFiles+1; i++ {
			sources[fmt.Sprintf("file%d.md", i)] = "one sentence"
		}
		assertFailureCode(t, mapInputs(sources), "file_count_limit")
	})
	t.Run("input bytes preflight", func(t *testing.T) {
		inputs := []Input{}
		for i := 0; i < 9; i++ {
			inputs = append(inputs, Input{SourceID: fmt.Sprint(i), RevisionID: "r", Path: fmt.Sprintf("large%d.md", i), Parser: "markdown", Content: bytes.Repeat([]byte("x"), MaxFileBytes)})
		}
		assertFailureCode(t, inputs, "input_bytes_limit")
	})
	t.Run("file nodes", func(t *testing.T) {
		var source strings.Builder
		source.WriteString("package fixture\n")
		for i := 0; i < MaxNodesPerFile; i++ {
			fmt.Fprintf(&source, "type T%d int\n", i)
		}
		assertFailureCode(t, mapInputs(map[string]string{"many.go": source.String()}), "file_nodes_limit")
	})
	t.Run("view nodes", func(t *testing.T) {
		var source strings.Builder
		source.WriteString("package fixture\n")
		for i := 0; i < 5000; i++ {
			fmt.Fprintf(&source, "type T%d int\n", i)
		}
		sources := map[string]string{}
		for i := 0; i < 7; i++ {
			sources[fmt.Sprintf("many%d.go", i)] = source.String()
		}
		assertFailureCode(t, mapInputs(sources), "view_nodes_limit")
	})
	t.Run("relation count", func(t *testing.T) {
		source := "function f() {}\n" + strings.Repeat("f();\n", MaxRelations+1)
		assertFailureCode(t, mapInputs(map[string]string{"calls.ts": source}), "relations_limit")
	})
	t.Run("depth", func(t *testing.T) {
		var source strings.Builder
		for i := 0; i < MaxDepth; i++ {
			fmt.Fprintf(&source, "function f%d(){", i)
		}
		source.WriteString(strings.Repeat("}", MaxDepth))
		assertFailureCode(t, mapInputs(map[string]string{"deep.ts": source.String()}), "structure_depth_limit")
	})
}

func assertFailureCode(t *testing.T, inputs []Input, code string) {
	t.Helper()
	output, err := Build(context.Background(), inputs, nil)
	var controlled *Error
	if !errors.As(err, &controlled) || controlled.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
	if len(output.Files) != 0 {
		t.Fatal("over-limit output leaked partial files")
	}
}

func mapInputs(sources map[string]string) []Input {
	paths := make([]string, 0, len(sources))
	for filename := range sources {
		paths = append(paths, filename)
	}
	sort.Strings(paths)
	inputs := make([]Input, 0, len(paths))
	for i, filename := range paths {
		parser := "typescript"
		if strings.HasSuffix(filename, ".tsx") {
			parser = "tsx"
		}
		if strings.HasSuffix(filename, ".md") {
			parser = "markdown"
		}
		if strings.HasSuffix(filename, ".go") {
			parser = "go"
		}
		inputs = append(inputs, Input{SourceID: fmt.Sprintf("%032x", i+1), RevisionID: fmt.Sprintf("%032x", i+101), Path: filename, Parser: parser, Content: []byte(sources[filename])})
	}
	return inputs
}

func rootNode(t *testing.T, file File) Node {
	t.Helper()
	for _, node := range file.Nodes {
		if node.Kind == "file" {
			return node
		}
	}
	t.Fatal("no file root")
	return Node{}
}
func passages(file File) []Node {
	out := []Node{}
	for _, node := range file.Nodes {
		if node.Kind == "passage" {
			out = append(out, node)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartByte < out[j].StartByte })
	return out
}
func overlaps(a, b, c, d int) bool { return a < d && c < b }

func assertCoverage(t *testing.T, file File, raw []byte) {
	t.Helper()
	root := rootNode(t, file)
	if root.StartByte != 0 || root.EndByte != len(raw) {
		t.Fatal("root does not cover original bytes")
	}
	joined := []byte{}
	expected := 0
	nodes := map[string]Node{}
	for _, node := range file.Nodes {
		nodes[node.Key] = node
	}
	for _, passage := range passages(file) {
		if passage.StartByte != expected || passage.EndByte <= passage.StartByte || passage.EndByte-passage.StartByte > PassageMax {
			t.Fatal("passage overlap/gap/empty/limit")
		}
		if !utf8.Valid(raw[passage.StartByte:passage.EndByte]) {
			t.Fatal("split UTF-8")
		}
		joined = append(joined, raw[passage.StartByte:passage.EndByte]...)
		expected = passage.EndByte
	}
	if !bytes.Equal(joined, raw) {
		t.Fatal("passage concatenation differs from source bytes")
	}
	for _, node := range file.Nodes {
		if node.StartLine != bytes.Count(raw[:node.StartByte], []byte{'\n'})+1 {
			t.Fatal("wrong physical start line")
		}
		end := node.StartByte
		if node.EndByte > node.StartByte {
			end = node.EndByte - 1
		}
		if node.EndLine != bytes.Count(raw[:end], []byte{'\n'})+1 {
			t.Fatal("wrong physical end line")
		}
		if node.ParentKey != "" {
			parent, ok := nodes[node.ParentKey]
			if !ok || parent.StartByte > node.StartByte || parent.EndByte < node.EndByte || parent.Kind == "passage" {
				t.Fatal("invalid parent containment")
			}
		}
	}
}

func assertRelations(t *testing.T, file File, files map[string]File, raw []byte) {
	t.Helper()
	refs := map[Ref]bool{}
	for _, candidate := range files {
		for _, node := range candidate.Nodes {
			refs[Ref{SourceID: candidate.SourceID, Key: node.Key}] = true
		}
	}
	for _, relation := range file.Relations {
		if !refs[relation.From] {
			t.Fatal("relation source is not in exact selected nodes")
		}
		if relation.To != nil && !refs[*relation.To] {
			t.Fatal("relation target escaped exact selection")
		}
		evidence := relation.Evidence
		if evidence.SourceID != file.SourceID || evidence.RevisionID != file.RevisionID || digest(raw[evidence.StartByte:evidence.EndByte]) != evidence.FragmentSHA256 {
			t.Fatal("evidence does not bind fixed raw source")
		}
		if len(relation.Reference) > 512 || !utf8.ValidString(relation.Reference) || !bytes.HasPrefix(raw[evidence.StartByte:evidence.EndByte], []byte(relation.Reference)) {
			t.Fatal("reference not bounded raw prefix")
		}
		if relation.Resolution == "resolved" && relation.To == nil {
			t.Fatal("resolved relation has no target")
		}
		if relation.Resolution == "unresolved" && relation.To != nil {
			t.Fatal("unresolved relation guessed a target")
		}
	}
}
