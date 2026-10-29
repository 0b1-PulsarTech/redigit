package wiring

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type Encloses struct{}

func (Encloses) Name() string { return "encloses" }

func (Encloses) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	byFile := declsource.GroupByFile(decls)

	var edges []entities.Edge

	for file, fileDecls := range byFile {
		types := typeNames(fileDecls)
		if len(types) == 0 {
			continue
		}

		found, err := enclosesInFile(snap, fileDecls, file, types)
		if err != nil {
			return nil, err
		}

		edges = append(edges, found...)
	}

	return edges, nil
}

func typeNames(decls []entities.Decl) map[string]entities.DeclKey {
	types := map[string]entities.DeclKey{}

	for _, decl := range decls {
		if decl.Key.Kind == entities.KindType {
			types[decl.Key.Name] = decl.Key
		}
	}

	return types
}

func enclosesInFile(
	snap entities.Snapshot,
	fileDecls []entities.Decl,
	file string,
	types map[string]entities.DeclKey,
) ([]entities.Edge, error) {
	source, err := declsource.ReadSource(snap, file)
	if err != nil {
		return nil, fmt.Errorf("enclosing source: %w", err)
	}

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, source, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}

	var edges []entities.Edge

	for _, decl := range parsed.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			edges = append(edges, funcEncloses(fileDecls, file, types, typed)...)
		case *ast.GenDecl:
			edges = append(edges, valueEncloses(fileDecls, file, types, typed)...)
		}
	}

	return edges, nil
}

func funcEncloses(
	fileDecls []entities.Decl,
	file string,
	types map[string]entities.DeclKey,
	fn *ast.FuncDecl,
) []entities.Edge {
	if fn.Recv != nil {
		return nil
	}

	typeKey, ok := singleResultType(fn, types)
	if !ok {
		return nil
	}

	funcKey, ok := declsource.FindDecl(fileDecls, file, entities.KindFunc, fn.Name.Name)
	if !ok {
		return nil
	}

	return []entities.Edge{{Dependent: funcKey, Provider: typeKey, Reason: entities.ReasonEncloses}}
}

func singleResultType(
	fn *ast.FuncDecl,
	types map[string]entities.DeclKey,
) (entities.DeclKey, bool) {
	if fn.Type.Results == nil {
		return entities.DeclKey{}, false
	}

	total := 0

	for _, field := range fn.Type.Results.List {
		total += max(len(field.Names), 1)
	}

	if total != 1 {
		return entities.DeclKey{}, false
	}

	expr := fn.Type.Results.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	ident, ok := expr.(*ast.Ident)
	if !ok {
		return entities.DeclKey{}, false
	}

	key, found := types[ident.Name]

	return key, found
}

func valueEncloses(
	fileDecls []entities.Decl,
	file string,
	types map[string]entities.DeclKey,
	gen *ast.GenDecl,
) []entities.Edge {
	if gen.Tok != token.CONST && gen.Tok != token.VAR {
		return nil
	}

	var edges []entities.Edge

	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || vs.Type == nil {
			continue
		}

		ident, ok := vs.Type.(*ast.Ident)
		if !ok {
			continue
		}

		typeKey, ok := types[ident.Name]
		if !ok {
			continue
		}

		for _, name := range vs.Names {
			if name.Name == "_" {
				continue
			}

			if valueKey, found := declsource.FindDecl(
				fileDecls,
				file,
				entities.KindValue,
				name.Name,
			); found {
				edges = append(edges, entities.Edge{
					Dependent: valueKey,
					Provider:  typeKey,
					Reason:    entities.ReasonEncloses,
				})
			}
		}
	}

	return edges
}

func (Encloses) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
