package cli

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

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
