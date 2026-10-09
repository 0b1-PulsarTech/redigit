package planning

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// FeatureID names a vertical slice. It is derived from where the slice's entry
// point lives, so the name a reader sees is a real directory rather than a
// generated label.
type FeatureID string

// SharedFeature is the bucket for declarations no single feature owns.
const SharedFeature FeatureID = ""

// entryPoints are the declarations nothing else consumes: a handler, a worker,
// a command. They are the tips of the dependency graph, and walking back from
// each one is what recovers a feature that a layer-by-layer order would smear
// across hundreds of commits.
//
// A test consumer does not make a declaration an entry point. Counting tests
// would make every tested function a feature of its own.
func entryPoints(graph entities.Graph, roots wiring) []entities.Decl {
	var out []entities.Decl

	for _, decl := range graph.Decls() {
		if isTestSide(decl) {
			continue
		}

		if roots[decl.Key] || isOrphanProse(graph, decl) {
			continue
		}

		if hasProductionConsumer(graph, decl.Key, roots) {
			continue
		}

		out = append(out, decl)
	}

	slices.SortFunc(out, func(a, b entities.Decl) int {
		return cmp.Compare(a.Key.String(), b.Key.String())
	})

	return out
}

func isOrphanProse(graph entities.Graph, decl entities.Decl) bool {
	if decl.Key.Kind != entities.KindOther || len(graph.Consumers(decl.Key)) > 0 {
		return false
	}

	for _, edge := range graph.Needs(decl.Key) {
		if !isStructural(edge.Reason) {
			return false
		}
	}

	return true
}

func isStructural(reason entities.EdgeReason) bool {
	switch reason {
	case entities.ReasonModule, entities.ReasonHosts, entities.ReasonWorkspace,
		entities.ReasonChecksum:
		return true
	default:
		return false
	}
}

func hasProductionConsumer(graph entities.Graph, key entities.DeclKey, roots wiring) bool {
	for _, edge := range graph.Consumers(key) {
		consumer, found := graph.Decl(edge.Dependent)
		if found && !isTestSide(consumer) && !roots[edge.Dependent] {
			return true
		}
	}

	return false
}

// featureOf names the slice an entry point opens. Two entry points of the same
// directory open the same feature: a package's handlers are one feature, not
// one feature each.
func featureOf(decl entities.Decl) FeatureID {
	dir := decl.Unit.Dir()
	if dir == "" {
		dir = parentDir(decl.Key.File)
	}

	return FeatureID(dir)
}

func parentDir(path string) string {
	if cut := strings.LastIndex(path, "/"); cut > 0 {
		return path[:cut]
	}

	return "."
}

// reachFrom walks the dependency closure of one entry point. The walk follows
// Needs, so it collects everything that has to exist before the entry point
// does - which is exactly the slice the entry point is the tip of.
func reachFrom(graph entities.Graph, start entities.DeclKey) map[entities.DeclKey]bool {
	seen := map[entities.DeclKey]bool{start: true}
	queue := []entities.DeclKey{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			if seen[edge.Provider] {
				continue
			}

			seen[edge.Provider] = true

			queue = append(queue, edge.Provider)
		}
	}

	return seen
}
