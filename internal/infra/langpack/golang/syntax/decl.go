package syntax

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/genmark"
)

const FileDeclName = "<file>"

func Declarations(_ context.Context, path string, source []byte) ([]entities.Decl, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(
		fset,
		path,
		source,
		parser.ParseComments|parser.SkipObjectResolution,
	)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	unit := unitOf(path, file.Name.Name)

	isTestFile := strings.HasSuffix(path, "_test.go")

	var decls []entities.Decl

	for _, decl := range file.Decls {
		decls = append(decls, buildDecls(fset, source, path, unit, isTestFile, decl)...)
	}

	if len(decls) == 0 {
		return []entities.Decl{wholeFile(fset, path, unit, source, file)}, nil
	}

	// A generated file is written by its generator in ONE pass, so its
	// declarations are born together. They STAY declarations: taking them out
	// of the graph took every reference edge to them along, and a test landed
	// 491 positions before the mock it uses. The files of one sqlc run are
	// grouped by the sql atom rule instead, one atom per table and per query.
	if genmark.IsGenerated(source) && !genmark.IsSqlcOutput(source) {
		for index := range decls {
			decls[index].Atom = path
		}
	}

	return decls, nil
}

func wholeFile(
	fset *token.FileSet,
	path string,
	unit entities.UnitID,
	source []byte,
	file *ast.File,
) entities.Decl {
	return entities.Decl{
		// A file with no declarations would vanish from the rebuilt tree; this stands for the whole file.
		// The name is deliberately not a legal Go identifier, so it never collides with a real declaration.
		Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: FileDeclName},
		Unit:     unit,
		Span:     entities.Span{Start: 0, End: len(source)},
		Selector: LocationOf(fset, path, source, file.Name.Pos()),
		IsTest:   strings.HasSuffix(path, "_test.go"),
	}
}

func unitOf(path, pkgName string) entities.UnitID {
	dir := filepath.Dir(path)
	if strings.HasSuffix(pkgName, "_test") {
		return entities.UnitID(dir + "#" + pkgName)
	}

	return entities.UnitID(dir)
}

func buildDecls(
	fset *token.FileSet,
	source []byte,
	path string,
	unit entities.UnitID,
	isTestFile bool,
	decl ast.Decl,
) []entities.Decl {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		return []entities.Decl{buildFuncDecl(fset, path, source, unit, isTestFile, typed)}
	case *ast.GenDecl:
		return buildGenDecl(fset, source, path, unit, typed)
	default:
		return nil
	}
}

func spanOf(fset *token.FileSet, start, end token.Pos) entities.Span {
	return entities.Span{
		Start: fset.Position(start).Offset,
		End:   fset.Position(end).Offset,
	}
}

func LocationOf(
	fset *token.FileSet,
	path string,
	source []byte,
	pos token.Pos,
) entities.Location {
	position := fset.PositionFor(pos, false)
	start := position.Offset - (position.Column - 1)

	return entities.Location{
		File: path,
		Line: position.Line - 1,
		// token Column counts bytes while LSP Char counts UTF-16 units.
		Char: utf16Units(source[start:position.Offset]),
	}
}

func utf16Units(encoded []byte) int {
	return len(utf16.Encode([]rune(string(encoded))))
}
