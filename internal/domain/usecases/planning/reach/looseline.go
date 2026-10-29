package reach

import "github.com/wrapped-owls/redigit/internal/domain/entities"

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

func unread(graph entities.Graph, key entities.DeclKey, loose Wiring) bool {
	for _, edge := range graph.Consumers(key) {
		consumer, found := graph.Decl(edge.Dependent)
		if found && !entities.IsTestSide(consumer) && !loose[edge.Dependent] {
			return false
		}
	}

	return true
}

func TiedToFresh(graph entities.Graph, key entities.DeclKey, fresh, touched Wiring) bool {
	for _, edge := range graph.Needs(key) {
		named := edge.Reason == entities.ReasonWorkspace || edge.Reason == entities.ReasonImport
		if named && (fresh[edge.Provider] || touched[edge.Provider]) {
			return true
		}
	}

	return false
}
