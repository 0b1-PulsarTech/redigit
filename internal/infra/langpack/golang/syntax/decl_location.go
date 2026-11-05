package syntax

import (
	"go/token"
	"unicode/utf16"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func spanOf(fset *token.FileSet, start, end token.Pos) entities.Span {
	return entities.Span{
		Start: fset.Position(start).Offset,
		End:   fset.Position(end).Offset,
	}
}

func LocationOf(
	fset *token.FileSet,
	path string,
	source []byte,
	pos token.Pos,
) entities.Location {
	position := fset.PositionFor(pos, false)
	start := position.Offset - (position.Column - 1)

	return entities.Location{
		File: path,
		Line: position.Line - 1,
		// token Column counts bytes while LSP Char counts UTF-16 units.
		Char: utf16Units(source[start:position.Offset]),
	}
}

func utf16Units(encoded []byte) int {
	return len(utf16.Encode([]rune(string(encoded))))
}
