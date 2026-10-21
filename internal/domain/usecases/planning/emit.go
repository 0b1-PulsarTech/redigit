package planning

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (p Planner) emit(
	graph entities.Graph,
	order entities.Ordering,
	own ownership,
	features []FeatureID,
) entities.Plan {
	byFeature := groupByFeature(graph, own)
	born := map[entities.DeclKey]bool{}

	var plan entities.Plan

	for _, feature := range features {
		members := byFeature[feature]

		// What several features need is born by the first of them to need it,
		// in the commits just before that feature's own. A kernel emitted ahead
		// of every feature is a block of hundreds of commits that no feature
		// explains, and the declarations in it are read for the first time
		// hundreds of commits later.
		fresh := map[entities.DeclKey]bool{}
		shared, late := splitLate(graph, unbornProviders(graph, own, members, born), members)

		if len(shared) > 0 {
			plan.Commits = append(
				plan.Commits,
				p.cut(graph, order, bucket{
					keys:    shared,
					shape:   entities.ShapeFeature,
					feature: feature,
					reason:  "shared by several features, born with the first that needs it",
				})...,
			)

			markBorn(born, shared)
			markBorn(fresh, shared)
		}

		// A deferred statement walked in ahead of its feature took the
		// declarations it reads with it; they are no longer the feature's to birth.
		members = slices.DeleteFunc(slices.Clone(members), func(key entities.DeclKey) bool {
			return born[key]
		})

		members = append(members, late...)

		markBorn(born, members)
		markBorn(fresh, members)

		// A test is born with the code it exercises when the feature completes
		// that code, so the suite and its subject are read together.
		owned := append(slices.Clone(members), ownTests(graph, own, born, fresh)...)

		plan.Commits = append(
			plan.Commits,
			p.cut(graph, order, bucket{
				keys:    owned,
				shape:   entities.ShapeFeature,
				feature: feature,
				reason:  "one feature, closed over its own dependencies",
			})...,
		)

		markBorn(born, owned)

		plan.Commits = append(
			plan.Commits,
			p.cutReady(graph, order, own, feature, births{born: born, fresh: fresh})...,
		)
	}

	p.closeOut(graph, order, born, &plan)

	return plan
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
	if rest := remaining(graph, born); len(rest) > 0 {
		plan.Commits = append(
			plan.Commits,
			p.cut(graph, order, bucket{
				keys:    rest,
				shape:   entities.ShapeLayer,
				feature: SharedFeature,
				reason:  "reached by no entry point",
			})...,
		)

		markBorn(born, rest)
	}

	for _, decl := range graph.Decls() {
		if !born[decl.Key] {
			plan.Unplaced = append(plan.Unplaced, decl.Key)
		}
	}

	slices.SortFunc(plan.Unplaced, func(a, b entities.DeclKey) int {
		return cmp.Compare(a.String(), b.String())
	})

	for index := range plan.Commits {
		plan.Commits[index].Position = entities.Position(index)
	}
}

func groupByFeature(graph entities.Graph, own ownership) map[FeatureID][]entities.DeclKey {
	out := map[FeatureID][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		feature := own.owner[decl.Key]
		out[feature] = append(out[feature], decl.Key)
	}

	return out
}

// births is what has been born so far, and the part of it the feature just built
// brought.
type births struct {
	born  map[entities.DeclKey]bool
	fresh map[entities.DeclKey]bool
}

// cutReady emits the deferred declarations the feature just built made possible,
// with whatever shared code they bring along.
func (p Planner) cutReady(
	graph entities.Graph,
	order entities.Ordering,
	own ownership,
	feature FeatureID,
	seen births,
) []entities.PlannedCommit {
	born := seen.born

	var keys []entities.DeclKey

	// What a round brings is fresh for the next: a statement that needs only
	// code born long ago and a registration born just now is ready only then.
	for {
		ready := readyDeferred(graph, own, born, seen.fresh)
		if len(ready) == 0 {
			break
		}

		round := append(unbornProviders(graph, own, ready, born), ready...)
		markBorn(born, round)
		markBorn(seen.fresh, round)

		keys = append(keys, round...)
	}

	if len(keys) == 0 {
		return nil
	}

	return p.cut(graph, order, bucket{
		keys:    keys,
		shape:   entities.ShapeFeature,
		feature: feature,
		reason:  "assembles the features built so far",
	})
}
