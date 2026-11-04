package emitting

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// sourceFile is a path's final content and mode, read once and reused across
// every commit that touches the path.
type sourceFile struct {
	content []byte
	mode    FileMode
}

// renderTouched renders and stores one blob per path touched by this commit.
// born carries every declaration born up to and including this commit, so a
// re-render of a file already seen produces its FULL content at this point,
// never a diff.
func (e Emitter) renderTouched(
	ctx context.Context,
	touched map[string]bool,
	born map[string][]entities.DeclKey,
	cache map[string]sourceFile,
) ([]Blob, error) {
	paths := make([]string, 0, len(touched))
	for path := range touched {
		paths = append(paths, path)
	}

	slices.SortFunc(paths, cmp.Compare)

	blobs := make([]Blob, 0, len(paths))

	for _, path := range paths {
		if e.derived.owns(path) {
			continue
		}

		file, cached := cache[path]
		if !cached {
			content, mode, err := e.source.Read(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			file = sourceFile{content: content, mode: mode}
			cache[path] = file
		}

		pieces, err := e.versions.pieces(born[path])
		if err != nil {
			return nil, fmt.Errorf("splice %s: %w", path, err)
		}

		rendered, err := Compose(ctx, e.renderer, Layout{
			File: path, Source: file.content, Pieces: pieces, Keep: born[path],
		})
		if err != nil {
			return nil, err
		}

		// A file whose declarations are all still unborn renders as nothing,
		// and writing it would leave only a package clause and imports.
		// A file the final tree holds EMPTY renders as nothing too, and that
		// one must be written: git tracks it, so skipping leaves the tree
		// short by one path.
		if len(rendered) == 0 && len(file.content) > 0 {
			continue
		}

		handle, err := e.sink.Write(ctx, path, rendered, file.mode)
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}

		blobs = append(blobs, Blob{Path: path, Mode: file.mode, Handle: handle})
	}

	return blobs, nil
}
