package dart

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func writeTree(t *testing.T, tree fakeTree) string {
	t.Helper()

	root := t.TempDir()

	for file, source := range tree {
		full := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", file, err)
		}

		if err := os.WriteFile(full, []byte(source), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	return root
}

func TestRulePackEdges(t *testing.T) {
	t.Parallel()

	tree := fakeTree{
		"pubspec.yaml":       "name: app\n",
		"lib/money.dart":     "class Money { Money twice() => Money(); }\n",
		"lib/use.dart":       "import 'money.dart';\nMoney a() => Money();\n",
		"test/use_test.dart": "import 'package:app/use.dart';\nvoid main() { a(); }\n",
	}
	snap := entities.Snapshot{Root: writeTree(t, tree), Files: tree.files()}
	decls := declareAll(t, tree)

	type testCase struct {
		name  string
		decls []entities.Decl
		want  int
	}

	testCases := []testCase{
		{"a repeated reference is one edge, a self-reference none", decls, 5},
		{"an edge to a declaration outside the run is dropped", decls[1:], 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			edges, err := NewRulePack().Edges(context.Background(), snap, tc.decls)
			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			if len(edges) != tc.want {
				t.Fatalf("got %d edges, want %d: %+v", len(edges), tc.want, edges)
			}
		})
	}

	anchors, err := NewRulePack().Anchors(context.Background(), snap, decls)
	if err != nil || len(anchors) != 1 ||
		anchors[0].To != key("lib/use.dart", entities.KindFunc, "a") {
		t.Fatalf("Anchors() = %+v, %v, want main anchored to a", anchors, err)
	}
}

func TestRulePackReadFailure(t *testing.T) {
	t.Parallel()

	snap := entities.Snapshot{Root: t.TempDir(), Files: []string{"lib/a.dart"}}
	decls := []entities.Decl{{Key: key("lib/a.dart", entities.KindType, "A")}}

	if _, err := NewRulePack().Edges(context.Background(), snap, decls); err == nil {
		t.Fatal("Edges() error = nil, want the read failure")
	}

	if _, err := NewRulePack().Anchors(context.Background(), snap, []entities.Decl{{
		Key:    key("test/a_test.dart", entities.KindFunc, "main"),
		IsTest: true,
	}}); err == nil {
		t.Fatal("Anchors() error = nil, want the mirrored file's read failure")
	}
}
