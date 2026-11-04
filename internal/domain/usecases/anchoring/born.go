package anchoring

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

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
		commits[idx].Subject = redraft(commits[idx].Subject, commits[idx].Decls)
	}

	return commits
}

func redraft(subject string, keys []entities.DeclKey) string {
	const shown = 3

	head, _, isCut := strings.Cut(subject, "): ")
	if !entities.IsDraft(subject) || !isCut || len(keys) == 0 {
		return subject
	}

	names := make([]string, 0, shown)
	for _, key := range keys[:min(shown, len(keys))] {
		names = append(names, key.Name)
	}

	text := strings.Join(names, ", ")
	if len(keys) > shown {
		text = fmt.Sprintf("%s and %d more", text, len(keys)-shown)
	}

	return head + "): " + text
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
	plan entities.Plan,
	graph entities.Graph,
	chosen []kept,
) (rev, reason string) {
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
				return chosen[idx].anchor.Rev, "its pre-image reads " + hit + ", born after it"
			}
		}
	}

	return "", ""
}
