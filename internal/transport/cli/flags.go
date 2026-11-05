package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/inputs"
	"github.com/wrapped-owls/redigit/internal/infra/lineageconf"
)

var ErrStopped = anchoring.ErrStopped

func newFlagSet() *flag.FlagSet {
	set := flag.NewFlagSet("redigit", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	return set
}

type fileFlags struct {
	layers, features, chunkMap, config, stateDir string
}

func addFileFlags(set *flag.FlagSet) *fileFlags {
	files := &fileFlags{}

	set.StringVar(&files.layers, "layers", "", "layer file: which path prefixes come first")
	set.StringVar(
		&files.features, "features", "", "feature file: the vertical slices layers act inside",
	)
	set.StringVar(
		&files.chunkMap, "chunk-map", "",
		"chunk map file: JSON spans for a format redigit cannot parse on its own",
	)
	set.StringVar(
		&files.config, "config", "", "config file: tags, virtual modules and rules for a project",
	)
	set.StringVar(&files.stateDir, "state-dir", "", "where a run that stops keeps its state")

	return files
}

func parse(set *flag.FlagSet, args []string) (bootstrap.Config, int, error) {
	cfg := bootstrap.Defaults()
	flags := defineFlags(set, &cfg)
	files := flags.files

	if err := set.Parse(args); err != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if flags.virtualErr != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --virtual: %w", ErrUsage, flags.virtualErr)
	}

	if cfg.Repo == "" {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --repo is required", ErrUsage)
	}

	if err := checkChoices(cfg); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := applyTags(&cfg, flags.tags); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := applyConfigFile(&cfg, set, files.config); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := loadSlicing(&cfg, files.layers, files.features); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := checkStateDir(cfg, files.stateDir); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := loadChunkMap(&cfg, files.chunkMap); err != nil {
		return bootstrap.Config{}, 0, err
	}

	return cfg, flags.far, nil
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

func checkStateDir(cfg bootstrap.Config, stateDir string) error {
	if cfg.Mode != bootstrap.ModeLineage || stateDir != "" {
		return nil
	}

	section, err := lineageconf.Decode(cfg.Sections.Lineage)
	if err != nil {
		return fmt.Errorf("%w: %w", bootstrap.ErrConfig, err)
	}

	if section.IsStopping() {
		return fmt.Errorf("%w: policy stop needs --state-dir", ErrUsage)
	}

	return nil
}
