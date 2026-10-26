package structure

import (
	"context"
	"path"
	"sort"
	"strings"
)

func resolveTypeScript(ctx context.Context, builders []*fileBuilder, aliases []Alias) error {
	paths := make(map[string][]*fileBuilder, len(builders))
	for _, b := range builders {
		paths[b.in.Path] = append(paths[b.in.Path], b)
	}
	for _, b := range builders {
		if b.ts == nil {
			continue
		}
		for _, imp := range b.ts.imports {
			if err := ctx.Err(); err != nil {
				return err
			}
			imp.target, imp.reason = resolveTSPath(b, imp, paths, aliases)
			var target *Ref
			if imp.target != nil {
				target = &Ref{SourceID: imp.target.in.SourceID, Key: "file"}
			}
			b.addRelation("imports_file", "file", target, imp.reason, imp.start, imp.end)
		}
	}
	for _, b := range builders {
		if b.ts == nil {
			continue
		}
		names := make([]string, 0, len(b.ts.exports))
		for name := range b.ts.exports {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			for _, exported := range b.ts.exports[name] {
				if err := ctx.Err(); err != nil {
					return err
				}
				target, reason := resolveTSExport(b, name, exported.typeOnly, map[string]bool{})
				if target == nil && !exported.typeOnly {
					target, reason = resolveTSExport(b, name, true, map[string]bool{})
				}
				if target != nil {
					reason = "named_export"
				}
				b.addRelation("exports_symbol", "file", target, reason, exported.start, exported.end)
			}
		}
		for _, star := range b.ts.stars {
			b.addRelation("exports_symbol", "file", nil, "wildcard_export", star.start, star.end)
		}
		for _, ref := range b.ts.references {
			if err := ctx.Err(); err != nil {
				return err
			}
			var target *Ref
			reason := ref.forced
			if reason == "" {
				target, reason = resolveTSBinding(b, ref.scope, ref.name, ref.kind == "references_type", ref.start, map[string]bool{})
			}
			b.addRelation(ref.kind, ref.from, target, reason, ref.start, ref.end)
		}
		if b.err != nil {
			return b.err
		}
	}
	return nil
}

func resolveTSPath(from *fileBuilder, imp *tsImport, paths map[string][]*fileBuilder, aliases []Alias) (*fileBuilder, string) {
	if !imp.plain || strings.ContainsAny(imp.module, "?#") {
		return nil, "unsupported_target"
	}
	base, reason := "", "external_package"
	if strings.HasPrefix(imp.module, "./") || strings.HasPrefix(imp.module, "../") {
		base, reason = path.Join(path.Dir(from.in.Path), imp.module), "explicit_relative"
	} else {
		for _, alias := range aliases {
			if strings.HasPrefix(imp.module, alias.Prefix) {
				base, reason = path.Join(alias.Target, strings.TrimPrefix(imp.module, alias.Prefix)), "explicit_alias"
				break
			}
		}
	}
	if base == "" {
		return nil, reason
	}
	if base == ".." || strings.HasPrefix(base, "../") || strings.HasPrefix(base, "/") {
		return nil, "outside_selection"
	}
	var candidates []string
	switch path.Ext(base) {
	case ".ts", ".tsx":
		candidates = []string{base}
	case "":
		candidates = []string{base + ".ts", base + ".tsx", path.Join(base, "index.ts"), path.Join(base, "index.tsx")}
	default:
		return nil, "unsupported_target"
	}
	var matches []*fileBuilder
	for _, candidate := range candidates {
		for _, item := range paths[candidate] {
			if item.ts != nil {
				matches = append(matches, item)
			}
		}
	}
	if len(matches) > 1 {
		return nil, "ambiguous"
	}
	if len(matches) == 0 {
		return nil, "outside_selection"
	}
	return matches[0], reason
}

func resolveTSBinding(file *fileBuilder, scope *tsScope, name string, wantType bool, position int, visiting map[string]bool) (*Ref, string) {
	for current := scope; current != nil; current = current.parent {
		var candidates []*tsBinding
		for _, binding := range current.bindings[name] {
			if (wantType && binding.typeOK) || (!wantType && binding.valueOK) {
				candidates = append(candidates, binding)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		if len(candidates) > 1 {
			return nil, "ambiguous"
		}
		binding := candidates[0]
		if binding.kind == "parameter" || binding.kind == "type_parameter" {
			return nil, "unsupported_target"
		}
		if !wantType && !binding.hoisted && position < binding.start {
			return nil, "unsupported_target"
		}
		if binding.imported != nil {
			imported := binding.imported
			if imported.namespace {
				return nil, "unsupported_target"
			}
			if imported.module.target == nil {
				return nil, imported.module.reason
			}
			target, reason := resolveTSExport(imported.module.target, imported.name, wantType, visiting)
			if target != nil {
				return target, "lexical_binding"
			}
			return nil, reason
		}
		if binding.key != "" {
			return &Ref{SourceID: file.in.SourceID, Key: binding.key}, "lexical_binding"
		}
		return nil, "unsupported_target"
	}
	return nil, "unsupported_target"
}

func resolveTSExport(file *fileBuilder, name string, wantType bool, visiting map[string]bool) (*Ref, string) {
	if file.ts == nil {
		return nil, "unsupported_target"
	}
	marker := file.in.SourceID + "/" + name
	if wantType {
		marker += "/type"
	} else {
		marker += "/value"
	}
	if visiting[marker] {
		return nil, "cycle"
	}
	visiting[marker] = true
	defer delete(visiting, marker)
	exports := file.ts.exports[name]
	if len(exports) == 0 {
		if len(file.ts.stars) > 0 {
			return nil, "wildcard_export"
		}
		return nil, "unsupported_target"
	}
	if len(exports) > 1 {
		return nil, "ambiguous"
	}
	exported := exports[0]
	if exported.typeOnly && !wantType {
		return nil, "unsupported_target"
	}
	if exported.module != nil {
		if exported.module.target == nil {
			return nil, exported.module.reason
		}
		return resolveTSExport(exported.module.target, exported.imported, wantType, visiting)
	}
	if exported.local != "" {
		return resolveTSBinding(file, file.ts.root, exported.local, wantType, len(file.in.Content), visiting)
	}
	if keys := file.ts.declKeys[exported.declStart]; len(keys) == 1 {
		for _, node := range file.file.Nodes {
			if node.Key != keys[0] {
				continue
			}
			isType := strings.Contains(node.LogicalKey, "/class/") || strings.Contains(node.LogicalKey, "/interface/") || strings.Contains(node.LogicalKey, "/type/") || strings.Contains(node.LogicalKey, "/enum/")
			isValue := strings.Contains(node.LogicalKey, "/function/") || strings.Contains(node.LogicalKey, "/anonymous_function/") || strings.Contains(node.LogicalKey, "/class/") || strings.Contains(node.LogicalKey, "/enum/") || strings.Contains(node.LogicalKey, "/variable/")
			if (wantType && isType) || (!wantType && isValue) {
				return &Ref{SourceID: file.in.SourceID, Key: keys[0]}, "named_export"
			}
		}
	}
	return nil, "unsupported_target"
}
