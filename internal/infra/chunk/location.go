package chunk

import (
	"slices"
	"unicode/utf16"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// locationAt converts a byte offset into source into the coordinates a
// language server speaks: Line is 0-based, and Char counts UTF-16 code units
// from the start of the line, not bytes - the same conversion
// internal/infra/langpack/golang/decl.go's locationOf does for Go source,
// done here for text this package's own line scanning already has spans for.
func locationAt(path string, source []byte, lines []entities.Span, offset int) entities.Location {
	idx, found := slices.BinarySearchFunc(lines, offset, func(line entities.Span, target int) int {
		switch {
		case line.End <= target:
			return -1
		case line.Start > target:
			return 1
		default:
			return 0
		}
	})
	if !found {
		return entities.Location{File: path}
	}

	line := lines[idx]

	return entities.Location{
		File: path,
		Line: idx,
		Char: utf16Units(source[line.Start:offset]),
	}
}

// utf16Units counts the UTF-16 code units a run of bytes encodes to. An emoji
// is a surrogate pair there and therefore counts as two.
func utf16Units(encoded []byte) int {
	return len(utf16.Encode([]rune(string(encoded))))
}
