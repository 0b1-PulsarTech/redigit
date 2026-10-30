package resolve

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestTypedDefinitionOutsideTheTree(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, map[string]string{
		"a.go": `package fixture

import "fmt"

func One() int { return 1 }

func Run(items []int) (string, error) {
	return fmt.Sprint(len(items), One()), nil
}
`,
	})

	testCases := []struct {
		name       string
		query      spot
		wantSuffix string
		wantInTree bool
	}{
		{"a standard library function", spot{"a.go", "Sprint(", 0}, "fmt/print.go", false},
		{"a builtin function", spot{"a.go", "len(", 0}, "builtin", false},
		{"a universe type", spot{"a.go", "error)", 0}, "builtin", false},
		{"a declaration of the tree", spot{"a.go", "One()", 1}, "a.go", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.resolver.Definition(context.Background(), fix.locate(t, testCase.query))
			if err != nil {
				t.Fatalf("Definition() error = %v, want an answer", err)
			}

			if file := filepath.ToSlash(got.File); !strings.HasSuffix(file, testCase.wantSuffix) {
				t.Fatalf("Definition() file = %q, want a suffix %q", file, testCase.wantSuffix)
			}

			if inTree := !filepath.IsAbs(got.File) &&
				got.File != "builtin"; inTree != testCase.wantInTree {
				t.Fatalf(
					"Definition() = %v, in the tree = %v, want %v",
					got,
					inTree,
					testCase.wantInTree,
				)
			}
		})
	}
}

func TestTypedLineDirectives(t *testing.T) {
	t.Parallel()

	generated := "package fixture\n\n//line other.go:1\nfunc Gen() int { return 1 }\n\n" +
		"func Use() int { return Gen() }\n"

	testCases := []struct {
		name  string
		other string
	}{
		{"the file the directive names is absent", ""},
		{"the file the directive names holds other text", "package fixture\n\n// padding\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{"gen.go": generated}
			if testCase.other != "" {
				files["other.go"] = testCase.other
			}

			fix := newFixture(t, files)
			ctx := context.Background()

			refs, err := fix.resolver.References(ctx, fix.locate(t, spot{"gen.go", "Gen() int", 0}))
			if err != nil {
				t.Fatalf("References() error = %v", err)
			}

			if want := fix.locations(t, []spot{{"gen.go", "Gen()", 1}}); !slices.Equal(refs, want) {
				t.Fatalf("References() = %v, want %v", refs, want)
			}

			def, err := fix.resolver.Definition(ctx, fix.locate(t, spot{"gen.go", "Gen()", 1}))
			if err != nil {
				t.Fatalf("Definition() error = %v", err)
			}

			if want := fix.locate(t, spot{"gen.go", "Gen() int", 0}); def != want {
				t.Fatalf("Definition() = %v, want %v", def, want)
			}
		})
	}
}

func TestTypedReadsPackagesWithErrors(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, map[string]string{
		"a.go": "package fixture\n\nfunc One() int { return 1 }\n\nfunc Run() int { return One() }\n",
		"bad/bad.go": "package bad\n\nimport \"example.test/fixture\"\n\n" +
			"func Broken() int { _ = fixture.One(); return \"s\" }\n",
	})
	ctx := context.Background()

	refs, err := fix.resolver.References(ctx, fix.locate(t, spot{"a.go", "One() int", 0}))
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}

	want := fix.locations(t, []spot{{"a.go", "One()", 1}, {"bad/bad.go", "One()", 0}})
	if !slices.Equal(refs, want) {
		t.Fatalf("References() = %v, want the readers of both packages %v", refs, want)
	}

	def, err := fix.resolver.Definition(ctx, fix.locate(t, spot{"bad/bad.go", "One()", 0}))
	if err != nil {
		t.Fatalf("Definition() error = %v, want an answer", err)
	}

	if want := fix.locate(t, spot{"a.go", "One() int", 0}); def != want {
		t.Fatalf("Definition() = %v, want %v", def, want)
	}

	declared := fix.locate(t, spot{"bad/bad.go", "Broken()", 0})

	if _, err = fix.resolver.References(ctx, declared); !errors.Is(err, ErrNotCovered) {
		t.Fatalf("References() on a declaration of the broken package error = %v", err)
	}

	if _, err = fix.resolver.Implementations(ctx, declared); !errors.Is(err, ErrNotCovered) {
		t.Fatalf("Implementations() on a declaration of the broken package error = %v", err)
	}
}

