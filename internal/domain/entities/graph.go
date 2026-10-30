package entities

import (
	"cmp"
	"maps"
	"slices"
)

// Graph is every declaration and why one waits for another; read many times, so indexed once.
type Graph struct {
	decls    map[DeclKey]Decl
	list     []Decl
	edges    []Edge
	anchors  []Anchor
	needs    map[DeclKey][]Edge
	provides map[DeclKey][]Edge
}

func compareDecls(left, right Decl) int {
	// Fields beyond the key break ties, so the declaration kept ignores arrival order.
	return cmp.Or(
		CompareKeys(left.Key, right.Key),
		cmp.Compare(left.Span.Start, right.Span.Start),
		cmp.Compare(left.Span.End, right.Span.End),
		cmp.Compare(left.Selector.File, right.Selector.File),
		cmp.Compare(left.Selector.Line, right.Selector.Line),
		cmp.Compare(left.Selector.Char, right.Selector.Char),
		cmp.Compare(left.Unit, right.Unit),
		cmp.Compare(left.Atom, right.Atom),
		cmp.Compare(left.GroupSize, right.GroupSize),
		cmp.Compare(left.Lines, right.Lines),
		cmp.Compare(left.Frame, right.Frame),
		cmp.Compare(flagBits(left), flagBits(right)),
	)
}

func orderEdges(list []Decl, edges []Edge) ([]Edge, []Edge) {
	// Sorting integers, not key strings: a key no declaration owns is ranked in among them.
	const rankBits = 32

	type edgeSlot struct {
		packed uint64
		index  int
	}

	ranks := make(map[DeclKey]int, len(list))
	for rank, decl := range list {
		ranks[decl.Key] = rank + 1
	}

	for _, edge := range edges {
		for _, key := range [...]DeclKey{edge.Dependent, edge.Provider} {
			if ranks[key] == 0 {
				ranks[key] = -1
			}
		}
	}

	if len(ranks) > len(list) {
		for rank, key := range slices.SortedFunc(maps.Keys(ranks), CompareKeys) {
			ranks[key] = rank + 1
		}
	}

	slots := make([]edgeSlot, len(edges))

	for index, edge := range edges {
		//nolint:gosec // Every rank is positive by now.
		packed := uint64(ranks[edge.Dependent])<<rankBits | uint64(ranks[edge.Provider])
		slots[index] = edgeSlot{packed, index}
	}

	slices.SortFunc(slots, func(left, right edgeSlot) int {
		return cmp.Or(
			cmp.Compare(left.packed, right.packed),
			cmp.Compare(edges[left.index].Reason, edges[right.index].Reason),
		)
	})

	next := make([]int, len(ranks)+1)
	for _, slot := range slots {
		next[slot.packed&(1<<rankBits-1)]++
	}

	for rank := 1; rank <= len(ranks); rank++ {
		next[rank] += next[rank-1]
	}

	dependents, providers := make([]Edge, len(edges)), make([]Edge, len(edges))

	for index, slot := range slots {
		rank := int(slot.packed & (1<<rankBits - 1))
		dependents[index] = edges[slot.index]
		providers[next[rank-1]] = edges[slot.index]
		next[rank-1]++
	}

	return dependents, providers
}

func groupRuns(edges []Edge, keyOf func(Edge) DeclKey) map[DeclKey][]Edge {
	// The cut capacity stops an append to one run reaching the next.
	out, start := map[DeclKey][]Edge{}, 0

	for index := 1; index <= len(edges); index++ {
		if index == len(edges) || keyOf(edges[index]) != keyOf(edges[start]) {
			out[keyOf(edges[start])] = edges[start:index:index]
			start = index
		}
	}

	return out
}

func NewGraph(decls []Decl, edges []Edge, anchors []Anchor) Graph {
	list := slices.Clone(decls)
	slices.SortFunc(list, compareDecls)
	list = slices.CompactFunc(list, func(left, right Decl) bool { return left.Key == right.Key })

	dependents, providers := orderEdges(list, edges)
	graph := Graph{
		decls:    make(map[DeclKey]Decl, len(list)),
		list:     list,
		edges:    dependents,
		anchors:  slices.Clone(anchors),
		needs:    groupRuns(dependents, func(edge Edge) DeclKey { return edge.Dependent }),
		provides: groupRuns(providers, func(edge Edge) DeclKey { return edge.Provider }),
	}

	slices.SortFunc(graph.anchors, func(left, right Anchor) int {
		return cmp.Or(
			CompareKeys(left.Decl, right.Decl),
			CompareKeys(left.To, right.To),
			cmp.Compare(left.Reason, right.Reason),
		)
	})

	for _, decl := range list {
		graph.decls[decl.Key] = decl
	}

	return graph
}

func (g Graph) Decl(key DeclKey) (Decl, bool) {
	decl, found := g.decls[key]

	return decl, found
}

func (g Graph) Decls() []Decl { return slices.Clone(g.list) }

func (g Graph) Len() int { return len(g.list) }

// Needs lists what this declaration cannot be born before.
func (g Graph) Needs(key DeclKey) []Edge { return g.needs[key] }

// Consumers lists every declaration waiting on this one, its own unit's included.
func (g Graph) Consumers(key DeclKey) []Edge { return g.provides[key] }

func (g Graph) ExternalConsumers(key DeclKey) (out []Edge) {
	// A unit is born when someone outside it needs it; counting its own declarations
	// as consumers pinned every package to the instant the source history created it.
	provider, found := g.decls[key]
	if !found {
		return nil
	}

	for _, edge := range g.provides[key] {
		if dependent, known := g.decls[edge.Dependent]; known && dependent.Unit != provider.Unit {
			out = append(out, edge)
		}
	}

	return out
}

func (g Graph) Anchors() []Anchor { return g.anchors }

func (g Graph) Edges() []Edge { return g.edges }
