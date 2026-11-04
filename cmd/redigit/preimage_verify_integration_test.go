//go:build integration

package main

import "testing"

func TestAnchoredProbeHonoursTheVerifySection(t *testing.T) {
	steps := []historyStep{
		{subject: "feat(p): add the sub module", files: map[string]string{
			"go.mod":     modFile,
			"a.go":       "package p\n\nfunc A() int { return 1 }\n",
			"sub/go.mod": "module example.com/p/sub\n\ngo 1.21\n",
			"sub/s.go":   "package sub\n\nfunc g() int { return 1 }\n\nfunc F() int { return g() }\n",
		}},
		{subject: "fix(sub): drop g", files: map[string]string{
			"sub/s.go": "package sub\n\nfunc F() int { return 1 }\n",
		}},
		{
			subject: "feat(p): add more",
			files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
		},
	}

	testCases := []historyCase{
		{
			name:    "an excluded nested module is reported unchecked and the fix stays",
			steps:   steps,
			anchors: []string{"fix(sub): drop g"},
			extra:   `,"verify":{"exclude":["sub"]}`,
			want:    shapeWant{kept: []string{"fix(sub): drop g"}},
		},
		{
			name:    "the same module without an exclusion folds the fix",
			steps:   steps,
			anchors: []string{"fix(sub): drop g"},
			want:    shapeWant{absent: []string{"fix(sub): drop g"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertShape(
				t,
				runHistory(t, testCase.steps, testCase.anchors, testCase.extra),
				testCase.want,
			)
		})
	}
}

func TestAnchoredProbeNamesOnlyTheOwnersItLeftOut(t *testing.T) {
	const (
		fixO     = "fix(sub): drop g"
		fixR     = "fix(p): drop g"
		exclude  = `,"verify":{"exclude":["sub"]}`
		rootMod  = "module example.com/p\n\ngo 1.21\n\nrequire example.com/p/sub v0.0.0\n\n"
		subMod   = "module example.com/p/sub\n\ngo 1.21\n"
		dropped  = "package other\n\nfunc F() int { return 1 }\n"
		withG    = "package p\n\nfunc F() int { return 1 + 1 }\n"
		withoutG = "package p\n\nfunc F() int { return 2 }\n"
	)

	assertCases(t, []historyCase{
		{
			name: "a module the root requires, edited and excluded, is not counted as built",
			steps: []historyStep{
				{subject: "feat(p): add the modules", files: map[string]string{
					"go.mod": rootMod + "replace example.com/p/sub => ./sub\n",
					"a.go": "package p\n\nimport \"example.com/p/sub/used\"\n\n" +
						"func A() int { return used.U() }\n",
					"sub/go.mod":     subMod,
					"sub/used/u.go":  "package used\n\nfunc U() int { return 1 }\n",
					"sub/other/o.go": "package other\n\nfunc g() int { return 1 }\n\nfunc F() int { return g() }\n",
				}},
				{subject: fixO, files: map[string]string{"sub/other/o.go": dropped}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{fixO}, extra: exclude,
			want: shapeWant{
				kept:    []string{fixO},
				reasons: []string{"excluded by verify.exclude and not built: sub"},
				without: []string{`reason="its pre-image builds"`},
			},
		},
		{
			name: "an excluded module that only depends on the edited root is not named",
			steps: []historyStep{
				{subject: "feat(p): add the modules", files: map[string]string{
					"go.mod":     "module example.com/p\n\ngo 1.21\n",
					"a.go":       withG,
					"sub/go.mod": subMod + "\nrequire example.com/p v0.0.0\n\nreplace example.com/p => ../\n",
					"sub/s.go":   "package sub\n\nimport \"example.com/p\"\n\nfunc S() int { return p.F() }\n",
				}},
				{subject: fixR, files: map[string]string{"a.go": withoutG}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{fixR}, extra: exclude,
			want: shapeWant{
				kept:    []string{fixR},
				reasons: []string{`reason="its pre-image builds"`},
				without: []string{"not built: sub"},
			},
		},
	})
}
