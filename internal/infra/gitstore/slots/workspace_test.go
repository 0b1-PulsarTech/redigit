package slots

import (
	"slices"
	"testing"
)

func TestNearestWorkspace(t *testing.T) {
	t.Parallel()

	found := map[string]workspace{
		".": {text: "root"}, "sub": {text: "sub"}, "a/b": {text: "ab"},
	}

	testCases := []struct {
		name string
		dir  string
		want string
	}{
		{"the root module", ".", "root"},
		{"a top-level module", "x", "root"},
		{"a module with its own go.work", "sub", "sub"},
		{"below a nested go.work", "sub/deep/er", "sub"},
		{"a go.work two levels up", "a/b/c", "ab"},
		{"a sibling of a nested go.work", "a/c", "root"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, hasWork := nearestWorkspace(testCase.dir, found)
			if !hasWork || got.text != testCase.want {
				t.Fatalf(
					"nearestWorkspace(%q) = %q, want %q",
					testCase.dir,
					got.text,
					testCase.want,
				)
			}
		})
	}

	if got, hasWork := nearestWorkspace("x", map[string]workspace{"sub": {}}); hasWork {
		t.Fatalf("no go.work above gives %+v, want none", got)
	}
}

func TestParseWork(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		text       string
		wantTarget []string
		wantOpaque bool
	}{
		{"use directories", "go 1.26\n\nuse (\n\t./a\n\t../b\n)\n", []string{"./a", "../b"}, false},
		{
			"a local replace joins the uses",
			"go 1.26\n\nuse ./a\n\nreplace x => ./r\n",
			[]string{"./a", "./r"},
			false,
		},
		{"a module replace is no directory", "go 1.26\n\nreplace x => y v1.0.0\n", nil, false},
		{"unparseable", "use (\n", nil, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := parseWork("w", testCase.text)
			if !slices.Equal(got.targets, testCase.wantTarget) ||
				got.isOpaque != testCase.wantOpaque {
				t.Fatalf("parseWork = %+v, want targets %v opaque %v",
					got, testCase.wantTarget, testCase.wantOpaque)
			}
		})
	}
}
