package web

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTestAnchorRule(t *testing.T) {
	t.Parallel()

	anchors, err := testAnchorRule{}.Anchors(context.Background(), appIndex(t))
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	got := map[string]string{}

	for _, anchor := range anchors {
		if anchor.Reason != entities.ReasonTestOf {
			t.Errorf(
				"anchor %v reason = %q, want %q",
				anchor.Decl,
				anchor.Reason,
				entities.ReasonTestOf,
			)
		}

		got[anchor.Decl.File] = anchor.To.File
	}

	want := map[string]string{
		"web/test/util.test.ts": "web/src/lib/util.ts",
		"web/test/page.mjs":     "web/src/routes/conversas/[id]/+page.svelte",
		"web/test/runner.mjs":   "web/src/routes/conversas/[id]/+page.svelte",
	}

	if len(got) != len(want) {
		t.Errorf("Anchors() = %v, want %v", got, want)
	}

	for test, subject := range want {
		if got[test] != subject {
			t.Errorf("anchor of %q = %q, want %q", test, got[test], subject)
		}
	}
}

func TestTestAnchorRuleOutsideAPackage(t *testing.T) {
	t.Parallel()

	tree := fakeTree{"test/a.mjs": "get('/funil');\n", "src/routes/funil/+page.svelte": ""}
	decls := []entities.Decl{
		{Key: fileKey("test/a.mjs"), IsTest: true},
		{Key: fileKey("src/routes/funil/+page.svelte")},
	}

	run := RulePack{}.index(entities.Snapshot{}, decls)
	run.space = tree.space()

	anchors, err := testAnchorRule{}.Anchors(context.Background(), run)
	if err != nil || len(anchors) != 0 {
		t.Errorf("Anchors() = %v, %v; want none", anchors, err)
	}
}
