package selector

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

func removedByText(preimage, final, method string, imports []string) bool {
	packages := locals(imports)

	// A selector both texts hold gives the final declaration an edge only when both name one referent.
	return selected(preimage, method, packages) && !selected(final, method, packages)
}

func selected(text, method string, packages []string) bool {
	needle := "." + method

	for from := 0; from < len(text); {
		at := strings.Index(text[from:], needle)
		if at < 0 {
			return false
		}

		at += from
		from = at + len(needle)

		if next, _ := utf8.DecodeRuneInString(text[from:]); from < len(text) && isWord(next) {
			continue
		}

		head := text[:at]
		qualifier := head[len(strings.TrimRightFunc(head, isWord)):]

		if !slices.Contains(packages, qualifier) {
			return true
		}
	}

	return false
}

func isWord(char rune) bool {
	return char == '_' || unicode.IsLetter(char) || unicode.IsDigit(char)
}
