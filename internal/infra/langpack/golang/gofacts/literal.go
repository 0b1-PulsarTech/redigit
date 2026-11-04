package gofacts

import (
	"go/ast"
	"go/token"
	"slices"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

type litScope struct {
	start, end token.Pos
	call       facts.Call
}

func (scan walker) within(at token.Pos) *facts.Call {
	scopes := *scan.lits
	for len(scopes) > 0 && scopes[len(scopes)-1].end <= at {
		scopes = scopes[:len(scopes)-1]
	}

	*scan.lits = scopes

	// Scopes below the top end no earlier, so the first one that has begun is the nearest.
	for idx := range slices.Backward(scopes) {
		if scopes[idx].start <= at {
			enclosing := scopes[idx].call

			return &enclosing
		}
	}

	return nil
}

func (scan walker) hold(call facts.Call, args []ast.Expr) {
	// The scope keeps the call without its own Within, so a chain of literals stays one link deep.
	call.Within = nil

	// The first literal goes on top: it is the first to end, and the later ones wait below it.
	for _, arg := range slices.Backward(args) {
		if lit, isLit := arg.(*ast.FuncLit); isLit {
			*scan.lits = append(*scan.lits, litScope{start: lit.Pos(), end: lit.End(), call: call})
		}
	}
}
