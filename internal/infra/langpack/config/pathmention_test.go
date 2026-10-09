package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A directory mentioned as a path names its code; one glued to a longer name
// or to a longer path does not.
func TestNames(t *testing.T) {
	t.Parallel()

	const target = "libs/engines"

	dirs := map[string][]entities.DeclKey{"libs/engines/ruleflow": nil}

	testCases := []struct {
		name string
		text string
		want bool
	}{
		{"in prose", "see libs/engines for more", true},
		{"in backticks", "`libs/engines`", true},
		{"a relative path", "open ./libs/engines", true},
		{"a parent-relative path", "open ../libs/engines/", true},
		{"a file below it", "open libs/engines/doc.go", true},
		{"a directory below it is its own mention", "open libs/engines/ruleflow", false},
		{"at the end of the text", "all in libs/engines", true},
		{"a sentence end", "it lives in libs/engines.", true},
		{"a longer last segment", "libs/engines2 is another", false},
		{"a hyphenated last segment", "libs/engines-old", false},
		{"the tail of a longer path", "vendor/libs/engines", false},
		{"glued to a word", "xlibs/engines", false},
		{"found at the second occurrence", "xlibs/engines and libs/engines", true},
		{"absent", "nothing here", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := names(testCase.text, target, dirs); got != testCase.want {
				t.Errorf("names(%q) = %v, want %v", testCase.text, got, testCase.want)
			}
		})
	}
}
