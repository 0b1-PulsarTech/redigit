package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/slots"
	"github.com/wrapped-owls/redigit/internal/infra/verifycache"
)

type (
	assembler func(ctx context.Context, settings plan) (verifying.Sweep, error)

	pruner interface{ Prune(now time.Time) error }
)

func pruneBuildCaches(builder verifying.Builder) {
	if pruning, canPrune := builder.(pruner); canPrune {
		if err := pruning.Prune(time.Now()); err != nil {
			slog.Warn("prune build caches", slog.String("error", err.Error()))
		}
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, build assembler) error {
	opts, err := parse(args)
	if err != nil {
		return err
	}

	settings, err := resolve(opts)
	if err != nil {
		return err
	}

	sweep, err := build(ctx, settings)
	if err != nil {
		return fmt.Errorf("assemble verifier: %w", err)
	}

	encoder := json.NewEncoder(stdout)
	started := time.Now()

	summary, err := sweep.Run(ctx, settings.opts, func(row verifying.Row) error {
		return encoder.Encode(newRowLine(row))
	})

	pruneBuildCaches(sweep.Builder)

	if errors.Is(err, verifying.ErrInvalid) {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if errors.Is(err, slots.ErrSlotsUnsupported) {
		return fmt.Errorf("verify cannot run on this platform, it needs unix file locks: %w", err)
	}

	if err != nil {
		return fmt.Errorf("verify commits: %w", err)
	}

	if err = encoder.Encode(newSummaryLine(summary, time.Since(started))); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	switch {
	case summary.Failed > 0:
		return fmt.Errorf("%w: %d failing module rows", ErrFailed, summary.Failed)
	case summary.Checked == 0:
		return fmt.Errorf("%w: no module was checked", ErrFailed)
	}

	return nil
}

func assemble(ctx context.Context, settings plan) (verifying.Sweep, error) {
	gitDir, err := slots.GitDir(ctx, settings.repo)
	if err != nil {
		return verifying.Sweep{}, fmt.Errorf("find git dir: %w", err)
	}

	root := settings.cacheDir
	if root == "" {
		if root, err = verifycache.DefaultDir(); err != nil {
			return verifying.Sweep{}, fmt.Errorf("find cache dir: %w", err)
		}
	}

	builder, err := checkrun.New(checkrun.Config{Languages: settings.languages, CacheDir: root})
	if err != nil {
		return verifying.Sweep{}, fmt.Errorf("create builder: %w", err)
	}

	return verifying.Sweep{
		Commits: slots.NewCommits(gitDir),
		Trees:   slots.NewTrees(gitDir, filepath.Join(root, "slots")),
		Modules: slots.NewModules(gitDir, settings.markers),
		Builder: builder,
		Cache:   verifycache.New(filepath.Join(root, "entries")),
		Logger:  slog.Default(),
	}, nil
}
