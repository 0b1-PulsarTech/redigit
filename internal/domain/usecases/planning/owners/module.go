package owners

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func moduleDir(graph entities.Graph, decl entities.Decl) string {
	for _, edge := range graph.Needs(decl.Key) {
		if edge.Reason != entities.ReasonModule {
			continue
		}

		// A module is the unit of build the owner reads as one piece of the product,
		// so it groups features better than a path prefix, which depends on how a
		// repository happens to be laid out.
		if cut := strings.LastIndex(edge.Provider.File, "/"); cut > 0 {
			return edge.Provider.File[:cut]
		}

		return "."
	}

	return ""
}

func isHead(graph entities.Graph, entry entities.Decl, roots reach.Wiring) bool {
	for _, edge := range graph.Consumers(entry.Key) {
		if roots[edge.Dependent] {
			return true
		}
	}

	return false
}

func shareLibraries(graph entities.Graph, own Ownership, heads map[string]bool) {
	read := readFromOutside(graph)

	// A library has no capability of its own, so no feature owns it; each
	// declaration is born with the first feature that needs it, and the package
	// grows one consumer at a time.
	for _, decl := range graph.Decls() {
		if entities.IsTestSide(decl) {
			continue
		}

		module := moduleDir(graph, decl)
		if module == "" || heads[module] || !read[module] {
			continue
		}

		own.Owner[decl.Key] = reach.SharedFeature
	}
}

func readFromOutside(graph entities.Graph) map[string]bool {
	out := map[string]bool{}

	for _, edge := range graph.Edges() {
		dependent, known := graph.Decl(edge.Dependent)
		provider, found := graph.Decl(edge.Provider)

		if !known || !found || entities.IsTestSide(dependent) ||
			entities.IsStructural(edge.Reason) {
			continue
		}

		module := moduleDir(graph, provider)
		if module != "" && module != moduleDir(graph, dependent) {
			out[module] = true
		}
	}

	return out
}

func atModuleRoot(graph entities.Graph, decl entities.Decl) bool {
	dir := decl.Unit.Dir()
	if dir == "" {
		dir = reach.ParentDir(decl.Key.File)
	}

	return dir == moduleDir(graph, decl)
}

func deferLibraryEntries(own Ownership, entries []entities.Decl) {
	for _, entry := range entries {
		if own.Owner[entry.Key] != reach.SharedFeature || own.Deferred[entry.Key] {
			continue
		}

		own.Deferred[entry.Key] = true
		own.Loose[entry.Key] = true
	}
}

func sharedPackages(graph entities.Graph, own Ownership) map[entities.UnitID][]entities.DeclKey {
	out := map[entities.UnitID][]entities.DeclKey{}

	// A package is one importable thing: the first feature to need any of it
	// brings the rest, so it is not rebuilt one consumer at a time over the
	// whole history.
	for _, decl := range graph.Decls() {
		if own.Owner[decl.Key] != reach.SharedFeature || own.Deferred[decl.Key] ||
			entities.IsTestSide(decl) || decl.Generated || decl.Key.Kind == entities.KindOther ||
			feedsData(graph, decl.Key) {
			continue
		}

		out[decl.Unit] = append(out[decl.Unit], decl.Key)
	}

	return out
}

func feedsData(graph entities.Graph, key entities.DeclKey) bool {
	for _, edge := range graph.Consumers(key) {
		if edge.Reason == entities.ReasonSchema || edge.Reason == entities.ReasonQuery {
			return true
		}
	}

	return false
}
