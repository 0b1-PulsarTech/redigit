package modmap

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestFilesRel(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "tree")
	elsewhere := filepath.Join(base, "elsewhere")

	for _, dir := range []string{root, elsewhere} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	if err := os.MkdirAll(filepath.Join(root, "p"), 0o750); err != nil {
		t.Fatalf("mkdir p: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "p", "q.go"), nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	link := filepath.Join(base, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "a file in the root", input: filepath.Join(root, "a.go"), want: "a.go"},
		{name: "a file below the root", input: filepath.Join(root, "p", "q.go"), want: "p/q.go"},
		{name: "a file through a symlink", input: filepath.Join(link, "p", "q.go"), want: "p/q.go"},
		{name: "a file outside the root", input: filepath.Join(elsewhere, "a.go"), want: ""},
		{name: "the parent of the root", input: filepath.Join(base, "a.go"), want: ""},
		{name: "a relative name with no module", input: "p/q.go", want: ""},
	}

	files := New(link)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range 2 {
				if got := files.Rel(testCase.input); got != testCase.want {
					t.Fatalf("Rel(%q) = %q, want %q", testCase.input, got, testCase.want)
				}
			}
		})
	}
}

func TestFilesReadingNamesACgoFileByTheTreeItIsRead(t *testing.T) {
	skipWithoutCgo(t)

	t.Setenv("GOCACHE", t.TempDir())

	trees := make([]string, 2)
	reads := make([][]*packages.Package, len(trees))

	for index := range trees {
		trees[index] = t.TempDir()
		testkit.WriteFile(t, trees[index], "go.mod", "module example.test/cgo\n\ngo 1.26\n")
		testkit.WriteFile(t, trees[index], "cg/cg.go", "package cg\n\n"+
			"// int twice(int v) { return v * 2; }\nimport \"C\"\n\n"+
			"func Twice(v int) int { return int(C.twice(C.int(v))) }\n")
		testkit.WriteFile(t, trees[index], "cg/plain.go", "package cg\n\nfunc Plain() {}\n")

		cfg := &packages.Config{
			Dir: trees[index], BuildFlags: []string{"-trimpath"},
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
				packages.NeedImports | packages.NeedTypes | packages.NeedSyntax |
				packages.NeedTypesInfo | packages.NeedModule,
		}

		pkgs, err := packages.Load(cfg, "./...")
		if err != nil || len(pkgs) != 1 || packages.PrintErrors(pkgs) > 0 {
			t.Fatalf("Load() = %d packages, error %v", len(pkgs), err)
		}

		reads[index] = pkgs
	}

	testCases := []struct {
		name   string
		tree   int
		member string
		want   string
	}{
		{name: "the first checkout", tree: 0, member: "Twice", want: "cg/cg.go"},
		{name: "a later checkout", tree: 1, member: "Twice", want: "cg/cg.go"},
		{name: "a plain file of a later checkout", tree: 1, member: "Plain", want: "cg/plain.go"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pkg := reads[testCase.tree][0]
			name := pkg.Fset.Position(pkg.Types.Scope().Lookup(testCase.member).Pos()).Filename
			files := New(trees[testCase.tree]).Reading(reads[testCase.tree])

			if got := files.Rel(name); got != testCase.want {
				t.Fatalf("Rel(%q) = %q, want %q", name, got, testCase.want)
			}
		})
	}
}

func TestFilesReadingNamesACgoFileWhateverNameTheCacheKeptForIt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		isCgo bool
		want  string
	}{
		{name: "a relative name of a cgo file", input: "cg.go", isCgo: true, want: "cg/cg.go"},
		{name: "a cgo file named by another base", input: "other.go", isCgo: true, want: ""},
		{name: "a relative name of a file cgo left alone", input: "cg.go", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			listed := filepath.Join(root, "cg", "cg.go")
			fset := token.NewFileSet()

			const clause = "package cg\n"

			file, err := parser.ParseFile(fset, testCase.input, clause, parser.PackageClauseOnly)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			pkg := &packages.Package{
				Fset: fset, Syntax: []*ast.File{file},
				GoFiles: []string{listed}, CompiledGoFiles: []string{listed},
			}
			if testCase.isCgo {
				pkg.CompiledGoFiles = []string{filepath.Join(root, "cache", "cg.cgo1.go")}
			}

			files := New(root).Reading([]*packages.Package{pkg})
			if got := files.Rel(testCase.input); got != testCase.want {
				t.Fatalf("Rel(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestFilesReadingReadsAVendoredFileNoPackageListedAsOutsideAnyRoot(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{name: "a file beside the test", input: dep + "@v1.0.0/table.go"},
		{name: "a file nothing holds", input: dep + "@v1.0.0/ghost.go"},
	}

	module := &packages.Module{Path: dep, Version: "v1.0.0"}
	files := New(".").Reading([]*packages.Package{{Module: module, PkgPath: dep}})

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := files.Rel(testCase.input); got != "" {
				t.Fatalf("Rel(%q) = %q, want nothing", testCase.input, got)
			}
		})
	}
}

