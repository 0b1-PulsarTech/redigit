package typed

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/loadonce"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestSourceClose(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		loaded  bool
		wantErr error
	}{
		{"before any load nothing starts afterwards", false, loadonce.ErrClosed},
		{"after a load the answer stays", true, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, _ := testkit.Fixture(t, map[string]string{"a.go": "package fixture\n"})
			source := NewSource(nil)

			if testCase.loaded {
				if _, err := source.Packages(context.Background(), snap); err != nil {
					t.Fatalf("Packages() error = %v", err)
				}
			}

			if err := source.Close(context.Background()); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			_, err := source.Packages(context.Background(), snap)
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("Packages() after Close error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestSourceTypeChecksARecompiledVariant(t *testing.T) {
	t.Parallel()

	snap, _ := testkit.Fixture(t, map[string]string{
		"p/p.go":      "package p\n",
		"p/p_test.go": "package p\n",
		"p/x_test.go": "package p_test\n\nimport _ \"example.test/fixture/q\"\n",
		"q/q.go":      "package q\n\nimport _ \"example.test/fixture/p\"\n",
	})

	roots, err := NewSource(nil).Packages(context.Background(), snap)
	if err != nil {
		t.Fatalf("Packages() error = %v", err)
	}

	variants := 0

	packages.Visit(roots, func(pkg *packages.Package) bool {
		if strings.HasSuffix(pkg.PkgPath, "/q") && pkg.ID != pkg.PkgPath {
			variants++

			if pkg.TypesInfo == nil {
				t.Errorf("variant %s has no TypesInfo", pkg.ID)
			}
		}

		return true
	}, nil)

	if variants == 0 {
		t.Fatal("the fixture produced no recompiled variant of q")
	}
}

func TestTypedSourceAttributesUsesToTheDeclarationsAsked(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"a.go": "package fixture\n\nfunc One() {}\n\nfunc Run() {\n\tOne()\n}\n",
	})

	source := NewSource(nil)

	whole, err := source.Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	run := testkit.DeclKey("a.go", entities.KindFunc, "Run")
	statement := testkit.DeclKey("a.go", entities.KindStmt, "Run#0")
	one := testkit.DeclKey("a.go", entities.KindFunc, "One")

	if !slices.Contains(whole.Uses, Use{Dependent: run, Provider: one}) {
		t.Fatalf("Uses = %v, want Run to use One", whole.Uses)
	}

	cut := slices.Clone(decls)

	for index, decl := range cut {
		if decl.Key != run {
			continue
		}

		head := decl
		head.Span.End = head.Span.Start + len("func Run() {")
		body := entities.Decl{
			Key:  statement,
			Unit: decl.Unit,
			Span: entities.Span{Start: head.Span.End + 2, End: decl.Span.End - 2},
		}
		cut[index] = head
		cut = append(cut, body)

		break
	}

	got, err := source.Facts(context.Background(), snap, cut)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	if !slices.Contains(got.Uses, Use{Dependent: statement, Provider: one}) ||
		slices.Contains(got.Uses, Use{Dependent: run, Provider: one}) {
		t.Fatalf("Uses = %v, want the statement to use One and the head not", got.Uses)
	}
}

func TestSourceEscapesThroughAnInterfaceOnlyAnotherPassDeclares(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		visible    string
		hidden     string
		wantEscape bool
	}{
		{
			name:       "a method an interface of the same pass names",
			visible:    "Do()",
			hidden:     "Other()",
			wantEscape: true,
		},
		{
			name:       "a method an interface only the hidden file names",
			visible:    "Other()",
			hidden:     "Do()",
			wantEscape: true,
		},
		{
			name:       "a method no interface names",
			visible:    "Other()",
			hidden:     "Another()",
			wantEscape: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{
				"a/a.go": "package a\n\ntype I interface{ " + testCase.visible + " }\n\n" +
					"type Impl struct{}\n\nfunc (Impl) Do() {}\n",
				"b/b.go": "package b\n\nimport (\n\t\"fmt\"\n\n\t\"example.test/fixture/a\"\n)\n\n" +
					"func UseEsc() {\n\tfmt.Println(a.Impl{})\n}\n",
				"b/hidden.go": "//go:build hid\n\npackage b\n\n" +
					"type Hidden interface{ " + testCase.hidden + " }\n",
			})

			facts, err := NewSource(nil).Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			method := testkit.DeclKey("a/a.go", entities.KindMethod, "Impl.Do")
			escaped := slices.ContainsFunc(facts.Escaped, func(req Requirement) bool {
				return req.Dependent.Name == "UseEsc" && slices.Contains(req.Methods, method)
			})

			if escaped != testCase.wantEscape {
				t.Fatalf("Escaped = %+v, %v escaped = %v", facts.Escaped, method, escaped)
			}
		})
	}
}

