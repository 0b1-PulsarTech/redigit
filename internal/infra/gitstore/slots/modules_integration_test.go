//go:build integration

package slots

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func TestModulesCarryTheWorkspaceTheTreeShips(t *testing.T) {
	repo := newRepo(t)
	withWork := commitFiles(t, repo, "workspace", []fixtureFile{
		{path: "go.work", content: "go 1.26\n\nuse (\n\t./a\n\t./b\n)\n", mode: 0o644},
		{path: "go.work.sum", content: "sum-text", mode: 0o644},
		{path: "a/go.mod", content: "module ex/a\n\ngo 1.26\n", mode: 0o644},
		{path: "b/go.mod", content: "module ex/b\n\ngo 1.26\n", mode: 0o644},
		{path: "sub/go.work", content: "go 1.26\n\nuse ./x\n", mode: 0o644},
		{path: "sub/x/go.mod", content: "module ex/x\n\ngo 1.26\n", mode: 0o644},
		{path: "testdata/go.work", content: "go 1.26\n", mode: 0o644},
		{path: "app/pubspec.yaml", content: "name: app\n", mode: 0o644},
	})
	without := commitFiles(t, repo, "no workspace", []fixtureFile{
		{path: "a/go.mod", content: "module ex/a\n\ngo 1.26\n", mode: 0o644},
	})
	lister := NewModules(
		filepath.Join(repo, ".git"),
		map[string]string{"go.mod": "go", "pubspec.yaml": "dart"},
	)

	got, err := lister.List(t.Context(), withWork)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	workspaces := map[string]string{}
	for _, module := range got.Modules {
		workspaces[module.Dir] = module.Workspace
	}

	rootWork := workspaces["a"]
	if !strings.Contains(rootWork, "./b") || !strings.Contains(rootWork, "sum-text") ||
		rootWork != workspaces["b"] {
		t.Fatalf("a and b share the root workspace and its sum, got %q and %q",
			workspaces["a"], workspaces["b"])
	}

	if !strings.Contains(workspaces["sub/x"], "use ./x") || workspaces["sub/x"] == workspaces["a"] {
		t.Fatalf("sub/x must carry the nearest go.work, got %q", workspaces["sub/x"])
	}

	if workspaces["app"] != "" {
		t.Fatalf("a non-go module has no workspace, got %q", workspaces["app"])
	}

	bare, err := lister.List(t.Context(), without)
	if err != nil || len(bare.Modules) != 1 || bare.Modules[0].Workspace != "" {
		t.Fatalf("a tree without go.work = %+v err = %v, want an empty workspace", bare, err)
	}
}

func TestModulesNameGoFilesOutsideEveryGoMod(t *testing.T) {
	repo := newRepo(t)
	tree := commitFiles(t, repo, "strays", []fixtureFile{
		{path: "main.go", content: "package main\n", mode: 0o644},
		{path: "gen/x.go", content: "package gen\n", mode: 0o644},
		{path: "mod/go.mod", content: "module ex/mod\n\ngo 1.26\n", mode: 0o644},
		{path: "mod/a.go", content: "package mod\n", mode: 0o644},
		{path: "mod/sub/b.go", content: "package sub\n", mode: 0o644},
		{path: "testdata/t.go", content: "package t\n", mode: 0o644},
		{path: "vendor/v.go", content: "package v\n", mode: 0o644},
		{path: "app/pubspec.yaml", content: "name: app\n", mode: 0o644},
		{path: "app/lib.go", content: "package app\n", mode: 0o644},
	})
	rooted := commitFiles(t, repo, "rooted", []fixtureFile{
		{path: "go.mod", content: "module ex/root\n\ngo 1.26\n", mode: 0o644},
		{path: "deep/x.go", content: "package deep\n", mode: 0o644},
	})
	lister := NewModules(
		filepath.Join(repo, ".git"),
		map[string]string{"go.mod": "go", "pubspec.yaml": "dart"},
	)

	got, err := lister.List(t.Context(), tree)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if want := []string{
		"app/lib.go",
		"gen/x.go",
		"main.go",
	}; !slices.Equal(
		sorted(got.Strays),
		want,
	) {
		t.Fatalf("strays = %v, want %v", got.Strays, want)
	}

	covered, err := lister.List(t.Context(), rooted)
	if err != nil || len(covered.Strays) != 0 {
		t.Fatalf("a root go.mod covers every go file, got %+v err = %v", covered, err)
	}
}

