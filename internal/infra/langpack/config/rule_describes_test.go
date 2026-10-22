package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func testDecl(file, name string) entities.Decl {
	decl := goDecl(file, name)
	decl.IsTest = true

	return decl
}

// A note in a package's own directory is about that package and nothing else:
// the package waits for it, so the two land together.
func TestDescribesRuleMakesThePackageBesideANoteWaitForIt(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"docs/specs/campaigns.md":          "Dispatch lives in `libs/engines/channelmux`.\n",
		"libs/engines/channelmux/NOTES.md": "# notes\n",
	})

	doc := piece("docs/specs/campaigns.md", "<file>", entities.Span{})
	notes := piece("libs/engines/channelmux/NOTES.md", "<file>", entities.Span{})
	mux := goDecl("libs/engines/channelmux/mux.go", "Mux")
	muxTest := testDecl("libs/engines/channelmux/mux_test.go", "TestMux")
	decls := []entities.Decl{doc, notes, mux, muxTest}

	edges, err := (describesRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, mux.Key, notes.Key) {
		t.Error("code does not wait for the note beside it")
	}

	if edgeBetween(edges, muxTest.Key, notes.Key) {
		t.Error("a test waits for the note: it is never the first of a package")
	}

	if edgeBetween(edges, mux.Key, doc.Key) {
		t.Error("code waits for a specification that only names it")
	}

	anchors, err := (describesRule{}).Anchors(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() = %v", err)
	}

	if len(anchors) != len(edges) {
		t.Errorf("anchors = %d, want one per edge (%d)", len(anchors), len(edges))
	}
}
