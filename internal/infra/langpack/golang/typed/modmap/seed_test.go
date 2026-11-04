package modmap

import (
	"go/token"
	"maps"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestSeed(t *testing.T) {
	t.Parallel()

	const (
		inside = fixture + "/a/a.go"
		other  = "fmt/print.go"
	)

	testCases := []struct {
		name  string
		files []string
		known map[string]string
		want  map[string]string
	}{
		{
			name:  "a trimmed name of the tree and one of the standard library",
			files: []string{inside, other},
			want:  map[string]string{inside: "a/a.go", other: ""},
		},
		{
			name:  "an absolute name is left to the caller",
			files: []string{filepath.Join(string(filepath.Separator), "abs", "a.go")},
			want:  map[string]string{},
		},
		{
			name:  "a name another group mapped the same way",
			files: []string{inside},
			known: map[string]string{inside: "a/a.go"},
			want:  map[string]string{inside: "a/a.go"},
		},
		{
			name:  "a name another group mapped to another file",
			files: []string{inside},
			known: map[string]string{inside: "b/b.go"},
			want:  map[string]string{inside: ""},
		},
		{
			name:  "a name another group found unmapped",
			files: []string{inside},
			known: map[string]string{inside: ""},
			want:  map[string]string{inside: "a/a.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			fset := token.NewFileSet()

			for _, name := range testCase.files {
				fset.AddFile(name, -1, 1)
			}

			module := &packages.Module{Path: fixture, Main: true, Dir: root}
			pkgs := []*packages.Package{
				{Fset: fset, Module: module},
				{Fset: fset, Module: module},
				{Module: module},
			}

			names := maps.Clone(testCase.known)
			if names == nil {
				names = map[string]string{}
			}

			Seed(root, pkgs, names)

			if !maps.Equal(names, testCase.want) {
				t.Fatalf("Seed() names = %v, want %v", names, testCase.want)
			}
		})
	}
}
