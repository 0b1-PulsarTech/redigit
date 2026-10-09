package dart

import "strings"

// item is one top-level construct: a directive or a declaration. first and
// last index its tokens; start and end are the bytes Render tiles the file
// with, comments above it and the rest of its last line included.
type item struct {
	first int
	last  int
	start int
	end   int
}

func (it item) tokens(all []token) []token { return all[it.first : it.last+1] }

// splitItems cuts the token stream at top level. An item ends at a top-level
// ";", or at the "}" closing its body when no top-level "=" or "=>" came
// first: that "=" is what tells "final m = {...};" from "class A {...}".
func splitItems(source string, toks []token) []item {
	var (
		items    []item
		depth    int
		first    int
		isAssign bool
	)

	for index, tok := range toks {
		closed := false

		switch tok.text {
		case "(", "[", "{":
			depth++
		case ")", "]":
			depth--
		case "}":
			depth--
			closed = depth == 0 && !isAssign
		case "=", "=>":
			isAssign = isAssign || depth == 0
		case ";":
			closed = depth == 0
		}

		if closed {
			items = append(items, item{first: first, last: index})
			first, isAssign, depth = index+1, false, 0
		}
	}

	if first < len(toks) {
		items = append(items, item{first: first, last: len(toks) - 1})
	}

	return placeItems(source, toks, items)
}

// placeItems gives each item its byte range. Everything between two items is
// whitespace or comments, and it travels with the item below it; above the
// first item only the comment run touching it does, so a license header stays
// with the file.
func placeItems(source string, toks []token, items []item) []item {
	prevEnd := 0

	for index := range items {
		it := &items[index]
		begin := toks[it.first].start

		if index == 0 {
			it.start = attachedCommentStart(source, begin)
		} else {
			it.start = max(prevEnd, lineStart(source, nextNonSpace(source, prevEnd)))
		}

		it.end = lineEnd(source, toks[it.last].end)
		prevEnd = it.end
	}

	return items
}

// lineEnd extends past a trailing line comment and the newline.
func lineEnd(source string, pos int) int {
	rest := source[pos:]
	cut := strings.IndexByte(rest, '\n')

	if cut < 0 {
		if strings.TrimSpace(rest) == "" || strings.HasPrefix(strings.TrimSpace(rest), "//") {
			return len(source)
		}

		return pos
	}

	line := strings.TrimSpace(rest[:cut])
	if line == "" || strings.HasPrefix(line, "//") {
		return pos + cut + 1
	}

	return pos
}

func lineStart(source string, pos int) int {
	return strings.LastIndexByte(source[:pos], '\n') + 1
}

func nextNonSpace(source string, pos int) int {
	for pos < len(source) && isSpace(source[pos]) {
		pos++
	}

	return pos
}

// attachedCommentStart walks up from the line holding pos over comment lines
// with no blank line between them and it.
func attachedCommentStart(source string, pos int) int {
	start := lineStart(source, pos)

	for start > 0 {
		above := lineStart(source, start-1)
		line := strings.TrimSpace(source[above : start-1])

		if !isCommentLine(line) {
			break
		}

		start = above
	}

	return start
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*")
}
