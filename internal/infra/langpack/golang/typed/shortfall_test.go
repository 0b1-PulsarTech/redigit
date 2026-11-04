package typed

import (
	"context"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestTypedSourceKeepsAPartialPassAsData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		wantBroken bool
	}{
		{
			"every package checks",
			map[string]string{"a.go": "package fixture\n\nfunc One() {}\n"},
			false,
		},
		{
			"a package does not type-check",
			map[string]string{"a.go": "package fixture\n\nvar Bad int = \"text\"\n"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, testCase.files)
			source := NewSource([]string{"integration"})

			if got := source.Shortfalls(); len(got) != 0 {
				t.Fatalf("Shortfalls() = %+v before any load", got)
			}

			if _, err := source.Facts(context.Background(), snap, decls); err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			got := source.Shortfalls()
			if (len(got) > 0) != testCase.wantBroken {
				t.Fatalf("Shortfalls() = %+v, want broken = %v", got, testCase.wantBroken)
			}

			if testCase.wantBroken &&
				(got[0].Broken < 1 || got[0].Of < got[0].Broken ||
					!slices.Equal(got[0].Tags, []string{"integration"})) {
				t.Fatalf("Shortfalls() = %+v, want the broken count, total and tags", got)
			}
		})
	}
}

func TestTypedSourceKeepsTheUsesOfASoundPackageBesideABrokenDependency(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"lib/lib.go": "package lib\n\nfunc One() {}\n",
		"bad/bad.go": "package bad\n\nfunc Broken() int { return \"s\" }\n",
		"a.go": "package fixture\n\nimport (\n\t\"example.test/fixture/bad\"\n\t" +
			"\"example.test/fixture/lib\"\n)\n\nfunc Run() {\n\tlib.One()\n\t_ = bad.Broken()\n}\n",
	})

	source := NewSource(nil)

	got, err := source.Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	run := testkit.DeclKey("a.go", entities.KindFunc, "Run")

	testCases := []struct {
		name     string
		provider entities.DeclKey
	}{
		{
			name:     "a use of the sound package",
			provider: testkit.DeclKey("lib/lib.go", entities.KindFunc, "One"),
		},
		{
			name:     "a use of the broken package",
			provider: testkit.DeclKey("bad/bad.go", entities.KindFunc, "Broken"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(got.Uses, Use{Dependent: run, Provider: testCase.provider}) {
				t.Fatalf("Uses = %v, want Run to use %v", got.Uses, testCase.provider)
			}
		})
	}

	if short := source.Shortfalls(); len(short) != 1 || short[0].Broken != 1 {
		t.Fatalf("Shortfalls() = %+v, want the one broken package", short)
	}
}

type whyCase struct {
	name       string
	files      map[string]string
	wantFirst  string
	wantCause  string
	wantBroken int
	wantShared int
}

const goModule = "module example.test/fixture\n\ngo 1.26\n"

func TestTypedSourceNamesTheCauseOfAPartialPass(t *testing.T) {
	t.Parallel()

	replaced := goModule + "\nrequire example.com/lifted v0.0.0\n\nreplace example.com/lifted => ./x/lifted\n"

	testCases := []whyCase{
		{
			name: "a replace to a directory that is absent, in workspace mode",
			files: map[string]string{
				"go.work": "go 1.26\n\nuse .\n",
				"go.mod":  replaced,
				"a/a.go":  "package a\n\nimport _ \"example.com/lifted\"\n",
				"b/b.go":  "package b\n\nimport _ \"example.com/lifted\"\n",
			},
			wantFirst:  "example.test/fixture/...",
			wantCause:  "replaced by ./x/lifted",
			wantBroken: 3,
			wantShared: 1,
		},
		{
			name: "a root package that imports the absent replace does not hide its pseudo-package",
			files: map[string]string{
				"go.work": "go 1.26\n\nuse .\n",
				"go.mod":  replaced,
				"root.go": "package fixture\n\nimport _ \"example.com/lifted\"\n",
				"a/a.go":  "package a\n\nimport _ \"example.com/lifted\"\n",
			},
			wantFirst:  "example.test/fixture/...",
			wantCause:  "replaced by ./x/lifted",
			wantBroken: 3,
			wantShared: 1,
		},
		{
			name: "a plain missing import",
			files: map[string]string{
				"go.mod": goModule,
				"a.go":   "package fixture\n\nimport _ \"example.test/missing/pkg\"\n",
			},
			wantFirst:  "example.test/fixture",
			wantCause:  "example.test/missing/pkg",
			wantBroken: 1,
			wantShared: 1,
		},
	}

	checkWhyPartial(t, testCases)
}

