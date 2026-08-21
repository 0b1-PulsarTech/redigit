package entities

// Graph is every declaration the run knows about and every reason one waits for
// another. It is built once and read many times, so the indexes are computed at
// construction instead of on each query.
type Graph struct {
	decls    map[DeclKey]Decl
	edges    []Edge
	anchors  []Anchor
	needs    map[DeclKey][]Edge
	provides map[DeclKey][]Edge
}

func NewGraph(decls []Decl, edges []Edge, anchors []Anchor) Graph {
	graph := Graph{
		decls:    make(map[DeclKey]Decl, len(decls)),
		edges:    edges,
		anchors:  anchors,
		needs:    make(map[DeclKey][]Edge, len(decls)),
		provides: make(map[DeclKey][]Edge, len(decls)),
	}

	for _, decl := range decls {
		graph.decls[decl.Key] = decl
	}

	for _, edge := range edges {
		graph.needs[edge.Dependent] = append(graph.needs[edge.Dependent], edge)
		graph.provides[edge.Provider] = append(graph.provides[edge.Provider], edge)
	}

	return graph
}

func (g Graph) Decl(key DeclKey) (Decl, bool) {
	decl, found := g.decls[key]

	return decl, found
}

func (g Graph) Decls() []Decl {
	out := make([]Decl, 0, len(g.decls))
	for _, decl := range g.decls {
		out = append(out, decl)
	}

	return out
}

func (g Graph) Len() int { return len(g.decls) }

// Needs lists what this declaration cannot be born before.
func (g Graph) Needs(key DeclKey) []Edge { return g.needs[key] }

// Consumers lists the declarations waiting on this one. The package-level rule
// that a unit is born when someone OUTSIDE it needs it reads this and filters by
// unit; counting a unit's own declarations as consumers pinned every package to
// whatever instant the source history happened to create it.
func (g Graph) Consumers(key DeclKey) []Edge { return g.provides[key] }

// ExternalConsumers drops the consumers that live in the same unit as the
// provider.
func (g Graph) ExternalConsumers(key DeclKey) []Edge {
	provider, found := g.decls[key]
	if !found {
		return nil
	}

	var out []Edge

	for _, edge := range g.provides[key] {
		dependent, known := g.decls[edge.Dependent]
		if known && dependent.Unit != provider.Unit {
			out = append(out, edge)
		}
	}

	return out
}

func (g Graph) Anchors() []Anchor { return g.anchors }

func (g Graph) Edges() []Edge { return g.edges }
