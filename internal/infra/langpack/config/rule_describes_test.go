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

// A spec naming a directory was born in the last decile; the code under that
// directory waits for it, so it lands with the first of them.
func TestDescribesRuleMakesTheNamedCodeWaitForTheDocument(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"docs/specs/campaigns.md":          "Dispatch lives in `libs/engines/channelmux` and the app apps/metro.\n",
		"libs/engines/channelmux/NOTES.md": "# notes\n",
		"apps/metro/go.mod":                "module example.com/metro\n",
	})

	doc := piece("docs/specs/campaigns.md", "<file>", entities.Span{})
	notes := piece("libs/engines/channelmux/NOTES.md", "<file>", entities.Span{})
	mux := goDecl("libs/engines/channelmux/mux.go", "Mux")
	muxTest := testDecl("libs/engines/channelmux/mux_test.go", "TestMux")
	ruleflow := goDecl("libs/engines/ruleflow/rule.go", "Rule")
	metro := goDecl("apps/metro/metro.go", "Run")
	header := piece("apps/metro/go.mod", "example.com/metro", entities.Span{})
	decls := []entities.Decl{doc, notes, mux, muxTest, ruleflow, metro, header}

	edges, err := (describesRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	for _, code := range []entities.Decl{mux, metro} {
		if !edgeBetween(edges, code.Key, doc.Key) {
			t.Errorf("%s does not wait for the document naming it", code.Key)
		}
	}

	if !edgeBetween(edges, mux.Key, notes.Key) {
		t.Error("code does not wait for the note beside it")
	}

	if edgeBetween(edges, muxTest.Key, doc.Key) {
		t.Error("a test waits for the document: it is never the first of a package")
	}

	if edgeBetween(edges, ruleflow.Key, doc.Key) {
		t.Error("code the document never names waits for it")
	}

	anchors, err := (describesRule{}).Anchors(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Anchors() = %v", err)
	}

	if len(anchors) != len(edges) {
		t.Errorf("anchors = %d, want one per edge (%d)", len(anchors), len(edges))
	}
}

func TestDescribesRuleNamesAModuleByItsPath(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"docs/rules/config.md": "The loader of example.com/app reads it.\n",
	})

	doc := piece("docs/rules/config.md", "<file>", entities.Span{})
	header := piece("apps/app/go.mod", "example.com/app", entities.Span{})
	code := goDecl("apps/app/load.go", "Load")

	edges, err := (describesRule{}).Edges(t.Context(), snap, []entities.Decl{doc, header, code})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, code.Key, doc.Key) {
		t.Error("the module's code does not wait for the document naming its module path")
	}
}

// A document naming nothing cannot be tied to code, and the end of the history
// is the one place it must not go: it falls back to the scaffold.
func TestDescribesRuleLeavesADocumentNamingNothingToTheScaffold(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{"docs/NOTES.md": "# Notes\n"})

	doc := piece("docs/NOTES.md", "<file>", entities.Span{})
	header := piece("apps/app/go.mod", "example.com/app", entities.Span{})
	code := goDecl("apps/app/load.go", "Load")
	decls := []entities.Decl{doc, header, code}

	edges, err := (describesRule{}).Edges(t.Context(), snap, decls)
	if err != nil || len(edges) != 0 {
		t.Errorf("Edges() = %v, %v, want none", edges, err)
	}

	scaffold, err := (scaffoldRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(scaffold, header.Key, doc.Key) {
		t.Error("the header does not wait for the document naming nothing")
	}
}
