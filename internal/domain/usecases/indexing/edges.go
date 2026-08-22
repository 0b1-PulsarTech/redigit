package indexing

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func compareKeys(a, b entities.DeclKey) int { return cmp.Compare(a.String(), b.String()) }

// resolve asks the resolver, per declaration, who reads it and what satisfies
// it. This is the step that replaces identifier matching: searching for the
// first consumer of 59 declarations by name put 24 of 53 destinations in the
// wrong place, on collisions, mentions inside comments, and a standard-library
// symbol matching a same-named one nothing ever calls.
func (i Indexer) resolve(
	ctx context.Context,
	decls []entities.Decl,
	where locator,
) ([]entities.Edge, []Blind, error) {
	var (
		guard sync.Mutex
		edges []entities.Edge
		blind []Blind
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(i.limit)

	for _, decl := range decls {
		group.Go(func() error {
			found, missed := i.edgesOf(groupCtx, decl, where)

			guard.Lock()
			defer guard.Unlock()

			edges = append(edges, found...)
			blind = append(blind, missed...)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, nil, fmt.Errorf("resolve edges: %w", err)
	}

	slices.SortFunc(edges, compareEdges)
	slices.SortFunc(blind, func(a, b Blind) int { return compareKeys(a.Decl, b.Decl) })

	return slices.CompactFunc(edges, func(a, b entities.Edge) bool {
		return compareEdges(a, b) == 0
	}), blind, nil
}

func compareEdges(a, b entities.Edge) int {
	if by := compareKeys(a.Dependent, b.Dependent); by != 0 {
		return by
	}

	if by := compareKeys(a.Provider, b.Provider); by != 0 {
		return by
	}

	return cmp.Compare(string(a.Reason), string(b.Reason))
}

// edgesOf asks the resolver about one declaration. A query the server cannot
// answer is recorded as a blind spot rather than ending the run: a language
// server refuses individual symbols for reasons of its own (a test-variant
// package, a file it never loaded), and letting one refusal discard the other
// ten thousand answers is worse than saying which ten it could not give.
// Silence would be worse still, which is why the misses are returned.
func (i Indexer) edgesOf(
	ctx context.Context,
	decl entities.Decl,
	where locator,
) ([]entities.Edge, []Blind) {
	var blind []Blind

	readers, err := i.resolver.References(ctx, decl.Selector)
	if err != nil {
		blind = append(blind, Blind{Decl: decl.Key, Query: "references", Reason: err.Error()})
	}

	edges := i.edgesFrom(readers, decl.Key, entities.ReasonReference, where)

	// Structural satisfaction is invisible to an import graph: an
	// implementation package need not import the package whose interface it
	// satisfies, and the injector that resolves by method set is the reason
	// that arrangement exists at all.
	//
	// Only a type or a method can have one, and a server asked about a value
	// answers with an error rather than an empty list, so the question is not
	// asked where it has no meaning.
	if !canBeImplemented(decl.Key.Kind) {
		return edges, blind
	}

	implementors, err := i.resolver.Implementations(ctx, decl.Selector)
	if err != nil {
		blind = append(blind, Blind{Decl: decl.Key, Query: "implementations", Reason: err.Error()})

		return edges, blind
	}

	return append(
		edges,
		i.edgesFrom(implementors, decl.Key, entities.ReasonImplements, where)...,
	), blind
}

func canBeImplemented(kind entities.DeclKind) bool {
	return kind == entities.KindType || kind == entities.KindMethod
}

// edgesFrom turns resolver locations into edges pointing at provider. A
// location inside the provider itself is dropped: a declaration does not wait
// for itself, and counting a unit's own mentions is what pinned every package
// to the instant its source history happened to create it.
func (i Indexer) edgesFrom(
	found []entities.Location,
	provider entities.DeclKey,
	reason entities.EdgeReason,
	where locator,
) []entities.Edge {
	edges := make([]entities.Edge, 0, len(found))

	for _, at := range found {
		dependent, enclosed := where.Enclosing(at)
		if !enclosed || dependent == provider {
			continue
		}

		edges = append(edges, entities.Edge{
			Dependent: dependent,
			Provider:  provider,
			Reason:    reason,
		})
	}

	return edges
}
