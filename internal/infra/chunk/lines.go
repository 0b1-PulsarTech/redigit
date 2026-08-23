package chunk

import "github.com/wrapped-owls/redigit/internal/domain/entities"

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
