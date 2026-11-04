package subjecting

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func Mark(message string) string {
	folded, rest := splitSubject(message)

	marked := entities.MarkWIP(folded)
	if marked == folded {
		return message
	}

	return marked + rest
}

func Unmark(message string) string {
	folded, rest := splitSubject(message)

	unmarked := entities.UnmarkWIP(folded)
	if unmarked == folded {
		return message
	}

	return unmarked + rest
}

func gitSubject(message string) string {
	folded, _ := splitSubject(message)

	return folded
}

func splitSubject(message string) (folded, rest string) {
	// Git shows the lines up to the first blank one, joined by single spaces, as %s.
	lines := strings.Split(message, "\n")
	parts := make([]string, 0, len(lines))

	for idx, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed != "" {
			parts = append(parts, trimmed)

			continue
		}

		rest = strings.Join(lines[idx:], "\n")
		if idx > 0 {
			rest = "\n" + rest
		}

		break
	}

	return strings.Join(parts, " "), rest
}
