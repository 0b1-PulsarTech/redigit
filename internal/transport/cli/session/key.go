package session

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

type Resolved struct {
	Key    string
	Commit string
}

type Env struct {
	Start     Starter
	CacheDir  string
	BuildID   func() (string, error)
	Commit    func(ctx context.Context, repo, rev string) (string, error)
	Digest    func(command string) (string, error)
	Toolchain func(ctx context.Context, repo, commit string) (string, error)
	Measure   func(stage string) func()
}

func Resolve(
	ctx context.Context,
	set *flag.FlagSet,
	cfg bootstrap.Config,
	env Env,
) (Resolved, error) {
	buildID, err := env.BuildID()
	if err != nil {
		return Resolved{}, fmt.Errorf("identify this build: %w", err)
	}

	commit, err := env.Commit(ctx, cfg.Repo, cfg.Rev)
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve the revision: %w", err)
	}

	server, err := env.Digest(cfg.ServerCommand)
	if err != nil {
		return Resolved{}, fmt.Errorf("identify the language server: %w", err)
	}

	toolchain, err := env.Toolchain(ctx, cfg.Repo, commit)
	if err != nil {
		return Resolved{}, fmt.Errorf("identify the go toolchain: %w", err)
	}

	key, err := plancache.Inputs{
		BuildID:    buildID,
		Commit:     commit,
		Server:     server,
		ServerArgs: cfg.ServerArgs,
		Toolchain:  toolchain,
		Virtual:    cfg.Virtual,
		Flags:      Flags(set, cfg.ConfigBytes),
	}.Key()
	if err != nil {
		return Resolved{}, fmt.Errorf("hash the cache inputs: %w", err)
	}

	return Resolved{Key: key, Commit: commit}, nil
}

func WithCache(ctx context.Context, set *flag.FlagSet, cfg bootstrap.Config, env Env) Starter {
	// A cache that cannot prove its inputs must not answer, so fall back.
	resolved, err := Resolve(ctx, set, cfg, env)
	if err != nil {
		slog.Warn("the plan cache is off for this run", slog.String("error", err.Error()))

		return env.Start
	}

	dir := env.CacheDir
	if dir == "" {
		if dir, err = plancache.DefaultDir(); err != nil {
			slog.Warn("the plan cache is off for this run", slog.String("error", err.Error()))

			return env.Start
		}
	}

	return CachingStart(env.Start, plancache.Open(dir), resolved, env.Measure)
}
