package anchoring

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type slotQuery struct {
	at        entities.Position
	born      []entities.DeclKey
	notBefore int
}

func (planner Planner) slot(commits []entities.PlannedCommit, query slotQuery) int {
	chosen := len(commits)

	for idx, commit := range commits {
		if planner.maxFloor(commit) >= query.at {
			chosen = idx

			break
		}
	}

	// What the anchor rewrites must exist before the anchor does.
	for idx, commit := range commits {
		if slices.ContainsFunc(query.born, func(key entities.DeclKey) bool {
			return slices.Contains(commit.Decls, key)
		}) {
			chosen = max(chosen, idx+1)
		}
	}

	return max(chosen, query.notBefore)
}

func (planner Planner) maxFloor(commit entities.PlannedCommit) entities.Position {
	highest := entities.Position(-1)

	for _, key := range commit.Decls {
		if floor, isKnown := planner.cfg.Floor[key]; isKnown {
			highest = max(highest, floor)
		}
	}

	return highest
}

func placedIn(plan entities.Plan) map[entities.DeclKey]bool {
	placed := map[entities.DeclKey]bool{}

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			placed[key] = true
		}
	}

	return placed
}

func keysOf(entries []entry) []entities.DeclKey {
	keys := make([]entities.DeclKey, 0, len(entries))

	for _, placed := range entries {
		keys = append(keys, placed.key)
	}

	return keys
}

func markCarriers(commits []entities.PlannedCommit, slot int, keys []entities.DeclKey) int {
	first := slot

	for idx := range slot {
		if slices.ContainsFunc(keys, func(key entities.DeclKey) bool {
			return slices.Contains(commits[idx].Decls, key)
		}) {
			first = idx

			break
		}
	}

	for idx := first; idx < slot; idx++ {
		commits[idx].IsWIP = true
		commits[idx].Subject = entities.MarkWIP(commits[idx].Subject)
	}

	return slot - first
}

func keySet(chosen []kept) map[entities.DeclKey]bool {
	set := map[entities.DeclKey]bool{}

	for _, one := range chosen {
		for _, key := range keysOf(one.entries) {
			set[key] = true
		}
	}

	return set
}

func prune(commits []entities.PlannedCommit, slots []int) ([]entities.PlannedCommit, []int) {
	gone := make([]int, len(commits)+1)
	out := make([]entities.PlannedCommit, 0, len(commits))

	for idx, commit := range commits {
		gone[idx+1] = gone[idx]

		if len(commit.Decls) == 0 && len(commit.Changes) == 0 {
			gone[idx+1]++

			continue
		}

		out = append(out, commit)
	}

	remapped := make([]int, len(slots))

	for idx, slot := range slots {
		remapped[idx] = slot - gone[slot]

		if gone[slot+1] != gone[slot] {
			remapped[idx] = -1
		}
	}

	return out, remapped
}

func liveEntries(chosen []kept) [][]entry {
	live := make([][]entry, len(chosen))
	next := map[entities.DeclKey]string{}

	for idx := len(chosen) - 1; idx >= 0; idx-- {
		for _, placed := range chosen[idx].entries {
			text := squeezed(placed.preimage)
			if held, isHeld := next[placed.key]; isHeld && held == text {
				continue
			}

			live[idx] = append(live[idx], placed)
			next[placed.key] = text
		}
	}

	return live
}

func (planner Planner) place(
	commits []entities.PlannedCommit,
	chosen []kept,
) ([]entities.PlannedCommit, []int) {
	slots := make([]int, 0, len(chosen))
	notBefore := 0

	for _, one := range chosen {
		keys := keysOf(one.entries)
		query := slotQuery{at: one.anchor.Position, born: keys, notBefore: notBefore}
		slot := planner.slot(commits, query)
		commits = slices.Insert(commits, slot, entities.PlannedCommit{
			Subject: one.revision.Subject,
			Shape:   entities.ShapeFeature,
			Changes: keys,
			Reason:  "anchor " + one.anchor.Rev + " kept as a real commit",
		})

		slots = append(slots, slot)
		notBefore = slot + 1
	}

	return commits, slots
}
