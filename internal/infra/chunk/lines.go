package chunk

import (
	"slices"
	"unicode/utf16"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// splitLines returns every line of source as a Span, the trailing newline
// included in the line it ends. Concatenating the spans in order reproduces
// source byte for byte, which is what lets every chunker below build spans
// out of line boundaries without ever losing a byte to a line ending.
func splitLines(source []byte) []entities.Span {
	var lines []entities.Span

	start := 0

	for i, b := range source {
		if b == '\n' {
			lines = append(lines, entities.Span{Start: start, End: i + 1})
			start = i + 1
		}
	}

	if start < len(source) {
		lines = append(lines, entities.Span{Start: start, End: len(source)})
	}

	return lines
}

// isSpaceByte reports whether b is ASCII whitespace. go.mod, go.work, and
// sqlc query files are always gofmt-adjacent plain text, so the ASCII-only
// check unicode.IsSpace would also give here is skipped in favor of the
// direct byte comparison.
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// trimmedSpan narrows line to the byte range that excludes its surrounding
// whitespace, the same range bytes.TrimSpace(source[line.Start:line.End])
// would return, but as offsets into source instead of a copy.
func trimmedSpan(source []byte, line entities.Span) (start, end int) {
	start, end = line.Start, line.End

	for start < end && isSpaceByte(source[start]) {
		start++
	}

	for end > start && isSpaceByte(source[end-1]) {
		end--
	}

	return start, end
}

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
