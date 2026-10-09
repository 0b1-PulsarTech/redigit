package web

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// onDisk writes tree under a temporary root and returns its snapshot.
func onDisk(t *testing.T, tree fakeTree) entities.Snapshot {
	t.Helper()

	root := t.TempDir()

	for file, source := range tree {
		target := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(target, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return entities.Snapshot{Root: root, Files: tree.files()}
}

func TestRulePackEdges(t *testing.T) {
	t.Parallel()

	tree := fakeTree{
		"package.json":            `{"name": "app"}`,
		"src/routes/+page.svelte": "<script>import { a } from './+page';import './+page.svelte';</script>\n",
		"src/routes/+page.ts":     "export const a = 1;\n",
	}

	edges, err := NewRulePack(nil).Edges(context.Background(), onDisk(t, tree), declareAll(t, tree))
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	page := fileKey("src/routes/+page.svelte")
	want := map[entities.Edge]bool{
		{Dependent: page, Provider: fileKey("src/routes/+page.ts"), Reason: entities.ReasonReference}: true,
		{Dependent: page, Provider: fileKey("package.json"), Reason: entities.ReasonHosts}:            true,
		{
			Dependent: fileKey("src/routes/+page.ts"),
			Provider:  fileKey("package.json"),
			Reason:    entities.ReasonHosts,
		}: true,
	}

	if len(edges) != len(want) {
		t.Errorf("Edges() = %v, want %d edges, no duplicate nor self-edge", edges, len(want))
	}

	for _, edge := range edges {
		if !want[edge] {
			t.Errorf("Edges() has unexpected %v", edge)
		}
	}
}

func TestRulePackUnreadableScript(t *testing.T) {
	t.Parallel()

	tree := fakeTree{"test/a.mjs": "import './b';\n"}
	snap := onDisk(t, tree)
	snap.Files = append(snap.Files, "test/gone.mjs")
	decls := []entities.Decl{{Key: fileKey("test/gone.mjs"), IsTest: true}}

	if _, err := NewRulePack(nil).Edges(context.Background(), snap, decls); err == nil ||
		!strings.Contains(err.Error(), "rule import") {
		t.Errorf("Edges() error = %v, want it to name rule import", err)
	}

	if _, err := NewRulePack(nil).Anchors(context.Background(), snap, decls); err == nil ||
		!strings.Contains(err.Error(), "rule test-anchor") {
		t.Errorf("Anchors() error = %v, want it to name rule test-anchor", err)
	}
}
