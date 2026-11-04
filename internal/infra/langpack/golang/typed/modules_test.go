package typed

import (
	"context"
	"errors"
	"go/types"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestModulesOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		written map[string]string
		listed  []string
		want    []module
		wantErr error
	}{
		{name: "no go.mod names no module", written: map[string]string{"a.go": "package a\n"}},
		{
			name:    "a root module",
			written: map[string]string{"go.mod": "module example.test/root\n"},
			listed:  []string{"go.mod", "a.go"},
			want:    []module{{dir: ".", path: "example.test/root"}},
		},
		{
			name: "nested modules keep their directories",
			written: map[string]string{
				"go.mod":       "module example.test/root\n",
				"tools/go.mod": "module example.test/tools\n",
			},
			listed: []string{"go.mod", "tools/go.mod"},
			want: []module{
				{dir: ".", path: "example.test/root"},
				{dir: "tools", path: "example.test/tools"},
			},
		},
		{
			name:    "a go.mod without a module line is skipped",
			written: map[string]string{"go.mod": "go 1.26\n"},
			listed:  []string{"go.mod"},
		},
		{
			name:    "a listed go.mod that cannot be read",
			written: map[string]string{},
			listed:  []string{"go.mod"},
			wantErr: fs.ErrNotExist,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			for name, content := range testCase.written {
				testkit.WriteFile(t, root, name, content)
			}

			got, err := modulesOf(entities.Snapshot{Root: root, Files: testCase.listed})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("modulesOf() error = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("modulesOf() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestAllPatterns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		modules []module
		want    []string
	}{
		{name: "no module has no pattern", want: []string{}},
		{
			name:    "one module is all its packages",
			modules: []module{{dir: ".", path: "example.test/root"}},
			want:    []string{"example.test/root/..."},
		},
		{
			name: "patterns come out sorted",
			modules: []module{
				{dir: "tools", path: "example.test/tools"},
				{dir: ".", path: "example.test/root"},
			},
			want: []string{"example.test/root/...", "example.test/tools/..."},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := allPatterns(testCase.modules); !slices.Equal(got, testCase.want) {
				t.Fatalf("allPatterns() = %v, want %v", got, testCase.want)
			}
		})
	}
}

type reading struct {
	key   entities.DeclKey
	found bool
}

func readEveryObject(
	t *testing.T,
	snap entities.Snapshot,
	decls []entities.Decl,
	flag string,
	patterns []string,
) map[string]reading {
	t.Helper()

	cfg := &packages.Config{
		Context: context.Background(), Dir: snap.Root, Tests: true, BuildFlags: []string{flag},
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil || len(pkgs) == 0 || packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("Load(%v) = %d packages, error %v", patterns, len(pkgs), err)
	}

	where := NewPositions(snap.Root, NewDeclLocator(decls)).Parsing(pkgs)
	read := map[string]reading{}
	seen := map[*types.Package]bool{}

	var visit func(pkg *packages.Package, tpkg *types.Package)

	visit = func(pkg *packages.Package, tpkg *types.Package) {
		if seen[tpkg] {
			return
		}

		seen[tpkg] = true

		members := map[string]types.Object{}

		for _, name := range tpkg.Scope().Names() {
			obj := tpkg.Scope().Lookup(name)
			members[tpkg.Path()+"."+name] = obj

			if named, isNamed := obj.Type().(*types.Named); isNamed {
				for method := range named.Methods() {
					members[tpkg.Path()+"."+name+"."+method.Name()] = method
				}
			}
		}

		for label, obj := range members {
			key, found := where.ObjectAt(pkg.Fset, obj)
			read[label] = reading{key: key, found: found}
		}

		for _, imported := range tpkg.Imports() {
			visit(pkg, imported)
		}
	}

	for _, pkg := range pkgs {
		visit(pkg, pkg.Types)
	}

	return read
}

const (
	header = "module example.test/fixture\n\ngo 1.26\n"
	nested = "module example.test/fixture/nested\n\ngo 1.26\n"
	source = "package nested\n\ntype N struct{}\n\nfunc (N) M() {}\n\nfunc F() {}\n"
	single = "package x\n\ntype N struct{}\n\nfunc (N) M() {}\n\nfunc F() {}\n"
)

type trimpathCase struct {
	name          string
	files         map[string]string
	outside       map[string]string
	imports       []string
	wantFiles     []string
	outsideObject string
}

func writeTree(
	t *testing.T,
	base string,
	files, outside map[string]string,
) (entities.Snapshot, []entities.Decl) {
	t.Helper()

	root := filepath.Join(base, "tree")
	snap := entities.Snapshot{Root: root}
	goFiles := []string{}

	for name, content := range files {
		testkit.WriteFile(t, root, name, content)
		snap.Files = append(snap.Files, name)

		if strings.HasSuffix(name, ".go") {
			goFiles = append(goFiles, name)
		}
	}

	for name, content := range outside {
		testkit.WriteFile(t, base, name, content)
	}

	slices.Sort(goFiles)

	return snap, testkit.DeclareAll(t, root, goFiles)
}

func mappedFiles(read map[string]reading) map[string]bool {
	mapped := map[string]bool{}

	for _, got := range read {
		mapped[got.key.File] = mapped[got.key.File] || got.found
	}

	return mapped
}
