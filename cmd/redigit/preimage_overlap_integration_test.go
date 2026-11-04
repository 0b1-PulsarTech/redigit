//go:build integration

package main

import "testing"

func TestAnchoredPreimageOverlap(t *testing.T) {
	const (
		fixA = "fix(p): repair a"
		fixB = "fix(p): repair b"
	)

	assertCases(t, []historyCase{
		{
			name:   "two wip anchors whose names share a final span do not break the emitter",
			policy: "wip",
			steps: []historyStep{
				{subject: "feat(p): add a and b", files: map[string]string{
					"go.mod": modFile,
					"a.go":   "package p\n\nvar a = 1\n\nvar b = 2\n",
					"u.go":   "package p\n\nfunc Use() int { return a + b }\n",
				}},
				{
					subject: fixA,
					files:   map[string]string{"a.go": "package p\n\nvar a = 3\n\nvar b = 2\n"},
				},
				{
					subject: fixB,
					files:   map[string]string{"a.go": "package p\n\nvar a = 3\n\nvar b = 6\n"},
				},
				{subject: "refactor(p): group a and b", files: map[string]string{
					"a.go": "package p\n\nvar a, b = 3, 6\n",
				}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{fixA, fixB},
			want: shapeWant{
				kept: []string{
					fixB,
				},
				absent:  []string{fixA},
				reasons: []string{"overlaps"},
				marked:  1,
			},
		},
		{
			name:  "two unchecked anchors whose names share a final span do not break the emitter",
			extra: `,"verify":{"exclude":["sub"]}`,
			steps: []historyStep{
				{subject: "feat(p): add a and b", files: map[string]string{
					"go.mod":     modFile,
					"w.go":       fixMore,
					"sub/go.mod": "module example.com/p/sub\n\ngo 1.21\n",
					"sub/s.go":   "package sub\n\nvar a = 1\n\nvar b = 2\n",
					"sub/u.go":   "package sub\n\nfunc Use() int { return a + b }\n",
				}},
				{subject: fixA, files: map[string]string{
					"sub/s.go": "package sub\n\nvar a = 3\n\nvar b = 2\n",
				}},
				{subject: fixB, files: map[string]string{
					"sub/s.go": "package sub\n\nvar a = 3\n\nvar b = 6\n",
				}},
				{subject: "refactor(sub): group a and b", files: map[string]string{
					"sub/s.go": "package sub\n\nvar a, b = 3, 6\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"v.go": "package p\n\nvar Also = 1\n"},
				},
			},
			anchors: []string{fixA, fixB},
			want: shapeWant{
				kept: []string{fixB}, absent: []string{fixA}, reasons: []string{"overlaps"},
			},
		},
	})
}
