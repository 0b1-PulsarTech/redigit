package languages

import (
	"context"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/dart"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/web"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

// ConfigExtensions are the files the config pack reads. The first four carry
// a chunker that splits them; the rest reach the tree as one whole-file piece.
// The empty string claims a file with NO extension - a Dockerfile, a LICENSE -
// which extension-keyed ownership would leave out with nothing reporting it.
func ConfigExtensions() []string {
	return slices.Clone([]string{
		".mod", ".work", ".sum", ".sql",
		".md", ".yml", ".yaml", ".xml", ".json", ".toml", ".nix", ".lock",
		".env", ".iml", ".mmd", ".gitignore", ".containerignore", "",
	})
}

// FinalTree is what the Dart and web packs resolve an import against: which
// files the tree holds, how to read one, and where each virtual module a
// bundler plugin makes up is compiled from.
type FinalTree struct {
	Files   []string
	Reader  TreeReader
	Virtual map[string]string
}

type TreeReader interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// SpecificLanguages is what the indexer reads a known format with. The config
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
func SpecificLanguages(
	configSyntax config.Syntax,
	tree FinalTree,
	source *typed.Source,
	slice sliceconf.Config,
) []indexing.Language {
	return []indexing.Language{
		{
			Name:       GoLanguage,
			Extensions: []string{".go"},
			Syntax:     golang.NewSyntaxWith(slice),
			Rules:      golang.NewRulePackWith(source),
			Resolved:   true,
		},
		{
			Name:       DartLanguage,
			Extensions: []string{".dart"},
			Syntax:     dart.NewSyntax(tree.Files, tree.Reader),
			Rules:      dart.NewRulePack(),
		},
		{
			Name:       WebLanguage,
			Extensions: web.Extensions(),
			Syntax:     web.NewSyntax(tree.Files, tree.Reader),
			Rules:      web.NewRulePack(tree.Virtual),
		},
		{
			Name:       ConfigLanguage,
			Extensions: ConfigExtensions(),
			Syntax:     configSyntax,
			Rules:      config.NewRulePack(),
		},
	}
}

// OpaqueLanguage claims every final-tree path neither "go" nor "config" owns.
// It shares configSyntax's own chunker list, so it reads such a path
// whole-file by default (chunk.WholeFile) unless a chunk map entry refines
// the cut - see docs/PLANO-EMISSAO.md:113-133. Without it, a format this tool
// has no reader for - .tmpl, a golden fixture - vanishes from the graph
// in silence and a plan built from this run can never reproduce it.
func OpaqueLanguage(configSyntax config.Syntax) indexing.Language {
	return indexing.Language{
		Name:       OpaqueName,
		Extensions: []string{indexing.AnyExtension},
		Syntax:     configSyntax,
	}
}
