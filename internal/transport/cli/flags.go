package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

var errVirtualForm = errors.New("--virtual wants ID=PATH")

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
		far          int
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
	set.StringVar(&cfg.ServerCommand, "server", cfg.ServerCommand, "language server command")
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

	set.Func("virtual", "ID=PATH: the file a virtual module compiles from", addVirtual(&cfg))

	if err := set.Parse(args); err != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: %s", ErrUsage, err)
	}

	if cfg.Repo == "" {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --repo is required", ErrUsage)
	}

	cfg.BuildTags = splitTags(tags)

	if err := loadSlicing(&cfg, layerFile, featureFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := loadChunkMap(&cfg, chunkMapFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	return cfg, far, nil
}

// loadSlicing reads the two files that decide how a layer may reorder: the
// layers themselves, and the features they act inside.
func loadSlicing(cfg *bootstrap.Config, layerFile, featureFile string) error {
	if layerFile != "" {
		layers, err := bootstrap.LoadLayers(layerFile)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrUsage, err)
		}

		cfg.Ordering.Layers = layers
	}

	if featureFile != "" {
		features, err := bootstrap.LoadFeatures(featureFile)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrUsage, err)
		}

		cfg.Ordering.Features = features
	}

	return nil
}

func splitTags(tags string) []string {
	if strings.TrimSpace(tags) == "" {
		return nil
	}

	parts := strings.Split(tags, ",")

	out := make([]string, 0, len(parts))

	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

// loadChunkMap reads the operator-supplied chunk map named by --chunk-map, if
// any. A malformed or missing file is a usage error: the tool cannot guess
// where the operator meant the cuts to go (see docs/PLANO-EMISSAO.md:113-133).
func loadChunkMap(cfg *bootstrap.Config, chunkMapFile string) error {
	if chunkMapFile == "" {
		return nil
	}

	chunkMap, err := bootstrap.LoadChunkMap(chunkMapFile)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUsage, err)
	}

	cfg.ChunkMap = chunkMap

	return nil
}

// addVirtual reads one --virtual flag into cfg; the flag repeats, one virtual
// module per occurrence.
func addVirtual(cfg *bootstrap.Config) func(string) error {
	return func(value string) error {
		id, file, found := strings.Cut(value, "=")
		if !found || strings.TrimSpace(id) == "" || strings.TrimSpace(file) == "" {
			return fmt.Errorf("%w, got %q", errVirtualForm, value)
		}

		if cfg.Virtual == nil {
			cfg.Virtual = map[string]string{}
		}

		cfg.Virtual[strings.TrimSpace(id)] = strings.TrimSpace(file)

		return nil
	}
}
