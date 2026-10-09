package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func goDecl(file, name string) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: entities.UnitID("x"),
	}
}

// The edge that makes a plan verifiable: source in a module directory cannot
// compile before the go.mod that makes it a module, and no reference query
// sees that - a package names its own module never, and the module file names
// its packages never.
func TestModuleRuleTiesCodeToItsModule(t *testing.T) {
	t.Parallel()

	header := piece("apps/lead/go.mod", "github.com/org/lead", entities.Span{})
	code := goDecl("apps/lead/internal/handler.go", "Handle")

	edges, err := (moduleRule{}).Edges(
		t.Context(), entities.Snapshot{}, []entities.Decl{header, code},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, code.Key, header.Key) {
		t.Fatalf("code does not wait for its go.mod; got %d edge(s)", len(edges))
	}
}

// A workspace nests modules inside each other's directories, and the outer one
// does not compile the inner one's packages. Picking the first match instead
// of the deepest ties a package to a module that never built it.
func TestModuleRuleChoosesTheDeepestModule(t *testing.T) {
	t.Parallel()

	root := piece("go.mod", "github.com/org/root", entities.Span{})
	inner := piece("modules/lead/go.mod", "github.com/org/lead", entities.Span{})
	code := goDecl("modules/lead/handler.go", "Handle")

	edges, err := (moduleRule{}).Edges(
		t.Context(), entities.Snapshot{}, []entities.Decl{root, inner, code},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, code.Key, inner.Key) {
		t.Fatal("the package was not tied to the module whose directory holds it")
	}

	if edgeBetween(edges, code.Key, root.Key) {
		t.Fatal("the package was also tied to the outer module, which never compiled it")
	}
}

// A go.mod's own pieces must not wait on their own header through this rule:
// the require and replace lines live in the file the header opens, and an
// edge from a piece to its own file's header is a self-dependency dressed up.
func TestModuleRuleDoesNotTieAGoModToItself(t *testing.T) {
	t.Parallel()

	header := piece("go.mod", "github.com/org/root", entities.Span{})
	require := piece("go.mod", "require github.com/x/y", entities.Span{})

	edges, err := (moduleRule{}).Edges(
		t.Context(), entities.Snapshot{}, []entities.Decl{header, require},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) inside one go.mod: %v", len(edges), edges)
	}
}

// A tree with no go.mod at all yields no edge rather than an error: the rule
// has nothing to say, and saying it loudly would fail every run on a
// repository that is not a Go module.
func TestModuleRuleWithNoModuleIsSilent(t *testing.T) {
	t.Parallel()

	edges, err := (moduleRule{}).Edges(
		t.Context(), entities.Snapshot{}, []entities.Decl{goDecl("main.go", "main")},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) with no go.mod in the tree", len(edges))
	}
}

// The scaffold is provider of the go.mod header, so a module edge back from it
// would be a cycle the ordering cannot place.
func TestModuleRuleSkipsTheScaffold(t *testing.T) {
	t.Parallel()

	header := piece("tools/go.mod", "example.com/tools", entities.Span{})
	lint := piece("tools/.golangci.yml", "<file>", entities.Span{})
	taskfile := piece("tools/Taskfile.yml", "<file>", entities.Span{})

	edges, err := (moduleRule{}).Edges(
		t.Context(), entities.Snapshot{}, []entities.Decl{header, lint, taskfile},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if edgeBetween(edges, lint.Key, header.Key) {
		t.Error("the lint configuration waits for the go.mod that waits for it")
	}

	if !edgeBetween(edges, taskfile.Key, header.Key) {
		t.Error("a module's own file stopped waiting for its go.mod")
	}
}
