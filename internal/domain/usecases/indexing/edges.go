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

	interfaces := make(map[entities.DeclKey]bool, len(decls))

	for _, decl := range decls {
		if decl.IsInterface {
			interfaces[decl.Key] = true
		}
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(i.limit)

	for _, decl := range decls {
		group.Go(func() error {
			found, missed := i.edgesOf(groupCtx, decl, where, interfaces)

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
	interfaces map[entities.DeclKey]bool,
) ([]entities.Edge, []Blind) {
	var blind []Blind

	readers, err := i.resolver.References(ctx, decl.Selector)
	if err != nil {
		blind = append(blind, Blind{Decl: decl.Key, Query: "references", Reason: err.Error()})
	}

	readers = i.directCallsOnly(ctx, decl, readers)

	edges := i.edgesFrom(readers, decl.Key, entities.ReasonReference, where)

	// Structural satisfaction is invisible to an import graph: an
	// implementation package need not import the package whose interface it
	// satisfies, and the injector that resolves by method set is the reason
	// that arrangement exists at all.
	//
	// The question is asked ONLY of an interface. A server answers it in both
	// directions - implementations of an interface, and interfaces a concrete
	// type satisfies - and keeping the answer either way inverts half the
	// edges: the interface would come out depending on whatever implements it.
	if !decl.IsInterface {
		return edges, blind
	}

	implementors, err := i.resolver.Implementations(ctx, decl.Selector)
	if err != nil {
		blind = append(blind, Blind{Decl: decl.Key, Query: "implementations", Reason: err.Error()})

		return edges, blind
	}

	satisfying := i.edgesFrom(implementors, decl.Key, entities.ReasonImplements, where)

	return append(edges, dropInterfaceToInterface(satisfying, interfaces)...), blind
}

// dropInterfaceToInterface removes an edge whose dependent is itself an
// interface. One interface satisfies another whenever their method sets match,
// and a codebase that declares its ports at the consumer ends up with the same
// small interface written in a dozen packages. Keeping those edges makes every
// copy appear to depend on every other and fabricates a cycle out of types that
// never reference each other at all. Real embedding is a different thing and is
// already carried by a reference edge, because the embedding interface names
// the embedded one.
func dropInterfaceToInterface(
	edges []entities.Edge,
	interfaces map[entities.DeclKey]bool,
) []entities.Edge {
	kept := edges[:0:0]

	for _, edge := range edges {
		if interfaces[edge.Dependent] {
			continue
		}

		kept = append(kept, edge)
	}

	return kept
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

// directCallsOnly drops the call sites that reach a method through an
// interface rather than through this concrete type.
//
// A server asked who references a concrete method answers with every call
// written against an interface the method satisfies, because any of the
// implementations could be the one that runs. For ordering that is wrong in
// both directions: it inflates how many things read the method, and it makes
// sibling implementations appear to reference each other, which fabricates a
// cycle out of types that never mention one another. The seven Error methods
// of one package all "referenced" each other for exactly this reason, through
// a single `e.Err.Error()` written against the error interface.
//
// The test is not a name heuristic: ask what the call site actually resolves
// to. Resolving back to this declaration means the call names it; resolving
// anywhere else means the call names an interface, and the edge belongs to
// that interface, not here.
func (i Indexer) directCallsOnly(
	ctx context.Context,
	decl entities.Decl,
	readers []entities.Location,
) []entities.Location {
	// Only a method can be reached through an interface. A function, a type
	// or a value is named directly or not at all.
	if decl.Key.Kind != entities.KindMethod || len(readers) == 0 {
		return readers
	}

	kept := readers[:0:0]

	for _, at := range readers {
		target, err := i.resolver.Definition(ctx, at)
		if err != nil {
			// An unanswerable definition is not evidence of dispatch, so the
			// call site is kept. Losing a real edge is worse than keeping a
			// doubtful one here, because the doubtful one only delays.
			kept = append(kept, at)

			continue
		}

		if target.File == decl.Selector.File && target.Line == decl.Selector.Line {
			kept = append(kept, at)
		}
	}

	return kept
}
