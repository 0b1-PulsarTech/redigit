package indexing

import (
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Language binds a syntax reader to the rules that language needs and to the
// file extensions it owns. Rules may be nil for a language that has nothing a
// reference graph cannot already see.
type Language struct {
	Name       string
	Extensions []string
	Syntax     Syntax
	Rules      RulePack

	// Resolved says a language server answers reference queries for these
	// files. It is false for the files a build needs and no server reads: a
	// query about a go.mod is a round trip that can only come back empty, and
	// counting those empties as blind spots buries the ones that are real.
	Resolved bool
}

// AnyExtension is the sentinel Extensions entry a catch-all language pack
// uses to claim every path no earlier-registered language already owns - the
// same last-refusal role chunk.WholeFile plays one level down, inside a
// single language's own chunker list.
const AnyExtension = "*"

// Owns reports whether file's extension is one this language reads. The
// composition root reuses it to name a final-tree path no configured
// language claims at all (see bootstrap.unclaimed), the same check the
// indexer's own read loop uses to decide what to parse.
func (l Language) Owns(file string) bool {
	return slices.Contains(l.Extensions, AnyExtension) ||
		slices.Contains(l.Extensions, path.Ext(file))
}

// Config tunes the index run.
type Config struct {
	Languages []Language
	// Concurrency bounds in-flight resolver requests. A language server is one
	// process answering everyone, so an unbounded fan-out queues rather than
	// parallelises.
	Concurrency int
}

const defaultConcurrency = 8

// Indexer turns a snapshot into a declaration graph: the declarations each
// file holds, the edges a resolver can prove, and the edges only a language's
// own rules can see.
type Indexer struct {
	resolver Resolver
	tree     Tree
	langs    []Language
	limit    int
}

func New(resolver Resolver, tree Tree, cfg Config) Indexer {
	limit := cfg.Concurrency
	if limit < 1 {
		limit = defaultConcurrency
	}

	return Indexer{resolver: resolver, tree: tree, langs: cfg.Languages, limit: limit}
}

// Index reads every file the configured languages own and returns the graph.
// maxBlindRatio is how much of the graph may rest on an unanswered question
// before the result is refused. Tolerating a few refusals keeps a run useful;
// tolerating most of them hands back a graph that looks like code with almost
// no edges, which is the shape of a confident wrong answer.
const (
	maxBlindRatio = 0.10
	asPercent     = 100
)

func (i Indexer) Index(ctx context.Context, snap entities.Snapshot) (Result, error) {
	if len(i.langs) == 0 {
		return Result{}, fmt.Errorf("%w: no language configured", apperr.ErrIncomplete)
	}

	decls, where, err := i.read(ctx, snap)
	if err != nil {
		return Result{}, err
	}

	if len(decls) == 0 {
		return Result{}, fmt.Errorf(
			"%w: no declaration found in %d files", apperr.ErrIncomplete, len(snap.Files),
		)
	}

	edges, blind, err := i.resolve(ctx, resolvable(i, decls), where)
	if err != nil {
		return Result{}, err
	}

	graph, err := i.assemble(ctx, snap, decls, edges)
	if err != nil {
		return Result{}, err
	}

	cut, err := i.cutWiring(ctx, graph, decls, where)
	if err != nil {
		return Result{}, err
	}

	if len(cut.replaced) > 0 {
		graph, err = i.assembleCut(ctx, snap, graph, edges, cut, where)
		if err != nil {
			return Result{}, err
		}
	}

	result := Result{Graph: graph, Blind: blind, Partial: i.partials()}

	if ratio := result.BlindRatio(); ratio > maxBlindRatio {
		return result, fmt.Errorf(
			"%w: the resolver could not answer for %d of %d declarations (%.0f%%), starting at %s: %s",
			apperr.ErrUnresolved,
			len(blind),
			result.Graph.Len(),
			ratio*asPercent,
			blind[0].Decl,
			blind[0].Reason,
		)
	}

	return result, nil
}