func TestModulesNameTheirLocalDirectories(t *testing.T) {
	appMod := func(extra string) fixtureFile {
		return fixtureFile{
			path: "app/go.mod", content: "module ex/app\n\ngo 1.26\n" + extra, mode: 0o644,
		}
	}
	shared := fixtureFile{path: "shared/s.go", content: "package shared\n", mode: 0o644}

	testCases := []struct {
		name       string
		files      []fixtureFile
		wantLocals []string
		wantWide   bool
	}{
		{"no replace", []fixtureFile{appMod("")}, nil, false},
		{
			"a replace to a sibling directory",
			[]fixtureFile{appMod("\nreplace ex/s => ../shared\n"), shared},
			[]string{"shared"},
			false,
		},
		{
			"a go.work use of a directory that is no module",
			[]fixtureFile{
				{
					path:    "go.work",
					content: "go 1.26\n\nuse (\n\t./app\n\t./shared\n)\n",
					mode:    0o644,
				},
				appMod(""),
				shared,
			},
			[]string{"app", "shared"},
			false,
		},
		{
			"a replace through a symlink",
			[]fixtureFile{
				appMod("\nreplace ex/s => ./link\n"), shared,
				{path: "app/link", link: "../shared"},
			},
			nil, true,
		},
		{
			"a replace leaving the repository",
			[]fixtureFile{appMod("\nreplace ex/s => ../../outside\n"), shared},
			nil, true,
		},
		{
			"a replace to a directory the tree lacks",
			[]fixtureFile{appMod("\nreplace ex/s => ./missing\n")},
			nil, true,
		},
		{
			"a go.mod the strict parser rejects",
			[]fixtureFile{appMod("\nfuture-directive x\n"), shared},
			nil, true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			app, commit := listApp(t, testCase.files)

			dirs := make([]string, 0, len(app.Locals))
			for _, local := range app.Locals {
				dirs = append(dirs, local.Dir)
			}

			if !slices.Equal(dirs, testCase.wantLocals) ||
				(app.Subtree == commit.Tree) != testCase.wantWide {
				t.Fatalf(
					"app = %+v, want locals %v wide %v",
					app,
					testCase.wantLocals,
					testCase.wantWide,
				)
			}
		})
	}
}

func TestModulesKeyTheWholeTreeWhenALinkCouldReachOutside(t *testing.T) {
	appMod := fixtureFile{path: "app/go.mod", content: "module ex/app\n\ngo 1.26\n", mode: 0o644}
	replaced := fixtureFile{
		path:    "app/go.mod",
		content: "module ex/app\n\ngo 1.26\n\nreplace ex/s => ../shared\n",
		mode:    0o644,
	}
	target := fixtureFile{path: "shared/shared.txt", content: "package app\n", mode: 0o644}

	testCases := []struct {
		name  string
		files []fixtureFile
		want  bool
	}{
		{"no link at all", []fixtureFile{appMod, target}, false},
		{
			"a file link inside the module",
			[]fixtureFile{appMod, target, {path: "app/shared.go", link: "../shared/shared.txt"}},
			true,
		},
		{
			"a link under a replace target",
			[]fixtureFile{replaced, target, {path: "shared/other.txt", link: "shared.txt"}},
			true,
		},
		{
			"a go.mod that is a link",
			[]fixtureFile{
				{path: "app/go.mod", link: "../shared/go.mod"},
				{path: "shared/go.mod", content: "module ex/app\n\ngo 1.26\n", mode: 0o644},
			},
			true,
		},
		{
			"a link in a sibling module",
			[]fixtureFile{appMod, target, {path: "other/ln", link: "../shared/shared.txt"}},
			true,
		},
		{
			"a link only inside testdata",
			[]fixtureFile{appMod, target, {path: "app/testdata/ln", link: "x"}},
			false,
		},
		{
			"a link under an underscore dir",
			[]fixtureFile{appMod, target, {path: "app/_gen/x.go", link: "../../shared/shared.txt"}},
			true,
		},
		{
			"a link under a dot dir",
			[]fixtureFile{appMod, target, {path: "app/.gen/x.go", link: "../../shared/shared.txt"}},
			true,
		},
		{
			"a link under vendor",
			[]fixtureFile{
				appMod, target, {path: "app/vendor/p/x.go", link: "../../../shared/shared.txt"},
			},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			app, commit := listApp(t, testCase.files)

			if (app.Subtree == commit.Tree) != testCase.want ||
				(testCase.want && len(app.Locals) != 0) {
				t.Fatalf("app = %+v, want wide %v", app, testCase.want)
			}
		})
	}
}

