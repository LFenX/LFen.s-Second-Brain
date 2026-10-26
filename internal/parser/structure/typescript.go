package structure

import (
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type tsScope struct {
	parent   *tsScope
	kind     string
	bindings map[string][]*tsBinding
}

type tsBinding struct {
	name     string
	key      string
	typeOK   bool
	valueOK  bool
	kind     string
	start    int
	hoisted  bool
	imported *tsImportBinding
}

type tsImport struct {
	module string
	start  int
	end    int
	target *fileBuilder
	reason string
	plain  bool
}

type tsImportBinding struct {
	module    *tsImport
	name      string
	namespace bool
}

type tsExport struct {
	name      string
	local     string
	declStart int
	module    *tsImport
	imported  string
	typeOnly  bool
	start     int
	end       int
}

type tsReference struct {
	kind   string
	name   string
	scope  *tsScope
	from   string
	start  int
	end    int
	forced string
}

type tsFile struct {
	root       *tsScope
	imports    []*tsImport
	exports    map[string][]*tsExport
	stars      []*tsExport
	references []tsReference
	declKeys   map[int][]string
}

type tsFrame struct {
	node   *ts.Node
	parent *ts.Node
	scope  *tsScope
	owner  string
}

func newTSScope(parent *tsScope, kind string) *tsScope {
	return &tsScope{parent: parent, kind: kind, bindings: map[string][]*tsBinding{}}
}

func (s *tsScope) bind(binding *tsBinding) {
	if binding.name != "" {
		s.bindings[binding.name] = append(s.bindings[binding.name], binding)
	}
}

func (b *fileBuilder) parseTypeScript() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	parser := ts.NewParser()
	defer parser.Close()
	language := typescript.LanguageTypescript()
	if b.in.Parser == "tsx" {
		language = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(ts.NewLanguage(language)); err != nil {
		return b.fail("parser_unavailable")
	}
	// Never use ParseCtx in binding 0.24.0. Closing an immediately cancelled
	// ParseCtx was shown to panic in the pinned dependency probe.
	parser.SetTimeoutMicros(5_000_000)
	tree := parser.Parse(b.in.Content, nil)
	if tree == nil {
		return b.fail("syntax_timeout")
	}
	defer tree.Close()
	if err := b.ctx.Err(); err != nil {
		return err
	}
	root := tree.RootNode()
	if root.HasError() {
		return b.fail("syntax_error")
	}
	b.ts = &tsFile{root: newTSScope(nil, "file"), imports: []*tsImport{}, exports: map[string][]*tsExport{}, stars: []*tsExport{}, references: []tsReference{}, declKeys: map[int][]string{}}
	stack := []tsFrame{{node: root, scope: b.ts.root, owner: "file"}}
	for len(stack) > 0 {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node, scope, owner := frame.node, frame.scope, frame.owner
		kind := node.Kind()
		start, end := int(node.StartByte()), int(node.EndByte())
		if start < 0 || end > len(b.in.Content) || start > end {
			return b.fail("invalid_producer_range")
		}
		if strings.HasSuffix(kind, "_statement") || strings.HasSuffix(kind, "_declaration") || kind == "variable_declarator" || kind == "method_definition" {
			b.ends = append(b.ends, end)
		}
		childScope, childOwner := scope, owner
		declaration := tsDeclarationKind(kind)
		if declaration != "" {
			nameNode := node.ChildByFieldName("name")
			name := b.tsText(nameNode)
			nodeStart, nodeEnd := start, end
			if kind == "variable_declarator" && frame.parent != nil && (frame.parent.Kind() == "lexical_declaration" || frame.parent.Kind() == "variable_declaration") {
				count := 0
				for i := uint(0); i < frame.parent.NamedChildCount(); i++ {
					if frame.parent.NamedChild(i).Kind() == "variable_declarator" {
						count++
					}
				}
				if count == 1 {
					nodeStart, nodeEnd = int(frame.parent.StartByte()), int(frame.parent.EndByte())
				}
			}
			if kind == "variable_declarator" && !isTSIdentifier(nameNode) {
				for _, identifier := range tsPatternIdentifiers(nameNode) {
					local := b.tsText(identifier)
					key := b.addNode(owner, "symbol", declaration, local, nodeStart, nodeEnd)
					bindingScope := scope
					if frame.parent != nil && frame.parent.Kind() == "variable_declaration" {
						bindingScope = tsFunctionScope(scope)
					}
					bindingScope.bind(&tsBinding{name: local, key: key, valueOK: true, kind: "variable", start: nodeStart})
					b.ts.declKeys[start] = append(b.ts.declKeys[start], key)
				}
			} else {
				key := b.addNode(owner, "symbol", declaration, name, nodeStart, nodeEnd)
				if b.err != nil {
					return b.err
				}
				childOwner = key
				b.ts.declKeys[start] = append(b.ts.declKeys[start], key)
				binding := &tsBinding{name: name, key: key, start: start, kind: declaration}
				switch declaration {
				case "type", "interface":
					binding.typeOK, binding.hoisted = true, true
				case "class", "enum":
					binding.typeOK, binding.valueOK = true, true
				case "function":
					binding.valueOK, binding.hoisted = true, true
				case "variable":
					binding.valueOK = true
				case "namespace":
					binding.typeOK, binding.valueOK = true, true
				}
				if declaration == "function" && (kind == "function_expression" || kind == "generator_function") {
					// Named function expressions bind only inside their own scope.
					childScope = newTSScope(scope, "function")
					childScope.bind(binding)
				} else if kind == "class" {
					childScope = newTSScope(scope, "container")
					childScope.bind(binding)
				} else if declaration != "method" && declaration != "anonymous_function" {
					bindingScope := scope
					if kind == "variable_declarator" && frame.parent != nil && frame.parent.Kind() == "variable_declaration" {
						bindingScope = tsFunctionScope(scope)
					}
					bindingScope.bind(binding)
				}
			}
		}
		if isTSFunction(kind) {
			if childScope == scope {
				childScope = newTSScope(scope, "function")
			}
			// These names are lexical shadows, not guessed callable targets.
			if parameter := node.ChildByFieldName("parameter"); parameter != nil {
				bindTSPattern(b, childScope, parameter, "parameter", false)
			}
		} else if kind == "class_declaration" || kind == "abstract_class_declaration" || kind == "class" || kind == "interface_declaration" || kind == "internal_module" || kind == "module" {
			if childScope == scope {
				childScope = newTSScope(scope, "container")
			}
		} else if kind == "statement_block" || kind == "for_statement" || kind == "for_in_statement" || kind == "switch_body" || kind == "catch_clause" {
			childScope = newTSScope(scope, "block")
		}
		switch kind {
		case "import_statement":
			b.parseTSImport(node, scope)
		case "export_statement":
			if scope == b.ts.root {
				b.parseTSExport(node)
			} else {
				b.ts.references = append(b.ts.references, tsReference{kind: "exports_symbol", scope: scope, from: owner, start: start, end: end, forced: "unsupported_target"})
			}
		case "required_parameter", "optional_parameter":
			pattern := node.ChildByFieldName("pattern")
			if pattern == nil {
				pattern = node.ChildByFieldName("name")
			}
			if pattern == nil {
				for i := uint(0); i < node.NamedChildCount(); i++ {
					child := node.NamedChild(i)
					if isTSIdentifier(child) || strings.HasSuffix(child.Kind(), "pattern") {
						pattern = child
						break
					}
				}
			}
			bindTSPattern(b, scope, pattern, "parameter", false)
		case "type_parameter":
			name := node.ChildByFieldName("name")
			if name != nil {
				scope.bind(&tsBinding{name: b.tsText(name), typeOK: true, kind: "type_parameter", start: start, hoisted: true})
			}
		case "catch_clause":
			bindTSPattern(b, childScope, node.ChildByFieldName("parameter"), "parameter", false)
		case "call_expression", "new_expression":
			function := node.ChildByFieldName("function")
			if function == nil {
				function = node.ChildByFieldName("constructor")
			}
			ref := tsReference{kind: "calls_local", scope: scope, from: owner, start: start, end: end, forced: "dynamic_call"}
			if function != nil && function.Kind() == "import" {
				ref.kind, ref.forced = "imports_file", "dynamic_import"
			} else if isTSIdentifier(function) {
				ref.name, ref.forced = b.tsText(function), ""
			}
			b.ts.references = append(b.ts.references, ref)
		case "nested_type_identifier":
			b.ts.references = append(b.ts.references, tsReference{kind: "references_type", scope: scope, from: owner, start: start, end: end, forced: "unsupported_target"})
		case "type_identifier":
			if !isTSTypeDeclarationName(node, frame.parent) {
				b.ts.references = append(b.ts.references, tsReference{kind: "references_type", name: b.tsText(node), scope: scope, from: owner, start: start, end: end})
			}
		}
		if b.err != nil {
			return b.err
		}
		if len(b.ts.references)+len(b.ts.imports) > MaxRelations {
			return b.fail("relations_limit")
		}
		// A qualified type's child identifiers must not become false local edges.
		if kind == "nested_type_identifier" {
			continue
		}
		for i := node.NamedChildCount(); i > 0; i-- {
			stack = append(stack, tsFrame{node: node.NamedChild(i - 1), parent: node, scope: childScope, owner: childOwner})
		}
	}
	return b.err
}

func tsDeclarationKind(kind string) string {
	switch kind {
	case "function_declaration", "generator_function_declaration", "function_signature", "function_expression", "generator_function":
		return "function"
	case "arrow_function":
		return "anonymous_function"
	case "method_definition", "method_signature", "abstract_method_signature":
		return "method"
	case "class_declaration", "abstract_class_declaration", "class":
		return "class"
	case "interface_declaration":
		return "interface"
	case "type_alias_declaration":
		return "type"
	case "enum_declaration":
		return "enum"
	case "internal_module", "module":
		return "namespace"
	case "variable_declarator":
		return "variable"
	}
	return ""
}

func isTSFunction(kind string) bool {
	d := tsDeclarationKind(kind)
	return d == "function" || d == "method" || d == "anonymous_function"
}
func isTSIdentifier(node *ts.Node) bool {
	return node != nil && (node.Kind() == "identifier" || node.Kind() == "type_identifier" || node.Kind() == "shorthand_property_identifier_pattern")
}
func (b *fileBuilder) tsText(node *ts.Node) string {
	if node == nil {
		return ""
	}
	return string(b.in.Content[node.StartByte():node.EndByte()])
}

func tsFunctionScope(scope *tsScope) *tsScope {
	for scope.parent != nil && scope.kind != "function" {
		scope = scope.parent
	}
	return scope
}

func tsPatternIdentifiers(node *ts.Node) []*ts.Node {
	if node == nil {
		return nil
	}
	out := []*ts.Node{}
	stack := []*ts.Node{node}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if isTSIdentifier(current) {
			out = append(out, current)
			continue
		}
		switch current.Kind() {
		case "pair_pattern":
			if value := current.ChildByFieldName("value"); value != nil {
				stack = append(stack, value)
			}
		case "assignment_pattern", "object_assignment_pattern":
			if left := current.ChildByFieldName("left"); left != nil {
				stack = append(stack, left)
			}
		case "object_pattern", "array_pattern", "rest_pattern", "required_parameter", "optional_parameter":
			for i := current.NamedChildCount(); i > 0; i-- {
				child := current.NamedChild(i - 1)
				if child.Kind() != "type_annotation" {
					stack = append(stack, child)
				}
			}
		}
	}
	return out
}

