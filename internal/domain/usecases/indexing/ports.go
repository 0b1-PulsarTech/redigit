package indexing

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// RevisionTree reads a path's content at one revision of the source history.
// It is separate from Tree because the two answer different questions: Tree
// reads the FINAL text every intermediate version is rendered from, while this
// reads what a commit actually held, which is how a declaration's real birth
// is found.
type RevisionTree interface {
	ReadAt(ctx context.Context, rev, path string) ([]byte, error)
}

// Chunker splits a file that is not source code into the pieces that can be
// born apart. A query file holding twenty statements is a Go file holding
// twenty functions: if the first commit reads three, three is what should be
// born, and the file-shaped alternative drags seventeen nobody reads yet.
//
// Handles reports whether this chunker claims a path. A path no chunker claims
// still becomes one whole-file piece, because a file with no reader for its
// format is still a file the final tree needs.
type Chunker interface {
	Name() string
	Handles(path string) bool
	Chunks(ctx context.Context, path string, source []byte) ([]entities.Decl, error)
}

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
