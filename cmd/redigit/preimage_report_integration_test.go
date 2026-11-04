//go:build integration

package main

import "testing"

func TestAnchoredReportsTellTheTruth(t *testing.T) {
	const (
		grouped = "package p\n\nconst (\n\tA = iota\n\tB\n)\n"
		start   = "fix(p): start the keys at one"
	)

	assertCases(t, []historyCase{
		{
			name: "a dropped anchor whose declaration was deleted later says nothing was placed",
			steps: []historyStep{
				{subject: "feat(p): add F and H", files: map[string]string{
					"go.mod": modFile,
					"a.go":   "package p\n\nfunc F() int { return 1 }\n\nfunc H() int { return 2 }\n",
				}},
				{subject: "fix(p): repair F", files: map[string]string{
					"a.go": "package p\n\nfunc F() int { return 3 }\n\nfunc H() int { return 2 }\n",
				}},
				{subject: "refactor(p): drop F", files: map[string]string{
					"a.go": "package p\n\nfunc H() int { return 2 }\n",
				}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{"fix(p): repair F"},
			want: shapeWant{
				reasons: []string{`reason="it changes no placed declaration"`},
				without: []string{"is left alone"},
			},
		},
		{
			name: "an iota group that gains a name that something uses folds with that name",
			steps: []historyStep{
				{subject: "feat(p): add keys", files: map[string]string{
					"go.mod": modFile, "k.go": grouped,
					"u.go": "package p\n\nfunc Use() int { return A + B }\n",
				}},
				{subject: start, files: map[string]string{
					"k.go": "package p\n\nconst (\n\tA = iota + 1\n\tB\n\tC\n)\n",
					"v.go": "package p\n\nfunc UseC() int { return C }\n",
				}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{start},
			want:    shapeWant{absent: []string{start}, reasons: []string{"undefined: C"}},
		},
	})
}
