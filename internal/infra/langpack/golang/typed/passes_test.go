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

func TestPackageOfFile(t *testing.T) {
	t.Parallel()

	root := []module{{dir: ".", path: "example.test/root"}}
	nested := []module{
		{dir: ".", path: "example.test/root"},
		{dir: "libs/kit", path: "example.test/kit"},
	}

	testCases := []struct {
		name    string
		modules []module
		file    string
		want    string
		wantOK  bool
	}{
		{"a root module's root file", root, "main.go", "example.test/root", true},
		{
			"a root module's nested package",
			root,
			"internal/store/db.go",
			"example.test/root/internal/store",
			true,
		},
		{
			"a root module's deeper package",
			root,
			"internal/a/b/c.go",
			"example.test/root/internal/a/b",
			true,
		},
		{"a nested module's root file", nested, "libs/kit/kit.go", "example.test/kit", true},
		{
			"a nested module's package",
			nested,
			"libs/kit/ids/id.go",
			"example.test/kit/ids",
			true,
		},
		{
			"a file the root module owns beside a nested module",
			nested,
			"libs/other/o.go",
			"example.test/root/libs/other",
			true,
		},
		{"a file outside every module", []module{nested[1]}, "docs/a.go", "", false},
		{"no modules at all", nil, "main.go", "", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, ok := packageOfFile(testCase.modules, testCase.file)
			if got != testCase.want || ok != testCase.wantOK {
				t.Fatalf(
					"packageOfFile(%q) = %q, %v; want %q, %v",
					testCase.file,
					got,
					ok,
					testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}

func TestHiddenPassesNameThePackagesOfARootModule(t *testing.T) {
	t.Parallel()

	snap, _ := testkit.Fixture(t, map[string]string{
		"top.go":                  "package fixture\n",
		"top_it.go":               "//go:build integration\n\npackage fixture\n",
		"internal/store/db.go":    "package store\n",
		"internal/store/db_it.go": "//go:build integration\n\npackage store\n",
	})

	modules, err := modulesOf(snap)
	if err != nil {
		t.Fatalf("modulesOf() error = %v", err)
	}

	got, err := hiddenPasses(snap, nil, modules)
	if err != nil {
		t.Fatalf("hiddenPasses() error = %v", err)
	}

	want := []loadPass{{
		tags: []string{"integration"},
		patterns: []string{
			"example.test/fixture",
			"example.test/fixture/internal/store",
		},
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hiddenPasses() = %+v, want %+v", got, want)
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

func TestOwnerOfCountsTheRootModuleAsDepthZero(t *testing.T) {
	t.Parallel()

	root := module{dir: ".", path: "example.test/root"}
	nested := module{dir: "x", path: "example.test/nested"}

	testCases := []struct {
		name    string
		modules []module
	}{
		{name: "the root module listed first", modules: []module{root, nested}},
		{name: "the nested module listed first", modules: []module{nested, root}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, ok := ownerOf(testCase.modules, "x/h.go")
			if !ok || got != nested {
				t.Fatalf("ownerOf() = %+v, %v; want %+v", got, ok, nested)
			}
		})
	}
}

func TestTypedSourceChecksAHiddenFileAgainstANestedModule(t *testing.T) {
	t.Parallel()

	const header = "module example.test/fixture\n\ngo 1.26\n"

	testCases := []struct {
		name  string
		gomod string
		work  string
	}{
		{
			name: "through a replace",
			gomod: header + "\nrequire example.test/fixture/nested v0.0.0\n\n" +
				"replace example.test/fixture/nested => ./nested\n",
		},
		{
			name:  "through a go.work",
			gomod: header,
			work:  "go 1.26\n\nuse (\n\t.\n\t./nested\n)\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"go.mod":        testCase.gomod,
				"nested/go.mod": "module example.test/fixture/nested\n\ngo 1.26\n",
				"nested/n.go":   "package nested\n\nfunc F() {}\n",
				"top/top.go":    "package top\n",
				"top/hid.go": "//go:build hid\n\npackage top\n\n" +
					"import \"example.test/fixture/nested\"\n\nfunc Hidden() { nested.F() }\n",
			}
			if testCase.work != "" {
				files["go.work"] = testCase.work
			}

			snap, decls := writeTree(t, t.TempDir(), files, nil)

			facts, err := NewSource(nil).Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			want := Use{
				Dependent: testkit.DeclKey("top/hid.go", entities.KindFunc, "Hidden"),
				Provider:  testkit.DeclKey("nested/n.go", entities.KindFunc, "F"),
			}
			if !slices.Contains(facts.Uses, want) {
				t.Fatalf("Uses = %+v, missing %+v", facts.Uses, want)
			}
		})
	}
}

func TestTypedSourceChecksAHiddenFileAgainstAModuleReachedOnlyTransitively(t *testing.T) {
	t.Parallel()

	const header = "module example.test/root\n\ngo 1.26\n"

	testCases := []struct {
		name  string
		gomod string
		work  string
	}{
		{
			name: "through a replace",
			gomod: header + "\nrequire example.test/base v0.0.0\n\n" +
				"replace example.test/base => ./libs/base\n",
		},
		{
			name:  "through a go.work",
			gomod: header + "\nrequire example.test/base v0.0.0\n",
			work:  "go 1.26\n\nuse (\n\t.\n\t./libs/base\n)\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"go.mod":            testCase.gomod,
				"libs/base/go.mod":  "module example.test/base\n\ngo 1.26\n",
				"libs/base/base.go": "package base\n\ntype Repo struct{}\n\nfunc (Repo) Save() {}\n",
				"repo/repo.go": "package repo\n\nimport \"example.test/base\"\n\n" +
					"type User struct{ base.Repo }\n",
				"top/top.go": "package top\n",
				"top/hid.go": "//go:build hid\n\npackage top\n\n" +
					"import \"example.test/root/repo\"\n\n" +
					"func Hidden(user repo.User) { user.Save() }\n",
			}
			if testCase.work != "" {
				files["go.work"] = testCase.work
			}

			snap, decls := writeTree(t, t.TempDir(), files, nil)

			facts, err := NewSource(nil).Facts(context.Background(), snap, decls)
			if err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			want := Use{
				Dependent: testkit.DeclKey("top/hid.go", entities.KindFunc, "Hidden"),
				Provider:  testkit.DeclKey("libs/base/base.go", entities.KindMethod, "Repo.Save"),
			}
			if !slices.Contains(facts.Uses, want) {
				t.Fatalf("Uses = %+v, missing %+v", facts.Uses, want)
			}
		})
	}
}
