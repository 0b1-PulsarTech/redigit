package reach

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// Wiring is the set of declarations that assemble the program, found by
// entities.FindWiring.
type Wiring map[entities.DeclKey]bool

// DeferTests adds to the wiring every test, and whatever else a test file
// declares, every afterword and every loose manifest line. A test belongs after
// the code it exercises and a test of the injector needs the whole program, so
// none of them is owned by a capability up front; each arrives once what it
// needs exists.
func DeferTests(graph entities.Graph, roots, support Wiring) Wiring {
	out := Wiring{}
	for key := range roots {
		out[key] = true
	}

	loose := LooseLines(graph)

	for _, decl := range graph.Decls() {
		if entities.IsTestSide(decl) || support[decl.Key] || IsAfterword(graph, decl.Key) ||
			IsAfterwordBase(graph, decl.Key) || loose[decl.Key] {
			out[decl.Key] = true
		}
	}

	return out
}

func TestSubjects(graph entities.Graph) map[entities.DeclKey][]entities.DeclKey {
	out := map[entities.DeclKey][]entities.DeclKey{}

	for _, anchor := range graph.Anchors() {
		if anchor.Reason == entities.ReasonTestOf {
			out[anchor.Decl] = append(out[anchor.Decl], anchor.To)
		}
	}

	return out
}
