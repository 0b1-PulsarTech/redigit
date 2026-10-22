package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A specification naming a directory opened the history, 20 commits ahead of the
// code it is about. It waits for that code instead.
func TestNamedRuleMakesTheDocumentWaitForTheCodeItNames(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"docs/specs/campaigns.md": "Dispatch lives in `libs/engines/channelmux` and the app apps/metro.\n",
	})

	doc := piece("docs/specs/campaigns.md", "<file>", entities.Span{})
	mux := goDecl("libs/engines/channelmux/mux.go", "Mux")
	muxTest := testDecl("libs/engines/channelmux/mux_test.go", "TestMux")
	ruleflow := goDecl("libs/engines/ruleflow/rule.go", "Rule")
	metro := goDecl("apps/metro/metro.go", "Run")
	header := piece("apps/metro/go.mod", "example.com/metro", entities.Span{})
	decls := []entities.Decl{doc, mux, muxTest, ruleflow, metro, header}

	edges, err := (namedRule{}).Edges(t.Context(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	for _, code := range []entities.Decl{mux, metro} {
		if !edgeBetween(edges, doc.Key, code.Key) {
			t.Errorf("the document does not wait for %s", code.Key)
		}
	}

	if edgeBetween(edges, doc.Key, muxTest.Key) {
		t.Error("the document waits for a test")
	}

	if edgeBetween(edges, doc.Key, ruleflow.Key) {
		t.Error("the document waits for code it never names")
	}

	for _, edge := range edges {
		if edge.Reason != entities.ReasonProse {
			t.Errorf("edge %v is %q, want a prose edge", edge, edge.Reason)
		}
	}
}

func TestNamedRuleNamesAModuleByItsPath(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"docs/rules/config.md": "The loader of example.com/app reads it.\n",
	})

	doc := piece("docs/rules/config.md", "<file>", entities.Span{})
	header := piece("apps/app/go.mod", "example.com/app", entities.Span{})
	code := goDecl("apps/app/load.go", "Load")

	edges, err := (namedRule{}).Edges(t.Context(), snap, []entities.Decl{doc, header, code})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, doc.Key, code.Key) {
		t.Error("the document does not wait for the module its text names by path")
	}
}

// A document naming nothing waits for nothing, and nothing pulls it forward: it
// is born in the closing commits, not in the first one.
func TestNamedRuleLeavesADocumentNamingNothingForTheEnd(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{"docs/NOTES.md": "# Notes\n"})

	doc := piece("docs/NOTES.md", "<file>", entities.Span{})
	header := piece("apps/app/go.mod", "example.com/app", entities.Span{})
	code := goDecl("apps/app/load.go", "Load")
	decls := []entities.Decl{doc, header, code}

	for _, each := range []rule{namedRule{}, describesRule{}, scaffoldRule{}} {
		edges, err := each.Edges(t.Context(), snap, decls)
		if err != nil {
			t.Fatalf("%s Edges() = %v", each.Name(), err)
		}

		if len(edges) != 0 {
			t.Errorf(
				"%s Edges() = %v, want none: the document has no place before the end",
				each.Name(),
				edges,
			)
		}
	}
}

// A directory holding no package and no module is a category: a rule forbidding
// imports of "libs/infra" waits for none of the packages below it.
func TestNamedRuleIgnoresADirectoryThatHoldsNoPackage(t *testing.T) {
	t.Parallel()

	snap := treeWith(t, map[string]string{
		"docs/rules/imports.md": "Nothing imports libs/infra or libs/infra/store.\n",
	})

	doc := piece("docs/rules/imports.md", "<file>", entities.Span{})
	header := piece("libs/go.mod", "example.com/libs", entities.Span{})
	category := goDecl("libs/infra/queue/queue.go", "Queue")
	store := goDecl("libs/infra/store/store.go", "Store")

	edges, err := (namedRule{}).Edges(
		t.Context(), snap, []entities.Decl{doc, header, category, store},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if edgeBetween(edges, doc.Key, category.Key) {
		t.Error("the document waits for a package outside the named one")
	}

	if !edgeBetween(edges, doc.Key, store.Key) {
		t.Error("the document does not wait for the named package")
	}
}
