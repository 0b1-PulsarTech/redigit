package chunk

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSplitLines(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		want   []entities.Span
	}{
		{"empty", "", nil},
		{"single line no newline", "abc", []entities.Span{{Start: 0, End: 3}}},
		{
			"two lines",
			"abc\ndef\n",
			[]entities.Span{{Start: 0, End: 4}, {Start: 4, End: 8}},
		},
		{
			"trailing line with no newline",
			"abc\ndef",
			[]entities.Span{{Start: 0, End: 4}, {Start: 4, End: 7}},
		},
		{"blank line", "\n\n", []entities.Span{{Start: 0, End: 1}, {Start: 1, End: 2}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := splitLines([]byte(testCase.source))
			if len(got) != len(testCase.want) {
				t.Fatalf("splitLines() = %v, want %v", got, testCase.want)
			}

			for i, line := range got {
				if line != testCase.want[i] {
					t.Fatalf("splitLines()[%d] = %v, want %v", i, line, testCase.want[i])
				}
			}
		})
	}
}

func TestTrimmedSpan(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		line   entities.Span
		want   string
	}{
		{"no whitespace", "abc", entities.Span{Start: 0, End: 3}, "abc"},
		{"leading and trailing spaces", "  abc  ", entities.Span{Start: 0, End: 7}, "abc"},
		{"leading tab, trailing newline", "\tabc\n", entities.Span{Start: 0, End: 5}, "abc"},
		{"all whitespace", "   \n", entities.Span{Start: 0, End: 4}, ""},
		{"empty line", "", entities.Span{Start: 0, End: 0}, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			start, end := trimmedSpan([]byte(testCase.source), testCase.line)
			if got := testCase.source[start:end]; got != testCase.want {
				t.Fatalf("trimmedSpan() = %q, want %q", got, testCase.want)
			}
		})
	}
}
