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
) ([]entities.Edge, error) {
	var (
		guard sync.Mutex
		edges []entities.Edge
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(i.limit)

	for _, decl := range decls {
		group.Go(func() error {
			found, err := i.edgesOf(groupCtx, decl, where)
			if err != nil {
				return err
			}

			guard.Lock()
			defer guard.Unlock()

			edges = append(edges, found...)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("resolve edges: %w", err)
	}

	slices.SortFunc(edges, compareEdges)

	return slices.CompactFunc(edges, func(a, b entities.Edge) bool {
		return compareEdges(a, b) == 0
	}), nil
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

func (i Indexer) edgesOf(
	ctx context.Context,
	decl entities.Decl,
	where locator,
) ([]entities.Edge, error) {
	readers, err := i.resolver.References(ctx, decl.Selector)
	if err != nil {
		return nil, fmt.Errorf("find references to %s: %w", decl.Key, err)
	}

	edges := i.edgesFrom(readers, decl.Key, entities.ReasonReference, where)

	// Structural satisfaction is invisible to an import graph: an
	// implementation package need not import the package whose interface it
	// satisfies, and the injector that resolves by method set is the reason
	// that arrangement exists at all.
	implementors, err := i.resolver.Implementations(ctx, decl.Selector)
	if err != nil {
		return nil, fmt.Errorf("find implementations of %s: %w", decl.Key, err)
	}

	return append(
		edges,
		i.edgesFrom(implementors, decl.Key, entities.ReasonImplements, where)...,
	), nil
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
