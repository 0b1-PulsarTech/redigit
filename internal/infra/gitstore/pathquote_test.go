package gitstore

import "testing"

func TestQuotePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want string
	}{
		{"plain path unchanged", "internal/foo/bar.go", "internal/foo/bar.go"},
		{"empty path unchanged", "", ""},
		{"space forces quoting", "dir with space/b.go", `"dir with space/b.go"`},
		{"embedded quote is escaped", `a"b.go`, `"a\"b.go"`},
		{"embedded backslash is escaped", `a\b.go`, `"a\\b.go"`},
		{"control byte is octal escaped", "a\tb.go", `"a\011b.go"`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := quotePath(testCase.path); got != testCase.want {
				t.Fatalf("quotePath(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}
