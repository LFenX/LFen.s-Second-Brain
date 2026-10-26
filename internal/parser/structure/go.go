package structure

import (
	goast "go/ast"
	goparser "go/parser"
	"go/token"
)

func (b *fileBuilder) parseGo() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	positions := token.NewFileSet()
	file, err := goparser.ParseFile(positions, b.in.Path, b.in.Content, goparser.AllErrors|goparser.ParseComments)
	if err != nil {
		return b.fail("syntax_error")
	}
	if err = b.ctx.Err(); err != nil {
		return err
	}
	physical := func(position token.Pos) int { return positions.PositionFor(position, false).Offset }
	objects := map[*goast.Object]string{}
	parents := []string{"file"}
	ancestors := []goast.Node{}
	goast.Inspect(file, func(node goast.Node) bool {
		if node == nil {
			parents = parents[:len(parents)-1]
			ancestors = ancestors[:len(ancestors)-1]
			return true
		}
		if b.err != nil {
			return false
		}
		if err := b.ctx.Err(); err != nil {
			b.err = err
			return false
		}
		parent := parents[len(parents)-1]
		nextParent := parent
		start, end := physical(node.Pos()), physical(node.End())
		switch typed := node.(type) {
		case *goast.FuncDecl:
			kind := "function"
			if typed.Recv != nil {
				kind = "method"
				if len(typed.Recv.List) > 0 {
					kind += ":" + goReceiverName(typed.Recv.List[0].Type)
				}
			}
			nextParent = b.addNode(parent, "symbol", kind, typed.Name.Name, start, end)
			if typed.Name.Obj != nil {
				objects[typed.Name.Obj] = nextParent
			}
		case *goast.TypeSpec:
			if len(ancestors) > 0 {
				if declaration, ok := ancestors[len(ancestors)-1].(*goast.GenDecl); ok && len(declaration.Specs) == 1 {
					start, end = physical(declaration.Pos()), physical(declaration.End())
				}
			}
			nextParent = b.addNode(parent, "symbol", "type", typed.Name.Name, start, end)
			if typed.Name.Obj != nil {
				objects[typed.Name.Obj] = nextParent
			}
		case *goast.ValueSpec:
			if len(ancestors) > 0 {
				if declaration, ok := ancestors[len(ancestors)-1].(*goast.GenDecl); ok && len(declaration.Specs) == 1 {
					start, end = physical(declaration.Pos()), physical(declaration.End())
				}
			}
			for _, name := range typed.Names {
				if name.Name == "_" {
					continue
				}
				key := b.addNode(parent, "symbol", "variable", name.Name, start, end)
				if name.Obj != nil {
					objects[name.Obj] = key
				}
				if nextParent == parent {
					nextParent = key
				}
			}
		}
		if _, ok := node.(goast.Stmt); ok {
			b.ends = append(b.ends, end)
		}
		if _, ok := node.(goast.Decl); ok {
			b.ends = append(b.ends, end)
		}
		parents = append(parents, nextParent)
		ancestors = append(ancestors, node)
		return true
	})
	if b.err != nil {
		return b.err
	}
	// Go parser object resolution is file-local. No go/packages, type checker,
	// compiler, module download, or cross-file fallback is used.
	for _, imp := range file.Imports {
		start, end := physical(imp.Path.Pos()), physical(imp.Path.End())
		if start < end-1 {
			start++
			end--
		}
		b.addRelation("imports_file", "file", nil, "go_package_unresolved", start, end)
	}
	typeSeen := map[*goast.Ident]bool{}
	var typeWalk func(goast.Expr)
	typeWalk = func(expression goast.Expr) {
		if expression == nil || b.err != nil {
			return
		}
		if err := b.ctx.Err(); err != nil {
			b.err = err
			return
		}
		switch typed := expression.(type) {
		case *goast.Ident:
			if typeSeen[typed] || isGoBuiltinType(typed.Name) {
				return
			}
			typeSeen[typed] = true
			start, end := physical(typed.Pos()), physical(typed.End())
			var target *Ref
			reason := "go_package_unresolved"
			if typed.Obj != nil {
				if key := objects[typed.Obj]; key != "" && typed.Obj.Kind == goast.Typ {
					target = &Ref{SourceID: b.in.SourceID, Key: key}
					reason = "lexical_binding"
				} else {
					reason = "unsupported_target"
				}
			}
			b.addRelation("references_type", b.owner(start, end), target, reason, start, end)
		case *goast.SelectorExpr:
			start, end := physical(typed.Pos()), physical(typed.End())
			b.addRelation("references_type", b.owner(start, end), nil, "go_package_unresolved", start, end)
		case *goast.ArrayType:
			typeWalk(typed.Elt) // Len is a value expression, not a type.
		case *goast.MapType:
			typeWalk(typed.Key)
			typeWalk(typed.Value)
		case *goast.ChanType:
			typeWalk(typed.Value)
		case *goast.StarExpr:
			typeWalk(typed.X)
		case *goast.Ellipsis:
			typeWalk(typed.Elt)
		case *goast.ParenExpr:
			typeWalk(typed.X)
		case *goast.IndexExpr:
			typeWalk(typed.X)
			typeWalk(typed.Index)
		case *goast.IndexListExpr:
			typeWalk(typed.X)
			for _, argument := range typed.Indices {
				typeWalk(argument)
			}
		case *goast.BinaryExpr:
			typeWalk(typed.X)
			typeWalk(typed.Y)
		case *goast.UnaryExpr:
			typeWalk(typed.X)
		case *goast.StructType:
			if typed.Fields != nil {
				for _, field := range typed.Fields.List {
					typeWalk(field.Type)
				}
			}
		case *goast.InterfaceType:
			if typed.Methods != nil {
				for _, field := range typed.Methods.List {
					typeWalk(field.Type)
				}
			}
		case *goast.FuncType:
			for _, fields := range []*goast.FieldList{typed.TypeParams, typed.Params, typed.Results} {
				if fields != nil {
					for _, field := range fields.List {
						typeWalk(field.Type)
					}
				}
			}
		}
	}
	goast.Inspect(file, func(node goast.Node) bool {
		if node == nil || b.err != nil {
			return b.err == nil
		}
		if err := b.ctx.Err(); err != nil {
			b.err = err
			return false
		}
		switch typed := node.(type) {
		case *goast.Field:
			typeWalk(typed.Type)
		case *goast.TypeSpec:
			typeWalk(typed.Type)
		case *goast.ValueSpec:
			typeWalk(typed.Type)
		case *goast.CompositeLit:
			typeWalk(typed.Type)
		case *goast.TypeAssertExpr:
			typeWalk(typed.Type)
		case *goast.CallExpr:
			start, end := physical(typed.Pos()), physical(typed.End())
			var target *Ref
			reason := "dynamic_call"
			if identifier, ok := typed.Fun.(*goast.Ident); ok {
				if identifier.Obj != nil && identifier.Obj.Kind == goast.Typ {
					typeWalk(identifier)
					break
				}
				if identifier.Obj == nil {
					if isGoBuiltinCall(identifier.Name) {
						break
					}
					reason = "go_package_unresolved"
				} else if key := objects[identifier.Obj]; key != "" {
					target = &Ref{SourceID: b.in.SourceID, Key: key}
					reason = "lexical_binding"
				}
			}
			b.addRelation("calls_local", b.owner(start, end), target, reason, start, end)
		}
		return b.err == nil
	})
	return b.err
}

func goReceiverName(expression goast.Expr) string {
	switch typed := expression.(type) {
	case *goast.Ident:
		return typed.Name
	case *goast.StarExpr:
		return goReceiverName(typed.X)
	case *goast.IndexExpr:
		return goReceiverName(typed.X)
	case *goast.IndexListExpr:
		return goReceiverName(typed.X)
	}
	return "anonymous"
}

func isGoBuiltinType(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "any", "comparable":
		return true
	}
	return false
}

func isGoBuiltinCall(name string) bool {
	if isGoBuiltinType(name) {
		return true
	}
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover":
		return true
	}
	return false
}
