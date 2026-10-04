package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// shareDependencies makes the shared bucket closed under what it needs.
//
// The shared declarations are born before every feature, so one that needs a
// declaration a single feature owns would be born before it exists. Reach
// counting keeps that from happening for what it counted, and three things
// escape the count: an atom is joined to its members' features after the walk,
// so a provider read only through one member keeps a single owner while the
// atom goes shared; a test inherits the ownership of what it exercises, and its
// helpers do not; and a test helper reached from a shared test belongs to no
// entry point at all. Each left a forward reference inside a layer commit (214
// of them, 32 wip commits on the real tree, "undefined: SecondsToDuration" and
// "undefined: enrichCase" in the trees between).
//
// A provider of a shared declaration is shared too, and so is what it needs.
// An atom goes whole: its members are born in one commit, so one of them
// staying with a feature would put the commit in two places.
func shareDependencies(graph entities.Graph, own ownership) {
	atoms := atomGroups(graph)
	queue := make([]entities.DeclKey, 0, graph.Len())

	for _, decl := range graph.Decls() {
		if own.owner[decl.Key] == SharedFeature {
			queue = append(queue, decl.Key)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		provided := []entities.DeclKey{}

		for _, edge := range graph.Needs(current) {
			provided = append(provided, edge.Provider)
		}

		if decl, found := graph.Decl(current); found && decl.Atom != "" {
			provided = append(provided, atoms[decl.Atom]...)
		}

		for _, key := range provided {
			owner, known := own.owner[key]
			if !known || owner == SharedFeature {
				continue
			}

			own.owner[key] = SharedFeature
			own.reach[key] = max(own.reach[key], own.reach[current])

			queue = append(queue, key)
		}
	}
}
