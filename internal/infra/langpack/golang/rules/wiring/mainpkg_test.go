package wiring

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

const pkgMain = "package main\n"

type mainForm struct {
	name    string
	files   map[string]string
	decls   []entities.DeclKey
	want    []entities.Edge
	wantErr string
}

func mainFn(file, name string) entities.DeclKey {
	return testkit.DeclKey(file, entities.KindFunc, name)
}

func mainHosts(dependent, provider entities.DeclKey) entities.Edge {
	return entities.Edge{Dependent: dependent, Provider: provider, Reason: entities.ReasonHosts}
}

func checkMainForms(t *testing.T, testCases []mainForm) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()

			for name, content := range testCase.files {
				testkit.WriteFile(t, root, name, content)
			}

			decls := make([]entities.Decl, 0, len(testCase.decls))
			for _, key := range testCase.decls {
				decls = append(decls, entities.Decl{Key: key})
			}

			got, err := MainPackage{}.Edges(
				context.Background(), entities.Snapshot{Root: root}, decls,
			)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("Edges() error = %v, want one naming %q", err, testCase.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Edges() = %+v, want %+v in this order", got, testCase.want)
			}
		})
	}
}

func TestMainPackageHostsTheDeclarationsOfItsFiles(t *testing.T) {
	t.Parallel()

	mainKey := mainFn("main.go", "main")
	runner := testkit.DeclKey("main.go", entities.KindType, "runner")

	testCases := []mainForm{
		{
			name:  "the helpers of the file that declares main wait for it",
			files: map[string]string{"main.go": pkgMain},
			decls: []entities.DeclKey{
				mainKey, mainFn("main.go", "run"), mainFn("main.go", "exitUsage"), runner,
			},
			want: []entities.Edge{
				mainHosts(mainFn("main.go", "exitUsage"), mainKey),
				mainHosts(mainFn("main.go", "run"), mainKey),
				mainHosts(runner, mainKey),
			},
		},
		{
			name:  "a sibling file of package main waits for main",
			files: map[string]string{"main.go": pkgMain, "helpers.go": pkgMain},
			decls: []entities.DeclKey{mainKey, mainFn("helpers.go", "help")},
			want:  []entities.Edge{mainHosts(mainFn("helpers.go", "help"), mainKey)},
		},
		{
			name:  "a test file of package main is left alone",
			files: map[string]string{"main.go": pkgMain, "main_test.go": pkgMain},
			decls: []entities.DeclKey{mainKey, mainFn("main_test.go", "TestMain")},
		},
		{
			name:  "a func main outside package main hosts nothing",
			files: map[string]string{"lib/lib.go": "package lib\n"},
			decls: []entities.DeclKey{mainFn("lib/lib.go", "main"), mainFn("lib/lib.go", "Other")},
		},
		{
			name: "each tag-switched main hosts its own file and the shared ones both",
			files: map[string]string{
				"main_unix.go":    "//go:build unix\n\npackage main\n",
				"main_windows.go": pkgMain,
				"shared.go":       pkgMain,
			},
			decls: []entities.DeclKey{
				mainFn("main_unix.go", "main"),
				mainFn("main_unix.go", "unixOnly"),
				mainFn("main_windows.go", "main"),
				mainFn("main_windows.go", "winOnly"),
				mainFn("shared.go", "shared"),
			},
			want: []entities.Edge{
				mainHosts(mainFn("main_unix.go", "unixOnly"), mainFn("main_unix.go", "main")),
				mainHosts(mainFn("main_windows.go", "winOnly"), mainFn("main_windows.go", "main")),
				mainHosts(mainFn("shared.go", "shared"), mainFn("main_unix.go", "main")),
				mainHosts(mainFn("shared.go", "shared"), mainFn("main_windows.go", "main")),
			},
		},
	}

	checkMainForms(t, testCases)
}

func TestMainPackageEdgeCases(t *testing.T) {
	t.Parallel()

	mainKey := mainFn("main.go", "main")
	doc := testkit.DeclKey("doc.go", entities.KindOther, "<file>")

	testCases := []mainForm{
		{
			name: "a go:build ignore program stays apart from the main beside it",
			files: map[string]string{
				"main.go": pkgMain,
				"gen.go":  "//go:build ignore\n\npackage main\n",
			},
			decls: []entities.DeclKey{
				mainKey,
				mainFn("main.go", "run"),
				mainFn("gen.go", "main"),
				mainFn("gen.go", "generate"),
			},
			want: []entities.Edge{
				mainHosts(mainFn("gen.go", "generate"), mainFn("gen.go", "main")),
				mainHosts(mainFn("main.go", "run"), mainKey),
			},
		},
		{
			name:  "a file without declarations waits for main",
			files: map[string]string{"main.go": pkgMain, "doc.go": pkgMain},
			decls: []entities.DeclKey{mainKey, doc},
			want:  []entities.Edge{mainHosts(doc, mainKey)},
		},
		{
			name:  "a sliced main hosts run on its head and its statements wait for nothing",
			files: map[string]string{"main.go": pkgMain},
			decls: []entities.DeclKey{
				mainKey,
				testkit.DeclKey("main.go", entities.KindStmt, "main#1"),
				mainFn("main.go", "run"),
			},
			want: []entities.Edge{mainHosts(mainFn("main.go", "run"), mainKey)},
		},
		{
			name:  "a package main directory without a main has no edges",
			files: map[string]string{"cmd/a/main.go": pkgMain, "cmd/b/x.go": pkgMain},
			decls: []entities.DeclKey{
				mainFn("cmd/a/main.go", "main"), mainFn("cmd/b/x.go", "help"),
			},
		},
		{
			name:    "a file that cannot be read names the file",
			files:   map[string]string{"main.go": pkgMain},
			decls:   []entities.DeclKey{mainKey, mainFn("gone.go", "help")},
			wantErr: "gone.go",
		},
		{
			name:    "a file that cannot be parsed names the file",
			files:   map[string]string{"main.go": pkgMain, "bad.go": "this is not go\n"},
			decls:   []entities.DeclKey{mainKey, mainFn("bad.go", "help")},
			wantErr: "bad.go",
		},
	}

	checkMainForms(t, testCases)
}
