package dart

import (
	"context"
	"slices"
	"strconv"
	"testing"
)

// scopeFor builds the scope of a file importing the library tree.
func scopeFor(t *testing.T, source string) scope {
	t.Helper()

	space := withLibrary(fakeTree{"lib/use.dart": source}).space()

	parsed, _, err := space.file(context.Background(), "lib/use.dart")
	if err != nil {
		t.Fatalf("file() error = %v", err)
	}

	found, err := space.scopeOf(context.Background(), parsed)
	if err != nil {
		t.Fatalf("scopeOf() error = %v", err)
	}

	return found
}

func refNames(refs []reference) []string {
	out := make([]string, 0, len(refs))

	for _, ref := range refs {
		out = append(out, ref.key.Name+"@"+strconv.Itoa(ref.through))
	}

	return out
}

func TestScopeLookup(t *testing.T) {
	t.Parallel()

	found := scopeFor(
		t,
		"import 'src/hidden.dart' show Shown;\nimport 'src/money.dart' as m;\nclass _Own {}\n",
	)

	type testCase struct {
		name   string
		lookup func() []reference
		want   []string
	}

	testCases := []testCase{
		{
			"a private own name resolves",
			func() []reference { return found.lookup("_Own") },
			[]string{"_Own@-1"},
		},
		{
			"a show list passes its names",
			func() []reference { return found.lookup("Shown") },
			[]string{"Shown@0"},
		},
		{
			"a show list holds the rest back",
			func() []reference { return found.lookup("Secret") },
			nil,
		},
		{
			"a prefixed import offers nothing unprefixed",
			func() []reference { return found.lookup("Money") },
			nil,
		},
		{
			"a prefix offers its names",
			func() []reference { return found.prefixed("m", "Money") },
			[]string{"Money@1"},
		},
		{
			"a prefixed import still offers members",
			func() []reference { return found.member("label") },
			[]string{"Labels@1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := refNames(tc.lookup()); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	if !found.hasPrefix("m") || found.hasPrefix("Money") {
		t.Fatal("hasPrefix() should know m and only m")
	}
}
