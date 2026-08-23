package chunk

import (
	"bytes"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// directiveEntry is one directive line: "require X v1" standing alone, or
// one line of a "require (...)" block. LineStart is where its Decl span
// begins; NameOffset is where the name itself sits, for the Selector.
type directiveEntry struct {
	Directive  string
	Name       string
	LineStart  int
	NameOffset int
}

// DeclName carries the directive as well as the module path, because a
// workspace module both requires a sibling and replaces it with a local
// path: the path alone gives those two lines one DeclKey, and a map keyed by
// DeclKey keeps only one of them.
func (e directiveEntry) DeclName() string { return e.Directive + " " + e.Name }

// scanDirective finds every entry for one go.mod/go.work directive keyword
// ("require", "replace", "use"), in both its single-line and parenthesized
// block forms. A blank or comment-only line inside a block, or a line
// commented out with "//", contributes no entry - which is what keeps a
// "// use ./not/really" line from becoming a phantom Decl.
func scanDirective(source []byte, lines []entities.Span, keyword string) []directiveEntry {
	var entries []directiveEntry

	blockOpen := keyword + " ("
	inBlock := false

	for _, line := range lines {
		start, end := trimmedSpan(source, line)
		trimmed := source[start:end]

		switch {
		case inBlock && string(trimmed) == ")":
			inBlock = false
		case inBlock:
			if entry, ok := directiveField(source, keyword, line, start, end); ok {
				entries = append(entries, entry)
			}
		case string(trimmed) == blockOpen:
			inBlock = true
		case hasKeywordPrefix(trimmed, keyword):
			if entry, ok := directiveField(source, keyword, line, start+len(keyword)+1, end); ok {
				entries = append(entries, entry)
			}
		}
	}

	return entries
}

func hasKeywordPrefix(trimmed []byte, keyword string) bool {
	return len(trimmed) > len(keyword) &&
		string(trimmed[:len(keyword)]) == keyword &&
		isSpaceByte(trimmed[len(keyword)])
}

func directiveField(
	source []byte,
	keyword string,
	line entities.Span,
	from, to int,
) (directiveEntry, bool) {
	name, offset, ok := firstField(source, from, to)
	if !ok {
		return directiveEntry{}, false
	}

	return directiveEntry{
		Directive:  keyword,
		Name:       name,
		LineStart:  line.Start,
		NameOffset: offset,
	}, true
}

// firstField returns the first whitespace-delimited token in source[base:limit]
// and its byte offset, or found=false when that range is blank or starts with
// a "//" comment.
func firstField(source []byte, base, limit int) (name string, offset int, found bool) {
	field := source[base:limit]

	start := 0
	for start < len(field) && isSpaceByte(field[start]) {
		start++
	}

	if start == len(field) {
		return "", 0, false
	}

	field = field[start:]
	if bytes.HasPrefix(field, []byte("//")) {
		return "", 0, false
	}

	end := 0
	for end < len(field) && !isSpaceByte(field[end]) {
		end++
	}

	return string(field[:end]), base + start, true
}
