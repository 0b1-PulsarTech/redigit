package entities

import "strings"

// DraftMarker prefixes every subject a planner invents: a generated subject is
// evidence for a reviewer, not a commit message. The previous rebuild ended up
// with 561 placeholders, and the ones that slipped through described work that
// had moved elsewhere.
const DraftMarker = "DRAFT: "

// IsDraft reports whether a subject still needs a human or an agent to write it.
func IsDraft(subject string) bool { return strings.HasPrefix(subject, DraftMarker) }

// WIPMarker declares a commit leaving a piece incomplete: a type without all
// its methods, a declaration reaching for something unborn. Only the final tree
// must compile, and unmarked, an honest vertical slice is indistinguishable
// from a defect - which reported 311 incompletenesses as 311 failures.
const WIPMarker = "[wip] "

func IsWIPSubject(subject string) bool {
	_, _, isMarked := splitWIP(subject)

	return isMarked
}

func MarkWIP(subject string) string {
	head, rest, _ := splitWIP(subject)

	return head + WIPMarker + rest
}

func UnmarkWIP(subject string) string {
	head, rest, _ := splitWIP(subject)

	return head + rest
}

func splitWIP(subject string) (head, rest string, isMarked bool) {
	rest = subject
	if IsDraft(subject) {
		head, rest = DraftMarker, subject[len(DraftMarker):]
	}

	// A marker ahead of the type is how older code wrote it: read as marked, moved to the slot.
	for _, measure := range []func(string) int{emojiLength, typeLength} {
		rest = skipWIP(rest)
		size := measure(rest)
		head, rest = head+rest[:size], rest[size:]
	}

	rest = skipWIP(rest)

	return head, rest, len(head)+len(rest) < len(subject)
}

func skipWIP(text string) string {
	after, hasMarker := strings.CutPrefix(text, WIPMarker)
	if !hasMarker {
		return text
	}

	return skipWIP(after)
}

func emojiLength(text string) int {
	code, _, isCode := strings.Cut(text, ": ")
	if !isCode || len(code) < 2 || code[0] != ':' ||
		strings.Trim(code[1:], "abcdefghijklmnopqrstuvwxyz0123456789_+-") != "" {
		return 0
	}

	return len(code) + len(": ")
}

func typeLength(text string) int {
	line, _, _ := strings.Cut(text, "\n")
	rest := strings.TrimLeft(line, "abcdefghijklmnopqrstuvwxyz")

	if rest == line {
		return 0
	}

	if scope, isScoped := strings.CutPrefix(rest, "("); isScoped {
		inner, after, _ := strings.Cut(scope, ")")
		if inner == "" || strings.Contains(inner, "(") {
			return 0
		}

		rest = after
	}

	if !strings.HasPrefix(rest, ": ") {
		return 0
	}

	return len(line) - len(rest) + len(": ")
}
