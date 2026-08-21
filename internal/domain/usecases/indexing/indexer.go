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
}

func (l Language) owns(file string) bool {
	return slices.Contains(l.Extensions, path.Ext(file))
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
func (i Indexer) Index(ctx context.Context, snap entities.Snapshot) (entities.Graph, error) {
	if len(i.langs) == 0 {
		return entities.Graph{}, fmt.Errorf("%w: no language configured", apperr.ErrIncomplete)
	}

	decls, where, err := i.read(ctx, snap)
	if err != nil {
		return entities.Graph{}, err
	}

	if len(decls) == 0 {
		return entities.Graph{}, fmt.Errorf(
			"%w: no declaration found in %d files", apperr.ErrIncomplete, len(snap.Files),
		)
	}

	edges, err := i.resolve(ctx, decls, where)
	if err != nil {
		return entities.Graph{}, err
	}

	ruleEdges, anchors, err := i.applyRules(ctx, snap, decls)
	if err != nil {
		return entities.Graph{}, err
	}

	return entities.NewGraph(decls, append(edges, ruleEdges...), anchors), nil
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
		if lang.owns(file) {
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
