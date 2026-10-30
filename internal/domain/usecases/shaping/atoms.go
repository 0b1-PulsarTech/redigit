package shaping

import (
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const ReasonAtomSplit entities.EdgeReason = "atom-split"

func indexAtoms(graph entities.Graph) map[string][]entities.DeclKey {
	atoms := map[string][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		if decl.Atom != "" {
			atoms[decl.Atom] = append(atoms[decl.Atom], decl.Key)
		}
	}

	return atoms
}

func keySet(keys []entities.DeclKey) map[entities.DeclKey]bool {
	set := make(map[entities.DeclKey]bool, len(keys))
	for _, key := range keys {
		set[key] = true
	}

	return set
}

func (draft *Draft) splitsAtom(keys []entities.DeclKey, inside map[entities.DeclKey]bool) bool {
	for _, key := range keys {
		decl, _ := draft.graph.Decl(key)
		slot, placed := draft.rankOf(key)

		// A member no commit holds proves nothing, and a mate in another slot is already apart,
		// so only a placed mate sharing the slot must travel along.
		for _, mate := range draft.atoms[decl.Atom] {
			mateSlot, mated := draft.rankOf(mate)
			if placed && mated && mateSlot == slot && !inside[mate] {
				return true
			}
		}
	}

	return false
}

func (draft *Draft) atomSpread() map[string]int {
	spread := make(map[string]int, len(draft.atoms))

	for name, mates := range draft.atoms {
		slots := map[int]bool{}

		for _, mate := range mates {
			if rank, placed := draft.rankOf(mate); placed {
				slots[rank] = true
			}
		}

		spread[name] = len(slots)
	}

	return spread
}

func (draft *Draft) wideningSplits(before map[string]int) []entities.Edge {
	var out []entities.Edge

	spread := draft.atomSpread()

	for _, name := range slices.Sorted(maps.Keys(spread)) {
		if spread[name] > before[name] {
			out = append(out, draft.atomEdges(name)...)
		}
	}

	return out
}

func (draft *Draft) AtomSplits() []entities.Edge {
	names := slices.Sorted(maps.Keys(draft.atoms))
	out := make([]entities.Edge, 0, len(names))

	for _, name := range names {
		out = append(out, draft.atomEdges(name)...)
	}

	return out
}

func (draft *Draft) atomEdges(name string) []entities.Edge {
	var (
		out      []entities.Edge
		first    entities.DeclKey
		firstAt  int
		hasFirst bool
	)

	for _, mate := range draft.atoms[name] {
		rank, placed := draft.rankOf(mate)

		switch {
		case !placed:
		case !hasFirst:
			first, firstAt, hasFirst = mate, rank, true
		case rank != firstAt:
			out = append(out, entities.Edge{
				Dependent: mate, Provider: first, Reason: ReasonAtomSplit,
			})
		}
	}

	return out
}
