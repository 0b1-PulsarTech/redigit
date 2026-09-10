package indexing

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

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

// resolvedFile reports whether a language server answers for a path. A path no
// language owns is not asked about either.
func (i Indexer) resolvedFile(file string) bool {
	lang, owned := i.languageFor(file)

	return owned && lang.Resolved
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

	ruleEdges, anchors, err := i.applyRules(ctx, snap, decls)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Graph: entities.NewGraph(decls, append(edges, ruleEdges...), anchors),
		Blind: blind,
	}

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

// read parses every owned file into declarations. Files are independent, so
// they are read concurrently; the locator is filled under a lock because a map
// is the one thing here that is not.
func (i Indexer) read(
	ctx context.Context,
	snap entities.Snapshot,
) ([]entities.Decl, locator, error) {
	var (
		guard sync.Mutex
		decls []entities.Decl
		where = newLocator()
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(i.limit)

	for _, file := range snap.Files {
		lang, owned := i.languageFor(file)
		if !owned {
			continue
		}

		group.Go(func() error {
			source, err := i.tree.Read(groupCtx, file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}

			found, err := lang.Syntax.Declarations(groupCtx, file, source)
			if err != nil {
				return fmt.Errorf("read declarations of %s: %w", file, err)
			}

			guard.Lock()
			defer guard.Unlock()

			decls = append(decls, found...)
			where.add(file, source, found)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, locator{}, fmt.Errorf("read declarations: %w", err)
	}

	slices.SortFunc(decls, func(a, b entities.Decl) int {
		return compareKeys(a.Key, b.Key)
	})

	return decls, where, nil
}

func (i Indexer) languageFor(file string) (Language, bool) {
	for _, lang := range i.langs {
		if lang.Owns(file) {
			return lang, true
		}
	}

	return Language{}, false
}

func (i Indexer) applyRules(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, []entities.Anchor, error) {
	var (
		edges   []entities.Edge
		anchors []entities.Anchor
	)

	for _, lang := range i.langs {
		if lang.Rules == nil {
			continue
		}

		found, err := lang.Rules.Edges(ctx, snap, decls)
		if err != nil {
			return nil, nil, fmt.Errorf("apply %s rules: %w", lang.Name, err)
		}

		pulled, err := lang.Rules.Anchors(ctx, snap, decls)
		if err != nil {
			return nil, nil, fmt.Errorf("apply %s anchors: %w", lang.Name, err)
		}

		edges = append(edges, found...)
		anchors = append(anchors, pulled...)
	}

	return edges, anchors, nil
}

// resolvable keeps the declarations a language server can answer about.
func resolvable(indexer Indexer, decls []entities.Decl) []entities.Decl {
	out := make([]entities.Decl, 0, len(decls))

	for _, decl := range decls {
		if indexer.resolvedFile(decl.Key.File) {
			out = append(out, decl)
		}
	}

	return out
}
