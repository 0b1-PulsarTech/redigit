package cli

import (
	"flag"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/inputs"
)

func applyConfigFile(cfg *bootstrap.Config, set *flag.FlagSet, configFile string) error {
	passed := map[string]bool{}

	set.Visit(func(visited *flag.Flag) { passed[visited.Name] = true })

	// An empty value given on purpose is a mistake, not "no config".
	if configFile == "" {
		if passed["config"] {
			return fmt.Errorf("%w: --config needs a path", bootstrap.ErrConfig)
		}

		return nil
	}

	file, err := bootstrap.LoadConfigFile(configFile)
	if err != nil {
		return err //nolint:wrapcheck // a config error names its file once; a second prefix would stutter
	}

	applied, err := bootstrap.ApplyConfigFile(
		*cfg, file, func(name string) bool { return passed[name] },
	)
	if err != nil {
		return err //nolint:wrapcheck // names its file once
	}

	*cfg = applied

	return nil
}

// loadSlicing reads the two files that decide how a layer may reorder: the
// layers themselves, and the features they act inside.
func loadSlicing(cfg *bootstrap.Config, layerFile, featureFile string) error {
	if layerFile != "" {
		layers, err := inputs.LoadLayers(layerFile)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUsage, err)
		}

		cfg.Ordering.Layers = layers
	}

	if featureFile != "" {
		features, err := inputs.LoadFeatures(featureFile)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUsage, err)
		}

		cfg.Ordering.Features = features
	}

	return nil
}

// loadChunkMap reads the operator-supplied chunk map named by --chunk-map, if
// any. A malformed or missing file is a usage error: the tool cannot guess
// where the operator meant the cuts to go (see docs/PLANO-EMISSAO.md:113-133).
func loadChunkMap(cfg *bootstrap.Config, chunkMapFile string) error {
	if chunkMapFile == "" {
		return nil
	}

	chunkMap, err := inputs.LoadChunkMap(chunkMapFile)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}

	cfg.ChunkMap = chunkMap

	return nil
}
