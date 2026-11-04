//go:build integration

package main

import "testing"

func TestAnchoredPreimageImports(t *testing.T) {
	testCases := []historyCase{
		{
			name: "bytes dropped by the fix commit is added back to the pre-image",
			steps: []historyStep{
				{subject: "feat(p): add size", files: map[string]string{
					"go.mod": modFile,
					"a.go": "package p\n\nimport \"bytes\"\n\n" +
						"func Size(data []byte) int { return len(bytes.TrimSpace(data)) }\n",
					"u.go": "package p\n\nfunc Use() int { return Size(nil) }\n",
				}},
				{subject: "fix(p): count the bytes", files: map[string]string{
					"a.go": "package p\n\nfunc Size(data []byte) int { return len(data) }\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): count the bytes"},
			want:    shapeWant{kept: []string{"fix(p): count the bytes"}},
		},
		{
			name: "an alias the final file gives to another package folds the anchor",
			steps: []historyStep{
				{subject: "feat(p): add upper", files: map[string]string{
					"go.mod": modFile,
					"a.go": "package p\n\nimport str \"strings\"\n\n" +
						"func Upper(text string) string { return str.ToUpper(text) }\n",
					"u.go": "package p\n\nfunc Use() string { return Upper(\"\") }\n",
				}},
				{subject: "fix(p): upper the bytes", files: map[string]string{
					"a.go": "package p\n\nimport str \"bytes\"\n\n" +
						"func Upper(text string) string { return string(str.ToUpper([]byte(text))) }\n",
				}},
				{subject: "feat(p): add count", files: map[string]string{
					"a.go": "package p\n\nimport str \"bytes\"\n\n" +
						"func Upper(text string) string { return string(str.ToUpper([]byte(text))) }\n\n" +
						"func Count(data []byte) int { return str.Count(data, nil) }\n",
				}},
			},
			anchors: []string{"fix(p): upper the bytes"},
			want:    shapeWant{absent: []string{"fix(p): upper the bytes"}},
		},
		{
			name: "a declaration moved after the anchor takes its imports from the parent file",
			steps: []historyStep{
				{subject: "feat(p): add trim", files: map[string]string{
					"go.mod": modFile,
					"a.go": "package p\n\nimport \"bytes\"\n\n" +
						"func Trim(data []byte) int { return len(bytes.TrimSpace(data)) }\n",
					"u.go": "package p\n\nfunc Use() int { return Trim(nil) }\n",
				}},
				{subject: "fix(p): trim by length", files: map[string]string{
					"a.go": "package p\n\nfunc Trim(data []byte) int { return len(data) }\n",
				}},
				{
					subject: "refactor(p): move trim",
					files: map[string]string{
						"b.go": "package p\n\nfunc Trim(data []byte) int { return len(data) }\n",
					},
					remove: []string{"a.go"},
				},
			},
			anchors: []string{"fix(p): trim by length"},
			want:    shapeWant{kept: []string{"fix(p): trim by length"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runHistory(t, testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}

func TestAnchoredPreimageUnions(t *testing.T) {
	testCases := []historyCase{
		{
			name: "two anchors in one file admit the union of their imports",
			steps: []historyStep{
				{subject: "feat(p): add helpers", files: map[string]string{
					"go.mod": modFile,
					"a.go": "package p\n\nimport (\n\t\"bytes\"\n\t\"strings\"\n)\n\n" +
						"func Trim(data []byte) int { return len(bytes.TrimSpace(data)) }\n\n" +
						"func Shout(text string) string { return strings.ToUpper(text) }\n",
					"u.go": "package p\n\nfunc Use() int { return Trim(nil) + len(Shout(\"\")) }\n",
				}},
				{subject: "fix(p): trim by length", files: map[string]string{
					"a.go": "package p\n\nimport \"strings\"\n\n" +
						"func Trim(data []byte) int { return len(data) }\n\n" +
						"func Shout(text string) string { return strings.ToUpper(text) }\n",
				}},
				{subject: "fix(p): shout plainly", files: map[string]string{
					"a.go": "package p\n\n" +
						"func Trim(data []byte) int { return len(data) }\n\n" +
						"func Shout(text string) string { return text }\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): trim by length", "fix(p): shout plainly"},
			want: shapeWant{
				kept: []string{"fix(p): trim by length", "fix(p): shout plainly"},
			},
		},
		{
			name: "an unaliased import whose package name differs, beside an aliased one",
			steps: []historyStep{
				{subject: "feat(p): add F", files: map[string]string{
					"go.mod":        modFile,
					"go-foo/foo.go": "package foo\n\nfunc X() int { return 1 }\n\nfunc Y() int { return 2 }\n",
					"a.go": "package p\n\nimport \"example.com/p/go-foo\"\n\n" +
						"func F() int { return foo.X() }\n",
					"u.go": "package p\n\nfunc Use() int { return F() }\n",
				}},
				{subject: "fix(p): constant F", files: map[string]string{
					"a.go": "package p\n\nfunc F() int { return 1 }\n",
				}},
				{subject: "fix(p): add G", files: map[string]string{
					"a.go": "package p\n\nimport foo \"example.com/p/go-foo\"\n\n" +
						"func F() int { return 1 }\n\nfunc G() int { return foo.Y() }\n",
					"v.go": "package p\n\nfunc UseG() int { return G() }\n",
				}},
				{subject: "fix(p): constant G", files: map[string]string{
					"a.go": "package p\n\nfunc F() int { return 1 }\n\nfunc G() int { return 2 }\n",
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(p): constant F", "fix(p): add G", "fix(p): constant G"},
			want:    shapeWant{absent: []string{"fix(p): constant F"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runHistory(t, testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}

func TestAnchoredPreimageLaterPackage(t *testing.T) {
	const pad = "\nfunc pad1() int {\n\treturn 1\n}\n\nfunc pad2() int {\n\treturn 2\n}\n\n" +
		"func pad3() int {\n\treturn 3\n}\n\nfunc pad4() int {\n\treturn 4\n}\n\n" +
		"func pad5() int {\n\treturn 5\n}\n\nfunc pad6() int {\n\treturn 6\n}\n"

	testCases := []historyCase{
		{
			name: "an aliased import of a later package is read through its alias",
			steps: []historyStep{
				{subject: "feat(aaa): add F", files: map[string]string{
					"go.mod": modFile,
					"internal/aaa/a.go": "package aaa\n\nimport z \"example.com/p/internal/zlib\"\n\n" +
						"func F() int { return z.X() }\n" + pad,
					"internal/zlib/z.go": "package zlib\n\nfunc X() int { return 1 }\n" + pad,
				}},
				{subject: "fix(aaa): drop the zlib call", files: map[string]string{
					"internal/aaa/a.go": "package aaa\n\nfunc F() int { return 1 }\n" + pad,
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(aaa): drop the zlib call"},
			want:    shapeWant{absent: []string{"fix(aaa): drop the zlib call"}},
		},
		{
			name: "a plain import of a later package folds the anchor the same way",
			steps: []historyStep{
				{subject: "feat(aaa): add F", files: map[string]string{
					"go.mod": modFile,
					"internal/aaa/a.go": "package aaa\n\nimport \"example.com/p/internal/zlib\"\n\n" +
						"func F() int { return zlib.X() }\n" + pad,
					"internal/zlib/z.go": "package zlib\n\nfunc X() int { return 1 }\n" + pad,
				}},
				{subject: "fix(aaa): drop the zlib call", files: map[string]string{
					"internal/aaa/a.go": "package aaa\n\nfunc F() int { return 1 }\n" + pad,
				}},
				{
					subject: "feat(p): add more",
					files:   map[string]string{"w.go": "package p\n\nvar More = 1\n"},
				},
			},
			anchors: []string{"fix(aaa): drop the zlib call"},
			want:    shapeWant{absent: []string{"fix(aaa): drop the zlib call"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runHistory(t, testCase.steps, testCase.anchors, testCase.extra)
			assertShape(t, got, testCase.want)
		})
	}
}
