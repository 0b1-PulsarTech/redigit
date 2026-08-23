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

// configExtensions are the files the config pack reads. Extension is how the
// indexer decides ownership, so a file with none is not covered here.
func configExtensions() []string {
	return slices.Clone([]string{".mod", ".work", ".sum", ".sql"})
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
