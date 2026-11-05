package resume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type (
	commitResolver func(ctx context.Context, repo, rev string) (string, error)

	keeper struct {
		flags  flagBuilder
		commit commitResolver
		log    *slog.Logger
	}
)

func Keep(ctx context.Context, args []string, runErr error, flags flagBuilder) error {
	return keeper{flags: flags, commit: gitstore.ResolveCommit}.keep(ctx, args, runErr)
}

func (one keeper) keep(ctx context.Context, args []string, runErr error) error {
	stopped, isStopped := errors.AsType[*anchoring.StoppedError](runErr)
	if !isStopped || len(args) == 0 || args[0] == Verb {
		return runErr
	}

	found, err := scan(one.flags, args[1:])
	if err == nil {
		err = one.save(ctx, args, found, stopped)
	}

	if err != nil {
		return notKept(err)
	}

	announce(one.log, found.stateDir, stopped)

	return runErr
}

func (one keeper) save(
	ctx context.Context,
	args []string,
	found target,
	stopped *anchoring.StoppedError,
) error {
	if found.stateDir == "" {
		return fmt.Errorf("%w: --state-dir is required", write.ErrUsage)
	}

	if found.isDerived(found.stateDir) {
		return fmt.Errorf("%w: --config names %s, which continue writes in the state directory",
			write.ErrUsage, anchorstate.DerivedFile)
	}

	commit, digest, err := fingerprint(ctx, one.commit, found)
	if err != nil {
		return err
	}

	err = anchorstate.Save(found.stateDir, anchorstate.State{
		Args: args, Commit: commit, Digest: digest, Conflicts: stopped.Conflicts,
	})
	if err != nil {
		return fmt.Errorf("save the stop state: %w", err)
	}

	return nil
}

func (one continuer) keepAgain(
	dir string,
	state anchorstate.State,
	decisions map[string]anchoring.Policy,
	runErr error,
) error {
	stopped, isStopped := errors.AsType[*anchoring.StoppedError](runErr)
	if !isStopped {
		return runErr
	}

	state.Decisions, state.Conflicts = decisions, stopped.Conflicts

	if err := anchorstate.Save(dir, state); err != nil {
		return notKept(err)
	}

	announce(one.log, dir, stopped)

	return runErr
}

func fingerprint(
	ctx context.Context,
	resolve commitResolver,
	found target,
) (commit, digest string, err error) {
	commit, err = resolve(ctx, found.repo, found.rev)
	if err != nil {
		return "", "", fmt.Errorf("resolve the revision: %w", err)
	}

	digest, err = anchorstate.DigestFile(found.config)
	if err != nil {
		return "", "", err //nolint:wrapcheck // DigestFile names the step
	}

	return commit, digest, nil
}

func notKept(err error) error {
	kept := fmt.Errorf(
		"the stop state was not kept (%w); rerun under policy fold or wip, "+
			"or with another --state-dir, to go on", err,
	)

	if errors.Is(err, anchorstate.ErrForeign) {
		return fmt.Errorf("%w: %w", write.ErrUsage, kept)
	}

	return kept
}

func announce(logger *slog.Logger, dir string, stopped *anchoring.StoppedError) {
	if logger == nil {
		logger = slog.Default()
	}

	for _, conflict := range stopped.Conflicts {
		logger.Warn("anchors: conflict",
			slog.String("rev", conflict.Rev), slog.String("detail", conflict.Detail),
			slog.Int("decls", len(conflict.Decls)),
		)
	}

	logger.Warn("anchors: stopped, state kept",
		slog.String("state_dir", dir), slog.Int("conflicts", len(stopped.Conflicts)),
	)
}
