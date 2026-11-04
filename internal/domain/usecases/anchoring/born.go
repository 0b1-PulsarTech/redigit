package anchoring

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (planner Planner) adopt(
	commits []entities.PlannedCommit,
	view scene,
	slot int,
) ([]entities.PlannedCommit, int) {
	index := indexOf(commits)

	var late []entities.DeclKey

	for _, key := range view.one.born {
		if _, isPlaced := index[key]; isPlaced {
			late = append(late, key)
		}
	}

	slices.SortFunc(late, entities.CompareKeys)
	late = slices.Compact(late)

	moving := map[entities.DeclKey]bool{}
	why := map[entities.DeclKey]string{}

	for _, key := range late {
		if view.owned[key] {
			why[key] = "an anchor up to this one changes it"

			continue
		}

		moving[key] = true
	}

	for isShrinking := true; isShrinking; {
		isShrinking = false

		for _, key := range late {
			if !moving[key] {
				continue
			}

			if reason := view.holds(key, moving, index, slot); reason != "" {
				delete(moving, key)

				why[key] = reason
				isShrinking = true
			}
		}
	}

	stayed := 0

	for _, key := range late {
		if !moving[key] {
			stayed++

			planner.cfg.Logger.Warn("anchors: born declaration stays",
				slog.String("rev", view.one.anchor.Rev),
				slog.String("decl", key.String()),
				slog.String("reason", why[key]),
			)
		}
	}

	commits[slot].Decls = slices.SortedFunc(maps.Keys(moving), entities.CompareKeys)

	return withdraw(commits, slot, moving), stayed
}

func indexOf(commits []entities.PlannedCommit) map[entities.DeclKey]int {
	index := map[entities.DeclKey]int{}

	for idx, commit := range commits {
		for _, key := range commit.Decls {
			index[key] = idx
		}
	}

	return index
}

func withdraw(
	commits []entities.PlannedCommit,
	slot int,
	moved map[entities.DeclKey]bool,
) []entities.PlannedCommit {
	for idx := range commits {
		if idx == slot || !slices.ContainsFunc(commits[idx].Decls, func(key entities.DeclKey) bool {
			return moved[key]
		}) {
			continue
		}

		commits[idx].Decls = slices.DeleteFunc(
			slices.Clone(commits[idx].Decls),
			func(key entities.DeclKey) bool { return moved[key] },
		)
		commits[idx].Subject = entities.Redraft(commits[idx].Subject, commits[idx].Decls)
	}

	return commits
}

func (planner Planner) adoptAll(
	commits []entities.PlannedCommit,
	graph entities.Graph,
	chosen []kept,
	slots []int,
) ([]entities.PlannedCommit, []int) {
	stayed := make([]int, len(chosen))
	view := newScene(graph, planner.cfg.Reader)

	for idx := len(chosen) - 1; idx >= 0; idx-- {
		view.one = chosen[idx]
		view.newer = keySet(chosen[idx:])
		view.owned = keySet(chosen[:idx+1])
		view.holders = holdersBefore(chosen, idx)
		commits, stayed[idx] = planner.adopt(commits, view, slots[idx])
	}

	return commits, stayed
}

func (planner Planner) misfit(
	ctx context.Context,
	plan entities.Plan,
	graph entities.Graph,
	chosen []kept,
	judge judgement,
) (rev, reason string, err error) {
	commits, slots := planner.place(slices.Clone(plan.Commits), chosen)
	commits, _ = planner.adoptAll(commits, graph, chosen, slots)
	index := indexOf(commits)
	view := newScene(graph, planner.cfg.Reader)
	previous := map[entities.DeclKey]int{}

	for idx, live := range liveEntries(chosen) {
		for _, placed := range live {
			activation := max(index[placed.key], previous[placed.key])
			previous[placed.key] = slots[idx]

			hit := view.readsAfter(placed, index, nil, activation)
			if hit != "" && chosen[idx].outcome == outcomeKept {
				return chosen[idx].anchor.Rev, "its pre-image may read " + hit + ", born after it", nil
			}
		}
	}

	keep := keysByFile(graph)

	// The emitter writes a commit from the declarations born by then, so the probe builds the
	// state just before the slot; a method born after its caller is the read check's. Newest
	// first: the anchor whose pre-image breaks the window falls, and an older one is judged again.
	for idx, one := range slices.Backward(chosen) {
		window := bornBefore(one, index, slots[idx])
		if one.trial == nil || len(window) == len(one.trial) {
			continue
		}

		verdict, probeErr := planner.verdictOf(ctx, one.found, editsOf(window, keep), judge)
		if probeErr != nil {
			return "", "", probeErr
		}

		if !verdict.IsBuilt && !verdict.IsUnchecked {
			return one.anchor.Rev, verdict.Detail, nil
		}
	}

	return "", "", nil
}
