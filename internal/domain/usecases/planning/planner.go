package planning

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/testplace"
)

// Config tunes where the planner cuts. Both knobs are deliberate: a tool that
// hides them reports one shape and cannot say why it reached it.
type Config struct {
	// MaxFiles caps how many files one commit touches. A commit spanning sixty
	// packages cannot be named truthfully.
	MaxFiles int

	// MaxAddedLines caps the hand-written lines one commit adds.
	MaxAddedLines int

	// MinCommitLines is the size below which a commit is a scrap that merges
	// into its neighbour, and an interface assertion on its own always does.
	// Zero keeps every commit the planner cut.
	MinCommitLines int

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
	cfg   Config
	sizes map[string]int
}

func New(cfg Config) Planner {
	return Planner{cfg: cfg.withDefaults()}
}

// Plan groups an ordering into commits, cutting along features where a feature
// owns the work and along layers where none does. The result is read and
// corrected before anything is committed: automatic grouping is the weak half.
func (p Planner) Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error) {
	if graph.Len() == 0 {
		return entities.Plan{}, fmt.Errorf("%w: nothing to plan", apperr.ErrIncomplete)
	}

	if len(p.cfg.Capabilities) > 0 {
		return p.emitByCapability(graph, order, ownershipFrom(p.cfg.Capabilities, graph)), nil
	}

	graph, order = testplace.AnchorTests(graph, order)
	p.sizes = generatedSizes(graph)
	own := owners.Assign(graph)

	// No entry point means no declaration is unconsumed, which only happens
	// when a cycle spans the whole graph. Emitting one flat layer commit
	// would look like success and would throw away every vertical slice.
	if len(own.Order) == 0 {
		return entities.Plan{}, fmt.Errorf(
			"%w: no entry point in %d declarations; the graph is entirely cyclic",
			apperr.ErrIncomplete, graph.Len(),
		)
	}

	sorted, cyclic := SortFeatures(own.Order, FeatureRules{
		Needs: FeatureNeeds(graph, own),
		Waits: own.Waits,
		At:    Earliest(own, order),
		Group: own.Group,
		Cost:  own.Cost,
	})

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
