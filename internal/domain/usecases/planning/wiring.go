package planning

import (
	"slices"
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
// root, the injection bootstrap, the route table.
//
// They consume every handler, sink and worker, so counting them as consumers
// leaves nothing unconsumed but the root itself, and the whole program becomes
// one feature. Left out, the handlers become the entry points, and the product
// splits along its capabilities.
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
// declares. A test belongs after the code it exercises and a test of the
// injector needs the whole program, so none of them is owned by a capability
// up front; each arrives once what it needs exists.
func deferTests(graph entities.Graph, roots wiring) wiring {
	out := wiring{}
	for key := range roots {
		out[key] = true
	}

	for _, decl := range graph.Decls() {
		if isTestSide(decl) {
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

// readyDeferred is the deferred declarations that can be born now: none of what
// they need belongs to a feature still to come, and something they need was
// born with the feature just built. They arrive right after the feature that
// completes what they need, so the composition root grows by the last of the
// features it wires and the tests of the whole program follow it. Without the
// second condition, whatever needs only shared code is ready at the first
// feature and drags that code in under its name.
func readyDeferred(
	graph entities.Graph,
	own ownership,
	born map[entities.DeclKey]bool,
	fresh map[entities.DeclKey]bool,
) []entities.DeclKey {
	subjects := testSubjects(graph)
	ready := map[entities.DeclKey]bool{}

	for key := range own.deferred {
		if !born[key] {
			ready[key] = true
		}
	}

	// A test is born once everything it exercises is: pulling what it needs
	// would put the code under it in the feature that happened to come first.
	// The wiring has no feature of its own, so it may bring what only it uses.
	waits := func(provider entities.DeclKey, strict bool) bool {
		switch {
		case born[provider]:
			return false
		case own.deferred[provider]:
			return !ready[provider]
		case own.owner[provider] != SharedFeature:
			return true
		default:
			return strict
		}
	}

	for changed := true; changed; {
		changed = false

		for key := range ready {
			strict := !own.roots[key]

			if !waitsOn(graph, subjects, key, func(provider entities.DeclKey) bool {
				return waits(provider, strict)
			}) {
				continue
			}

			delete(ready, key)

			changed = true
		}
	}

	return sortedKeys(touched(graph, subjects, ready, fresh))
}

// touched keeps the ready declarations that need something fresh, directly or
// through another ready declaration. A manifest line does not count: every test
// of a module needs its go.mod, and that is no reason for the test to arrive
// with the first declaration that pulled the manifest in.
func touched(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	ready, fresh map[entities.DeclKey]bool,
) map[entities.DeclKey]bool {
	out := map[entities.DeclKey]bool{}

	for changed := true; changed; {
		changed = false

		for key := range ready {
			if out[key] || !needsFresh(graph, subjects, key, fresh, out) {
				continue
			}

			out[key] = true
			changed = true
		}
	}

	return out
}

func needsFresh(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	key entities.DeclKey,
	fresh, touched map[entities.DeclKey]bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if isStructural(edge.Reason) || edge.Reason == entities.ReasonImport {
			continue
		}

		if fresh[edge.Provider] || touched[edge.Provider] {
			return true
		}
	}

	for _, subject := range subjects[key] {
		if fresh[subject] || touched[subject] {
			return true
		}
	}

	return false
}

func waitsOn(
	graph entities.Graph,
	subjects map[entities.DeclKey][]entities.DeclKey,
	key entities.DeclKey,
	waits func(entities.DeclKey) bool,
) bool {
	for _, edge := range graph.Needs(key) {
		if waits(edge.Provider) {
			return true
		}
	}

	return slices.ContainsFunc(subjects[key], waits)
}
