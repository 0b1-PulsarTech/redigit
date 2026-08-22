package ordering

import (
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Config configures the fixed point. Slack is how many spare positions the
// order carries past the source history's last touched position (or past 0
// with no history) for tail commits that read each other back; the prior
// work used 8. RoundLimit bounds how many rounds the fixed point may take
// before the guard treats it as a runaway instead of a slow convergence; the
// prior work used 2000 and observed real runs converging in 109-467 rounds.
type Config struct {
	Slack      int
	RoundLimit int
}

// defaultSlack and defaultRoundLimit are the values the previous rebuild
// settled on: eight spare positions past the source history's end, and a
// round guard far above the 109 to 467 rounds real runs converged in.
const (
	defaultSlack      = 8
	defaultRoundLimit = 2000
)

// withDefaults fills a zero Config. Without it a caller that forgets to set
// RoundLimit gets a loop that never executes and an ordering that is nothing
// but its own seed - which still returns a full map of positions and so looks
// exactly like a settled one.
func (c Config) withDefaults() Config {
	if c.Slack < 1 {
		c.Slack = defaultSlack
	}

	if c.RoundLimit < 1 {
		c.RoundLimit = defaultRoundLimit
	}

	return c
}

// Orderer turns a Graph plus a Snapshot into an Ordering by monotone fixed
// point: a declaration only ever moves later inside the loop, never earlier.
// Cycles are named with the reasons that formed them rather than hidden, and
// a declaration the fixed point cannot place within Slack of the source
// history is reported rather than silently accepted.
type Orderer struct {
	config Config
}

// New builds an Orderer from its Config.
func New(config Config) Orderer {
	return Orderer{config: config.withDefaults()}
}

// ErrNotConverged reports a fixed point that hit its round guard. It is fatal
// where a cycle or an unresolved declaration is not: those come with a settled
// ordering a caller can still act on, while this one comes with an ordering
// that never settled at all.
var ErrNotConverged = errors.New("fixed point did not converge")

// UnresolvedError names the declarations still pinned at limit after
// convergence: a real unresolved dependency, not a warning. Use errors.As to
// recover which ones.
type UnresolvedError struct {
	Declarations []entities.DeclKey
}

func (e *UnresolvedError) Error() string {
	return fmt.Sprintf("%d declaration(s) pinned at limit", len(e.Declarations))
}

func (e *UnresolvedError) Unwrap() error { return apperr.ErrIncomplete }

// Order computes the fixed point for graph given the evidence in snap. The
// returned Ordering always carries what was computed, even alongside a
// non-nil error: a cycle or an unresolved declaration is data a caller acts
// on, not merely a reason to discard the result.
func (o Orderer) Order(graph entities.Graph, snap entities.Snapshot) (entities.Ordering, error) {
	eng := newEngine(graph, snap, o.config.Slack)

	rounds, err := eng.converge(o.config.RoundLimit)
	if err != nil {
		return eng.ordering(rounds, nil), err
	}

	cycles := findCycles(graph)
	unresolved := eng.unresolved()
	result := eng.ordering(rounds, cycles)

	var errs []error

	if len(cycles) > 0 {
		errs = append(
			errs,
			fmt.Errorf("%w: ordering holds %d cycle(s)", apperr.ErrIncomplete, len(cycles)),
		)
	}

	if len(unresolved) > 0 {
		errs = append(errs, &UnresolvedError{Declarations: unresolved})
	}

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	return result, nil
}
