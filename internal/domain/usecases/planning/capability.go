package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Capabilities is a caller-supplied answer to which slice each declaration
// belongs to, replacing the reachability walk. Reachability asks "what does
// this handler need", which on a real repository left 28% of declarations
// reached by no entry point and 14% reached by several: 42% in layer commits
// with no slice of their own.
type Capabilities map[entities.DeclKey]FeatureID

// ownershipFrom builds the ownership a caller supplied. Every declaration the
// map names belongs to exactly one slice, so nothing is shared and nothing is
// unreached; a declaration the map does not name stays SharedFeature, which is
// where the unreached already went.
func ownershipFrom(capabilities Capabilities, graph entities.Graph) ownership {
	own := ownership{
		owner: make(map[entities.DeclKey]FeatureID, graph.Len()),
		reach: make(map[entities.DeclKey]int, graph.Len()),
	}

	seen := map[FeatureID]bool{}

	for _, decl := range graph.Decls() {
		feature, named := capabilities[decl.Key]
		if !named {
			feature = SharedFeature
		}

		own.owner[decl.Key] = feature
		own.reach[decl.Key] = 1

		if feature != SharedFeature && !seen[feature] {
			seen[feature] = true

			own.order = append(own.order, feature)
		}
	}

	slices.SortFunc(own.order, func(a, b FeatureID) int { return cmp.Compare(a, b) })

	return own
}

// emitByCapability walks every declaration once in birth order, starting a new
// commit whenever the slice changes or MaxFiles is reached. One pass, not a
// loop over slices: a slice's members and what it pulls from elsewhere have to
// interleave by position, because a pulled declaration may itself need a
// member of the slice that pulled it.
func (p Planner) emitByCapability(
	graph entities.Graph,
	order entities.Ordering,
	own ownership,
) entities.Plan {
	keys := make([]entities.DeclKey, 0, graph.Len())
	for _, decl := range graph.Decls() {
		keys = append(keys, decl.Key)
	}

	atoms := make(map[entities.DeclKey]string, graph.Len())
	for _, decl := range graph.Decls() {
		if decl.Atom != "" {
			atoms[decl.Key] = decl.Atom
		}
	}

	// The affinity a tie is resolved by: the atom first, because its members
	// must end up in ONE commit, then the slice.
	keys = sortForBirth(graph, order, keys, func(key entities.DeclKey) FeatureID {
		if atom, grouped := atoms[key]; grouped {
			return FeatureID(atom)
		}

		return own.owner[key]
	})

	var plan entities.Plan

	current := []entities.DeclKey{}
	files := map[string]bool{}
	slice := SharedFeature
	atom := ""
	started := false

	flush := func() {
		if len(current) == 0 {
			return
		}

		plan.Commits = append(plan.Commits, entities.PlannedCommit{
			Position: entities.Position(len(plan.Commits)),
			Subject:  draftSubject(entities.ShapeFeature, slice, current),
			Shape:    entities.ShapeFeature,
			Feature:  string(slice),
			Decls:    slices.Clone(current),
			Reason:   "one capability, in the order the fixed point settled",
		})
		current = current[:0]
		files = map[string]bool{}
		atom = ""
	}

	for _, key := range keys {
		owner := own.owner[key]
		keyAtom := atoms[key]

		// A commit never splits an atom, whatever else changes. The members
		// of one were written by a tool in a single pass, and they can carry
		// different slices only because each was DATED separately - which is
		// an artefact of reading births per declaration, not a reason to
		// write half a generated file.
		inAtom := started && keyAtom != "" && keyAtom == atom

		tooWide := !files[key.File] && len(files) >= p.cfg.MaxFiles
		if started && !inAtom && (owner != slice || tooWide) {
			flush()
		}

		slice = owner
		atom = keyAtom
		started = true
		files[key.File] = true

		current = append(current, key)
	}

	flush()

	return plan
}
