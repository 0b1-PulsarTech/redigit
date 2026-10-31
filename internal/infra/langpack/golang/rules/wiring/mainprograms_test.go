package wiring

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestMainPackageSharedFileWaitsForTheMainsThatBuild(t *testing.T) {
	t.Parallel()

	mainKey := mainFn("main.go", "main")
	shared := mainFn("shared.go", "shared")

	testCases := []mainForm{
		{
			name: "a go:build ignore main is a separate program nobody waits for",
			files: map[string]string{
				"main.go":   pkgMain,
				"shared.go": pkgMain,
				"gen.go":    "//go:build ignore\n\npackage main\n",
			},
			decls: []entities.DeclKey{mainKey, mainFn("gen.go", "main"), shared},
			want:  []entities.Edge{mainHosts(shared, mainKey)},
		},
		{
			name: "an ignore main alone leaves a shared file without a main to wait for",
			files: map[string]string{
				"shared.go": pkgMain, "gen.go": "//go:build ignore\n\npackage main\n",
			},
			decls: []entities.DeclKey{mainFn("gen.go", "main"), shared},
		},
		{
			name: "an unconstrained main is the only one a shared file waits for",
			files: map[string]string{
				"main.go":     pkgMain,
				"shared.go":   pkgMain,
				"main_tag.go": "//go:build integration\n\npackage main\n",
				"main_win.go": "//go:build windows\n\npackage main\n",
			},
			decls: []entities.DeclKey{
				mainKey, mainFn("main_tag.go", "main"), mainFn("main_win.go", "main"), shared,
			},
			want: []entities.Edge{mainHosts(shared, mainKey)},
		},
		{
			name: "a main behind a file name suffix is not unconstrained",
			files: map[string]string{
				"main.go":         pkgMain,
				"shared.go":       pkgMain,
				"main_windows.go": pkgMain,
			},
			decls: []entities.DeclKey{mainKey, mainFn("main_windows.go", "main"), shared},
			want:  []entities.Edge{mainHosts(shared, mainKey)},
		},
		{
			name: "a constraint that cannot be parsed names the file",
			files: map[string]string{
				"main.go": pkgMain, "bad.go": "//go:build &&\n\npackage main\n",
			},
			decls:   []entities.DeclKey{mainKey, mainFn("bad.go", "help")},
			wantErr: "bad.go",
		},
		{
			name: "with every main constrained a shared file waits for each of them",
			files: map[string]string{
				"shared.go":  pkgMain,
				"main_a.go":  "//go:build a\n\npackage main\n",
				"main_na.go": "//go:build !a\n\npackage main\n",
			},
			decls: []entities.DeclKey{
				mainFn("main_a.go", "main"), mainFn("main_na.go", "main"), shared,
			},
			want: []entities.Edge{
				mainHosts(shared, mainFn("main_a.go", "main")),
				mainHosts(shared, mainFn("main_na.go", "main")),
			},
		},
	}

	checkMainForms(t, testCases)
}

func TestMainPackageNamesTheFirstBadConstraintByFile(t *testing.T) {
	t.Parallel()

	mainKey := mainFn("main.go", "main")

	testCases := []mainForm{
		{
			name: "the first bad constraint by file name is the one named",
			files: map[string]string{
				"main.go":  pkgMain,
				"d_bad.go": "//go:build &&\n\npackage main\n",
				"b_bad.go": "//go:build &&\n\npackage main\n",
				"a_bad.go": "//go:build &&\n\npackage main\n",
				"c_bad.go": "//go:build &&\n\npackage main\n",
			},
			decls: []entities.DeclKey{
				mainKey, mainFn("d_bad.go", "help"), mainFn("b_bad.go", "help"),
				mainFn("a_bad.go", "help"), mainFn("c_bad.go", "help"),
			},
			wantErr: "a_bad.go",
		},
	}

	checkMainForms(t, testCases)
}

func TestMainPackageLicencedProgramsKeepTheirConstraint(t *testing.T) {
	t.Parallel()

	mainKey := mainFn("main.go", "main")
	shared := mainFn("shared.go", "shared")

	testCases := []mainForm{
		{
			name: "a licence block before go:build ignore still sets the program apart",
			files: map[string]string{
				"main.go":   pkgMain,
				"shared.go": pkgMain,
				"gen.go":    "/*\nCopyright 2026 X.\n*/\n\n//go:build ignore\n\npackage main\n",
			},
			decls: []entities.DeclKey{mainKey, mainFn("gen.go", "main"), shared},
			want:  []entities.Edge{mainHosts(shared, mainKey)},
		},
		{
			name: "a go:build line inside a block comment leaves the main unconstrained",
			files: map[string]string{
				"main.go":   pkgMain,
				"shared.go": pkgMain,
				"gen.go":    "/*\n//go:build ignore\n*/\n\npackage main\n",
			},
			decls: []entities.DeclKey{mainKey, mainFn("gen.go", "main"), shared},
			want: []entities.Edge{
				mainHosts(shared, mainFn("gen.go", "main")), mainHosts(shared, mainKey),
			},
		},
		{
			name: "a licence block before a constraint keeps the main constrained",
			files: map[string]string{
				"shared.go":  pkgMain,
				"main_a.go":  "/* licence */\n\n//go:build a\n\npackage main\n",
				"main_na.go": "//go:build !a\n\npackage main\n",
			},
			decls: []entities.DeclKey{
				mainFn("main_a.go", "main"), mainFn("main_na.go", "main"), shared,
			},
			want: []entities.Edge{
				mainHosts(shared, mainFn("main_a.go", "main")),
				mainHosts(shared, mainFn("main_na.go", "main")),
			},
		},
	}

	checkMainForms(t, testCases)
}

func TestMainPackageFilesGoWouldNotBuildEverywhere(t *testing.T) {
	t.Parallel()

	mainKey := mainFn("main.go", "main")
	shared := mainFn("shared.go", "shared")

	testCases := []mainForm{
		{
			name: "a main importing C is constrained, so the shared file waits for both",
			files: map[string]string{
				"shared.go":  pkgMain,
				"main_c.go":  "package main\n\nimport \"C\"\n",
				"main_nc.go": "//go:build !cgo\n\npackage main\n",
			},
			decls: []entities.DeclKey{
				mainFn("main_c.go", "main"), mainFn("main_nc.go", "main"), shared,
			},
			want: []entities.Edge{
				mainHosts(shared, mainFn("main_c.go", "main")),
				mainHosts(shared, mainFn("main_nc.go", "main")),
			},
		},
		{
			name: "files Go ignores by an underscore or dot prefix host and wait for nothing",
			files: map[string]string{
				"main.go": pkgMain, "shared.go": pkgMain, "_skip.go": pkgMain, ".hid.go": pkgMain,
			},
			decls: []entities.DeclKey{
				mainKey, mainFn("_skip.go", "main"), mainFn("_skip.go", "help"),
				mainFn(".hid.go", "hidden"), shared,
			},
			want: []entities.Edge{mainHosts(shared, mainKey)},
		},
	}

	checkMainForms(t, testCases)
}
