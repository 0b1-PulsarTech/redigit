package golang

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

// enclosesRule keeps a value or constructor with the type it belongs to: a
// value declaration typed as a type in the same file, or a function whose
// sole result is a type in the same file, starts where that type starts.
// Without it, "type Channel string" lands in one commit and the constants
// enumerating it land, alone, five commits later.
//
// The link is read from the declaration's own syntax (a ValueSpec's declared
// type, a FuncType's single result), not from a reference graph: RulePack
// gets no Resolver, only the declarations Syntax already found.
type enclosesRule struct{}

func (enclosesRule) Name() string { return "encloses" }

func (enclosesRule) Edges(
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

	for _, d := range decls {
		if d.Key.Kind == entities.KindType {
			types[d.Key.Name] = d.Key
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
		return nil, err
	}

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, source, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}

	var edges []entities.Edge

	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			edges = append(edges, funcEncloses(fileDecls, file, types, d)...)
		case *ast.GenDecl:
			edges = append(edges, valueEncloses(fileDecls, file, types, d)...)
		}
	}

	return edges, nil
}

// funcEncloses links a receiver-less function to the type it constructs,
// when it returns exactly that one type.
func funcEncloses(
	fileDecls []entities.Decl,
	file string,
	types map[string]entities.DeclKey,
	d *ast.FuncDecl,
) []entities.Edge {
	if d.Recv != nil {
		return nil
	}

	typeKey, ok := singleResultType(d, types)
	if !ok {
		return nil
	}

	funcKey, ok := declsource.FindDecl(fileDecls, file, entities.KindFunc, d.Name.Name)
	if !ok {
		return nil
	}

	return []entities.Edge{{Dependent: funcKey, Provider: typeKey, Reason: entities.ReasonEncloses}}
}

func singleResultType(d *ast.FuncDecl, types map[string]entities.DeclKey) (entities.DeclKey, bool) {
	if d.Type.Results == nil {
		return entities.DeclKey{}, false
	}

	total := 0

	for _, field := range d.Type.Results.List {
		total += max(len(field.Names), 1)
	}

	if total != 1 {
		return entities.DeclKey{}, false
	}

	expr := d.Type.Results.List[0].Type
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

// valueEncloses links a const/var to the type it is declared with.
func valueEncloses(
	fileDecls []entities.Decl,
	file string,
	types map[string]entities.DeclKey,
	d *ast.GenDecl,
) []entities.Edge {
	if d.Tok != token.CONST && d.Tok != token.VAR {
		return nil
	}

	var edges []entities.Edge

	for _, spec := range d.Specs {
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

			if valueKey, found := declsource.FindDecl(fileDecls, file, entities.KindValue, name.Name); found {
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

func (enclosesRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
