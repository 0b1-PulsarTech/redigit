package planning

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/deferral"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
)

func (p Planner) emit(
	graph entities.Graph,
	order entities.Ordering,
	own owners.Ownership,
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
		shared, late := deferral.SplitLate(
			graph,
			owners.UnbornProviders(graph, own, members, born),
			members,
		)

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

			owners.MarkBorn(born, shared)
			owners.MarkBorn(fresh, shared)
		}

		// A deferred statement walked in ahead of its feature took the
		// declarations it reads with it; they are no longer the feature's to birth.
		members = slices.DeleteFunc(slices.Clone(members), func(key entities.DeclKey) bool {
			return born[key]
		})

		members = append(members, late...)

		owners.MarkBorn(born, members)
		owners.MarkBorn(fresh, members)

		// A test is born with the code it exercises when the feature completes
		// that code, so the suite and its subject are read together.
		owned := append(slices.Clone(members), deferral.OwnTests(graph, own, born, fresh)...)

		plan.Commits = append(
			plan.Commits,
			p.cut(graph, order, bucket{
				keys:    owned,
				shape:   entities.ShapeFeature,
				feature: feature,
				reason:  "one feature, closed over its own dependencies",
			})...,
		)

		owners.MarkBorn(born, owned)

		plan.Commits = append(
			plan.Commits,
			p.cutReady(graph, order, own, feature, births{born: born, fresh: fresh})...,
		)
	}

	plan.Commits = append(plan.Commits, p.sweepLast(graph, order, own, features, born)...)

	p.closeOut(graph, order, born, &plan)

	return plan
}

func groupByFeature(graph entities.Graph, own owners.Ownership) map[FeatureID][]entities.DeclKey {
	out := map[FeatureID][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		feature := own.Owner[decl.Key]
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
	own owners.Ownership,
	feature FeatureID,
	seen births,
) []entities.PlannedCommit {
	born := seen.born

	var keys []entities.DeclKey

	// What a round brings is fresh for the next: a statement that needs only
	// code born long ago and a registration born just now is ready only then.
	for {
		ready := deferral.ReadyDeferred(graph, own, born, seen.fresh)
		if len(ready) == 0 {
			break
		}

		round := append(owners.UnbornProviders(graph, own, ready, born), ready...)
		owners.MarkBorn(born, round)
		owners.MarkBorn(seen.fresh, round)

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
