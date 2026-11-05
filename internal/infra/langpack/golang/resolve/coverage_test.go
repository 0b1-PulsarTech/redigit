package resolve

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
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

func TestTypedNotCovered(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, usesFiles())

	testCases := []struct {
		name string
		at   entities.Location
	}{
		{"a file no pass loaded", entities.Location{File: "elsewhere.go"}},
		{
			"a file in a package with errors",
			entities.Location{File: "bad/bad.go", Line: 2, Char: 5},
		},
		{"a line past the end of the file", entities.Location{File: "a.go", Line: 999}},
		{"a column inside a surrogate pair", fix.locate(t, spot{"a.go", "\U0001F600", 0})},
	}
	testCases[3].at.Char++

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := fix.resolver.References(
				context.Background(),
				testCase.at,
			); !errors.Is(
				err,
				ErrNotCovered,
			) {
				t.Fatalf("References() error = %v, want ErrNotCovered", err)
			}

			if _, err := fix.resolver.Implementations(
				context.Background(),
				testCase.at,
			); !errors.Is(
				err,
				ErrNotCovered,
			) {
				t.Fatalf("Implementations() error = %v, want ErrNotCovered", err)
			}

			if _, err := fix.resolver.Definition(
				context.Background(),
				testCase.at,
			); !errors.Is(
				err,
				ErrNotCovered,
			) {
				t.Fatalf("Definition() error = %v, want ErrNotCovered", err)
			}
		})
	}
}

func adhocFiles() map[string]string {
	return map[string]string{
		"a.go": "package fixture\n\nfunc One() int { return 1 }\n",
		"testdata/fix/a.go": `package fix

type Reader interface{ Read() int }

type ReaderA struct{}

func (ReaderA) Read() int { return 0 }

var _ Reader = (*ReaderA)(nil)

type Stack struct{}

func (stack *Stack) Push() { _ = stack }
`,
		"testdata/fix/a_x_test.go": `package fix_test

import "example.test/fixture/testdata/fix"

func TestX() { _ = fix.ReaderA{} }
`,
		"testdata/sib/b.go": "package sib\n\ntype Far struct{}\n\nfunc (Far) Read() int { return 0 }\n",
	}
}

const (
	askReferences = iota
	askImplementations
	askDefinition
)

func answerOf(
	resolver indexing.Resolver,
	kind int,
	at entities.Location,
) ([]entities.Location, error) {
	ctx := context.Background()

	switch kind {
	case askReferences:
		found, err := resolver.References(ctx, at)
		if err != nil {
			return nil, fmt.Errorf("references: %w", err)
		}

		return found, nil
	case askImplementations:
		found, err := resolver.Implementations(ctx, at)
		if err != nil {
			return nil, fmt.Errorf("implementations: %w", err)
		}

		return found, nil
	}

	found, err := resolver.Definition(ctx, at)
	if err != nil {
		return nil, fmt.Errorf("definition: %w", err)
	}

	return []entities.Location{found}, nil
}

func TestTypedAnswersTheAdhocPackagesWithoutTheFallback(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, adhocFiles())

	testCases := []struct {
		name  string
		kind  int
		query spot
		want  []spot
	}{
		{
			"a type read by its method receiver",
			askReferences,
			spot{"testdata/fix/a.go", "Stack struct", 0},
			[]spot{{"testdata/fix/a.go", "Stack)", 0}},
		},
		{
			"an interface read by its blank assertion",
			askReferences,
			spot{"testdata/fix/a.go", "Reader interface", 0},
			[]spot{{"testdata/fix/a.go", "Reader =", 0}},
		},
		{
			"an implementor in the same directory and one in a sibling, as gopls once both are loaded",
			askImplementations,
			spot{"testdata/fix/a.go", "Reader interface", 0},
			[]spot{
				{"testdata/fix/a.go", "ReaderA struct", 0},
				{"testdata/sib/b.go", "Far struct", 0},
			},
		},
		{
			"a use in the external test package",
			askDefinition,
			spot{"testdata/fix/a_x_test.go", "ReaderA{}", 0},
			[]spot{{"testdata/fix/a.go", "ReaderA struct", 0}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			secondary := NewMockSecondary(gomock.NewController(t))
			fallback := NewFallback(fix.resolver, secondary, quietLogger())

			got, err := answerOf(fallback, testCase.kind, fix.locate(t, testCase.query))
			if err != nil {
				t.Fatalf("query error = %v", err)
			}

			want := fix.locations(t, testCase.want)
			if !slices.Equal(got, want) {
				t.Fatalf("answer = %v, want %v", got, want)
			}

			if fallback.Fallbacks() != 0 {
				t.Fatalf("Fallbacks() = %d, want 0", fallback.Fallbacks())
			}
		})
	}

	fix.requireNoFactsUnder(t, "testdata/")
}

func (fix fixture) requireNoFactsUnder(t *testing.T, prefix string) {
	t.Helper()

	facts, err := fix.source.Facts(context.Background(), fix.resolver.snap, fix.decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	for _, use := range facts.Uses {
		if strings.HasPrefix(use.Dependent.File, prefix) {
			t.Fatalf("Facts() holds %+v from the ad hoc pass", use)
		}
	}
}

func TestTypedAdhocFailureLeavesThoseFilesUncovered(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, adhocFiles())

	if _, err := fix.source.Packages(context.Background(), fix.resolver.snap); err != nil {
		t.Fatalf("Packages() error = %v", err)
	}

	if err := os.Remove(filepath.Join(fix.resolver.snap.Root, "go.mod")); err != nil {
		t.Fatalf("remove go.mod: %v", err)
	}

	testCases := []struct {
		name    string
		query   spot
		wantErr error
	}{
		{"a file of the main load is still answered", spot{"a.go", "One() int", 0}, nil},
		{
			"a file of the failed ad hoc pass is left to the fallback",
			spot{"testdata/fix/a.go", "Stack struct", 0},
			ErrNotCovered,
		},
	}

	for _, testCase := range testCases {
		_, err := fix.resolver.References(context.Background(), fix.locate(t, testCase.query))
		if !errors.Is(err, testCase.wantErr) {
			t.Fatalf("%s: References() error = %v, want %v", testCase.name, err, testCase.wantErr)
		}
	}
}
