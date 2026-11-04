package bootstrap

import (
	"context"
	"fmt"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/dart"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/web"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
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

// finalTree is what the Dart and web packs resolve an import against: which
// files the tree holds, how to read one, and where each virtual module a
// bundler plugin makes up is compiled from.
type finalTree struct {
	files   []string
	reader  treeReader
	virtual map[string]string
}

type treeReader interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// specificLanguages is what the indexer reads a known format with. The config
// entry covers the files a build needs and no language server reads: without
// them a plan holds no go.mod, so every tree it describes has source and
// nothing to compile it.
//
// Kept apart from opaqueLanguage so validateChunkMap can check a supplied
// chunk map against real ownership before the catch-all pack - which would
// otherwise also claim every path - is added.
//
// Dart and the web files are not Resolved: the one language server a run
// starts is gopls, so their rule packs carry the reference edges themselves.
func specificLanguages(
	configSyntax config.Syntax,
	tree finalTree,
	source *typed.Source,
	slice sliceconf.Config,
) []indexing.Language {
	return []indexing.Language{
		{
			Name:       goLanguage,
			Extensions: []string{".go"},
			Syntax:     golang.NewSyntaxWith(slice),
			Rules:      golang.NewRulePackWith(source),
			Resolved:   true,
		},
		{
			Name:       dartLanguage,
			Extensions: []string{".dart"},
			Syntax:     dart.NewSyntax(tree.files, tree.reader),
			Rules:      dart.NewRulePack(),
		},
		{
			Name:       webLanguage,
			Extensions: web.Extensions(),
			Syntax:     web.NewSyntax(tree.files, tree.reader),
			Rules:      web.NewRulePack(tree.virtual),
		},
		{
			Name:       configLanguage,
			Extensions: configExtensions(),
			Syntax:     configSyntax,
			Rules:      config.NewRulePack(),
		},
	}
}

// opaqueLanguage claims every final-tree path neither "go" nor "config" owns.
// It shares configSyntax's own chunker list, so it reads such a path
// whole-file by default (chunk.WholeFile) unless a chunk map entry refines
// the cut - see docs/PLANO-EMISSAO.md:113-133. Without it, a format this tool
// has no reader for - .tmpl, a golden fixture - vanishes from the graph
// in silence and a plan built from this run can never reproduce it.
func opaqueLanguage(configSyntax config.Syntax) indexing.Language {
	return indexing.Language{
		Name:       opaqueName,
		Extensions: []string{indexing.AnyExtension},
		Syntax:     configSyntax,
	}
}

func decodeSlice(cfg Config) (sliceconf.Config, error) {
	section, err := sliceconf.Decode(cfg.Sections.Slice)
	if err != nil {
		return sliceconf.Config{}, fileError(cfg.ConfigPath, err)
	}

	return section, nil
}
