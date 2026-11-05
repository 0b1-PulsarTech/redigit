package indexing

import (
	"slices"
	"unicode/utf16"
)

// LineIndex converts between the byte offsets a syntax reader works in and the
// line/character coordinates a language server speaks. Neither side can be
// used as the other: a language server reports UTF-16 code units, so a line
// holding an accented letter or an emoji offsets differently in each.
type LineIndex struct {
	source []byte
	starts []int
}

func NewLineIndex(source []byte) LineIndex {
	starts := []int{0}

	for offset, char := range source {
		if char == '\n' {
			starts = append(starts, offset+1)
		}
	}

	return LineIndex{source: source, starts: starts}
}

// LineOf reports the zero-based line holding a byte offset.
func (l LineIndex) LineOf(offset int) int {
	found, exact := slices.BinarySearch(l.starts, offset)
	if exact {
		return found
	}

	return found - 1
}

// UTF16Column reports the character offset a language server expects for a byte
// offset, counting UTF-16 code units from the start of the line. An emoji is a
// surrogate pair there and therefore counts as two.
func (l LineIndex) UTF16Column(offset int) int {
	line := l.LineOf(offset)
	if line < 0 || line >= len(l.starts) {
		return 0
	}

	return len(utf16.Encode([]rune(string(l.source[l.starts[line]:offset]))))
}

// ByteOffset converts a language server's line and UTF-16 character back into a
// byte offset. A character past the end of the line clamps to the line's end
// rather than reading into the next one.
func (l LineIndex) ByteOffset(line, char int) int {
	if line < 0 || line >= len(l.starts) {
		return -1
	}

	start := l.starts[line]

	end := len(l.source)
	if line+1 < len(l.starts) {
		end = l.starts[line+1]
	}

	units := 0

	for offset := start; offset < end; {
		if units >= char {
			return offset
		}

		size := runeSizeAt(l.source, offset)
		units += utf16Units(l.source[offset : offset+size])
		offset += size
	}

	return end
}
