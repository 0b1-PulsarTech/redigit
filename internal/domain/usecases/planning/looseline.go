package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// LooseLines are the lines of a build manifest that no code needs: a workspace
// entry, a replace whose require nothing imports, an indirect require. Owned as
// entry points they formed a feature per manifest and pulled every module it
// names hundreds of commits before the module's first package. They are
// deferred like a test and born once the modules they name exist.
func LooseLines(graph entities.Graph) Wiring {
	out := Wiring{}

	for changed := true; changed; {
		changed = false

		for _, decl := range graph.Decls() {
			if out[decl.Key] || !IsManifestLine(graph, decl) || !unread(graph, decl.Key, out) {
				continue
			}

			out[decl.Key] = true
			changed = true
		}
	}

	return out
}

// IsManifestLine is a declaration that exists for the build around the code: it
// waits for a workspace entry or a require and for nothing the program does.
func IsManifestLine(graph entities.Graph, decl entities.Decl) bool {
	if decl.Key.Kind != entities.KindOther {
		return false
	}

	tied := false

	for _, edge := range graph.Needs(decl.Key) {
		switch {
		case edge.Reason == entities.ReasonWorkspace || edge.Reason == entities.ReasonImport:
			tied = true
		case !entities.IsStructural(edge.Reason):
			return false
		}
	}

	return tied
}

// unread is whether nothing but tests and other loose lines needs a declaration.
func unread(graph entities.Graph, key entities.DeclKey, loose Wiring) bool {
	for _, edge := range graph.Consumers(key) {
		consumer, found := graph.Decl(edge.Dependent)
		if found && !entities.IsTestSide(consumer) && !loose[edge.Dependent] {
			return false
		}
	}

	return true
}

// TiedToFresh is whether a manifest line names something not yet born: the
// module a workspace entry points at, the package a require provides. The
// manifest it sits in is no reason to bring it, or one go.work brings them all.
func TiedToFresh(graph entities.Graph, key entities.DeclKey, fresh, touched Wiring) bool {
	for _, edge := range graph.Needs(key) {
		named := edge.Reason == entities.ReasonWorkspace || edge.Reason == entities.ReasonImport
		if named && (fresh[edge.Provider] || touched[edge.Provider]) {
			return true
		}
	}

	return false
}
