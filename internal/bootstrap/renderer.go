package bootstrap

import (
	"context"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
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
	if len(keep) == 0 {
		return r.fallback.Render(ctx, source, keep)
	}

	if chosen, found := r.byExt[path.Ext(keep[0].File)]; found {
		return chosen.Render(ctx, source, keep)
	}

	return r.fallback.Render(ctx, source, keep)
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

// configRenderers maps every extension the config pack owns to it, so the
// dispatch table and the language registration can never disagree about which
// files it covers.
func configRenderers(syntax renderer) map[string]renderer {
	out := make(map[string]renderer, len(configExtensions()))
	for _, ext := range configExtensions() {
		out[ext] = syntax
	}

	return out
}

// renderersByExtension is byExtension's dispatch table: ".go" reads with
// goSyntax, every extension the config pack owns reads with configSyntax, and
// configSyntax is also passed as byExtension's fallback - the opaque pack
// shares configSyntax too, so a path neither list names still renders through
// the Syntax it was indexed with, never through the Go syntax.
func renderersByExtension(goSyntax, configSyntax renderer) map[string]renderer {
	out := configRenderers(configSyntax)
	out[".go"] = goSyntax

	return out
}