func TestSourcePositionsOfNestedModulesAreTheSameFilesWithAndWithoutTrimpath(t *testing.T) {
	t.Parallel()

	replaced := header + "\nrequire example.test/fixture/nested v0.0.0\n\n" +
		"replace example.test/fixture/nested => ./nested\n"

	testCases := []trimpathCase{
		{
			name: "a nested module reached through a replace",
			files: map[string]string{
				"go.mod":        replaced,
				"nested/go.mod": nested,
				"nested/n.go":   source,
			},
			imports:   []string{"example.test/fixture/nested"},
			wantFiles: []string{"a/a.go", "nested/n.go"},
		},
		{
			name: "a nested module reached only through another package",
			files: map[string]string{
				"go.mod":        replaced,
				"nested/go.mod": nested,
				"nested/n.go":   source,
				"a/a.go": "package a\n\nimport \"example.test/fixture/nested\"\n\n" +
					"type T struct{ nested.N }\n",
			},
			wantFiles: []string{"a/a.go", "nested/n.go"},
		},
		{
			name: "a nested module of a go.work",
			files: map[string]string{
				"go.mod": header, "nested/go.mod": nested, "nested/n.go": source,
				"go.work": "go 1.26\n\nuse (\n\t.\n\t./nested\n)\n",
			},
			imports:   []string{"example.test/fixture/nested"},
			wantFiles: []string{"a/a.go", "nested/n.go"},
		},
		{
			name: "a one character nested directory beside a longer sibling",
			files: map[string]string{
				"go.mod": header + "\nrequire example.test/fixture/x v0.0.0\n\n" +
					"replace example.test/fixture/x => ./x\n",
				"x/go.mod": "module example.test/fixture/x\n\ngo 1.26\n", "x/x.go": single,
				"xx/xx.go": "package xx\n\nfunc F() {}\n",
			},
			imports:   []string{"example.test/fixture/x", "example.test/fixture/xx"},
			wantFiles: []string{"x/x.go", "xx/xx.go"},
		},
	}

	checkSameFiles(t, testCases)
}

func TestSourcePositionsOfOtherModuleShapesAreTheSameFilesWithAndWithoutTrimpath(t *testing.T) {
	t.Parallel()

	testCases := []trimpathCase{
		{
			name: "a replace directive outside the tree",
			files: map[string]string{
				"go.mod": header + "\nrequire example.test/ext v0.0.0\n\nreplace example.test/ext => ../ext\n",
			},
			outside: map[string]string{
				"ext/go.mod": "module example.test/ext\n\ngo 1.26\n",
				"ext/e.go":   "package ext\n\nfunc E() {}\n",
			},
			imports:       []string{"example.test/ext"},
			wantFiles:     []string{"a/a.go"},
			outsideObject: "example.test/ext.E",
		},
		{
			name: "a vendor directory",
			files: map[string]string{
				"go.mod":                       header + "\nrequire example.test/dep v1.0.0\n",
				"vendor/modules.txt":           "# example.test/dep v1.0.0\n## explicit; go 1.26\nexample.test/dep\n",
				"vendor/example.test/dep/d.go": "package dep\n\ntype D struct{}\n\nfunc (D) M() {}\n",
			},
			imports:   []string{"example.test/dep"},
			wantFiles: []string{"vendor/example.test/dep/d.go"},
		},
		{
			name: "a testdata package and a file a line directive renames",
			files: map[string]string{
				"go.mod":          header,
				"testdata/x/x.go": single,
				"a/gen.go":        "package a\n\n//line gen.tmpl:5\nfunc Gen() {}\n",
			},
			imports:   []string{"example.test/fixture/testdata/x"},
			wantFiles: []string{"a/a.go", "testdata/x/x.go"},
		},
	}

	checkSameFiles(t, testCases)
}

