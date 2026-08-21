package ordering

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// findCycles runs Tarjan's algorithm over graph's edges and names every
// strongly connected component of size greater than one as a Cycle, carrying
// the distinct reasons that formed it. Naming the reason is the point: every
// ordering defect the prior work produced was an edge the graph already held
// and nothing represented, so a cycle has to be explainable by what made it.
func findCycles(graph entities.Graph) []entities.Cycle {
	decls := graph.Decls()
	slices.SortFunc(decls, func(a, b entities.Decl) int { return compareKeys(a.Key, b.Key) })

	finder := &tarjan{
		graph:   graph,
		index:   map[entities.DeclKey]int{},
		lowlink: map[entities.DeclKey]int{},
		onStack: map[entities.DeclKey]bool{},
	}

	for _, decl := range decls {
		if _, seen := finder.index[decl.Key]; !seen {
			finder.strongConnect(decl.Key)
		}
	}

	cycles := make([]entities.Cycle, 0, len(finder.cycles))
	for _, members := range finder.cycles {
		cycles = append(cycles, buildCycle(graph, members))
	}

	slices.SortFunc(
		cycles,
		func(a, b entities.Cycle) int { return compareKeys(a.Members[0], b.Members[0]) },
	)

	return cycles
}

// tarjan is Tarjan's strongly-connected-components algorithm, scoped to one
// findCycles call.
type tarjan struct {
	graph   entities.Graph
	index   map[entities.DeclKey]int
	lowlink map[entities.DeclKey]int
	onStack map[entities.DeclKey]bool
	stack   []entities.DeclKey
	counter int
	cycles  [][]entities.DeclKey
}

func (t *tarjan) strongConnect(v entities.DeclKey) {
	t.index[v] = t.counter
	t.lowlink[v] = t.counter
	t.counter++
	t.stack = append(t.stack, v)
	t.onStack[v] = true

	for _, edge := range t.graph.Needs(v) {
		w := edge.Provider

		if _, seen := t.index[w]; !seen {
			t.strongConnect(w)
			t.lowlink[v] = min(t.lowlink[v], t.lowlink[w])
		} else if t.onStack[w] {
			t.lowlink[v] = min(t.lowlink[v], t.index[w])
		}
	}

	if t.lowlink[v] != t.index[v] {
		return
	}

	t.popComponent(v)
}

// popComponent drains the stack down to and including v, which closes the
// strongly connected component v roots.
func (t *tarjan) popComponent(v entities.DeclKey) {
	var members []entities.DeclKey

	for {
		top := len(t.stack) - 1
		w := t.stack[top]
		t.stack = t.stack[:top]
		t.onStack[w] = false

		members = append(members, w)

		if w == v {
			break
		}
	}

	if len(members) > 1 {
		t.cycles = append(t.cycles, members)
	}
}

// buildCycle names the members and the distinct edge reasons that connect
// them, both sorted so the result is reproducible.
func buildCycle(graph entities.Graph, members []entities.DeclKey) entities.Cycle {
	inCycle := make(map[entities.DeclKey]bool, len(members))
	for _, key := range members {
		inCycle[key] = true
	}

	reasonSet := map[entities.EdgeReason]bool{}

	for _, key := range members {
		for _, edge := range graph.Needs(key) {
			if inCycle[edge.Provider] {
				reasonSet[edge.Reason] = true
			}
		}
	}

	sortedMembers := slices.Clone(members)
	slices.SortFunc(sortedMembers, compareKeys)

	reasons := make([]entities.EdgeReason, 0, len(reasonSet))
	for reason := range reasonSet {
		reasons = append(reasons, reason)
	}

	slices.Sort(reasons)

	return entities.Cycle{Members: sortedMembers, Reasons: reasons}
}