func skipWithoutCgo(t *testing.T) {
	t.Helper()

	cc := cmp.Or(os.Getenv("CC"), "gcc")
	if _, err := exec.LookPath(cc); err != nil {
		t.Skip("cgo is not usable here")
	}

	enabled, err := exec.CommandContext(t.Context(), "go", "env", "CGO_ENABLED").Output()
	if err != nil || strings.TrimSpace(string(enabled)) != "1" {
		t.Skip("cgo is not usable here")
	}
}

func loadByName(t *testing.T, dir string) map[string]*packages.Package {
	t.Helper()

	cfg := &packages.Config{
		Dir: dir, BuildFlags: []string{"-trimpath"},
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedModule,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil || packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("Load() = %d packages, error %v", len(pkgs), err)
	}

	byName := map[string]*packages.Package{}
	for _, pkg := range pkgs {
		byName[pkg.Name] = pkg
	}

	return byName
}

func TestFilesReadingLeavesTheLineOfAListedFileAlone(t *testing.T) {
	skipWithoutCgo(t)
	t.Setenv("GOCACHE", t.TempDir())

	root := t.TempDir()
	testkit.WriteFile(t, root, "go.mod", "module example.test/cglh\n\ngo 1.26\n")
	testkit.WriteFile(t, root, "cg/cg.go", "package cg\n\n"+
		"// int twice(int v) { return v * 2; }\nimport \"C\"\n\n"+
		"func Twice(v int) int { return int(C.twice(C.int(v))) }\n")
	testkit.WriteFile(t, root, "cg/plain.go", "//line ../other/plain.go:1\npackage cg\n")
	testkit.WriteFile(t, root, "other/plain.go", "package other\n\nfunc Shadow() {}\n")

	byName := loadByName(t, root)
	files := New(root).Reading([]*packages.Package{byName["cg"], byName["other"]})

	testCases := []struct {
		name   string
		pkg    string
		member string
		want   string
	}{
		{
			name: "the file a listed file names", pkg: "other",
			member: "Shadow", want: "other/plain.go",
		},
		{name: "the cgo file itself", pkg: "cg", member: "Twice", want: "cg/cg.go"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pkg := byName[testCase.pkg]
			name := pkg.Fset.Position(pkg.Types.Scope().Lookup(testCase.member).Pos()).Filename

			if got := files.Rel(name); got != testCase.want {
				t.Fatalf("Rel(%q) = %q, want %q", name, got, testCase.want)
			}
		})
	}
}

func TestFilesOffsetOfACgoFileDoesNotFollowTheCheckout(t *testing.T) {
	skipWithoutCgo(t)
	t.Setenv("GOCACHE", t.TempDir())

	const source = "package cg\n\n// int twice(int v) { return v * 2; }\nimport \"C\"\n\n" +
		"func Twice(v int) int { return int(C.twice(C.int(v))) }\n\nfunc Other() int { return 1 }\n"

	base := t.TempDir()
	trees := []string{base, filepath.Join(base, strings.Repeat("d", 160))}
	byTree := make([]map[string]*packages.Package, len(trees))

	for index, tree := range trees {
		testkit.WriteFile(t, tree, "go.mod", "module example.test/cgoff\n\ngo 1.26\n")
		testkit.WriteFile(t, tree, "cg/cg.go", source)

		byTree[index] = loadByName(t, tree)
	}

	testCases := []struct {
		name   string
		tree   int
		member string
	}{
		{name: "the checkout that filled the cache", tree: 0, member: "Other"},
		{name: "a checkout with a far longer path", tree: 1, member: "Other"},
		{name: "the first declaration of a long checkout", tree: 1, member: "Twice"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pkg := byTree[testCase.tree]["cg"]
			at := pkg.Fset.Position(pkg.Types.Scope().Lookup(testCase.member).Pos())
			files := New(trees[testCase.tree]).Reading([]*packages.Package{pkg})

			want := strings.Index(source, "func "+testCase.member) + len("func ")
			if got := files.Offset(at); got != want {
				t.Fatalf("Offset(%s) = %d, want %d", at, got, want)
			}
		})
	}
}
