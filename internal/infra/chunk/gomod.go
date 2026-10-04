package chunk

import (
	"context"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// goModFileName is the base name a go.mod always has, factored out because
// goconst counts its occurrences across this package's test files too.
const goModFileName = "go.mod"

// GoMod splits a go.mod into one Decl per require/replace line, plus one
// header Decl for the module/go/toolchain directives. A module cannot
// require a sibling that does not exist yet, so each line is dead weight
// until its own module does - the same reasoning that already governs
// splitting a query file by "-- name:" block.
type GoMod struct{}

var _ indexing.Chunker = GoMod{}

func (GoMod) Name() string { return "go-mod" }

func (GoMod) Handles(path string) bool { return filepath.Base(path) == goModFileName }

func (GoMod) Chunks(_ context.Context, path string, source []byte) ([]entities.Decl, error) {
	lines := splitLines(source)
	unit := entities.UnitID(filepath.Dir(path))

	entries := mergeDirectiveEntries(
		scanDirective(source, lines, requireDirective),
		scanDirective(source, lines, replaceDirective),
	)

	header := moduleDirectiveEntry(source, lines)
	starts := entryStarts(entries, len(source))

	decls := make([]entities.Decl, 0, len(entries)+1)
	decls = append(decls, entities.Decl{
		Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: header.Name},
		Unit:     unit,
		Atom:     workspaceAtom(filepath.Dir(path)),
		Span:     entities.Span{Start: 0, End: starts[0]},
		Selector: locationAt(path, source, lines, header.NameOffset),
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

// moduleDirectiveEntry returns the file's own "module" directive, which is
// what a require/replace line elsewhere names when it points back at this
// module. fileDeclName stands in for a go.mod with no module directive, a
// file too malformed to build anyway.
func moduleDirectiveEntry(source []byte, lines []entities.Span) directiveEntry {
	for _, entry := range scanDirective(source, lines, "module") {
		return entry
	}

	return directiveEntry{Name: fileDeclName}
}

// mergeDirectiveEntries interleaves two already-ordered entry lists back into
// file order, since require and replace lines can appear in either order or
// alternate between single lines and blocks.
func mergeDirectiveEntries(a, b []directiveEntry) []directiveEntry {
	merged := make([]directiveEntry, 0, len(a)+len(b))

	for len(a) > 0 || len(b) > 0 {
		switch {
		case len(b) == 0 || (len(a) > 0 && a[0].LineStart < b[0].LineStart):
			merged = append(merged, a[0])
			a = a[1:]
		default:
			merged = append(merged, b[0])
			b = b[1:]
		}
	}

	return merged
}

// entryStarts returns one more offset than len(entries): starts[i] is where
// entry i's Decl span begins, and starts[len(entries)] is where the last one
// ends. Each entry's span reaches forward to the next entry's line, so a
// comment or blank line between two require lines is not left uncovered.
func entryStarts(entries []directiveEntry, sourceLen int) []int {
	starts := make([]int, len(entries)+1)
	for i, entry := range entries {
		starts[i] = entry.LineStart
	}

	starts[len(entries)] = sourceLen

	return starts
}
