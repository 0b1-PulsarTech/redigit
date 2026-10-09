package chunk

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// Changelog cuts a CHANGELOG.md into its entries, one per level-two heading, so
// each can be born after the feature it names. Cut by lines, a changelog of
// dated fixes lands at the start of the history, describing code that does not
// exist yet. The text above the first entry travels with that entry.
type Changelog struct{}

var _ indexing.Chunker = Changelog{}

func (Changelog) Name() string { return "changelog" }

func (Changelog) Handles(path string) bool { return filepath.Base(path) == "CHANGELOG.md" }

func (Changelog) Chunks(_ context.Context, path string, source []byte) ([]entities.Decl, error) {
	var starts []int

	for _, line := range splitLines(source) {
		if bytes.HasPrefix(source[line.Start:line.End], []byte("## ")) {
			starts = append(starts, line.Start)
		}
	}

	if len(starts) <= 1 {
		return []entities.Decl{wholeFileDecl(path, source)}, nil
	}

	starts[0] = 0

	pieces := make([]entities.Decl, 0, len(starts))

	for index, start := range starts {
		end := len(source)
		if index+1 < len(starts) {
			end = starts[index+1]
		}

		name := fileDeclName
		if index > 0 {
			name = fmt.Sprintf("%s#%d", fileDeclName, index+1)
		}

		pieces = append(pieces, entities.Decl{
			Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: name},
			Unit:     entities.UnitID(filepath.Dir(path)),
			Span:     entities.Span{Start: start, End: end},
			Selector: entities.Location{File: path, Line: index},
		})
	}

	return pieces, nil
}
