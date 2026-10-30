package typed

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestShowingTags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		constraint string
		tags       []string
		want       []string
		wantOK     bool
	}{
		{
			"a negated tag is shown by dropping it",
			"//go:build !dev",
			[]string{"dev", "x"},
			[]string{"x"},
			true,
		},
		{
			"a missing tag is shown by adding it",
			"//go:build integration",
			nil,
			[]string{"integration"},
			true,
		},
		{"a conjunction needs both", "//go:build a && b", []string{"a"}, []string{"a", "b"}, true},
		{"a contradiction is shown by nothing", "//go:build a && !a", nil, nil, false},
		{"not a constraint at all", "// just a comment", nil, nil, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, ok := showingTags(testCase.constraint, testCase.tags)
			if ok != testCase.wantOK || (ok && !reflect.DeepEqual(got, testCase.want)) {
				t.Fatalf(
					"showingTags() = %v, %v; want %v, %v",
					got,
					ok,
					testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}

func TestTypedSourceChecksTheFilesTheTagsHide(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"verifier.go": "package fixture\n\nfunc NewGateway() int { return 1 }\n",
		"verifier_prod.go": `//go:build !dev

package fixture

func NewVerifier() int { return NewGateway() }
`,
		"verifier_dev.go": "//go:build dev\n\npackage fixture\n\nfunc NewVerifier() int { return 0 }\n",
	})

	facts, err := NewSource([]string{"dev"}).Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	want := Use{
		Dependent: testkit.DeclKey("verifier_prod.go", entities.KindFunc, "NewVerifier"),
		Provider:  testkit.DeclKey("verifier.go", entities.KindFunc, "NewGateway"),
	}

	if slices.Contains(facts.Uses, want) {
		return
	}

	t.Fatalf(
		"Uses = %+v, missing %+v: the file hidden by the tags was never checked",
		facts.Uses,
		want,
	)
}

func TestSourcePackages(t *testing.T) {
	t.Parallel()

	tagged := map[string]string{
		"base.go":    "package fixture\n\nfunc Base() int { return 1 }\n",
		"dev.go":     "//go:build dev\n\npackage fixture\n\nfunc Dev() int { return 0 }\n",
		"not_dev.go": "//go:build !dev\n\npackage fixture\n\nfunc Dev() int { return 2 }\n",
	}

	testCases := []struct {
		name     string
		tags     []string
		wantFile string
		skipFile string
	}{
		{"the run tags pick the files of pass zero", []string{"dev"}, "dev.go", "not_dev.go"},
		{"no tags pick the other file", nil, "not_dev.go", "dev.go"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, tagged)
			source := NewSource(testCase.tags)

			pkgs, err := source.Packages(context.Background(), snap)
			if err != nil {
				t.Fatalf("Packages() error = %v", err)
			}

			files := map[string]bool{}

			for _, pkg := range pkgs {
				for _, file := range pkg.GoFiles {
					files[filepath.Base(file)] = true
				}
			}

			if !files[testCase.wantFile] || files[testCase.skipFile] {
				t.Fatalf(
					"pass 0 files = %v, want %s and not %s",
					files,
					testCase.wantFile,
					testCase.skipFile,
				)
			}

			again, err := source.Packages(context.Background(), snap)
			if err != nil || len(again) == 0 || again[0] != pkgs[0] {
				t.Fatalf("Packages() reloaded: %v, %v", again, err)
			}

			if _, err = source.Facts(context.Background(), snap, decls); err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			shared, err := source.Packages(context.Background(), snap)
			if err != nil || shared[0] != pkgs[0] {
				t.Fatalf("Facts reloaded the packages: %v", err)
			}
		})
	}
}

func TestSourcePackagesPropagatesALoadError(t *testing.T) {
	t.Parallel()

	snap := entities.Snapshot{Root: t.TempDir(), Files: []string{"go.mod"}}
	source := NewSource(nil)

	if pkgs, err := source.Packages(context.Background(), snap); err == nil || pkgs != nil {
		t.Fatalf("Packages() = %v, %v, want the load error", pkgs, err)
	}

	if _, err := source.Facts(context.Background(), snap, nil); err == nil {
		t.Fatal("Facts() error = nil, want the same load error")
	}
}

func TestSourceLoadSurvivesACancelledFirstCaller(t *testing.T) {
	t.Parallel()

	snap, _ := testkit.Fixture(t, map[string]string{"a.go": "package fixture\n"})
	source := NewSource(nil)

	gone, cancel := context.WithCancel(context.Background())
	cancel()

	t.Cleanup(func() { _ = source.Close(context.Background()) })

	if _, err := source.Packages(gone, snap); !errors.Is(err, context.Canceled) {
		t.Fatalf("Packages() by a cancelled first caller error = %v, want context.Canceled", err)
	}

	pkgs, err := source.Packages(context.Background(), snap)
	if err != nil || len(pkgs) == 0 {
		t.Fatalf(
			"Packages() by the second caller = %d packages, %v; want the answer",
			len(pkgs),
			err,
		)
	}
}
