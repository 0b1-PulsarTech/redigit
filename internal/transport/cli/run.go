package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
	"github.com/wrapped-owls/redigit/internal/infra/profiling"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
)

//go:generate go tool -modfile=../../../tools/go.mod mockgen -source=session/pipeline.go -destination=pipeline_mock_test.go -package=cli

type runOptions struct {
	cpuProfile  string
	memProfile  string
	tracePath   string
	memLimit    string
	isTiming    bool
	isFromCache bool
}

func registerRunFlags(set *flag.FlagSet) *runOptions {
	opts := &runOptions{}

	set.StringVar(&opts.cpuProfile, "cpuprofile", "", "write a CPU profile to this file")
	set.StringVar(&opts.memProfile, "memprofile", "", "write a heap profile to this file")
	set.StringVar(&opts.tracePath, "trace", "", "write an execution trace to this file")
	set.StringVar(&opts.memLimit, "mem-limit", "", "soft memory limit, GOMEMLIMIT syntax")
	set.BoolVar(
		&opts.isTiming,
		"timings",
		false,
		"print per-stage wall time as one JSON line on stderr",
	)
	set.BoolVar(&opts.isFromCache, "from-cache", false, "reuse a saved graph, ordering and plan")

	return opts
}

func startProfiling(opts *runOptions, stderr io.Writer) (*profiling.Session, error) {
	// Before the pipeline starts, so the indexing that dominates a run is inside.
	profile, err := profiling.Start(profiling.Options{
		CPUProfile: opts.cpuProfile,
		MemProfile: opts.memProfile,
		Trace:      opts.tracePath,
		MemLimit:   opts.memLimit,
	})

	switch {
	case errors.Is(err, profiling.ErrMemLimit), errors.Is(err, profiling.ErrSamePath):
		_, _ = fmt.Fprint(stderr, usage+runUsage)

		return nil, fmt.Errorf("%w: profiling flags: %w", ErrUsage, err)
	case err != nil:
		return nil, fmt.Errorf("start profiling: %w", err)
	}

	return profile, nil
}

type runEnv = session.Env

func realEnv() runEnv {
	return runEnv{
		Start:     realStart,
		BuildID:   plancache.ExecutableDigest,
		Commit:    gitstore.ResolveCommit,
		Digest:    plancache.CommandDigest,
		Toolchain: plancache.GoToolchain,
	}
}

func stopProfiling(profile *profiling.Session, runErr error) error {
	if err := profile.Stop(); err != nil {
		return errors.Join(runErr, fmt.Errorf("stop profiling: %w", err))
	}

	return runErr
}

func finishRun(
	profile *profiling.Session,
	timings *profiling.Timings,
	opts *runOptions,
	stderr io.Writer,
	runErr error,
) error {
	if opts.isTiming {
		if err := timings.Write(stderr); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}

	return stopProfiling(profile, runErr)
}

type (
	pipeline = session.Pipeline
	starter  = session.Starter
)

//nolint:ireturn // The pipeline port is the product, as bootstrap.Start hands it over.
func realStart(ctx context.Context, cfg bootstrap.Config) (pipeline, error) {
	pl, err := bootstrap.Start(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("bootstrap start: %w", err)
	}

	return pl, nil
}
