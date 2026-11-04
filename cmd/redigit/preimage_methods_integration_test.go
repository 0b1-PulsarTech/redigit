//go:build integration

package main

import "testing"

const (
	methodsTypeOnly = "package bbb\n\ntype T struct{}\n" + fixPad
	methodsWithM    = "package bbb\n\ntype T struct{}\n\nfunc (T) M() int { return 1 }\n" + fixPad
	methodsDropFile = "internal/aaa/a.go"
)

func TestAnchoredPreimageMethods(t *testing.T) {
	const drop = "fix(aaa): drop the M call"

	steps := []historyStep{
		{subject: "feat(aaa): add F", files: map[string]string{
			"go.mod":            modFile,
			"internal/bbb/b.go": methodsTypeOnly,
			methodsDropFile: "package aaa\n\nimport \"example.com/p/internal/bbb\"\n\n" +
				"func F(t bbb.T) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(bbb): add M", files: map[string]string{
			"internal/bbb/b.go": methodsWithM,
			methodsDropFile: "package aaa\n\nimport \"example.com/p/internal/bbb\"\n\n" +
				"func F(t bbb.T) int { return t.M() }\n" + fixPad,
		}},
		{subject: drop, files: map[string]string{
			methodsDropFile: "package aaa\n\nimport \"example.com/p/internal/bbb\"\n\n" +
				"func F(t bbb.T) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
	}

	assertCases(t, []historyCase{
		{
			name:    "a method of an imported package born after the pre-image folds the anchor",
			steps:   steps,
			anchors: []string{drop},
			want:    shapeWant{absent: []string{drop}, reasons: []string{"its pre-image may read"}},
		},
		{
			name: "the same anchor under wip is kept and marked", steps: steps, policy: "wip",
			anchors: []string{drop},
			want:    shapeWant{kept: []string{drop}, marked: 1},
		},
	})
}

func TestAnchoredPreimageMethodsThroughAnAlias(t *testing.T) {
	const drop = "fix(aaa): drop the M call"

	assertCases(t, []historyCase{
		{
			name: "a method reached through an aliased import of its package",
			steps: []historyStep{
				{subject: "feat(aaa): add F", files: map[string]string{
					"go.mod":            modFile,
					"internal/bbb/b.go": methodsTypeOnly,
					methodsDropFile: "package aaa\n\nimport z \"example.com/p/internal/bbb\"\n\n" +
						"func F(t z.T) int { return 1 }\n" + fixPad,
				}},
				{subject: "feat(bbb): add M", files: map[string]string{
					"internal/bbb/b.go": methodsWithM,
					methodsDropFile: "package aaa\n\nimport z \"example.com/p/internal/bbb\"\n\n" +
						"func F(t z.T) int { return t.M() }\n" + fixPad,
				}},
				{subject: drop, files: map[string]string{
					methodsDropFile: "package aaa\n\nimport z \"example.com/p/internal/bbb\"\n\n" +
						"func F(t z.T) int { return 1 }\n" + fixPad,
				}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{drop},
			want:    shapeWant{absent: []string{drop}, reasons: []string{"its pre-image may read"}},
		},
	})
}

func methodsStepsEndingIn(final string) []historyStep {
	header := "package aaa\n\nimport \"example.com/p/internal/bbb\"\n\n"

	return []historyStep{
		{subject: "feat(aaa): add F", files: map[string]string{
			"go.mod": modFile, "internal/bbb/b.go": methodsTypeOnly,
			methodsDropFile: header + "func F(t bbb.T) int { return 1 }\n" + fixPad,
		}},
		{subject: "feat(bbb): add M", files: map[string]string{
			"internal/bbb/b.go": methodsWithM,
			methodsDropFile:     header + "func F(t bbb.T) int { return t.M() }\n" + fixPad,
		}},
		{subject: "fix(aaa): drop the M call", files: map[string]string{
			methodsDropFile: header + final + fixPad,
		}},
		{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
	}
}

func TestAnchoredPreimageMethodsFinalTextWords(t *testing.T) {
	want := shapeWant{
		absent:  []string{"fix(aaa): drop the M call"},
		reasons: []string{"its pre-image may read"},
	}
	anchors := []string{"fix(aaa): drop the M call"}

	assertCases(t, []historyCase{
		{
			name:    "a final text naming the selector only in a comment",
			steps:   methodsStepsEndingIn("// t.M is gone\nfunc F(t bbb.T) int { return 1 }\n"),
			anchors: anchors, want: want,
		},
		{
			name:    "a final text naming the selector only in a string",
			steps:   methodsStepsEndingIn("func F(t bbb.T) int { _ = \"t.M\"; return 1 }\n"),
			anchors: anchors, want: want,
		},
	})
}
