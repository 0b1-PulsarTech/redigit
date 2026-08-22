package golang

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// Syntax implements indexing.Syntax for Go source: declaration boundaries via
// go/ast, partial-file rendering via github.com/dave/dst.
type Syntax struct{}

// NewSyntax builds the Go syntax adapter. It carries no state: every call is
// self-contained on the source it is given.
func NewSyntax() Syntax { return Syntax{} }

var _ indexing.Syntax = Syntax{}

// Declarations parses source and returns one Decl per top-level declaration.
// A parenthesized const group where a later spec omits both its type and its
// value (an iota chain) is kept as one atomic declaration: splitting it would
// let a later commit change what an untouched constant evaluates to.
// fileDeclName is the name given to the single declaration standing for a file
// that declares nothing of its own. It is not a legal Go identifier on purpose,
// so it can never collide with a real declaration.
const fileDeclName = "<file>"

func (Syntax) Declarations(_ context.Context, path string, source []byte) ([]entities.Decl, error) {
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

	return decls, nil
}

// wholeFile stands in for a file that declares nothing: a package clause and
// imports, and no more. Real trees hold them - a file that only pins blank
// imports, a package doc file - and without a declaration to stand for it the
// file belongs to no commit, is never written, and silently vanishes from the
// rebuilt tree. Giving it one declaration spanning the whole file makes it a
// citizen of the ordering like everything else.
func wholeFile(
	fset *token.FileSet,
	path string,
	unit entities.UnitID,
	source []byte,
	file *ast.File,
) entities.Decl {
	return entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: fileDeclName},
		Unit:     unit,
		Span:     entities.Span{Start: 0, End: len(source)},
		Selector: locationOf(fset, path, source, file.Name.Pos()),
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
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return []entities.Decl{buildFuncDecl(fset, path, source, unit, isTestFile, d)}
	case *ast.GenDecl:
		return buildGenDecl(fset, source, path, unit, d)
	default:
		return nil
	}
}

func buildFuncDecl(
	fset *token.FileSet,
	path string,
	source []byte,
	unit entities.UnitID,
	isTestFile bool,
	decl *ast.FuncDecl,
) entities.Decl {
	kind := entities.KindFunc
	name := decl.Name.Name

	if decl.Recv != nil {
		kind = entities.KindMethod

		if recv := receiverName(decl.Recv); recv != "" {
			name = recv + "." + name
		}
	}

	start := decl.Pos()
	if decl.Doc != nil {
		start = decl.Doc.Pos()
	}

	return entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: kind, Name: name},
		Unit:     unit,
		Span:     spanOf(fset, start, decl.End()),
		Selector: locationOf(fset, path, source, decl.Name.Pos()),
		IsTest:   isTestFile && decl.Recv == nil && isTestName(decl.Name.Name),
	}
}

// receiverName returns the base type name of a method receiver, stripping a
// pointer and any generic type parameters: "*Repo[T]" and "Repo[T]" both name
// "Repo". Without the receiver, two types that both implement Error()
// collapse into one declaration.
func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverBase(t.X)
	case *ast.IndexListExpr:
		return receiverBase(t.X)
	default:
		return ""
	}
}

func receiverBase(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}

	return ""
}

// isTestName reports whether name is a Test/Benchmark/Fuzz/Example function
// the go test binary would run: the prefix followed by nothing, or by a rune
// that is not lowercase.
func isTestName(name string) bool {
	for _, prefix := range [...]string{"Test", "Benchmark", "Fuzz", "Example"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		rest := name[len(prefix):]
		if rest == "" {
			return true
		}

		r, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsLower(r) {
			return true
		}
	}

	return false
}

func spanOf(fset *token.FileSet, start, end token.Pos) entities.Span {
	return entities.Span{
		Start: fset.Position(start).Offset,
		End:   fset.Position(end).Offset,
	}
}

// locationOf reports the 0-based line/char of pos, the position a language
// server query anchors on.
// locationOf converts a parser position into the coordinates a language server
// speaks. token.Position.Column counts BYTES, while entities.Location.Char
// counts UTF-16 code units, so the two diverge on any line holding a non-ASCII
// character and the server would be queried about the wrong one.
func locationOf(
	fset *token.FileSet,
	path string,
	source []byte,
	pos token.Pos,
) entities.Location {
	position := fset.Position(pos)
	start := position.Offset - (position.Column - 1)

	return entities.Location{
		File: path,
		Line: position.Line - 1,
		Char: utf16Units(source[start:position.Offset]),
	}
}

// utf16Units counts the UTF-16 code units a run of bytes encodes to. An emoji
// is a surrogate pair there and therefore counts as two.
func utf16Units(encoded []byte) int {
	return len(utf16.Encode([]rune(string(encoded))))
}