func TestTypedAnswersCRLFSources(t *testing.T) {
	t.Parallel()

	source := "package fixture\r\n\r\nfunc One() int { return 1 }\r\n\r\nfunc Run() int {\r\n" +
		"\treturn One()\r\n}\r\n"
	fix := newFixture(t, map[string]string{"a.go": source})
	ctx := context.Background()

	refs, err := fix.resolver.References(ctx, fix.locate(t, spot{"a.go", "One() int", 0}))
	if err != nil {
		t.Fatalf("References() error = %v", err)
	}

	if want := fix.locations(t, []spot{{"a.go", "One()", 1}}); !slices.Equal(refs, want) {
		t.Fatalf("References() = %v, want %v", refs, want)
	}

	def, err := fix.resolver.Definition(ctx, fix.locate(t, spot{"a.go", "One()", 1}))
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}

	if want := fix.locate(t, spot{"a.go", "One() int", 0}); def != want {
		t.Fatalf("Definition() = %v, want %v", def, want)
	}

	pastEnd := fix.locate(t, spot{"a.go", "One()", 1})
	pastEnd.Char = len("\treturn One()") + 1

	if _, err = fix.resolver.References(ctx, pastEnd); !errors.Is(err, ErrNotCovered) {
		t.Fatalf("References() past the line end error = %v, want ErrNotCovered", err)
	}
}

type stubImporter map[string]*types.Package

func (stub stubImporter) Import(path string) (*types.Package, error) { return stub[path], nil }

type libScenario struct {
	name     string
	where    string
	source   string
	isCgo    bool
	isListed bool
	wantFile string
	wantFar  bool
}

func (scenario libScenario) collect(t *testing.T) (map[site]entities.Location, site) {
	t.Helper()

	pkgs, root, use := scenario.build(t)

	return collectUses(pkgs, newPlaces(entities.Snapshot{Root: root})).defs, use
}

func (scenario libScenario) build(t *testing.T) ([]*packages.Package, string, site) {
	t.Helper()

	root, cache, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	dirs := map[string]string{"root": root, "cache": cache, "elsewhere": elsewhere}
	libPath := filepath.Join(dirs[scenario.where], "dep.go")
	libSource := strings.ReplaceAll(scenario.source, "ROOT", root)
	userSource := "package user\n\nimport \"example.test/lib\"\n\nfunc Run() { lib.Do() }\n"

	testkit.WriteFile(t, root, "user.go", userSource)
	testkit.WriteFile(t, root, "dep.go", libSource)

	fset := token.NewFileSet()
	libFile, libErr := parser.ParseFile(fset, libPath, libSource, 0)
	userFile, userErr := parser.ParseFile(fset, filepath.Join(root, "user.go"), userSource, 0)

	if libErr != nil || userErr != nil {
		t.Fatalf("parse: %v, %v", libErr, userErr)
	}

	libTypes, err := (&types.Config{}).Check("example.test/lib", fset, []*ast.File{libFile}, nil)
	if err != nil {
		t.Fatalf("check lib: %v", err)
	}

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: stubImporter{"example.test/lib": libTypes}}

	if _, err = conf.Check("example.test/user", fset, []*ast.File{userFile}, info); err != nil {
		t.Fatalf("check user: %v", err)
	}

	compiled := []string{libPath}
	if scenario.isCgo {
		compiled = []string{filepath.Join(cache, "dep.cgo1.go")}
	}

	pkgs := []*packages.Package{{
		ID: "user", PkgPath: "example.test/user", Fset: fset, Syntax: []*ast.File{userFile},
		TypesInfo: info, GoFiles: []string{filepath.Join(root, "user.go")},
		CompiledGoFiles: []string{filepath.Join(root, "user.go")},
	}}
	if scenario.isListed {
		pkgs = append(pkgs, &packages.Package{
			ID: "lib", PkgPath: "example.test/lib", Fset: fset, Syntax: []*ast.File{libFile},
			TypesInfo: &types.Info{}, GoFiles: []string{libPath}, CompiledGoFiles: compiled,
		})
	}

	if scenario.isCgo {
		pkgs = append(pkgs, &packages.Package{
			ID:              "lib [lib.test]",
			PkgPath:         "example.test/lib",
			Fset:            fset,
			TypesInfo:       &types.Info{},
			GoFiles:         []string{libPath},
			CompiledGoFiles: []string{libPath},
		})
	}

	use := site{file: "user.go", offset: strings.Index(userSource, "Do()")}

	return pkgs, root, use
}

