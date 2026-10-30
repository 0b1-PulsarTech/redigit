package bootstrap

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// ErrChunkMapUnknownPath reports a chunk map entry naming a path outside the
// tree being read. A wrong map is worse than no map, and a path that is not
// there at all is the plainest way a map can be wrong.
var ErrChunkMapUnknownPath = errors.New("chunk map names a path outside the tree")

// ErrChunkMapConflict reports a chunk map entry naming a path a language pack
// already claims structurally. The map only refines the opaque pack's
// whole-file default (docs/PLANO-EMISSAO.md:113-133); it does not override a
// pack that already reads the format.
var ErrChunkMapConflict = errors.New("chunk map path is already claimed by a language pack")

// validateChunkMap refuses a chunk map naming a path outside snap, or a path
// a specific (non-opaque) language pack already claims - both cross what the
// tree holds and what every OTHER pack owns, answerable only here.
// chunk.MapApplier already enforces non-overlap and in-range spans on its own
// (internal/infra/chunk/chunkmap.go).
func validateChunkMap(
	chunkMap entities.ChunkMap,
	snap entities.Snapshot,
	specific []indexing.Language,
) error {
	files := make(map[string]bool, len(snap.Files))
	for _, file := range snap.Files {
		files[file] = true
	}

	for _, path := range slices.Sorted(maps.Keys(chunkMap)) {
		if !files[path] {
			return fmt.Errorf("%w: %q", ErrChunkMapUnknownPath, path)
		}

		if lang, claimed := ownerOf(specific, path); claimed {
			return fmt.Errorf("%w: %q by the %s pack", ErrChunkMapConflict, path, lang.Name)
		}
	}

	return nil
}

// ownerOf reports the first specific language pack that owns path.
func ownerOf(specific []indexing.Language, path string) (indexing.Language, bool) {
	for _, lang := range specific {
		if lang.Owns(path) {
			return lang, true
		}
	}

	return indexing.Language{}, false
}
