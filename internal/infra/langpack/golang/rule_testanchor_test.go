package golang

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// TestTestAnchorRuleLastMatchNotFirst is the named trap: TestFSStorePutGet
// names the type FS and the methods Put and Get. Taking the longest-matching
// candidate FIRST anchors the test to the type FS, born long before the
// methods it actually exercises, and reports a defect where the test was
// correctly waiting. The rule must take the LAST match along the walk - Get.
func TestTestAnchorRuleLastMatchNotFirst(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, filepath.Join("store", "fs_store.go"), `package store

type FS struct{}

func (FS) Put(key string, value []byte) error { return nil }

func (FS) Get(key string) ([]byte, error) { return nil, nil }
`)
	writeFile(t, root, filepath.Join("store", "fs_store_test.go"), `package store

import "testing"

func TestFSStorePutGet(t *testing.T) {}
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{
		filepath.Join("store", "fs_store.go"),
		filepath.Join("store", "fs_store_test.go"),
	})

	anchors, err := testAnchorRule{}.Anchors(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	want := entities.DeclKey{
		File: filepath.Join("store", "fs_store.go"),
		Kind: entities.KindMethod,
		Name: "FS.Get",
	}

	if len(anchors) != 1 {
		t.Fatalf("got %d anchors, want 1: %+v", len(anchors), anchors)
	}

	if anchors[0].To != want {
		t.Fatalf("anchor.To = %+v, want %+v (the method, not the type FS)", anchors[0].To, want)
	}

	if anchors[0].Reason != entities.ReasonTestOf {
		t.Fatalf("Reason = %v, want %v", anchors[0].Reason, entities.ReasonTestOf)
	}
}

func TestTestAnchorRuleTwinFileLongestPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		testBase string
		files    []string
		want     string
	}{
		{
			"exact stem match wins",
			"fs_store_test.go",
			[]string{"fs_store.go", "fs.go"},
			"fs_store.go",
		},
		{
			"cut at underscore falls back to shorter file",
			"fs_store_test.go",
			[]string{"fs.go"},
			"fs.go",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			byFile := map[string][]entities.Decl{}
			for _, f := range testCase.files {
				byFile[filepath.Join("pkg", f)] = nil
			}

			got, found := twinFile("pkg", testCase.testBase, byFile)
			if !found {
				t.Fatalf("twinFile() found = false, want true")
			}

			if got != filepath.Join("pkg", testCase.want) {
				t.Fatalf("twinFile() = %q, want %q", got, filepath.Join("pkg", testCase.want))
			}
		})
	}
}

func TestTestAnchorRuleNoTwinFileSearchesUnit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, filepath.Join("store", "types.go"), `package store

type Cache struct{}

func (Cache) Get(key string) ([]byte, error) { return nil, nil }
`)
	writeFile(t, root, filepath.Join("store", "cache_test.go"), `package store

import "testing"

func TestCacheGet(t *testing.T) {}
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{
		filepath.Join("store", "types.go"),
		filepath.Join("store", "cache_test.go"),
	})

	anchors, err := testAnchorRule{}.Anchors(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	want := entities.DeclKey{
		File: filepath.Join("store", "types.go"),
		Kind: entities.KindMethod,
		Name: "Cache.Get",
	}

	if len(anchors) != 1 || anchors[0].To != want {
		t.Fatalf("anchors = %+v, want one anchoring to %+v", anchors, want)
	}
}

func TestTestAnchorRuleNoSubjectNoAnchor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	writeFile(t, root, filepath.Join("store", "helper_test.go"), `package store

import "testing"

func TestZzzUnrelated(t *testing.T) {}
`)

	snap := entities.Snapshot{Root: root}
	syntax := NewSyntax()

	decls := declareAll(t, syntax, root, []string{filepath.Join("store", "helper_test.go")})

	anchors, err := testAnchorRule{}.Anchors(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	if len(anchors) != 0 {
		t.Fatalf("got %d anchors with no matching subject, want 0: %+v", len(anchors), anchors)
	}
}

func TestCamelOrSnakeTokens(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		want []string
	}{
		{"acronym then words", "FSStorePutGet", []string{"FS", "Store", "Put", "Get"}},
		{"snake case", "user_created", []string{"user", "created"}},
		{"single word", "Foo", []string{"Foo"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := camelOrSnakeTokens(testCase.in)
			if len(got) != len(testCase.want) {
				t.Fatalf("camelOrSnakeTokens(%q) = %v, want %v", testCase.in, got, testCase.want)
			}

			for i, tok := range got {
				if tok != testCase.want[i] {
					t.Fatalf(
						"camelOrSnakeTokens(%q)[%d] = %q, want %q",
						testCase.in,
						i,
						tok,
						testCase.want[i],
					)
				}
			}
		})
	}
}

func TestTestAnchorRuleEdgesIsEmpty(t *testing.T) {
	t.Parallel()

	edges, err := testAnchorRule{}.Edges(context.Background(), entities.Snapshot{}, nil)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf(
			"got %d edges, want 0 (this rule only ever pulls backward, as an Anchor)",
			len(edges),
		)
	}
}
