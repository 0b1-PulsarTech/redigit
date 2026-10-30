package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/inputs"
)

func newFlagSet() *flag.FlagSet {
	set := flag.NewFlagSet("redigit", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	return set
}

func parse(set *flag.FlagSet, args []string) (bootstrap.Config, int, error) {
	cfg := bootstrap.Defaults()

	var (
		tags         string
		layerFile    string
		featureFile  string
		chunkMapFile string
		configFile   string
		far          int
		virtualErr   error
	)

	set.StringVar(&cfg.Repo, "repo", "", "repository to read")
	set.StringVar(&cfg.Rev, "rev", cfg.Rev, "revision to read")
	set.StringVar(&tags, "tags", "", "comma-separated build tags")
	set.IntVar(&far, "far", 0, "positions before a declaration counts as adrift")
	set.StringVar(
		&cfg.BlobDir,
		"blob-dir",
		"",
		"write blobs to this directory instead of the object database",
	)
	addServerFlags(set, &cfg)
	set.StringVar(&layerFile, "layers", "", "layer file: which path prefixes come first")
	set.StringVar(
		&featureFile,
		"features",
		"",
		"feature file: the vertical slices layers act inside",
	)
	set.StringVar(
		&chunkMapFile,
		"chunk-map",
		"",
		"chunk map file: JSON spans for a format redigit cannot parse on its own",
	)

	set.StringVar(
		&configFile,
		"config",
		"",
		"config file: tags, virtual modules and rules for a project",
	)

	add := addVirtual(&cfg, &virtualErr)
	set.Func("virtual", "ID=PATH: the file a virtual module compiles from", add)

	if err := set.Parse(args); err != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if virtualErr != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --virtual: %w", ErrUsage, virtualErr)
	}

	if cfg.Repo == "" {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --repo is required", ErrUsage)
	}

	if err := checkChoices(cfg); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := applyTags(&cfg, tags); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := applyConfigFile(&cfg, set, configFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := loadSlicing(&cfg, layerFile, featureFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := loadChunkMap(&cfg, chunkMapFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	return cfg, far, nil
}

func addServerFlags(set *flag.FlagSet, cfg *bootstrap.Config) {
	set.StringVar(&cfg.ServerCommand, "server", cfg.ServerCommand, "language server command")
	set.StringVar(
		&cfg.Resolver,
		"resolver",
		bootstrap.ResolverTyped,
		"typed with lsp fallback, or lsp",
	)
	set.StringVar(&cfg.Mode, "mode", bootstrap.ModeHistory, "history, or lineage")
}

func checkChoices(cfg bootstrap.Config) error {
	if err := bootstrap.CheckResolver(cfg.Resolver); err != nil {
		return fmt.Errorf("%w: --resolver: %w", ErrUsage, err)
	}

	if err := bootstrap.CheckMode(cfg.Mode); err != nil {
		return fmt.Errorf("%w: --mode: %w", ErrUsage, err)
	}

	return nil
}

func addVirtual(cfg *bootstrap.Config, refused *error) func(string) error {
	// The flag repeats. A refusal is kept, not returned: flag flattens it to text.
	return func(value string) error {
		if cfg.Virtual == nil {
			cfg.Virtual = map[string]string{}
		}

		id, file, _ := strings.Cut(value, "=")
		if err := inputs.AddVirtual(cfg.Virtual, id, file); err != nil && *refused == nil {
			*refused = err
		}

		return nil
	}
}

func applyTags(cfg *bootstrap.Config, tags string) error {
	parsed, err := inputs.ParseTags(tags)
	if err != nil {
		return fmt.Errorf("%w: --tags: %w", ErrUsage, err)
	}

	cfg.BuildTags = parsed

	return nil
}
