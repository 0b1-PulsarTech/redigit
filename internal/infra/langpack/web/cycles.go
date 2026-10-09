package web

import (
	"maps"
	"slices"
)

const minCycle = 2

// cycleAtoms names, for every file in an import cycle, the atom it shares with
// the rest of the cycle: TypeScript allows two type modules to import each
// other, and neither compiles in a commit the other is missing from.
func cycleAtoms(imports map[string][]string) map[string]string {
	walk := tarjan{
		imports: imports,
		index:   map[string]int{},
		low:     map[string]int{},
		onStack: map[string]bool{},
	}

	for _, file := range slices.Sorted(maps.Keys(imports)) {
		if _, seen := walk.index[file]; !seen {
			walk.visit(file)
		}
	}

	out := map[string]string{}

	for _, component := range walk.components {
		if len(component) < minCycle {
			continue
		}

		name := slices.Min(component)
		for _, file := range component {
			out[file] = name
		}
	}

	return out
}

// tarjan finds the strongly connected components of the import graph.
type tarjan struct {
	imports    map[string][]string
	index      map[string]int
	low        map[string]int
	onStack    map[string]bool
	stack      []string
	components [][]string
}

func (t *tarjan) visit(file string) {
	t.index[file], t.low[file] = len(t.index), len(t.index)
	t.stack = append(t.stack, file)
	t.onStack[file] = true

	for _, target := range t.imports[file] {
		if _, seen := t.index[target]; !seen {
			t.visit(target)
			t.low[file] = min(t.low[file], t.low[target])
		} else if t.onStack[target] {
			t.low[file] = min(t.low[file], t.index[target])
		}
	}

	if t.low[file] != t.index[file] {
		return
	}

	var component []string

	for {
		top := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		t.onStack[top] = false

		component = append(component, top)

		if top == file {
			break
		}
	}

	t.components = append(t.components, component)
}