func checkSameFiles(t *testing.T, testCases []trimpathCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			top := "package top\n\nimport (\n"

			for _, path := range testCase.imports {
				top += "\t_ \"" + path + "\"\n"
			}

			files := map[string]string{
				"a/a.go":     "package a\n\ntype T struct{}\n\nfunc (T) M() {}\n\nvar V = 1\n\nfunc F() {}\n",
				"top/top.go": top + "\t_ \"example.test/fixture/a\"\n)\n",
			}
			maps.Copy(files, testCase.files)

			snap, decls := writeTree(t, base, files, testCase.outside)

			modules, err := modulesOf(snap)
			if err != nil {
				t.Fatalf("modulesOf() error = %v", err)
			}

			for _, patterns := range [][]string{{"example.test/fixture/top"}, allPatterns(modules)} {
				before := readEveryObject(t, snap, decls, "-trimpath=false", patterns)
				after := readEveryObject(t, snap, decls, "-trimpath", patterns)

				if !maps.Equal(before, after) {
					t.Fatalf("%v: without %v, with %v", patterns, before, after)
				}

				mapped := mappedFiles(after)

				for _, want := range testCase.wantFiles {
					if !mapped[want] {
						t.Fatalf("%v read no object from %s, got files %v", patterns, want, mapped)
					}
				}

				far, known := after[testCase.outsideObject]
				if testCase.outsideObject != "" && (!known || far.found) {
					t.Fatalf(
						"%s = %+v, %v, want an outside object",
						testCase.outsideObject,
						far,
						known,
					)
				}
			}
		})
	}
}

func TestSourceLoadsPackagesWithTrimpath(t *testing.T) {
	t.Setenv("GOFLAGS", "-trimpath=false")

	snap, _ := testkit.Fixture(t, map[string]string{
		"a/a.go": "package a\n\ntype T struct{}\n\nfunc F() {}\n",
		"b/b.go": "package b\n\nimport _ \"example.test/fixture/a\"\n",
	})

	const want = "example.test/fixture/a/a.go"

	testCases := []struct {
		name   string
		tags   []string
		member string
	}{
		{name: "a func of an imported package", member: "F"},
		{name: "a type of an imported package", member: "T"},
		{name: "a func read with build tags", tags: []string{"integration"}, member: "F"},
		{name: "a type read with build tags", tags: []string{"integration"}, member: "T"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pass := loadPass{tags: testCase.tags, patterns: []string{"example.test/fixture/b"}}

			pkgs, err := typeCheck(context.Background(), snap.Root, pass)
			if err != nil || len(pkgs) != 1 {
				t.Fatalf("typeCheck() = %d packages, error %v", len(pkgs), err)
			}

			obj := pkgs[0].Types.Imports()[0].Scope().Lookup(testCase.member)
			if got := pkgs[0].Fset.Position(obj.Pos()).Filename; got != want {
				t.Fatalf("%s is in %q, want %q", testCase.member, got, want)
			}
		})
	}
}

func TestSourceObjectsOfALineDirectiveLeavingTheirDirectoryReadTheSameFromAnyCheckout(
	t *testing.T,
) {
	t.Setenv("GOCACHE", t.TempDir())

	files := map[string]string{
		"go.mod": header,
		"gen/parser.go": "package gen\n\n//line ../other/other.go:3\nfunc Shadow() {}\n\n" +
			"//line /nonexistent/abs/thing.go:7\nfunc FarAway() {}\n",
		"gen/real.go":    "package gen\n\nfunc Real() {}\n",
		"other/other.go": "package other\n\nfunc Shadow() {}\n",
		"top/top.go": "package top\n\nimport (\n\t_ \"example.test/fixture/gen\"\n" +
			"\t_ \"example.test/fixture/other\"\n)\n",
	}

	reads := make([]map[string]reading, 0, 2)

	for range 2 {
		snap, decls := writeTree(t, t.TempDir(), files, nil)
		patterns := []string{"example.test/fixture/top"}
		reads = append(reads, readEveryObject(t, snap, decls, "-trimpath", patterns))
	}

	testCases := []struct {
		name   string
		object string
		want   bool
	}{
		{name: "a name that climbs into a sibling", object: "example.test/fixture/gen.Shadow"},
		{name: "a name no checkout holds", object: "example.test/fixture/gen.FarAway"},
		{name: "a file of the package itself", object: "example.test/fixture/gen.Real", want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for index, read := range reads {
				if got := read[testCase.object].found; got != testCase.want {
					t.Fatalf("checkout %d read %s as found = %v, want %v",
						index, testCase.object, got, testCase.want)
				}
			}
		})
	}
}
