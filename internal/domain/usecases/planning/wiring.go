package planning

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// wiringShare is the fraction of the program, as 1/wiringShare, that a
// declaration may need before it stops describing a capability and assembles
// the program. A handler's closure is a few dozen declarations; main, the
// injector and the route table each need thousands.
const wiringShare = 20

// wiringFloor is the fewest declarations a root can need. A program smaller than
// this has no assembly worth setting apart, and every tiny graph would read as
// all wiring.
const wiringFloor = 20

// wiring is the set of declarations that assemble the program: the composition
// root, the injection bootstrap, the route table. Counted as consumers they
// would make the whole program one feature; left out, the handlers are the
// entry points.
type wiring map[entities.DeclKey]bool

func findWiring(graph entities.Graph) wiring {
	total := 0

	for _, decl := range graph.Decls() {
		if !isTestSide(decl) {
			total++
		}
	}

	out := wiring{}

	for _, decl := range graph.Decls() {
		if isTestSide(decl) || len(graph.Needs(decl.Key)) == 0 {
			continue
		}

		size := closureSize(graph, decl.Key)
		if size >= wiringFloor && size*wiringShare >= total {
			out[decl.Key] = true
		}
	}

	return out
}

// isTestSide is a test or anything declared in a test file: the helpers, mocks
// and fixtures a suite builds are not part of the product they exercise.
func isTestSide(decl entities.Decl) bool {
	return decl.IsTest || strings.HasSuffix(decl.Key.File, "_test.go")
}

// closureSize counts the production declarations a declaration needs, itself
// included. Manifest and dependency edges are left out: a go.mod line is not
// something a capability is made of.
func closureSize(graph entities.Graph, start entities.DeclKey) int {
	seen := map[entities.DeclKey]bool{start: true}
	queue := []entities.DeclKey{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			if seen[edge.Provider] || isStructural(edge.Reason) ||
				edge.Reason == entities.ReasonImport {
				continue
			}

			if provider, found := graph.Decl(edge.Provider); !found || isTestSide(provider) {
				continue
			}

			seen[edge.Provider] = true

			queue = append(queue, edge.Provider)
		}
	}

	return len(seen)
}

// deferTests adds to the wiring every test, and whatever else a test file
// declares, and every afterword. A test belongs after the code it exercises and a test of the
// injector needs the whole program, so none of them is owned by a capability
// up front; each arrives once what it needs exists.
func deferTests(graph entities.Graph, roots wiring) wiring {
	out := wiring{}
	for key := range roots {
		out[key] = true
	}

	for _, decl := range graph.Decls() {
		if isTestSide(decl) || isAfterword(graph, decl.Key) {
			out[decl.Key] = true
		}
	}

	return out
}

func testSubjects(graph entities.Graph) map[entities.DeclKey][]entities.DeclKey {
	out := map[entities.DeclKey][]entities.DeclKey{}

	for _, anchor := range graph.Anchors() {
		if anchor.Reason == entities.ReasonTestOf {
			out[anchor.Decl] = append(out[anchor.Decl], anchor.To)
		}
	}

	return out
}
