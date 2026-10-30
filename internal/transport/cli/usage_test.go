package cli

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRunUsageFitsAnEightyColumnTerminal(t *testing.T) {
	t.Parallel()

	const columns = 80

	for number, line := range strings.Split(runUsage, "\n") {
		if width := utf8.RuneCountInString(line); width > columns {
			t.Errorf("line %d is %d columns, want at most %d: %q", number+1, width, columns, line)
		}
	}
}
