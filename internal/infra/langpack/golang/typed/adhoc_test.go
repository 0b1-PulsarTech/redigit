package typed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestAdhocLoads(t *testing.T) {
	t.Parallel()

	root := module{dir: ".", path: "example.test/m"}

	testCases := []struct {
		name    string
		modules []module
		files   []string
		want    []adhocLoad
	}{
		{
			"a testdata directory",
			[]module{root},
			[]string{"go.mod", "a.go", "testdata/fix/a.go", "testdata/fix/b.go"},
			[]adhocLoad{{".", []string{"./testdata/fix"}}},
		},
		{
			"a nested testdata, an underscore and a dot directory",
			[]module{root},
			[]string{"pkg/testdata/x/a.go", "_scratch/a.go", ".hidden/a.go", "pkg/a.go"},
			[]adhocLoad{{".", []string{"./.hidden", "./_scratch", "./pkg/testdata/x"}}},
		},
		{
			"vendor is skipped wherever it sits",
			[]module{{dir: ".", path: "example.test/m"}},
			[]string{"vendor/testdata/a.go", "testdata/vendor/a.go", "_x/vendor/a.go"},
			nil,
		},
		{
			"only go files name a directory",
			[]module{{dir: ".", path: "example.test/m"}},
			[]string{"testdata/fix/data.json", "testdata/README.md"},
			nil,
		},
		{
			"the module directory itself is not read as a segment",
			[]module{{dir: ".dot/mod", path: "example.test/m"}},
			[]string{".dot/mod/a.go", ".dot/mod/lib/a.go", ".dot/mod/testdata/a.go"},
			[]adhocLoad{{".dot/mod", []string{"./testdata"}}},
		},
		{
			"the nearest module owns the file and is where it loads from",
			[]module{root, {dir: "tools", path: "example.test/t"}},
			[]string{
				"tools/lib/a.go", "tools/testdata/a.go", "other/a.go", "testdata/b.go",
				"tools/testdata/sub/a.go",
			},
			[]adhocLoad{
				{".", []string{"./testdata"}},
				{"tools", []string{"./testdata", "./testdata/sub"}},
			},
		},
		{
			"a file outside every module is left out",
			[]module{{dir: "sub", path: "example.test/s"}},
			[]string{"testdata/a.go"},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := adhocLoads(entities.Snapshot{Files: testCase.files}, testCase.modules)
			if !slices.EqualFunc(got, testCase.want, func(left, right adhocLoad) bool {
				return left.dir == right.dir && slices.Equal(left.patterns, right.patterns)
			}) {
				t.Fatalf("adhocLoads() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestSourceAdhocKeepsTheMainLoadApart(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		files     map[string]string
		wantAdhoc []string
	}{
		{
			"a testdata package with an external test package",
			map[string]string{
				"a.go": "package fixture\n\nfunc One() int { return 1 }\n",
				"testdata/fix/a.go": `package fix

func Two() int { return 2 }

func Run() int { return Two() }
`,
				"testdata/fix/a_x_test.go": `package fix_test

import "example.test/fixture/testdata/fix"

func TestX() { _ = fix.Two() }
`,
				"testdata/bad/b.go": "package bad\n\nfunc Broken() int { return \"s\" }\n",
			},
			[]string{"testdata/fix/a.go", "testdata/fix/a_x_test.go", "testdata/bad/b.go"},
		},
		{
			"a tree without any such directory loads nothing apart",
			map[string]string{"a.go": "package fixture\n\nfunc One() int { return 1 }\n"},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, testCase.files)
			source := NewSource(nil)

			adhoc, err := source.Adhoc(context.Background(), snap)
			if err != nil {
				t.Fatalf("Adhoc() error = %v", err)
			}

			facts, err := source.Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			main, err := source.Packages(context.Background(), snap)
			if err != nil {
				t.Fatalf("Packages() error = %v", err)
			}

			inAdhoc, inMain := sourceFiles(slices.Concat(adhoc...)), sourceFiles(main)
			for _, want := range testCase.wantAdhoc {
				if !inAdhoc[want] || inMain[want] {
					t.Fatalf("%s: adhoc = %v, main = %v", want, inAdhoc, inMain)
				}
			}

			for _, use := range facts.Uses {
				if strings.HasPrefix(use.Dependent.File, "testdata/") {
					t.Fatalf("Facts() holds %+v from the ad hoc pass", use)
				}
			}

			if shortfalls := source.Shortfalls(); len(shortfalls) != 0 {
				t.Fatalf("Shortfalls() = %v, want none: the ad hoc pass is not noted", shortfalls)
			}
		})
	}
}

func sourceFiles(pkgs []*packages.Package) map[string]bool {
	files := map[string]bool{}

	for _, pkg := range pkgs {
		for _, name := range pkg.GoFiles {
			rel := filepath.ToSlash(name)
			if at := strings.Index(rel, "/testdata/"); at >= 0 {
				rel = rel[at+1:]
			} else {
				rel = filepath.Base(rel)
			}

			files[rel] = true
		}
	}

	return files
}

func TestSourceFactsLeaveTheAdhocPassUnrun(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"a.go":              "package fixture\n\nfunc One() int { return 1 }\n",
		"testdata/fix/a.go": "package fix\n\nfunc Two() int { return 2 }\n",
	})
	source := NewSource(nil)

	if _, err := source.Facts(context.Background(), snap, decls); err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	probed := false

	_, err := source.adhoc.Do(
		context.Background(),
		func(context.Context) ([][]*packages.Package, error) {
			probed = true

			return nil, nil
		},
	)
	if err != nil || !probed {
		t.Fatalf("Facts() ran the ad hoc pass, which only the resolver reads: %v", err)
	}
}

