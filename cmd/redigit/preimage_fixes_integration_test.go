//go:build integration

package main

import "testing"

const (
	fixPad = "\nfunc pad1() int {\n\treturn 1\n}\n\nfunc pad2() int {\n\treturn 2\n}\n\n" +
		"func pad3() int {\n\treturn 3\n}\n\nfunc pad4() int {\n\treturn 4\n}\n\n" +
		"func pad5() int {\n\treturn 5\n}\n\nfunc pad6() int {\n\treturn 6\n}\n"
	fixMore = "package p\n\nvar More = 1\n"
)

func TestAnchoredPreimageQualifiers(t *testing.T) {
	const (
		drop    = "fix(aaa): drop the call"
		fixDrop = "package aaa\n\nfunc F() int { return 1 }\n" + fixPad
	)

	want := shapeWant{absent: []string{drop}, reasons: []string{"its pre-image may read"}}

	assertCases(t, []historyCase{
		{
			name: "a directory base shared by the standard library and a local package",
			steps: []historyStep{
				{subject: "feat(aaa): add F", files: map[string]string{
					"go.mod": modFile,
					"aaa/a.go": "package aaa\n\nimport (\n\tstderrors \"errors\"\n\n" +
						"\t\"example.com/p/errors\"\n)\n\nfunc F() int {\n" +
						"\t_ = stderrors.New(\"x\")\n\n\treturn errors.X()\n}\n" + fixPad,
					"errors/e.go": "package errors\n\nfunc X() int { return 1 }\n" + fixPad,
				}},
				{subject: drop, files: map[string]string{"aaa/a.go": fixDrop}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{drop}, want: want,
		},
		{
			name: "an aliased import of a package in a module below the repository root",
			steps: []historyStep{
				{subject: "feat(aaa): add F", files: map[string]string{
					"backend/go.mod": modFile,
					"backend/internal/aaa/a.go": "package aaa\n\n" +
						"import z \"example.com/p/internal/zlib\"\n\n" +
						"func F() int { return z.X() }\n" + fixPad,
					"backend/internal/zlib/z.go": "package zlib\n\nfunc X() int { return 1 }\n" + fixPad,
				}},
				{subject: drop, files: map[string]string{"backend/internal/aaa/a.go": fixDrop}},
				{subject: "feat(p): add more", files: map[string]string{
					"backend/w.go": "package backend\n\nvar More = 1\n",
				}},
			},
			anchors: []string{drop}, want: want,
		},
		{
			name: "an aliased import of the module root package",
			steps: []historyStep{
				{subject: "feat(aaa): add F", files: map[string]string{
					"go.mod": modFile,
					"internal/aaa/a.go": "package aaa\n\nimport r \"example.com/p\"\n\n" +
						"func F() int { return r.X() }\n" + fixPad,
					"z.go": "package p\n\nfunc X() int { return 1 }\n" + fixPad,
				}},
				{subject: drop, files: map[string]string{"internal/aaa/a.go": fixDrop}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{drop}, want: want,
		},
	})
}

func TestAnchoredPreimageWindows(t *testing.T) {
	const (
		k1      = "package p\n\nfunc k1() int { return 1 + 1 }\n"
		fixFoo  = "package foo\n\nfunc X() int { return 1 }\n\nfunc Y() int { return 2 }\n"
		fixUse  = "package p\n\nfunc Use() int { return F() }\n"
		fixAddF = "package p\n\nimport \"example.com/p/go-foo\"\n\nfunc F() int { return foo.X() }\n"
		fixAddG = "package p\n\nimport foo \"example.com/p/go-foo\"\n\nfunc F() int { return 1 }\n" +
			"\nfunc G() int { return foo.Y() }\n"
		fixConst = "package p\n\nfunc F() int { return 1 }\n\nfunc G() int { return 2 }\n"
	)

	assertCases(t, []historyCase{
		{
			name: "an innocent repair stays when a newer anchor breaks the window",
			steps: []historyStep{
				{subject: "feat(p): add F", files: map[string]string{
					"go.mod": modFile, "go-foo/foo.go": fixFoo, "a.go": fixAddF, "u.go": fixUse,
					"k.go": "package p\n\nfunc k() int { return 1 + 1 }\n",
				}},
				{subject: "fix(p): repair k", files: map[string]string{
					"k.go": "package p\n\nfunc k() int { return 2 }\n",
				}},
				{subject: "fix(p): constant F", files: map[string]string{
					"a.go": "package p\n\nfunc F() int { return 1 }\n",
				}},
				{subject: "fix(p): add G", files: map[string]string{
					"a.go": fixAddG, "v.go": "package p\n\nfunc UseG() int { return G() }\n",
				}},
				{subject: "fix(p): constant G", files: map[string]string{"a.go": fixConst}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{
				"fix(p): repair k", "fix(p): constant F", "fix(p): add G", "fix(p): constant G",
			},
			want: shapeWant{kept: []string{"fix(p): repair k"}},
		},
		{
			name: "three innocent repairs stay when a newer anchor breaks the window",
			steps: []historyStep{
				{subject: "feat(p): add F", files: map[string]string{
					"go.mod": modFile, "go-foo/foo.go": fixFoo, "a.go": fixAddF, "u.go": fixUse,
					"k1.go": k1, "k2.go": "package p\n\nfunc k2() int { return 1 + 1 }\n",
					"k3.go": "package p\n\nfunc k3() int { return 1 + 1 }\n",
				}},
				{subject: "fix(p): repair k1", files: map[string]string{
					"k1.go": "package p\n\nfunc k1() int { return 2 }\n",
				}},
				{subject: "fix(p): repair k2", files: map[string]string{
					"k2.go": "package p\n\nfunc k2() int { return 2 }\n",
				}},
				{subject: "fix(p): repair k3", files: map[string]string{
					"k3.go": "package p\n\nfunc k3() int { return 2 }\n",
				}},
				{subject: "fix(p): constant F", files: map[string]string{
					"a.go": "package p\n\nfunc F() int { return 1 }\n",
				}},
				{subject: "fix(p): add G", files: map[string]string{
					"a.go": fixAddG, "v.go": "package p\n\nfunc UseG() int { return G() }\n",
				}},
				{subject: "fix(p): constant G", files: map[string]string{"a.go": fixConst}},
				{subject: "feat(p): add more", files: map[string]string{"w.go": fixMore}},
			},
			anchors: []string{
				"fix(p): repair k1", "fix(p): repair k2", "fix(p): repair k3",
				"fix(p): constant F", "fix(p): add G", "fix(p): constant G",
			},
			want: shapeWant{kept: []string{
				"fix(p): repair k1", "fix(p): repair k2", "fix(p): repair k3",
			}},
		},
	})
}