func bindTSPattern(b *fileBuilder, scope *tsScope, node *ts.Node, kind string, typeOnly bool) {
	for _, name := range tsPatternIdentifiers(node) {
		scope.bind(&tsBinding{name: b.tsText(name), kind: kind, valueOK: !typeOnly, typeOK: typeOnly, start: int(name.StartByte()), hoisted: true})
	}
}

func isTSTypeDeclarationName(node, parent *ts.Node) bool {
	if parent == nil {
		return false
	}
	kind := parent.Kind()
	if kind == "type_parameter" || kind == "import_specifier" || kind == "export_specifier" {
		return true
	}
	if tsDeclarationKind(kind) != "" {
		name := parent.ChildByFieldName("name")
		return name != nil && name.StartByte() == node.StartByte() && name.EndByte() == node.EndByte()
	}
	return false
}

func hasTSToken(node *ts.Node, token string) bool {
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == token {
			return true
		}
	}
	return false
}

func (b *fileBuilder) moduleImport(node *ts.Node) *tsImport {
	if node == nil {
		return nil
	}
	start, end := int(node.StartByte()), int(node.EndByte())
	value := b.in.Content[start:end]
	plain := len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"'))
	if plain {
		start++
		end--
		value = value[1 : len(value)-1]
	}
	if strings.ContainsAny(string(value), "\\\r\n\x00") {
		plain = false
	}
	imp := &tsImport{module: string(value), start: start, end: end, plain: plain}
	b.ts.imports = append(b.ts.imports, imp)
	return imp
}

