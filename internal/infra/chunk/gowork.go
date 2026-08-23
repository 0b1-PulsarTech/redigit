package chunk

import (
	"context"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// goWorkFileName is the base name a go.work always has, factored out because
// goconst counts its three occurrences across this package's test files too.
const goWorkFileName = "go.work"

// GoWork splits a go.work into one Decl per "use" line, plus one header Decl
// for the "go" directive. A workspace lists every module directory from its
// first commit while the go.mod files that make them real arrive across
// hundreds of commits, so every "use" line needs a birth of its own too.
type GoWork struct{}

var _ indexing.Chunker = GoWork{}

func (GoWork) Name() string { return "go-work" }

func (GoWork) Handles(path string) bool { return filepath.Base(path) == goWorkFileName }

func (GoWork) Chunks(_ context.Context, path string, source []byte) ([]entities.Decl, error) {
	lines := splitLines(source)
	unit := entities.UnitID(filepath.Dir(path))

	entries := scanDirective(source, lines, "use")
	starts := entryStarts(entries, len(source))

	decls := make([]entities.Decl, 0, len(entries)+1)
	decls = append(decls, entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: fileDeclName},
		Unit:     unit,
		Span:     entities.Span{Start: 0, End: starts[0]},
		Selector: locationAt(path, source, lines, 0),
	})

	for i, entry := range entries {
		decls = append(decls, entities.Decl{
			Key: entities.DeclKey{
				File: path,
				Kind: entities.KindOther,
				Name: entry.DeclName(),
			},
			Unit:     unit,
			Span:     entities.Span{Start: starts[i], End: starts[i+1]},
			Selector: locationAt(path, source, lines, entry.NameOffset),
		})
	}

	return decls, nil
}