func listApp(t *testing.T, files []fixtureFile) (verifying.Module, verifying.Commit) {
	t.Helper()

	repo := newRepo(t)
	commit := commitFiles(t, repo, "locals", files)
	lister := NewModules(filepath.Join(repo, ".git"), map[string]string{"go.mod": "go"})

	got, err := lister.List(t.Context(), commit)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	for _, module := range got.Modules {
		if module.Dir == "app" {
			return module, commit
		}
	}

	return verifying.Module{}, commit
}

func TestModuleKeyFollowsAReplaceTarget(t *testing.T) {
	repo := newRepo(t)
	lister := NewModules(filepath.Join(repo, ".git"), map[string]string{"go.mod": "go"})
	files := func(body string) []fixtureFile {
		return []fixtureFile{
			{
				path:    "app/go.mod",
				content: "module ex/app\n\ngo 1.26\n\nreplace ex/s => ../shared\n",
				mode:    0o644,
			},
			{path: "shared/s.go", content: body, mode: 0o644},
		}
	}

	keyOf := func(body string) string {
		listing, err := lister.List(t.Context(), commitFiles(t, repo, body, files(body)))
		if err != nil {
			t.Fatalf("list: %v", err)
		}

		return verifying.Key(verifying.Resolve(listing.Modules)[0], "id")
	}

	if first, second := keyOf("package a\n"), keyOf("package b\n"); first == second {
		t.Fatal("editing a replace target must change the key of the module that replaces to it")
	}
}

func TestModuleKeyFollowsTheWorkspaceVendor(t *testing.T) {
	testCases := []struct {
		name string
		root string
	}{
		{"a go.work at the repository root", ""},
		{"a go.work in a subdirectory", "sub/"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newRepo(t)
			lister := NewModules(filepath.Join(repo, ".git"), map[string]string{"go.mod": "go"})
			files := func(body string) []fixtureFile {
				return []fixtureFile{
					{
						path:    testCase.root + "go.work",
						content: "go 1.26\n\nuse ./app\n",
						mode:    0o644,
					},
					{
						path:    testCase.root + "app/go.mod",
						content: "module ex/app\n\ngo 1.26\n",
						mode:    0o644,
					},
					{path: testCase.root + "vendor/x/x.go", content: body, mode: 0o644},
				}
			}

			keyOf := func(body string) string {
				listing, err := lister.List(t.Context(), commitFiles(t, repo, body, files(body)))
				if err != nil {
					t.Fatalf("list: %v", err)
				}

				return verifying.Key(verifying.Resolve(listing.Modules)[0], "id")
			}

			if first, second := keyOf("package a\n"), keyOf("package b\n"); first == second {
				t.Fatal("editing the workspace vendor tree must change the key of its modules")
			}
		})
	}
}

func sorted(values []string) []string {
	return slices.Sorted(slices.Values(values))
}
