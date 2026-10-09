package dart

import (
	"slices"
	"testing"
)

func TestLex(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		source string
		want   []string
	}

	testCases := []testCase{
		{
			"skips line and nested block comments",
			"a /* x /* y */ z */ b // c\nd",
			[]string{"a", "b", "d"},
		},
		{
			"keeps multi-character operators whole",
			"a ??= b?.c => d...e",
			[]string{"a", "??=", "b", "?.", "c", "=>", "d", "...", "e"},
		},
		{"reads a raw string as one token", `r'$x' y`, []string{`r'$x'`, "y"}},
		{
			"reads a triple-quoted string across lines",
			"'''a\n'b'\n''' c",
			[]string{"'''a\n'b'\n'''", "c"},
		},
		{"reads a fraction as one number", "12.5.abs()", []string{"12.5", ".", "abs", "(", ")"}},
		{"reads a non-ASCII identifier", "café = 1", []string{"café", "=", "1"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := texts(lex(tc.source)); !slices.Equal(got, tc.want) {
				t.Fatalf("lex() = %q, want %q", got, tc.want)
			}
		})
	}
}
