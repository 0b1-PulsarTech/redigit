//go:build integration

package main

import (
	"strings"
	"testing"
)

func TestAnchoredPreimageShapes(t *testing.T) {
	const (
		groupLo = "package p\n\nconst Base = 1\n\nconst (\n\tA = Base + 0x01\n\tB = Base + 0x02\n)\n"
		groupHi = "package p\n\nconst Base = 1\n\nconst (\n\tA = Base + 1 + iota\n\tB\n)\n"
	)

	testCases := []historyCase{
		{
			name: "an explicit const group that became an iota group is left alone",
			steps: []historyStep{
				{subject: "feat(p): add keys", files: map[string]string{
					"go.mod": modFile, "k.go": groupLo,
					"u.go": "package p\n\nfunc Use() int { return A + B }\n",
				}},
				{subject: "fix(p): number the keys", files: map[string]string{"k.go": groupHi}},
				{subject: "feat(p): add key C", files: map[string]string{
					"k.go": strings.Replace(groupHi, "\tB\n", "\tB\n\tC\n", 1),
					"v.go": "package p\n\nfunc UseC() int { return C }\n",
				}},
			},
			anchors: []string{"fix(p): number the keys"},
			want:    shapeWant{absent: []string{"fix(p): number the keys"}},
		},
		{
			name: "an iota group that gains a name keeps its fix and the two names it changes",
			steps: []historyStep{
				{subject: "feat(p): add keys", files: map[string]string{
					"go.mod": modFile,
					"k.go":   "package p\n\nconst (\n\tA = iota\n\tB\n)\n",
					"u.go":   "package p\n\nfunc Use() int { return A + B }\n",
				}},
				{subject: "fix(p): start the keys at one", files: map[string]string{
					"k.go": "package p\n\nconst (\n\tA = iota + 1\n\tB\n\tC\n)\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): start the keys at one"},
			want:    shapeWant{kept: []string{"fix(p): start the keys at one"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runHistory(t, testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}

func TestAnchoredPreimageGroups(t *testing.T) {
	testCases := []historyCase{
		{
			name: "a multi-name var spec rewritten by the anchor is replaced whole",
			steps: []historyStep{
				{subject: "feat(p): add pair", files: map[string]string{
					"go.mod": modFile,
					"a.go":   "package p\n\nfunc pair() (int, int) { return 1, 2 }\n\nvar left, right = pair()\n",
					"u.go":   "package p\n\nfunc Use() int { return left + right }\n",
				}},
				{subject: "fix(p): swap the pair", files: map[string]string{
					"a.go": "package p\n\nfunc pair() (int, int) { return 1, 2 }\n\nvar right, left = pair()\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): swap the pair"},
			want:    shapeWant{kept: []string{"fix(p): swap the pair"}},
		},
		{
			name: "a grouped var block keeps its other specs when one is rewritten",
			steps: []historyStep{
				{subject: "feat(p): add vars", files: map[string]string{
					"go.mod": modFile,
					"a.go":   "package p\n\nvar (\n\tOne   = 1\n\tTwo   = 2\n\tThree = 3\n)\n",
					"u.go":   "package p\n\nfunc Use() int { return One + Two + Three }\n",
				}},
				{subject: "fix(p): repair two", files: map[string]string{
					"a.go": "package p\n\nvar (\n\tOne   = 1\n\tTwo   = 20\n\tThree = 3\n)\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): repair two"},
			want:    shapeWant{kept: []string{"fix(p): repair two"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runHistory(t, testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}

func TestAnchoredPreimageBlocks(t *testing.T) {
	testCases := []historyCase{
		{
			name: "a grouped type block keeps its other specs when one is rewritten",
			steps: []historyStep{
				{subject: "feat(p): add types", files: map[string]string{
					"go.mod": modFile,
					"a.go":   "package p\n\ntype (\n\tFirst  struct{ N int }\n\tSecond struct{ N int }\n)\n",
					"u.go":   "package p\n\nfunc Use() int { return First{}.N + Second{}.N }\n",
				}},
				{subject: "fix(p): widen second", files: map[string]string{
					"a.go": "package p\n\ntype (\n\tFirst  struct{ N int }\n\tSecond struct{ N, M int }\n)\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): widen second"},
			want:    shapeWant{kept: []string{"fix(p): widen second"}},
		},
		{
			name: "two init functions in one file, one rewritten by the anchor",
			steps: []historyStep{
				{subject: "feat(p): add inits", files: map[string]string{
					"go.mod": modFile,
					"a.go":   "package p\n\nvar Seen int\n\nfunc init() { Seen = 1 }\n\nfunc init() { Seen += 2 }\n",
				}},
				{subject: "fix(p): repair the second init", files: map[string]string{
					"a.go": "package p\n\nvar Seen int\n\nfunc init() { Seen = 1 }\n\nfunc init() { Seen += 3 }\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): repair the second init"},
			want:    shapeWant{absent: []string{"fix(p): repair the second init"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runHistory(t, testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}