func TestTypedSourceCountsThePackagesSharingTheFirstCause(t *testing.T) {
	t.Parallel()

	testCases := []whyCase{
		{
			name: "the smallest path among the real causes leads, and a symptom does not",
			files: map[string]string{
				"go.mod": goModule,
				"z/z.go": "package z\n\nvar Bad int = \"text\"\n",
				"m/m.go": "package m\n\nvar Bad int = \"other\"\n",
				"k/k.go": "package k\n\nimport _ \"example.test/missing/pkg\"\n",
			},
			wantFirst:  "example.test/fixture/m",
			wantCause:  "cannot use",
			wantBroken: 3,
			wantShared: 1,
		},
		{
			name: "packages failing with the same message are counted together",
			files: map[string]string{
				"go.mod": goModule,
				"c/c.go": "package c\n\nimport _ \"example.test/missing/pkg\"\n",
				"d/d.go": "package d\n\nimport _ \"example.test/missing/pkg\"\n",
				"e/e.go": "package e\n\nimport _ \"example.test/missing/other\"\n",
			},
			wantFirst:  "example.test/fixture/c",
			wantCause:  "example.test/missing/pkg",
			wantBroken: 3,
			wantShared: 2,
		},
		{
			name: "a type error is named by its message and not by its banner",
			files: map[string]string{
				"go.mod": goModule,
				"z/z.go": "package z\n\nvar Bad int = \"text\"\n",
			},
			wantFirst:  "example.test/fixture/z",
			wantCause:  "cannot use",
			wantBroken: 1,
			wantShared: 1,
		},
	}

	checkWhyPartial(t, testCases)
}

func checkWhyPartial(t *testing.T, testCases []whyCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := writeTree(t, t.TempDir(), testCase.files, nil)
			source := NewSource(nil)

			if _, err := source.Facts(context.Background(), snap, decls); err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			got := source.Shortfalls()
			if len(got) != 1 {
				t.Fatalf("Shortfalls() = %+v, want one", got)
			}

			if got[0].FirstPackage != testCase.wantFirst ||
				!strings.Contains(got[0].FirstError, testCase.wantCause) ||
				strings.Contains(got[0].FirstError, "\n") ||
				got[0].Broken != testCase.wantBroken || got[0].SameMessage != testCase.wantShared {
				t.Fatalf("Shortfalls() = %+v, want %+v", got[0], testCase)
			}
		})
	}
}

func TestShortfallOfNamesTheBrokenPackageOverItsSymptom(t *testing.T) {
	t.Parallel()

	type fault struct {
		pkgPath string
		message string
	}

	testCases := []struct {
		name      string
		faults    []fault
		wantFirst string
		wantShare int
	}{
		{
			name: "a symptom that sorts first does not lead",
			faults: []fault{
				{pkgPath: "x/a", message: `could not import x/z (invalid package name: "")`},
				{pkgPath: "x/z", message: "cannot use text as int"},
			},
			wantFirst: "x/z",
			wantShare: 1,
		},
		{
			name: "the pseudo-package leads a cause that sorts earlier",
			faults: []fault{
				{pkgPath: "x/a", message: "cannot use text as int"},
				{pkgPath: "x/...", message: "reading lifted/go.mod: no such file"},
			},
			wantFirst: "x/...",
			wantShare: 1,
		},
		{
			name: "only symptoms fall back to the smallest path",
			faults: []fault{
				{pkgPath: "x/b", message: `could not import y (invalid package name: "")`},
				{pkgPath: "x/a", message: `could not import y (invalid package name: "")`},
			},
			wantFirst: "x/a",
			wantShare: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var pkgs []*packages.Package

			for _, found := range testCase.faults {
				pkgs = append(pkgs, &packages.Package{
					ID:      found.pkgPath,
					PkgPath: found.pkgPath,
					Errors:  []packages.Error{{Msg: found.message}},
				})
			}

			got := shortfallOf(pkgs)
			if got.FirstPackage != testCase.wantFirst || got.SameMessage != testCase.wantShare {
				t.Fatalf("shortfallOf() = %+v, want first %q sharing %d",
					got, testCase.wantFirst, testCase.wantShare)
			}
		})
	}
}
