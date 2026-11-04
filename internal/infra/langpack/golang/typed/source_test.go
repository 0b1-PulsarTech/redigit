package typed

import (
	"context"
	"errors"
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
