package golang

import (
	"go/ast"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

// builtTypes reports the local types a declaration HANDS somewhere: a keyed
// struct field's value, or a call argument - the positions an interface is
// expected in, whose declared type lives elsewhere. Only those two: counting
// every construction would bind most types, and "x := T{}" converts nothing.
func builtTypes(
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) []string {
	node := declarationOf(decl, snap, sources)
	if node == nil {
		return nil
	}

	seen := map[string]bool{}

	var out []string

	collect := func(expr ast.Expr) {
		if name, built := declsource.ConcreteTypeIn(expr); built && !seen[name] {
			seen[name] = true

			out = append(out, name)
		}
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			collect(node.Value)
		case *ast.CallExpr:
			for _, arg := range node.Args {
				collect(arg)
			}
		}

		return true
	})

	return out
}

// declarationOf parses a declaration out of its own span, once per file read.
func declarationOf(
	decl entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) ast.Decl {
	source, cached := sources[decl.Key.File]
	if !cached {
		read, err := declsource.ReadSource(snap, decl.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[decl.Key.File] = source
	}

	text := declsource.SliceSpan(source, decl.Span)
	if text == "" {
		return nil
	}

	return declsource.ParseDeclaration(text)
}
