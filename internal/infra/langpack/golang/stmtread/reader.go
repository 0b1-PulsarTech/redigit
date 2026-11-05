package stmtread

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

type Reader struct {
	read  func(ctx context.Context, file string) ([]byte, error)
	mains map[entities.UnitID]bool
}

var _ linting.Syntax = Reader{}

func New(
	graph entities.Graph,
	read func(ctx context.Context, file string) ([]byte, error),
) Reader {
	mains := map[entities.UnitID]bool{}

	for _, decl := range graph.Decls() {
		if decl.Key.Kind == entities.KindFunc && decl.Key.Name == "main" &&
			isGoFile(decl.Key.File) {
			mains[decl.Unit] = true
		}
	}

	return Reader{read: read, mains: mains}
}

func isGoFile(file string) bool { return strings.HasSuffix(file, ".go") }

func (syntax Reader) Statements(
	ctx context.Context,
	file string,
	spans map[entities.DeclKey]entities.Span,
) (map[entities.DeclKey]linting.Statement, error) {
	if !isGoFile(file) {
		return nil, nil //nolint:nilnil // a file of another language has no statement to read
	}

	source, err := syntax.read(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, source, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}

	// The outermost statement starting at an offset is the one a span names.
	starts := map[int]ast.Stmt{}

	ast.Inspect(parsed, func(node ast.Node) bool {
		if stmt, isStmt := node.(ast.Stmt); isStmt {
			offset := fset.Position(stmt.Pos()).Offset
			if _, seen := starts[offset]; !seen {
				starts[offset] = stmt
			}
		}

		return true
	})

	offsets := slices.Sorted(maps.Keys(starts))
	facts := map[entities.DeclKey]linting.Statement{}

	for key, span := range spans {
		// The slicer starts a span at the comment that leads its statement: the first statement
		// inside [Start, End) is the one the span names.
		at, _ := slices.BinarySearch(offsets, span.Start)
		if at < len(offsets) && offsets[at] < span.End {
			facts[key] = readStatement(starts[offsets[at]])
		}
	}

	return facts, nil
}

func (syntax Reader) Exempt(decl entities.Decl) string {
	switch {
	case !isGoFile(decl.Key.File):
		return "not Go"
	case decl.Key.Kind == entities.KindFunc && (decl.Key.Name == "main" || decl.Key.Name == "init"):
		return "root"
	case !syntax.mains[decl.Unit] && ast.IsExported(decl.Key.Name):
		return "exported"
	}

	return ""
}