func (b *fileBuilder) parseTSImport(node *ts.Node, scope *tsScope) {
	module := b.moduleImport(node.ChildByFieldName("source"))
	if module == nil {
		return
	}
	typeOnly := hasTSToken(node, "type")
	for i := uint(0); i < node.NamedChildCount(); i++ {
		clause := node.NamedChild(i)
		if clause.Kind() != "import_clause" {
			continue
		}
		for j := uint(0); j < clause.NamedChildCount(); j++ {
			child := clause.NamedChild(j)
			switch child.Kind() {
			case "identifier":
				scope.bind(&tsBinding{name: b.tsText(child), typeOK: true, valueOK: !typeOnly, kind: "import", imported: &tsImportBinding{module: module, name: "default"}, hoisted: true})
			case "namespace_import":
				name := child.ChildByFieldName("name")
				if name == nil && child.NamedChildCount() > 0 {
					name = child.NamedChild(child.NamedChildCount() - 1)
				}
				if name != nil {
					scope.bind(&tsBinding{name: b.tsText(name), typeOK: true, valueOK: !typeOnly, kind: "import", imported: &tsImportBinding{module: module, namespace: true}, hoisted: true})
				}
			case "named_imports":
				for k := uint(0); k < child.NamedChildCount(); k++ {
					spec := child.NamedChild(k)
					if spec.Kind() != "import_specifier" {
						continue
					}
					name, alias := spec.ChildByFieldName("name"), spec.ChildByFieldName("alias")
					if name == nil {
						continue
					}
					if alias == nil {
						alias = name
					}
					localTypeOnly := typeOnly || hasTSToken(spec, "type")
					scope.bind(&tsBinding{name: b.tsText(alias), typeOK: true, valueOK: !localTypeOnly, kind: "import", imported: &tsImportBinding{module: module, name: b.tsText(name)}, hoisted: true})
				}
			}
		}
	}
}

