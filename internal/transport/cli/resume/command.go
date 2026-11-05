package resume

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

const (
	Verb  = "continue"
	Usage = `
  redigit continue --state DIR [--fold REV] [--wip REV] [--rest fold|wip] [--abort]
      Go on from a run that stopped (exit 3) on anchors whose pre-image does
      not build. DIR is the --state-dir of that run. Each decision names a
      conflicting anchor (an unambiguous prefix of its rev); an anchor left
      undecided stops the run again, and the state is kept again with every
      decision so far. --rest fold|wip decides every anchor still stopping.
      The original command line runs again with a config that carries the
      decisions, written to .redigit-continue.json in DIR. Refused when the
      revision or the original config changed since the stop, when the config
      is not under the 1 MiB limit once the decisions are in it, or when DIR
      holds a state.json or conflicts/ without redigit's marker. --abort
      removes the texts the marker in conflicts/ lists (a listed text you
      edited goes too) and redigit's reserved names (.redigit-stage,
      .redigit-old, .redigit-continue.json, .redigit-tmp-N: a file there is
      redigit's, a directory there goes only with what its marker lists).
      Your other files stay, and DIR itself goes only when the stopped run
      made it and nothing else is in it.
  --state-dir DIR   (plan and emit) where a run that stops keeps its state for
                    redigit continue; policy stop needs it. If the state cannot
                    be kept the run exits 2 or 1, never 3: rerun under policy
                    fold or wip, or with another --state-dir.
`
)

type (
	dispatcher func(ctx context.Context, args []string, stdout, stderr io.Writer) error

	continuer struct {
		dispatch dispatcher
		flags    flagBuilder
		commit   commitResolver
		log      *slog.Logger
	}
)

func With(dispatch dispatcher, flags flagBuilder) func(context.Context, []string, io.Writer) error {
	return continuer{dispatch: dispatch, flags: flags, commit: gitstore.ResolveCommit}.run
}

func (one continuer) run(ctx context.Context, args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	if opts.isAbort {
		if err = anchorstate.Remove(opts.state); err != nil {
			return refuse("abort the stop", err)
		}

		return nil
	}

	state, err := load(opts.state)
	if err != nil {
		return err
	}

	decisions, err := decisionsOf(opts, state)
	if err != nil {
		return err
	}

	found, err := scan(one.flags, state.Args[1:])
	if err != nil {
		return refuse("read the stop state", err)
	}

	if err = one.verify(ctx, state, found, opts.state); err != nil {
		return err
	}

	merged := map[string]anchoring.Policy{}
	maps.Copy(merged, state.Decisions)
	maps.Copy(merged, decisions)

	derived, err := derive(found.config, merged, opts.rest)
	if err != nil {
		return err
	}

	if err = anchorstate.WriteConfig(opts.state, derived); err != nil {
		return fmt.Errorf("write the derived config: %w", err)
	}

	argv := found.withConfig(state.Args, filepath.Join(opts.state, anchorstate.DerivedFile))

	return one.keepAgain(opts.state, state, merged, one.dispatch(ctx, argv, stdout, os.Stderr))
}
