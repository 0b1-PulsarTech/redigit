package planning

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Config tunes where the planner cuts. Both knobs are deliberate: a tool that
// hides them reports one shape and cannot say why it reached it.
type Config struct {
	// MaxFiles caps how many files one commit touches. A commit spanning sixty
	// packages cannot be named truthfully.
	MaxFiles int

	// MaxAddedLines caps the hand-written lines one commit adds.
	MaxAddedLines int

	// Capabilities replaces the reachability walk with a slicing the caller
	// read out of the published history. Empty means derive it.
	Capabilities Capabilities
}

const defaultMaxFiles = 12

func (c Config) withDefaults() Config {
	if c.MaxFiles < 1 {
		c.MaxFiles = defaultMaxFiles
	}

	if c.MaxAddedLines < 1 {
		c.MaxAddedLines = entities.DefaultMaxAddedLines
	}

	return c
}

type Planner struct {
	cfg Config
}

func New(cfg Config) Planner {
	return Planner{cfg: cfg.withDefaults()}
}

// Plan groups an ordering into commits, cutting along features where a feature
// owns the work and along layers where none does.
//
// The result is never written straight to a history. Automatic grouping is
// measurably the weak half of this problem: agents reach replay validity above
// 0.988 while grouping quality sits between 0.03 and 0.46, so the plan exists to
// be read and corrected before anything is committed.
func (p Planner) Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error) {
	if graph.Len() == 0 {
		return entities.Plan{}, fmt.Errorf("%w: nothing to plan", apperr.ErrIncomplete)
	}

	if len(p.cfg.Capabilities) > 0 {
		return p.emitByCapability(graph, order, ownershipFrom(p.cfg.Capabilities, graph)), nil
	}

	own := assign(graph)

	// No entry point means no declaration is unconsumed, which only happens
	// when a cycle spans the whole graph. Emitting one flat layer commit
	// would look like success and would throw away every vertical slice.
	if len(own.order) == 0 {
		return entities.Plan{}, fmt.Errorf(
			"%w: no entry point in %d declarations; the graph is entirely cyclic",
			apperr.ErrIncomplete, graph.Len(),
		)
	}

	sorted, cyclic := sortFeatures(
		own.order,
		featureNeeds(graph, own),
		own.waits,
		earliest(own, order),
		own.group,
		own.cost,
	)

	// Reachability ownership should make this unreachable: if a declaration
	// of feature A needs one of feature B, everything that reaches the first
	// reaches the second, so the second is reached by both and becomes shared
	// rather than B's. The guard stays because that argument is reasoning, not
	// proof, and misordering silently is the worse failure.
	if len(cyclic) > 0 {
		return entities.Plan{}, fmt.Errorf(
			"%w: %d features need each other, starting at %q",
			apperr.ErrIncomplete, len(cyclic), cyclic[0],
		)
	}

	return p.emit(graph, order, own, sorted), nil
}

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

		if shared := unbornProviders(graph, own, members, born); len(shared) > 0 {
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

		plan.Commits = append(
			plan.Commits,
			p.cut(graph, order, bucket{
				keys:    members,
				shape:   entities.ShapeFeature,
				feature: feature,
				reason:  "one feature, closed over its own dependencies",
			})...,
		)

		markBorn(born, members)
		markBorn(fresh, members)

		plan.Commits = append(plan.Commits, p.cutReady(graph, order, own, feature, born, fresh)...)
	}

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

	return plan
}

func groupByFeature(graph entities.Graph, own ownership) map[FeatureID][]entities.DeclKey {
	out := map[FeatureID][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		feature := own.owner[decl.Key]
		out[feature] = append(out[feature], decl.Key)
	}

	return out
}

// cutReady emits the deferred declarations the feature just built made possible,
// with whatever shared code they bring along.
func (p Planner) cutReady(
	graph entities.Graph,
	order entities.Ordering,
	own ownership,
	feature FeatureID,
	born, fresh map[entities.DeclKey]bool,
) []entities.PlannedCommit {
	ready := readyDeferred(graph, own, born, fresh)
	if len(ready) == 0 {
		return nil
	}

	keys := append(unbornProviders(graph, own, ready, born), ready...)
	markBorn(born, keys)

	return p.cut(graph, order, bucket{
		keys:    keys,
		shape:   entities.ShapeFeature,
		feature: feature,
		reason:  "assembles the features built so far",
	})
}
