package indexing

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=indexing

// Resolver answers the semantic questions exactly. Text matching is what this
// replaces: searching for the first consumer of 59 declarations by identifier
// put 24 of 53 destinations in the wrong place, including a standard-library
// symbol matching a same-named one no commit ever calls.
//
// Two adapters implement it: a language server over LSP, and a precomputed
// index. Both are language-agnostic; neither knows what Go is.
type Resolver interface {
	References(ctx context.Context, at entities.Location) ([]entities.Location, error)
	Implementations(ctx context.Context, at entities.Location) ([]entities.Location, error)
	Definition(ctx context.Context, at entities.Location) (entities.Location, error)
}

// Syntax owns what no index reports reliably: where a declaration starts and
// ends, and how to write a file back holding only some of them. It is per
// language because the hazards are: filtering declarations without moving their
// comments leaves a doc block describing code that is not there, and swallows
// the build-constraint line above the first one removed.
type Syntax interface {
	Declarations(ctx context.Context, path string, source []byte) ([]entities.Decl, error)
	Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error)
}

// RulePack contributes the edges no reference graph can see. Generated code
// never names its generator; a structural double records what it mirrors only in
// a header comment; a migration sits between the schema a generator read and the
// queries that read it; and a file behind a build constraint is invisible to
// every index query unless the tag is passed.
type RulePack interface {
	Name() string
	Edges(
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
	) ([]entities.Edge, error)
	Anchors(
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
	) ([]entities.Anchor, error)
}

// Tree reads the final content of a path. Every intermediate version is rendered
// from the final text, never from an intermediate blob, which is what lets a
// file grow declaration by declaration.
type Tree interface {
	Read(ctx context.Context, path string) ([]byte, error)
}
