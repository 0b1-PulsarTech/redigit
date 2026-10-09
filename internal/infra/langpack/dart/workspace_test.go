package dart

import (
	"context"
	"errors"
	"testing"
)

var monorepo = fakeTree{
	"packages/core/pubspec.yaml":       "name: core\nresolution: workspace\n",
	"packages/core/lib/core.dart":      "export 'src/a.dart';\n",
	"packages/core/lib/src/a.dart":     "class A {}\n",
	"packages/core/test/a_test.dart":   "void main() {}\n",
	"web/pubspec.yaml":                 "name: 'web'\n",
	"web/src/routes/+page.server.dart": "Object? load() => null;\n",
	"pubspec.yaml":                     "name: root\n",
	"packages/core/lib/src/b/b.dart":   "class B {}\n",
}

func TestResolve(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		from string
		uri  string
		want string
	}

	testCases := []testCase{
		{
			"a package URI lands in the package's lib",
			"web/src/routes/+page.server.dart",
			"package:core/core.dart",
			"packages/core/lib/core.dart",
		},
		{
			"a relative URI joins the importer's directory",
			"packages/core/lib/core.dart",
			"src/a.dart",
			"packages/core/lib/src/a.dart",
		},
		{
			"a relative URI may climb",
			"packages/core/lib/src/b/b.dart",
			"../a.dart",
			"packages/core/lib/src/a.dart",
		},
		{"the SDK lies outside", "packages/core/lib/core.dart", "dart:async", ""},
		{
			"an unknown package lies outside",
			"packages/core/lib/core.dart",
			"package:http/http.dart",
			"",
		},
		{"a file the tree lacks lies outside", "packages/core/lib/core.dart", "src/gone.dart", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := monorepo.space().resolve(context.Background(), tc.from, tc.uri)
			if err != nil {
				t.Fatalf("resolve() error = %v", err)
			}

			if got != tc.want {
				t.Fatalf("resolve(%q) = %q, want %q", tc.uri, got, tc.want)
			}
		})
	}
}

func TestWorkspaceReadFailures(t *testing.T) {
	t.Parallel()

	files := []string{"pubspec.yaml", "lib/a.dart"}
	failing := newWorkspace(
		files,
		func(context.Context, string) ([]byte, error) { return nil, errMissing },
	)

	if _, _, err := failing.file(context.Background(), "lib/a.dart"); !errors.Is(err, errMissing) {
		t.Fatalf("file() error = %v, want %v", err, errMissing)
	}

	if _, err := failing.resolve(
		context.Background(),
		"lib/a.dart",
		"package:a/a.dart",
	); !errors.Is(
		err,
		errMissing,
	) {
		t.Fatalf("resolve() error = %v, want %v", err, errMissing)
	}

	if _, isOwned, err := failing.file(
		context.Background(),
		"lib/absent.dart",
	); isOwned ||
		err != nil {
		t.Fatalf("file(absent) = %v, %v, want not owned and no error", isOwned, err)
	}
}

func TestPubspecField(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		source string
		field  string
		want   string
	}

	testCases := []testCase{
		{"plain scalar", "name: core\n", "name", "core"},
		{"quoted scalar", "name: \"core\"\n", "name", "core"},
		{"an indented key is not top-level", "dependencies:\n  name: other\n", "name", ""},
		{"absent field", "name: core\n", "resolution", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := pubspecField(tc.source, tc.field); got != tc.want {
				t.Fatalf("pubspecField() = %q, want %q", got, tc.want)
			}
		})
	}
}
