package modmap

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

type moduleCase struct {
	path      string
	version   string
	dir       string
	vendored  string
	inside    string
	isOutside bool
	isMain    bool
}

type trimmedCase struct {
	name    string
	modules []moduleCase
	input   string
	want    string
}

const (
	fixture = "example.test/fixture"
	nested  = "example.test/fixture/nested"
	dep     = "example.test/dep"
	depFile = "vendor/example.test/dep/d.go"
)

func stubsOf(root, outside string, modules []moduleCase) map[string]*packages.Package {
	stubs := make(map[string]*packages.Package, len(modules))

	for index, found := range modules {
		module := &packages.Module{Path: found.path, Version: found.version, Main: found.isMain}
		if found.dir != "" {
			module.Dir = filepath.Join(root, found.dir)
		}

		if found.isOutside {
			module.Dir = filepath.Join(outside, found.dir)
		}

		stub := &packages.Package{Module: module, PkgPath: found.path + found.inside}
		if found.vendored != "" {
			stub.GoFiles = []string{filepath.Join(root, found.vendored)}
		}

		stubs[strconv.Itoa(index)] = stub
	}

	return stubs
}

func checkTrimmed(t *testing.T, testCases []trimmedCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			root := filepath.Join(base, "tree")
			outside := filepath.Join(base, "outside")

			for _, dir := range []string{root, outside} {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}

			pkgs := []*packages.Package{{Imports: stubsOf(root, outside, testCase.modules)}}
			files := New(root).Reading(pkgs)

			if got := files.Rel(testCase.input); got != testCase.want {
				t.Fatalf("Rel(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestFilesReadingNamesTheDeepestOwner(t *testing.T) {
	t.Parallel()

	treeMain := moduleCase{path: fixture, dir: ".", isMain: true}

	testCases := []trimmedCase{
		{
			name:    "a file of the root module",
			modules: []moduleCase{treeMain},
			input:   fixture + "/a/a.go",
			want:    "a/a.go",
		},
		{
			name:    "a file beside the root go.mod",
			modules: []moduleCase{treeMain},
			input:   fixture + "/main.go",
			want:    "main.go",
		},
		{
			name: "the deepest module owns a nested directory",
			modules: []moduleCase{
				treeMain, {path: nested, dir: "mods/n", isMain: true},
			},
			input: nested + "/n.go",
			want:  "mods/n/n.go",
		},
		{
			name: "a sibling of a nested module stays with the root module",
			modules: []moduleCase{
				treeMain, {path: nested, dir: "mods/n", isMain: true},
			},
			input: fixture + "/nested2/n.go",
			want:  "nested2/n.go",
		},
		{
			name: "a one character nested directory is not the root module",
			modules: []moduleCase{
				treeMain, {path: fixture + "/x", dir: "q", isMain: true},
			},
			input: fixture + "/x/f.go",
			want:  "q/f.go",
		},
		{
			name: "a one character module path does not claim a longer name",
			modules: []moduleCase{
				treeMain, {path: fixture + "/x", dir: "q", isMain: true},
			},
			input: fixture + "/xx/f.go",
			want:  "xx/f.go",
		},
	}

	checkTrimmed(t, testCases)
}

func TestFilesReadingMapsOnlyWhatTheTreeHolds(t *testing.T) {
	t.Parallel()

	treeMain := moduleCase{path: fixture, dir: ".", isMain: true}

	testCases := []trimmedCase{
		{
			name: "a replace directive inside the tree",
			modules: []moduleCase{
				treeMain, {path: "example.test/rep", version: "v0.0.0", dir: "third/rep"},
			},
			input: "example.test/rep@v0.0.0/r.go",
			want:  "third/rep/r.go",
		},
		{
			name: "a replace directive outside the tree",
			modules: []moduleCase{
				treeMain,
				{path: "example.test/ext", version: "v0.0.0", dir: "ext", isOutside: true},
			},
			input: "example.test/ext@v0.0.0/e.go",
			want:  "",
		},
		{
			name: "a release of a tree module path read from the module cache",
			modules: []moduleCase{
				treeMain, {path: fixture, version: "v1.2.3", dir: "cache", isOutside: true},
			},
			input: fixture + "@v1.2.3/a/a.go",
			want:  "",
		},
		{
			name:    "a file a //line directive names in the package directory",
			modules: []moduleCase{treeMain},
			input:   fixture + "/a/gen.tmpl",
			want:    "a/gen.tmpl",
		},
		{
			name:    "a testdata package",
			modules: []moduleCase{treeMain},
			input:   fixture + "/testdata/x/x.go",
			want:    "testdata/x/x.go",
		},
	}

	checkTrimmed(t, testCases)
}

func TestFilesReadingMapsAVendoredPackageByTheFileGoListed(t *testing.T) {
	t.Parallel()

	treeMain := moduleCase{path: fixture, dir: ".", isMain: true}

	testCases := []trimmedCase{
		{
			name: "a vendored package the go tool listed",
			modules: []moduleCase{
				treeMain,
				{path: dep, version: "v1.0.0", vendored: depFile},
			},
			input: dep + "@v1.0.0/d.go",
			want:  depFile,
		},
		{
			name: "a vendored package below the module path",
			modules: []moduleCase{
				treeMain,
				{
					path: dep, version: "v1.0.0", inside: "/sub",
					vendored: "vendor/example.test/dep/sub/s.go",
				},
			},
			input: dep + "@v1.0.0/sub/s.go",
			want:  "vendor/example.test/dep/sub/s.go",
		},
		{
			name: "a vendored package a stale vendor of a member module also holds",
			modules: []moduleCase{
				treeMain,
				{path: nested, dir: "other", isMain: true},
				{path: dep, version: "v1.0.0", vendored: depFile},
			},
			input: dep + "@v1.0.0/d.go",
			want:  depFile,
		},
		{
			name: "a vendored package of which the go tool listed no file",
			modules: []moduleCase{
				treeMain, {path: dep, version: "v1.0.0"},
			},
			input: dep + "@v1.0.0/d.go",
			want:  "",
		},
	}

	checkTrimmed(t, testCases)
}

func TestFilesReadingRefusesWhatItCannotPlace(t *testing.T) {
	t.Parallel()

	treeMain := moduleCase{path: fixture, dir: ".", isMain: true}

	testCases := []trimmedCase{
		{
			name: "a module that two packages place in different directories",
			modules: []moduleCase{
				treeMain, {path: nested, dir: "one"}, {path: nested, dir: "two"},
			},
			input: nested + "/n.go",
			want:  "",
		},
		{
			name: "a vendored package listed in two directories",
			modules: []moduleCase{
				treeMain,
				{path: dep, version: "v1.0.0", vendored: depFile},
				{path: dep, version: "v1.0.0", vendored: "other/" + dep + "/d.go"},
			},
			input: dep + "@v1.0.0/d.go",
			want:  "",
		},
		{
			name: "a vendored package listed in a directory its path does not name",
			modules: []moduleCase{
				treeMain,
				{
					path: dep, version: "v1.0.0", inside: "/sub",
					vendored: "vendor/example.test/o/o.go",
				},
			},
			input: dep + "@v1.0.0/sub/o.go",
			want:  "",
		},
		{
			name:    "a standard library name",
			modules: []moduleCase{treeMain},
			input:   "fmt/print.go",
			want:    "",
		},
		{
			name:    "a module no package names",
			modules: []moduleCase{treeMain},
			input:   "other.test/x/f.go",
			want:    "",
		},
		{
			name:    "a name that climbs out of its module",
			modules: []moduleCase{treeMain},
			input:   fixture + "/../../secret.go",
			want:    "",
		},
		{
			name:    "a name with a backslash is one file name, not a directory",
			modules: []moduleCase{treeMain},
			input:   fixture + "\\a\\a.go",
			want:    "",
		},
		{
			name:    "no module at all",
			modules: nil,
			input:   fixture + "/a/a.go",
			want:    "",
		},
	}

	checkTrimmed(t, testCases)
}

func TestFilesReadingKeepsAbsoluteNamesAsTheyWere(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module := &packages.Module{Path: "example.test/fixture", Main: true, Dir: root}
	files := New(root).Reading([]*packages.Package{{Module: module}})
	testCases := []struct{ name, input string }{
		{name: "a file in the root", input: filepath.Join(root, "a.go")},
		{name: "a file below the root", input: filepath.Join(root, "p", "q.go")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got, want := files.Rel(testCase.input), New(root).Rel(testCase.input); got != want {
				t.Fatalf("Rel(%q) = %q, want %q", testCase.input, got, want)
			}
		})
	}
}

func TestFilesReadingStartsAFreshCache(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "a name the first reader could not place",
			input: fixture + "/a/a.go",
			want:  "a/a.go",
		},
		{name: "a name no reader can place", input: "other.test/a.go", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			plain := New(root)

			if got := plain.Rel(testCase.input); got != "" {
				t.Fatalf("Rel(%q) before Reading = %q, want nothing", testCase.input, got)
			}

			module := &packages.Module{Path: fixture, Main: true, Dir: root}
			reading := plain.Reading([]*packages.Package{{Module: module}})

			if got := reading.Rel(testCase.input); got != testCase.want {
				t.Fatalf("Rel(%q) after Reading = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestFilesReadingMapsTheVendorBesideAGoWork(t *testing.T) {
	t.Parallel()

	const (
		modules = "# example.test/dep v1.0.0\n## explicit; go 1.26\nexample.test/dep\n"
		stale   = "# example.test/dep v0.9.0\n## explicit; go 1.26\nexample.test/dep\n"
		require = "module example.test/a\n\ngo 1.26\n\nrequire example.test/dep v1.0.0\n"
		pkgDep  = "package dep\n\ntype D struct{}\n\nfunc (D) M() {}\n\nfunc F() D { return D{} }\n"
		members = "go 1.26\n\nuse (\n\t./a\n\t./b\n)\n"
		other   = "module example.test/b\n\ngo 1.26\n"
	)

	testCases := []struct {
		name   string
		member string
		files  map[string]string
	}{
		{
			name:   "a workspace with no module at its root",
			member: "a/",
			files: map[string]string{
				"go.work": members, "a/go.mod": require, "b/go.mod": other,
				"vendor/modules.txt": "## workspace\n" + modules,
			},
		},
		{
			name: "a workspace whose root holds a module",
			files: map[string]string{
				"go.work": "go 1.26\n\nuse (\n\t.\n\t./b\n)\n",
				"go.mod":  require, "b/go.mod": other,
				"vendor/modules.txt": "## workspace\n" + modules,
			},
		},
		{
			name:   "a member that keeps a stale vendor of its own",
			member: "a/",
			files: map[string]string{
				"go.work": members, "a/go.mod": require, "b/go.mod": other,
				"vendor/modules.txt":             "## workspace\n" + modules,
				"a/vendor/modules.txt":           stale,
				"a/vendor/example.test/dep/d.go": pkgDep,
			},
		},
		{
			name:   "a module at the root beside a member that keeps a stale vendor",
			member: "a/",
			files: map[string]string{
				"go.work": "go 1.26\n\nuse (\n\t.\n\t./a\n\t./b\n)\n",
				"go.mod":  "module example.test/root\n\ngo 1.26\n", "root.go": "package root\n",
				"a/go.mod": require, "b/go.mod": other,
				"vendor/modules.txt":             "## workspace\n" + modules,
				"a/vendor/modules.txt":           stale,
				"a/vendor/example.test/dep/d.go": pkgDep,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			testCase.files[testCase.member+"a.go"] = "package a\n\nimport \"example.test/dep\"\n\n" +
				"func Run() { dep.F().M() }\n"
			testCase.files["vendor/example.test/dep/d.go"] = pkgDep

			for name, content := range testCase.files {
				testkit.WriteFile(t, root, name, content)
			}

			pkgs, name := loadedDepFile(t, root)

			if got := New(root).Reading(pkgs).Rel(name); got != depFile {
				t.Fatalf("Rel(%q) = %q, want %q", name, got, depFile)
			}
		})
	}
}

func loadedDepFile(t *testing.T, root string) ([]*packages.Package, string) {
	t.Helper()

	cfg := &packages.Config{
		Dir: root, BuildFlags: []string{"-trimpath"},
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedModule,
	}

	pkgs, err := packages.Load(cfg, "example.test/a")
	if err != nil || len(pkgs) != 1 || packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("Load() = %d packages, error %v", len(pkgs), err)
	}

	object := pkgs[0].Types.Imports()[0].Scope().Lookup("D")

	return pkgs, pkgs[0].Fset.Position(object.Pos()).Filename
}
