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

func TestUsageDocumentsTheModeFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		needle string
	}{
		{"the flag", "--mode M"},
		{"the history choice and its default", "history (default)"},
		{"the lineage choice", "lineage orders"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(usage, testCase.needle) {
				t.Fatalf("usage holds no %q", testCase.needle)
			}
		})
	}
}

func TestUsageDocumentsTheResolverFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		needle string
	}{
		{"the flag", "--resolver R"},
		{"the typed choice and its default", "typed (default)"},
		{"the lsp choice", "lsp starts"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(usage, testCase.needle) {
				t.Fatalf("usage holds no %q", testCase.needle)
			}
		})
	}
}
