package lint

import (
	"flag"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/inputs"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type pipeOptions struct {
	repo      string
	rev       string
	tags      string
	resolver  string
	mode      string
	fromCache bool
}

func registerPipeFlags(set *flag.FlagSet, pipe *pipeOptions) {
	set.StringVar(&pipe.repo, "repo", "", "")
	set.StringVar(&pipe.rev, "rev", bootstrap.Defaults().Rev, "")
	set.StringVar(&pipe.tags, "tags", "", "")
	set.StringVar(&pipe.resolver, "resolver", bootstrap.ResolverTyped, "")
	set.StringVar(&pipe.mode, "mode", bootstrap.ModeHistory, "")
	set.BoolVar(&pipe.fromCache, "from-cache", false, "")
}

func checkPipeFlags(opts options) error {
	if opts.pipe.repo != "" {
		return nil
	}

	var stray string

	pipeNames := []string{"rev", "tags", "resolver", "mode", "from-cache"}

	opts.set.Visit(func(visited *flag.Flag) {
		if slices.Contains(pipeNames, visited.Name) && stray == "" {
			stray = visited.Name
		}
	})

	if stray != "" {
		return fmt.Errorf("%w: --%s needs --repo", write.ErrUsage, stray)
	}

	return nil
}

func pipelineConfig(opts options, file *bootstrap.ConfigFile) (bootstrap.Config, error) {
	cfg := bootstrap.Defaults()
	cfg.Repo, cfg.Rev = opts.pipe.repo, opts.pipe.rev
	cfg.Resolver, cfg.Mode = opts.pipe.resolver, opts.pipe.mode

	if err := bootstrap.CheckResolver(cfg.Resolver); err != nil {
		return bootstrap.Config{}, fmt.Errorf("%w: --resolver: %w", write.ErrUsage, err)
	}

	if err := bootstrap.CheckMode(cfg.Mode); err != nil {
		return bootstrap.Config{}, fmt.Errorf("%w: --mode: %w", write.ErrUsage, err)
	}

	tags, err := inputs.ParseTags(opts.pipe.tags)
	if err != nil {
		return bootstrap.Config{}, fmt.Errorf("%w: --tags: %w", write.ErrUsage, err)
	}

	cfg.BuildTags = tags

	if file == nil {
		return cfg, nil
	}

	passed := map[string]bool{}

	opts.set.Visit(func(visited *flag.Flag) { passed[visited.Name] = true })

	applied, err := bootstrap.ApplyConfigFile(
		cfg,
		*file,
		func(name string) bool { return passed[name] },
	)
	if err != nil {
		return bootstrap.Config{}, err //nolint:wrapcheck // names its file once
	}

	return applied, nil
}

func keyFlags(opts options) *flag.FlagSet {
	// The cache key reads only the flags the pipeline sees, so lint's own never enter it.
	keys := flag.NewFlagSet("lint", flag.ContinueOnError)

	var scratch pipeOptions

	registerPipeFlags(keys, &scratch)
	keys.String("config", "", "")

	opts.set.Visit(func(visited *flag.Flag) {
		if keys.Lookup(visited.Name) != nil {
			_ = keys.Set(visited.Name, visited.Value.String())
		}
	})

	return keys
}
