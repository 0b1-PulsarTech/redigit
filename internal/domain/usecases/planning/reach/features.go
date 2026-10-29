package reach

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

// EntryPoints are the declarations nothing else consumes: a handler, a worker,
// a command. A test consumer does not count, or every tested function would be
// a feature of its own.
func EntryPoints(graph entities.Graph, roots Wiring) []entities.Decl {
	var out []entities.Decl

	loose := LooseLines(graph)

	for _, decl := range graph.Decls() {
		if entities.IsTestSide(decl) {
			continue
		}

		if roots[decl.Key] || loose[decl.Key] || isOrphanProse(graph, decl) ||
			IsAfterword(graph, decl.Key) || IsAfterwordBase(graph, decl.Key) {
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
		if !entities.IsStructural(edge.Reason) {
			return false
		}
	}

	return true
}

func hasProductionConsumer(graph entities.Graph, key entities.DeclKey, roots Wiring) bool {
	for _, edge := range graph.Consumers(key) {
		consumer, found := graph.Decl(edge.Dependent)
		if found && !entities.IsTestSide(consumer) && !roots[edge.Dependent] &&
			!IsAfterword(graph, edge.Dependent) {
			return true
		}
	}

	return false
}

// FeatureOf names the slice an entry point opens. Two entry points of the same
// directory open the same feature: a package's handlers are one feature, not
// one feature each.
func FeatureOf(decl entities.Decl) FeatureID {
	dir := decl.Unit.Dir()
	if dir == "" {
		dir = ParentDir(decl.Key.File)
	}

	return FeatureID(dir)
}

func ParentDir(path string) string {
	if cut := strings.LastIndex(path, "/"); cut > 0 {
		return path[:cut]
	}

	return "."
}

// ReachFrom walks the dependency closure of one entry point. The walk follows
// Needs, so it collects everything that has to exist before the entry point
// does - which is exactly the slice the entry point is the tip of.
func ReachFrom(graph entities.Graph, start entities.DeclKey) map[entities.DeclKey]bool {
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
