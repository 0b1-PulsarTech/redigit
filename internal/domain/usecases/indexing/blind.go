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

// Blind is one question the resolver could not answer. It is data, not a log
// line: a graph missing a tenth of its edges looks exactly like a graph whose
// code has a tenth fewer edges, and only a count kept alongside the graph
// tells them apart.
type Blind struct {
	Decl   entities.DeclKey
	Query  string
	Reason string
}

// Result is the graph plus what building it could not see.
type Result struct {
	Graph   entities.Graph
	Blind   []Blind
	Partial []Partial
}

// BlindRatio is how much of the graph rests on an unanswered question.
func (r Result) BlindRatio() float64 {
	if r.Graph.Len() == 0 {
		return 0
	}

	return float64(len(r.Blind)) / float64(r.Graph.Len())
}

type (
	Partial struct {
		Broken       int
		Of           int
		Tags         []string
		FirstPackage string
		FirstError   string
		SameMessage  int
	}

	Shortfall interface {
		Partial() []Partial
	}
)

func (indexer Indexer) partials() []Partial {
	// Asked last: the rules load their type facts lazily while the graph is built.
	var partial []Partial

	for _, lang := range indexer.langs {
		if shortfall, isShortfall := lang.Rules.(Shortfall); isShortfall {
			partial = append(partial, shortfall.Partial()...)
		}
	}

	return partial
}

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

	if err := cmp.Or(group.Wait(), ctx.Err()); err != nil {
		return nil, nil, fmt.Errorf("resolve edges: %w", err)
	}

	slices.SortFunc(edges, compareEdges)
	slices.SortFunc(blind, compareBlind)

	return slices.Compact(edges), blind, nil
}

func compareBlind(left, right Blind) int {
	return cmp.Or(
		entities.CompareKeys(left.Decl, right.Decl),
		cmp.Compare(left.Query, right.Query),
		cmp.Compare(left.Reason, right.Reason),
	)
}

func compareEdges(a, b entities.Edge) int {
	return cmp.Or(
		entities.CompareKeys(a.Dependent, b.Dependent),
		entities.CompareKeys(a.Provider, b.Provider),
		cmp.Compare(a.Reason, b.Reason),
	)
}
