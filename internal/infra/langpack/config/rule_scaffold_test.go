package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A dotfile nothing reads was born in the trailing commit of a 1208-commit
// history: the header waits for it so the first package cannot be born first.
func TestScaffoldRuleMakesEveryModuleHeaderWaitForTheScaffold(t *testing.T) {
	t.Parallel()

	ignore := piece(".gitignore", "<file>", entities.Span{})
	lint := piece("tools/.golangci.yml", "<file>", entities.Span{})
	appHeader := piece("apps/lead/go.mod", "example.com/app", entities.Span{})
	toolsHeader := piece("tools/go.mod", "example.com/tools", entities.Span{})
	readme := piece("apps/lead/README.md", "<file>", entities.Span{})
	decls := []entities.Decl{ignore, lint, appHeader, toolsHeader, readme}

	edges, err := (scaffoldRule{}).Edges(t.Context(), entities.Snapshot{}, decls)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	for _, header := range []entities.Decl{appHeader, toolsHeader} {
		for _, file := range []entities.Decl{ignore, lint} {
			if !edgeBetween(edges, header.Key, file.Key) {
				t.Errorf("%s does not wait for %s", header.Key, file.Key)
			}
		}

		if edgeBetween(edges, header.Key, readme.Key) {
			t.Errorf("%s waits for a module's own file", header.Key)
		}
	}

	anchors, err := (scaffoldRule{}).Anchors(t.Context(), entities.Snapshot{}, decls)
	if err != nil {
		t.Fatalf("Anchors() = %v", err)
	}

	if len(anchors) != len(edges) {
		t.Errorf("anchors = %d, want one per edge (%d)", len(anchors), len(edges))
	}
}

func TestScaffoldRuleIsSilentWithoutAModule(t *testing.T) {
	t.Parallel()

	ignore := piece(".gitignore", "<file>", entities.Span{})

	edges, err := (scaffoldRule{}).Edges(t.Context(), entities.Snapshot{}, []entities.Decl{ignore})
	if err != nil || len(edges) != 0 {
		t.Errorf("Edges() = %v, %v, want none", edges, err)
	}
}
