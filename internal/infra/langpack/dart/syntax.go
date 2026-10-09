package dart

import (
	"context"
	"path"
	"strings"
	"unicode/utf16"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// Tree reads a path of the final tree. Render needs it because which imports
// a partial file still uses depends on what each imported library declares.
type Tree interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// Syntax implements indexing.Syntax for Dart with a lexer of its own: there
// is no Dart parser in Go, and the analyzer is a Dart program.
type Syntax struct {
	space *workspace
}

var _ indexing.Syntax = Syntax{}

// NewSyntax reads declarations with no context at all; files and tree are
// only what Render resolves imports against.
func NewSyntax(files []string, tree Tree) Syntax {
	return Syntax{
		space: newWorkspace(files, func(ctx context.Context, file string) ([]byte, error) {
			return tree.Read(ctx, file)
		}),
	}
}

// fileDeclName stands for a file declaring nothing, as in the Go pack: such a
// file still has to be written, and only a declaration gets a commit.
const fileDeclName = "<file>"

func (Syntax) Declarations(
	_ context.Context,
	file string,
	source []byte,
) ([]entities.Decl, error) {
	parsed := parseFile(file, string(source))
	unit := entities.UnitID(path.Dir(file))
	isTestFile := strings.HasSuffix(file, "_test.dart")
	atom := partAtom(parsed)

	if atom == "" && isGenerated(parsed.source) {
		atom = file
	}

	decls := make([]entities.Decl, 0, len(parsed.decls()))

	for _, it := range parsed.decls() {
		decls = append(decls, entities.Decl{
			Key:      it.key,
			Unit:     unit,
			Span:     entities.Span{Start: it.start, End: parsed.toks[it.last].end},
			Selector: parsed.selector(it),
			IsTest:   isTestFile && it.key.Kind == entities.KindFunc && it.key.Name == "main",
			Atom:     atom,
		})
	}

	if len(decls) > 0 {
		return decls, nil
	}

	return []entities.Decl{{
		Key:      entities.DeclKey{File: file, Kind: entities.KindOther, Name: fileDeclName},
		Unit:     unit,
		Span:     entities.Span{Start: 0, End: len(source)},
		Selector: entities.Location{File: file},
	}}, nil
}

// selector is where a server would be asked about the item: its name, or its
// first token when it has none.
func (f parsedFile) selector(it parsedItem) entities.Location {
	at := it.first
	if it.nameAt >= 0 {
		at += it.nameAt
	}

	offset := f.toks[at].start
	start := lineStart(f.source, offset)

	return entities.Location{
		File: f.path,
		Line: strings.Count(f.source[:start], "\n"),
		Char: len(utf16.Encode([]rune(f.source[start:offset]))),
	}
}

// isGenerated follows the Go convention dart's own generators also write.
func isGenerated(source string) bool {
	head, _, _ := strings.Cut(source, "\n\n")

	return strings.Contains(head, "GENERATED CODE") || strings.Contains(head, "DO NOT EDIT")
}

// partAtom makes a library and its part files one atom, named after the
// library: a part file names its library and the library names the part, so
// neither compiles alone.
func partAtom(f parsedFile) string {
	for _, it := range f.items {
		own := it.tokens(f.toks)

		switch {
		case it.kind == itemPart:
			return f.path
		case it.kind == itemFrame && len(own) > 2 && own[0].text == "part" && own[1].text == "of":
			if own[2].kind == tokString {
				return path.Join(path.Dir(f.path), own[2].value)
			}

			return f.path
		}
	}

	return ""
}
