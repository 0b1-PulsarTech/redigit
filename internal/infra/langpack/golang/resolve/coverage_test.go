package resolve

import (
	"go/types"
	"maps"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

type coveredRow struct {
	name string
	pkgs []*packages.Package
	want map[string]bool
}

func cgoRows() []coveredRow {
	dep := packages.Package{ID: "dep", GoFiles: []string{"b.go"}, CompiledGoFiles: []string{"b.go"}}
	mid := packages.Package{ID: "mid", Imports: map[string]*packages.Package{"dep": &dep}}
	cgoOver := func(imports map[string]*packages.Package) packages.Package {
		return packages.Package{
			ID: "cgo", GoFiles: []string{"a.go"}, CompiledGoFiles: []string{"gen.go"},
			Imports: imports,
		}
	}

	return []coveredRow{
		{
			"a package the cgo package imports",
			pkgsOf(cgoOver(map[string]*packages.Package{"dep": &dep}), dep),
			map[string]bool{"b.go": true},
		},
		{
			"a package the cgo package imports through another",
			pkgsOf(cgoOver(map[string]*packages.Package{"mid": &mid}), mid, dep),
			map[string]bool{"b.go": true},
		},
		{
			"a package the cgo package does not import",
			pkgsOf(cgoOver(nil), dep),
			map[string]bool{"b.go": false},
		},
	}
}

func variantRows() []coveredRow {
	owner := packages.Package{
		ID: "p", PkgPath: "p", GoFiles: []string{"a.go"}, CompiledGoFiles: []string{"a.go"},
	}
	readerOver := func(variant packages.Package) packages.Package {
		return packages.Package{
			ID: "x_test", PkgPath: "x_test",
			Imports: map[string]*packages.Package{"p": &variant},
		}
	}

	return []coveredRow{
		{
			"a package recompiled for a test with no type-check leaves its files unsettled",
			pkgsOf(owner, readerOver(packages.Package{ID: "p [x.test]", PkgPath: "p"})),
			map[string]bool{"a.go": true},
		},
		{
			"a package recompiled for a test with a type-check is compared",
			pkgsOf(owner, readerOver(packages.Package{
				ID: "p [x.test]", PkgPath: "p", TypesInfo: &types.Info{},
			})),
			map[string]bool{"a.go": false},
		},
		{
			"a recompiled package of no root is left alone",
			pkgsOf(owner, readerOver(packages.Package{ID: "q [x.test]", PkgPath: "q"})),
			map[string]bool{"a.go": false},
		},
	}
}

func coveredRows() []coveredRow {
	clean := packages.Package{GoFiles: []string{"a.go"}, CompiledGoFiles: []string{"a.go"}}
	broken := packages.Package{
		GoFiles: []string{"a.go"}, CompiledGoFiles: []string{"a.go"},
		Errors: []packages.Error{{Msg: "broken"}},
	}
	rewrote := packages.Package{GoFiles: []string{"a.go"}, CompiledGoFiles: []string{"gen.go"}}

	return append(append(cgoRows(), variantRows()...), []coveredRow{
		{
			"a package compiled from the files it lists",
			pkgsOf(clean),
			map[string]bool{"a.go": false},
		},
		{"a package whose compiled files are generated from its own", pkgsOf(rewrote), nil},
		{
			"a package whose compiled files are not known",
			pkgsOf(packages.Package{GoFiles: []string{"a.go"}}),
			nil,
		},
		{
			"a package with errors is read but not asked",
			pkgsOf(broken),
			map[string]bool{"a.go": true},
		},
		{
			"a broken variant leaves the file unsettled",
			pkgsOf(clean, broken),
			map[string]bool{"a.go": true},
		},
		{
			"a clean variant does not clear a file another variant rewrote",
			pkgsOf(clean, rewrote),
			nil,
		},
	}...)
}

func TestUnseen(t *testing.T) {
	t.Parallel()

	owner := packages.Package{ID: "p", PkgPath: "p"}
	readerOf := func(variant packages.Package) packages.Package {
		return packages.Package{
			ID: "x_test", PkgPath: "x_test",
			Imports: map[string]*packages.Package{"p": &variant},
		}
	}

	testCases := []struct {
		name string
		pkgs []*packages.Package
		want bool
	}{
		{"a clean group", pkgsOf(owner), false},
		{
			"a recompiled package with no type-check",
			pkgsOf(owner, readerOf(packages.Package{ID: "p [x.test]", PkgPath: "p"})),
			true,
		},
		{
			"a recompiled package with a type-check",
			pkgsOf(owner, readerOf(packages.Package{
				ID: "p [x.test]", PkgPath: "p", TypesInfo: &types.Info{},
			})),
			false,
		},
		{
			"a cgo package",
			pkgsOf(packages.Package{GoFiles: []string{"a.go"}, CompiledGoFiles: []string{"g.go"}}),
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := unseen(testCase.pkgs); got != testCase.want {
				t.Fatalf("unseen() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestPlacesCovered(t *testing.T) {
	t.Parallel()

	for _, testCase := range coveredRows() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			testkit.WriteFile(t, root, "a.go", "package fixture\n")
			testkit.WriteFile(t, root, "b.go", "package fixture\n")

			where := newPlaces(entities.Snapshot{Root: root})

			for _, pkg := range testCase.pkgs {
				pkg.GoFiles = absolute(root, pkg.GoFiles)
				pkg.CompiledGoFiles = absolute(root, pkg.CompiledGoFiles)
			}

			got := map[string]bool{}
			for rel, file := range where.covered(testCase.pkgs) {
				got[rel] = file.incomplete
			}

			if !maps.Equal(got, testCase.want) && (len(got) != 0 || len(testCase.want) != 0) {
				t.Fatalf("covered() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func pkgsOf(values ...packages.Package) []*packages.Package {
	out := make([]*packages.Package, 0, len(values))

	for idx := range values {
		copied := values[idx]
		out = append(out, &copied)
	}

	return out
}

func absolute(root string, names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(root, name))
	}

	return out
}
