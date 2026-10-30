package shaping

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	Slot struct {
		ID     int
		Commit entities.PlannedCommit
		Added  bool
	}

	Draft struct {
		graph    entities.Graph
		hard     []entities.Edge
		slots    []Slot
		ranks    map[int]int
		at       *SlotMap
		next     int
		unplaced []entities.DeclKey
		atoms    map[string][]entities.DeclKey
	}
)

const NoSlot = -1

func NewDraft(graph entities.Graph, plan entities.Plan) *Draft {
	draft := &Draft{
		graph:    graph,
		slots:    make([]Slot, len(plan.Commits)),
		at:       NewSlotMap(),
		next:     len(plan.Commits),
		unplaced: slices.Clone(plan.Unplaced),
		atoms:    indexAtoms(graph),
	}

	for _, edge := range graph.Edges() {
		if entities.IsHard(edge.Reason) {
			draft.hard = append(draft.hard, edge)
		}
	}

	for id, commit := range plan.Commits {
		commit.Decls = slices.Clone(commit.Decls)
		draft.slots[id] = Slot{ID: id, Commit: commit}

		for _, key := range commit.Decls {
			draft.at.Place(key, id)
		}
	}

	draft.reindex()

	return draft
}

func (draft *Draft) Graph() entities.Graph {
	// A shared read-only view: the draft indexes it, so a caller must not edit it.
	return draft.graph
}

func (draft *Draft) Slots() []Slot {
	// A shared read-only view: ranks index it, so a caller must change it only through the draft.
	return draft.slots
}

func (draft *Draft) Of(key entities.DeclKey) (int, bool) { return draft.at.Slot(key) }

func (draft *Draft) Rank(id int) (int, bool) {
	rank, live := draft.ranks[draft.at.Resolve(id)]

	return rank, live
}

func (draft *Draft) Violations() []entities.Edge {
	var out []entities.Edge

	// An endpoint no commit holds proves nothing either way.
	for _, edge := range draft.hard {
		provider, hasProvider := draft.rankOf(edge.Provider)
		dependent, hasDependent := draft.rankOf(edge.Dependent)

		if hasProvider && hasDependent && provider > dependent {
			out = append(out, edge)
		}
	}

	return out
}

func (draft *Draft) Fits(keys []entities.DeclKey, after int) bool {
	cut := NoSlot

	if after != NoSlot {
		rank, live := draft.Rank(after)
		if !live {
			return false
		}

		cut = rank
	}

	inside := keySet(keys)
	if draft.splitsAtom(keys, inside) {
		return false
	}

	// Only a hard edge to a decl outside the moved set can point the wrong way.
	for _, key := range keys {
		for _, edge := range draft.graph.Needs(key) {
			late := func(rank int) bool { return rank > cut }
			if draft.misplaced(edge, edge.Provider, inside, late) {
				return false
			}
		}

		for _, edge := range draft.graph.Consumers(key) {
			early := func(rank int) bool { return rank <= cut }
			if draft.misplaced(edge, edge.Dependent, inside, early) {
				return false
			}
		}
	}

	return true
}

func (draft *Draft) Clone() *Draft {
	clone := *draft
	clone.slots = make([]Slot, len(draft.slots))

	for index, slot := range draft.slots {
		slot.Commit.Decls = slices.Clone(slot.Commit.Decls)
		clone.slots[index] = slot
	}

	clone.at = draft.at.Clone()
	clone.reindex()

	return &clone
}

func (draft *Draft) reindex() {
	draft.ranks = make(map[int]int, len(draft.slots))
	for rank, slot := range draft.slots {
		draft.ranks[slot.ID] = rank
	}
}

func (draft *Draft) rankOf(key entities.DeclKey) (int, bool) {
	id, placed := draft.at.Slot(key)
	if !placed {
		return 0, false
	}

	rank, live := draft.ranks[id]

	return rank, live
}

func (draft *Draft) misplaced(
	edge entities.Edge,
	other entities.DeclKey,
	inside map[entities.DeclKey]bool,
	wrong func(rank int) bool,
) bool {
	if !entities.IsHard(edge.Reason) || inside[other] {
		return false
	}

	rank, placed := draft.rankOf(other)

	return placed && wrong(rank)
}
