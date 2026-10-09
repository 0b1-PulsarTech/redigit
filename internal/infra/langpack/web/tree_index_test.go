package web

import (
	"context"
	"errors"
	"testing"
)

func TestTreeIndexIsTest(t *testing.T) {
	t.Parallel()

	type testCase struct {
		file   string
		isTest bool
	}

	testCases := []testCase{
		{file: "web/test/page.mjs", isTest: true},
		{file: "web/test/bare.mjs", isTest: true},
		{file: "web/test/util.test.ts", isTest: true},
		{file: "web/test/harness.mjs", isTest: false},
		{file: "web/src/lib/util.ts", isTest: false},
		{file: "web/src/routes/+page.svelte", isTest: false},
	}

	tree := &treeIndex{space: app.space()}
	if err := tree.load(context.Background()); err != nil {
		t.Fatalf("load() error = %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			if got := tree.isTest(tc.file); got != tc.isTest {
				t.Errorf("isTest(%q) = %v, want %v", tc.file, got, tc.isTest)
			}
		})
	}
}

func TestImportGraph(t *testing.T) {
	t.Parallel()

	graph, err := app.space().importGraph(context.Background())
	if err != nil {
		t.Fatalf("importGraph() error = %v", err)
	}

	want := map[string][]string{
		"web/src/lib/a.ts":            {"web/src/lib/b.ts"},
		"web/test/page.mjs":           {"web/test/harness.mjs"},
		"web/test/bare.mjs":           nil,
		"ui/svelte/atoms/Icon.svelte": {"ui/svelte/format.ts"},
	}

	for file, targets := range want {
		if got := graph[file]; len(got) != len(targets) || (len(got) > 0 && got[0] != targets[0]) {
			t.Errorf("importGraph()[%q] = %q, want %q", file, got, targets)
		}
	}

	if _, found := graph["web/package.json"]; found {
		t.Error("importGraph() scanned web/package.json, want scripts only")
	}
}

func TestImportGraphUnreadable(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		files []string
	}

	testCases := []testCase{
		{name: "an unreadable script", files: []string{"a.ts"}},
		{name: "an unreadable manifest", files: []string{"package.json", "b.ts"}},
	}

	tree := fakeTree{"b.ts": "import './c';\n"}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			index := &treeIndex{space: newWorkspace(tc.files, tree.Read)}
			if err := index.load(context.Background()); !errors.Is(err, errMissing) {
				t.Errorf("load() error = %v, want %v", err, errMissing)
			}
		})
	}
}