func TestSourceAdhocFailureIsNotTheRunsFailure(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"a.go":              "package fixture\n\nfunc One() int { return 1 }\n",
		"testdata/fix/a.go": "package fix\n\nfunc Two() int { return 2 }\n",
	})
	source := NewSource(nil)

	if _, err := source.Packages(context.Background(), snap); err != nil {
		t.Fatalf("Packages() error = %v", err)
	}

	if err := os.Remove(filepath.Join(snap.Root, "go.mod")); err != nil {
		t.Fatalf("remove go.mod: %v", err)
	}

	for range 2 {
		_, err := source.Adhoc(context.Background(), snap)
		if err == nil || !strings.Contains(err.Error(), "ad hoc pass") {
			t.Fatalf("Adhoc() error = %v, want it worded as the ad hoc pass", err)
		}
	}

	if _, err := source.Facts(context.Background(), snap, decls); err != nil {
		t.Fatalf("Facts() error = %v after an ad hoc failure", err)
	}

	if _, err := source.Packages(context.Background(), snap); err != nil {
		t.Fatalf("Packages() error = %v after an ad hoc failure", err)
	}
}

func TestSourceAdhocLoadsANestedModulesTestdataFromThatModule(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"go.mod":                "module example.test/fixture\n\ngo 1.26\n",
		"a.go":                  "package fixture\n",
		"tools/go.mod":          "module example.test/tools\n\ngo 1.26\n",
		"tools/lib.go":          "package tools\n",
		"tools/testdata/x/a.go": "package x\n\nfunc One() int { return 1 }\n",
	}
	snap := entities.Snapshot{Root: t.TempDir()}

	for name, content := range files {
		testkit.WriteFile(t, snap.Root, name, content)

		snap.Files = append(snap.Files, name)
	}

	groups, err := NewSource(nil).Adhoc(context.Background(), snap)
	if err != nil {
		t.Fatalf("Adhoc() error = %v", err)
	}

	if loaded := sourceFiles(slices.Concat(groups...)); !loaded["testdata/x/a.go"] {
		t.Fatalf("Adhoc() loaded %v, want tools/testdata/x/a.go", loaded)
	}
}

func TestSourceAdhocSurvivesACancelledFirstCaller(t *testing.T) {
	t.Parallel()

	snap, _ := testkit.Fixture(t, map[string]string{
		"a.go":              "package fixture\n",
		"testdata/fix/a.go": "package fix\n\nfunc Two() int { return 2 }\n",
	})
	source := NewSource(nil)

	gone, cancel := context.WithCancel(context.Background())
	cancel()

	t.Cleanup(func() { _ = source.Close(context.Background()) })

	if _, err := source.Adhoc(gone, snap); !errors.Is(err, context.Canceled) {
		t.Fatalf("Adhoc() by a cancelled first caller error = %v, want context.Canceled", err)
	}

	groups, err := source.Adhoc(context.Background(), snap)
	if err != nil || len(groups) == 0 {
		t.Fatalf("Adhoc() by the second caller = %d groups, %v; want the answer", len(groups), err)
	}
}