func TestCollectUsesLeavesAUsedDependencyUnread(t *testing.T) {
	t.Parallel()

	const plain = "package lib\n\nfunc Do() {}\n"

	testCases := []struct {
		name       string
		where      string
		inTree     bool
		wantUnread bool
	}{
		{"a dependency of the tree whose position maps nowhere", "cache", true, true},
		{"a dependency written outside the tree", "elsewhere", false, false},
		{"a dependency whose position maps into the tree", "root", true, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			scenario := libScenario{where: testCase.where, source: plain}
			pkgs, root, _ := scenario.build(t)
			user := pkgs[0]
			user.Imports = map[string]*packages.Package{
				"example.test/lib": {
					ID: "example.test/lib", PkgPath: "example.test/lib",
					GoFiles: []string{filepath.Join(root, "dep.go")},
				},
			}

			if !testCase.inTree {
				user.Imports["example.test/lib"].GoFiles = []string{"/elsewhere/dep.go"}
			}

			found := collectUses(pkgs, newPlaces(entities.Snapshot{Root: root}))
			if got := found.unread["dep.go"]; got != testCase.wantUnread {
				t.Fatalf("unread = %v, want %v", got, testCase.wantUnread)
			}
		})
	}
}

func TestCollectUsesDefinitions(t *testing.T) {
	t.Parallel()

	const plain = "package lib\n\nfunc Do() {}\n"

	testCases := []libScenario{
		{"a tree file the group parsed", "root", plain, false, true, "dep.go", false},
		{"a tree file the group never parsed", "root", plain, false, false, "", false},
		{"a function outside the tree", "elsewhere", plain, false, true, "", true},
		{
			"a cgo function that a line directive maps into the tree",
			"cache", "package lib\n\n//line ROOT/dep.go:3\nfunc Do() {}\n", true, true, "", false,
		},
		{
			"a function that a line directive maps into the tree from outside it",
			"cache", "package lib\n\n//line ROOT/dep.go:3\nfunc Do() {}\n", false, true, "", false,
		},
		{"a cgo function written in the cache", "cache", plain, true, true, "", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			defs, use := testCase.collect(t)
			got, known := defs[use]

			switch {
			case testCase.wantFar && (!known || !filepath.IsAbs(got.File)):
				t.Fatalf("defs = %v, %v, want a path outside the tree", got, known)
			case !testCase.wantFar && known != (testCase.wantFile != ""):
				t.Fatalf("defs = %v, %v, want recorded %v", got, known, testCase.wantFile != "")
			case testCase.wantFile != "" && got.File != testCase.wantFile:
				t.Fatalf("defs file = %q, want %q", got.File, testCase.wantFile)
			}
		})
	}
}

func recompiledFiles() map[string]string {
	return map[string]string{
		"pk/pk.go":      "package pk\n\ntype T struct{}\n\nfunc P() {}\n",
		"pk/in_test.go": "package pk\n\nfunc inside() { P() }\n",
		"pk/x_test.go": "package pk_test\n\nimport (\n\t\"example.test/fixture/pk\"\n" +
			"\t\"example.test/fixture/pkhelp\"\n)\n\nfunc use() { pkhelp.F() }\n\n" +
			"type Dot struct{}\n\nfunc (Dot) Draw(pk.T) {}\n",
		"pkhelp/help.go": "package pkhelp\n\nimport \"example.test/fixture/pk\"\n\n" +
			"func F() { pk.P() }\n\ntype Shape interface{ Draw(pk.T) }\n",
	}
}

func TestTypedAnswersThroughARecompiledPackage(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, recompiledFiles())

	testCases := []struct {
		name            string
		implementations bool
		query           spot
		want            []spot
	}{
		{
			"a function read by an external test of a package it imports",
			false,
			spot{"pkhelp/help.go", "F()", 0},
			[]spot{{"pk/x_test.go", "F()", 0}},
		},
		{
			"a function read by the recompiled package and the package itself",
			false,
			spot{"pk/pk.go", "P()", 0},
			[]spot{{"pk/in_test.go", "P()", 0}, {"pkhelp/help.go", "P()", 0}},
		},
		{
			"an interface met by a type of an external test that names the recompiled package",
			true,
			spot{"pkhelp/help.go", "Shape interface", 0},
			[]spot{{"pk/x_test.go", "Dot struct", 0}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			query := fix.resolver.References
			if testCase.implementations {
				query = fix.resolver.Implementations
			}

			got, err := query(context.Background(), fix.locate(t, testCase.query))
			if err != nil {
				t.Fatalf("query error = %v", err)
			}

			want := fix.locations(t, testCase.want)
			slices.SortFunc(want, compareLocations)

			if !slices.Equal(got, want) {
				t.Fatalf("query = %v, want %v", got, want)
			}
		})
	}
}
