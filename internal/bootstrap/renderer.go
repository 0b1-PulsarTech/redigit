package bootstrap

import (
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// renderer is what byExtension dispatches to: the same Render the indexing
// Syntax port declares, named here for the one method the emitter needs.
type renderer interface {
	Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error)
}

// byExtension picks a renderer by the file being written. One for the whole
// tree was enough only while the tree was Go: a go.mod rendered by the Go
// syntax comes back empty, and the manifest still carries that blob.
type byExtension struct {
	fallback renderer
	byExt    map[string]renderer
}

var _ emitting.Renderer = byExtension{}

func newRenderer(fallback renderer, byExt map[string]renderer) byExtension {
	return byExtension{fallback: fallback, byExt: byExt}
}

// Render dispatches on the extension of the file the kept declarations belong
// to. Every key in one call names the same file, which is how the emitter
// calls it: one render per path per commit.
func (r byExtension) Render(
	ctx context.Context,
	source []byte,
	keep []entities.DeclKey,
) ([]byte, error) {
	chosen, ext := r.fallback, ""

	if len(keep) > 0 {
		ext = path.Ext(keep[0].File)

		if byExt, found := r.byExt[ext]; found {
			chosen = byExt
		}
	}

	out, err := chosen.Render(ctx, source, keep)
	if err != nil {
		return nil, fmt.Errorf("render a %q file: %w", ext, err)
	}

	return out, nil
}

// configExtensions are the files the config pack reads. The first four carry
// a chunker that splits them; the rest reach the tree as one whole-file piece.
// The empty string claims a file with NO extension - a Dockerfile, a LICENSE -
// which extension-keyed ownership would leave out with nothing reporting it.
func configExtensions() []string {
	return slices.Clone([]string{
		".mod", ".work", ".sum", ".sql",
		".md", ".yml", ".yaml", ".xml", ".json", ".toml", ".nix", ".lock",
		".env", ".iml", ".mmd", ".gitignore", ".containerignore", "",
	})
}

// renderersByExtension is byExtension's dispatch table: each extension a
// language claims reads with that language's Syntax. configSyntax is
// byExtension's fallback and the opaque pack shares it, so a path no language
// lists still renders through the Syntax it was indexed with, never through
// the Go syntax.
func renderersByExtension(languages []indexing.Language) map[string]renderer {
	out := map[string]renderer{}

	for _, language := range languages {
		for _, ext := range language.Extensions {
			out[ext] = language.Syntax
		}
	}

	return out
}
