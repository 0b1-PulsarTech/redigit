package entities

import "strings"

// wiringShare is the fraction of the program, as 1/wiringShare, that a
// declaration may need before it stops describing a capability and assembles
// the program. A handler's closure is a few dozen declarations; main, the
// injector and the route table each need thousands.
const wiringShare = 20

// wiringFloor is the fewest declarations a root can need. A program smaller than
// this has no assembly worth setting apart, and every tiny graph would read as
// all wiring.
const wiringFloor = 20

// FindWiring is the declarations that assemble the program: the composition
// root, the injection bootstrap, the route table. Counted as consumers they
// would make the whole program one feature; left out, the handlers are the
// entry points.
//
// A statement of such a function is wiring by kind: it was cut out of a root,
// and nothing but the root's own assembly reads it. So is the signature the
// statements sit under.
func FindWiring(graph Graph) map[DeclKey]bool {
	total := 0

	for _, decl := range graph.Decls() {
		if !IsTestSide(decl) {
			total++
		}
	}

	out := map[DeclKey]bool{}

	for _, decl := range graph.Decls() {
		if IsTestSide(decl) {
			continue
		}

		if decl.Key.Kind == KindStmt {
			out[decl.Key] = true

			if head, found := headOf(graph, decl.Key); found {
				out[head] = true
			}

			continue
		}

		if len(graph.Needs(decl.Key)) == 0 {
			continue
		}

		size := ClosureSize(graph, decl.Key)
		if size >= wiringFloor && size*wiringShare >= total {
			out[decl.Key] = true
		}
	}

	return out
}

// IsTestSide is a test or anything declared in a test file: the helpers, mocks
// and fixtures a suite builds are not part of the product they exercise.
func IsTestSide(decl Decl) bool {
	return decl.IsTest || strings.HasSuffix(decl.Key.File, "_test.go")
}

// IsStructural is a reason that ties a declaration to the build around it (the
// manifest, the workspace, the checksum), not to anything the program does.
func IsStructural(reason EdgeReason) bool {
	switch reason {
	case ReasonModule, ReasonHosts, ReasonWorkspace, ReasonChecksum:
		return true
	default:
		return false
	}
}

// ClosureSize counts the production declarations a declaration needs, itself
// included. Manifest and dependency edges are left out: a go.mod line is not
// something a capability is made of.
func ClosureSize(graph Graph, start DeclKey) int {
	seen := map[DeclKey]bool{start: true}
	queue := []DeclKey{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range graph.Needs(current) {
			if seen[edge.Provider] || IsStructural(edge.Reason) || edge.Reason == ReasonImport {
				continue
			}

			if provider, found := graph.Decl(edge.Provider); !found || IsTestSide(provider) {
				continue
			}

			seen[edge.Provider] = true

			queue = append(queue, edge.Provider)
		}
	}

	return len(seen)
}

// headOf is the function a statement was cut from.
func headOf(graph Graph, stmt DeclKey) (DeclKey, bool) {
	cut := strings.LastIndex(stmt.Name, "#")
	if cut < 0 {
		return DeclKey{}, false
	}

	for _, kind := range [...]DeclKind{KindFunc, KindMethod} {
		head := DeclKey{File: stmt.File, Kind: kind, Name: stmt.Name[:cut]}
		if _, found := graph.Decl(head); found {
			return head, true
		}
	}

	return DeclKey{}, false
}
