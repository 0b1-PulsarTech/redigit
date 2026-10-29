package planning

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// moduleDir is the directory of the manifest that makes a declaration's unit a
// module, or "" when the graph names none. It is the unit of build the owner
// reads as one piece of the product, so it groups features better than a path
// prefix, which depends on how a repository happens to be laid out.
func moduleDir(graph entities.Graph, decl entities.Decl) string {
	for _, edge := range graph.Needs(decl.Key) {
		if edge.Reason != entities.ReasonModule {
			continue
		}

		if cut := strings.LastIndex(edge.Provider.File, "/"); cut > 0 {
			return edge.Provider.File[:cut]
		}

		return "."
	}

	return ""
}

// isHead reports whether an entry point is plugged into the wiring: a handler,
// a sink, a worker. An unconsumed declaration nothing registers is dead code or
// a program of its own, and says nothing about the module it sits in.
func isHead(graph entities.Graph, entry entities.Decl, roots reach.Wiring) bool {
	for _, edge := range graph.Consumers(entry.Key) {
		if roots[edge.Dependent] {
			return true
		}
	}

	return false
}

// shareLibraries makes every module that is read from outside and holds no
// head shared. A library has no capability of its own, so no feature owns it;
// each declaration is born with the first feature that needs it, and the
// package grows one consumer at a time.
func shareLibraries(graph entities.Graph, own ownership, heads map[string]bool) {
	read := readFromOutside(graph)

	for _, decl := range graph.Decls() {
		if entities.IsTestSide(decl) {
			continue
		}

		module := moduleDir(graph, decl)
		if module == "" || heads[module] || !read[module] {
			continue
		}

		own.owner[decl.Key] = reach.SharedFeature
	}
}

// readFromOutside is the modules some production declaration of another module
// reads.
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

// atModuleRoot is true for a declaration in the directory of its module's
// manifest: the module's own entry into the program.
func atModuleRoot(graph entities.Graph, decl entities.Decl) bool {
	dir := decl.Unit.Dir()
	if dir == "" {
		dir = reach.ParentDir(decl.Key.File)
	}

	return dir == moduleDir(graph, decl)
}

// deferLibraryEntries defers the entries of a library: a function nothing calls
// yet, an interface assertion. No feature needs them and no feature owns them,
// so left to the end they stretched a library's history from its first package
// to the last commit; deferred, each is born right after what it names.
func deferLibraryEntries(own ownership, entries []entities.Decl) {
	for _, entry := range entries {
		if own.owner[entry.Key] != reach.SharedFeature || own.deferred[entry.Key] {
			continue
		}

		own.deferred[entry.Key] = true
		own.loose[entry.Key] = true
	}
}

// sharedPackages groups the shared production declarations by package. A
// package is one importable thing: the first feature to need any of it brings
// the rest, so it is not rebuilt one consumer at a time over the whole history.
func sharedPackages(graph entities.Graph, own ownership) map[entities.UnitID][]entities.DeclKey {
	out := map[entities.UnitID][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		if own.owner[decl.Key] != reach.SharedFeature || own.deferred[decl.Key] ||
			entities.IsTestSide(decl) || decl.Generated || decl.Key.Kind == entities.KindOther ||
			feedsData(graph, decl.Key) {
			continue
		}

		out[decl.Unit] = append(out[decl.Unit], decl.Key)
	}

	return out
}

// feedsData is whether a declaration is one table's schema or query: each is
// born with the first repository that reads it, not with the rest of its
// package, or the whole database arrives in the first commit that touches one.
func feedsData(graph entities.Graph, key entities.DeclKey) bool {
	for _, edge := range graph.Consumers(key) {
		if edge.Reason == entities.ReasonSchema || edge.Reason == entities.ReasonQuery {
			return true
		}
	}

	return false
}
