package chunk

import (
	"testing"
)

func TestScanDirective(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		source  string
		keyword string
		want    []string
	}{
		{"single line", "require github.com/x/y v1.2.3\n", "require", []string{"github.com/x/y"}},
		{
			"block with multiple entries",
			"require (\n\tgithub.com/x/y v1.2.3\n\tgithub.com/x/z v0.1.0\n)\n",
			"require",
			[]string{"github.com/x/y", "github.com/x/z"},
		},
		{
			"mixed single lines and a block",
			"require github.com/a/a v1.0.0\n" +
				"require (\n\tgithub.com/b/b v1.0.0\n)\n" +
				"require github.com/c/c v1.0.0\n",
			"require",
			[]string{"github.com/a/a", "github.com/b/b", "github.com/c/c"},
		},
		{
			"replace with a local path",
			"replace github.com/x/y => ../y\n",
			"replace",
			[]string{"github.com/x/y"},
		},
		{
			"commented-out entry inside a block contributes nothing",
			"use (\n\t./apps/foo\n\t// ./apps/bar\n)\n",
			"use",
			[]string{"./apps/foo"},
		},
		{
			"blank line inside a block contributes nothing",
			"require (\n\tgithub.com/x/y v1.0.0\n\n\tgithub.com/x/z v1.0.0\n)\n",
			"require",
			[]string{"github.com/x/y", "github.com/x/z"},
		},
		{"no matching keyword", "module github.com/x/y\n\ngo 1.26\n", "require", nil},
		{"empty source", "", "require", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)
			lines := splitLines(source)

			got := scanDirective(source, lines, testCase.keyword)
			if len(got) != len(testCase.want) {
				t.Fatalf("scanDirective() = %v, want %v", got, testCase.want)
			}

			for i, entry := range got {
				if entry.Name != testCase.want[i] {
					t.Fatalf(
						"scanDirective()[%d].Name = %q, want %q",
						i,
						entry.Name,
						testCase.want[i],
					)
				}
			}
		})
	}
}

func TestFirstField(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		source     string
		base, to   int
		wantName   string
		wantOffset int
		wantFound  bool
	}{
		{"plain token", "abc def", 0, 7, "abc", 0, true},
		{"leading spaces", "   abc", 0, 6, "abc", 3, true},
		{"blank range", "   ", 0, 3, "", 0, false},
		{"comment only", "// nope", 0, 7, "", 0, false},
		{"empty range", "", 0, 0, "", 0, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			name, offset, found := firstField([]byte(testCase.source), testCase.base, testCase.to)
			if name != testCase.wantName || offset != testCase.wantOffset ||
				found != testCase.wantFound {
				t.Fatalf(
					"firstField() = (%q, %d, %v), want (%q, %d, %v)",
					name, offset, found, testCase.wantName, testCase.wantOffset, testCase.wantFound,
				)
			}
		})
	}
}
