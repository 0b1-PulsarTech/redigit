package resume

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/infra/lineageconf"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func refuse(step string, err error) error {
	if errors.Is(err, anchorstate.ErrNoState) || errors.Is(err, anchorstate.ErrForeign) {
		return fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	return fmt.Errorf("%s: %w", step, err)
}

func load(dir string) (anchorstate.State, error) {
	state, err := anchorstate.Load(dir)
	if err != nil {
		return anchorstate.State{}, refuse("read the stop state", err)
	}

	if state.Args[0] == Verb {
		return anchorstate.State{}, fmt.Errorf(
			"%w: the command to run again is continue", anchorstate.ErrCorrupt,
		)
	}

	return state, nil
}

func derive(
	config string,
	decisions map[string]anchoring.Policy,
	rest anchoring.Policy,
) ([]byte, error) {
	raw, err := os.ReadFile(config) // #nosec G304 -- the config path the stopped run used.
	if err != nil {
		return nil, fmt.Errorf("read the config: %w", err)
	}

	derived, err := lineageconf.Decide(raw, decisions, rest)
	if err != nil {
		return nil, fmt.Errorf("continue the stop: %w", err)
	}

	return derived, nil
}

func (one continuer) verify(
	ctx context.Context,
	state anchorstate.State,
	found target,
	dir string,
) error {
	if found.isDerived(dir) {
		return fmt.Errorf("%w: --config of the stopped run names %s",
			write.ErrUsage, anchorstate.DerivedFile)
	}

	commit, digest, err := fingerprint(ctx, one.commit, found)
	if err != nil {
		return err
	}

	if err = state.Fresh(commit, digest); err != nil {
		return fmt.Errorf("check the stop state: %w", err)
	}

	return nil
}
