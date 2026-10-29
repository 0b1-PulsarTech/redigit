package planning

import (
	"cmp"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

// sweepLast births what is ready once the last feature is built. Nothing is
// still to come: what waited for a feature to freshen what it needs is born
// here instead of in the tail nobody reached.
func (p Planner) sweepLast(
	graph entities.Graph,
	order entities.Ordering,
	own Ownership,
	features []FeatureID,
	born map[entities.DeclKey]bool,
) []entities.PlannedCommit {
	if len(features) == 0 {
		return nil
	}

	return p.cutReady(graph, order, own, features[len(features)-1], births{
		born:  born,
		fresh: maps.Clone(born),
	})
}

func (p Planner) closeOut(
	graph entities.Graph,
	order entities.Ordering,
	born map[entities.DeclKey]bool,
	plan *entities.Plan,
) {
	// Anything no feature reached is still part of the final tree. Emitting it
	// as a trailing layer commit is honest; dropping it would leave a hole the
	// tree invariant only catches once every explanation is gone.
	if rest := Remaining(graph, born); len(rest) > 0 {
		plan.Commits = append(
			plan.Commits,
			p.cut(graph, order, bucket{
				keys:    rest,
				shape:   entities.ShapeLayer,
				feature: reach.SharedFeature,
				reason:  "reached by no entry point",
			})...,
		)

		MarkBorn(born, rest)
	}

	for _, decl := range graph.Decls() {
		if !born[decl.Key] {
			plan.Unplaced = append(plan.Unplaced, decl.Key)
		}
	}

	slices.SortFunc(plan.Unplaced, func(a, b entities.DeclKey) int {
		return cmp.Compare(a.String(), b.String())
	})

	plan.Commits = p.mergeFragments(graph, plan.Commits)

	for index := range plan.Commits {
		plan.Commits[index].Position = entities.Position(index)
	}
}