func (b *fileBuilder) parseTSExport(node *ts.Node) {
	module := b.moduleImport(node.ChildByFieldName("source"))
	typeOnly := hasTSToken(node, "type")
	start, end := int(node.StartByte()), int(node.EndByte())
	var clause *ts.Node
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if node.NamedChild(i).Kind() == "export_clause" {
			clause = node.NamedChild(i)
			break
		}
	}
	if clause != nil {
		for i := uint(0); i < clause.NamedChildCount(); i++ {
			spec := clause.NamedChild(i)
			if spec.Kind() != "export_specifier" {
				continue
			}
			name, alias := spec.ChildByFieldName("name"), spec.ChildByFieldName("alias")
			if name == nil {
				continue
			}
			if alias == nil {
				alias = name
			}
			export := &tsExport{name: b.tsText(alias), local: b.tsText(name), module: module, imported: b.tsText(name), typeOnly: typeOnly || hasTSToken(spec, "type"), declStart: -1, start: int(spec.StartByte()), end: int(spec.EndByte())}
			b.ts.exports[export.name] = append(b.ts.exports[export.name], export)
		}
		return
	}
	if module != nil {
		// export-star and export-namespace never enumerate or infer a namespace.
		b.ts.stars = append(b.ts.stars, &tsExport{module: module, typeOnly: typeOnly, declStart: -1, start: start, end: end})
		return
	}
	declaration := node.ChildByFieldName("declaration")
	if declaration == nil {
		declaration = node.ChildByFieldName("value")
	}
	if declaration == nil {
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() != "comment" {
				declaration = child
				break
			}
		}
	}
	if declaration == nil {
		return
	}
	if hasTSToken(node, "default") {
		export := &tsExport{name: "default", local: b.tsText(declaration.ChildByFieldName("name")), declStart: int(declaration.StartByte()), start: start, end: end}
		if isTSIdentifier(declaration) {
			export.local = b.tsText(declaration)
			export.declStart = -1
		}
		b.ts.exports["default"] = append(b.ts.exports["default"], export)
		return
	}
	if declaration.Kind() == "lexical_declaration" || declaration.Kind() == "variable_declaration" {
		for i := uint(0); i < declaration.NamedChildCount(); i++ {
			child := declaration.NamedChild(i)
			if child.Kind() != "variable_declarator" {
				continue
			}
			for _, name := range tsPatternIdentifiers(child.ChildByFieldName("name")) {
				local := b.tsText(name)
				export := &tsExport{name: local, local: local, declStart: int(child.StartByte()), start: start, end: end}
				b.ts.exports[local] = append(b.ts.exports[local], export)
			}
		}
		return
	}
	if name := declaration.ChildByFieldName("name"); name != nil {
		local := b.tsText(name)
		export := &tsExport{name: local, local: local, declStart: int(declaration.StartByte()), typeOnly: typeOnly, start: start, end: end}
		b.ts.exports[local] = append(b.ts.exports[local], export)
	}
}
