package subjecting

import (
	"strings"
	"unicode/utf8"
)

func prefixLength(line string) (int, bool) {
	rest, hasColon := strings.CutPrefix(line, ":")
	emoji, afterEmoji, hasEmoji := strings.Cut(rest, ": ")

	if !hasColon || !hasEmoji || emoji == "" || strings.ContainsFunc(emoji, isNotSlug) {
		return 0, false
	}

	head, description, hasType := strings.Cut(afterEmoji, ": ")
	if !hasType || strings.TrimSpace(description) == "" || !isTypeAndScope(head) {
		return 0, false
	}

	return utf8.RuneCountInString(line) - utf8.RuneCountInString(description), true
}

func isTypeAndScope(head string) bool {
	kind, scope, hasScope := strings.Cut(head, "(")
	if hasScope {
		inner, isClosed := strings.CutSuffix(scope, ")")
		if !isClosed || inner == "" || strings.ContainsAny(inner, "()") {
			return false
		}
	}

	return kind != "" && !strings.ContainsFunc(kind, func(char rune) bool {
		return char < 'a' || char > 'z'
	})
}

func isNotSlug(char rune) bool {
	return (char < 'a' || char > 'z') && (char < '0' || char > '9') &&
		char != '_' && char != '+' && char != '-'
}
