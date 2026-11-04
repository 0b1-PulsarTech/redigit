package anchorprobe

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
)

func apply(
	ctx context.Context,
	renderer Renderer,
	dir string,
	edits []anchoring.Edit,
) (func() error, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open tree: %w", err)
	}

	byFile := map[string][]anchoring.Edit{}
	for _, edit := range edits {
		byFile[edit.File] = append(byFile[edit.File], edit)
	}

	originals := map[string][]byte{}
	restore := func() error {
		var failure error

		for file, original := range originals {
			failure = errors.Join(failure, root.WriteFile(file, original, filePerm))
		}

		return errors.Join(failure, root.Close())
	}

	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		group := byFile[file]

		if !filepath.IsLocal(file) {
			return nil, errors.Join(fmt.Errorf("%w: %s leaves the tree", ErrEdit, file), restore())
		}

		if err = rewrite(ctx, renderer, root, group, originals); err != nil {
			return nil, errors.Join(err, restore())
		}
	}

	return restore, nil
}

func rewrite(
	ctx context.Context,
	renderer Renderer,
	root *os.Root,
	group []anchoring.Edit,
	originals map[string][]byte,
) error {
	file := group[0].File

	original, err := root.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	pieces := make([]emitting.Piece, 0, len(group))
	for _, edit := range group {
		pieces = append(
			pieces,
			emitting.Piece{Span: edit.Span, Text: edit.Text, Imports: edit.Imports},
		)
	}

	// The emitter renders every file, dropping the imports nothing uses any more.
	changed, err := emitting.Compose(ctx, renderer, emitting.Layout{
		File: file, Source: original, Pieces: pieces, Keep: group[0].Keep,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnfit, err)
	}

	originals[file] = original

	if err = root.WriteFile(file, changed, filePerm); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}

	return nil
}
