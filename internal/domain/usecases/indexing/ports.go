package indexing

import (
	"context"
	"errors"
	"iter"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var (
	ErrBlobMissing = errors.New("blob missing")
	ErrDeadGlob    = errors.New("slice.functions glob matches no function that can be cut")
)

type RevisionLog interface {
	Changes(ctx context.Context) iter.Seq2[RevisionChange, error]
	ReadBlob(ctx context.Context, blob string) ([]byte, error)
}

type RevisionChange struct {
	Rev   string
	Files []ChangedFile
}

type ChangedFile struct {
	Path      string
	Blob      string
	IsDeleted bool
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

// Slicer is the optional part of a Syntax that cuts a function into the
// statements it is made of. The functions that assemble a program (the route
// table, the injector, the bootstrap steps) are written one registration at a
// time, each with the feature it registers, so a function born whole waits for
// the last feature and drags the program's entry to the end of the history.
type Slicer interface {
	// Slice splits the function fn declares into its signature and its
	// statements. It reports false for a function whose body cannot be cut and
	// still compile: one with labels, or an early return between statements.
	Slice(ctx context.Context, fn entities.Decl, source []byte) (Sliced, bool, error)
}

type Selector interface {
	Functions() []string
}

// Sliced is a function cut into pieces. Head keeps the key of the function and
// spans its signature; each member is one statement, in source order, and the
// edges say which statements a member needs from the body itself.
type Sliced struct {
	Head    entities.Decl
	Members []entities.Decl
	Edges   []entities.Edge
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

// Atomizer is the optional part of a RulePack that groups declarations which
// must be born in one commit: each needs the other to compile, so an edge in
// either direction would be a cycle. A declaration that already belongs to an
// atom brings the whole atom into the rule's group.
type Atomizer interface {
	Atoms(
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
	) (map[entities.DeclKey]string, error)
}

// EdgeFilter is the optional part of a RulePack that removes edges from the
// merged set, the reference graph's and every rule's alike. It is for edges a
// language knows to be noise whichever side produced them.
type EdgeFilter interface {
	FilterEdges(snap entities.Snapshot, edges []entities.Edge) []entities.Edge
}

// Tree reads the final content of a path. Every intermediate version is rendered
// from the final text, never from an intermediate blob, which is what lets a
// file grow declaration by declaration.
type Tree interface {
	Read(ctx context.Context, path string) ([]byte, error)
}
