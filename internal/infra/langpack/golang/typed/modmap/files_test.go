package modmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesRel(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "tree")
	elsewhere := filepath.Join(base, "elsewhere")

	for _, dir := range []string{root, elsewhere} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	if err := os.MkdirAll(filepath.Join(root, "p"), 0o750); err != nil {
		t.Fatalf("mkdir p: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "p", "q.go"), nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	link := filepath.Join(base, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "a file in the root", input: filepath.Join(root, "a.go"), want: "a.go"},
		{name: "a file below the root", input: filepath.Join(root, "p", "q.go"), want: "p/q.go"},
		{name: "a file through a symlink", input: filepath.Join(link, "p", "q.go"), want: "p/q.go"},
		{name: "a file outside the root", input: filepath.Join(elsewhere, "a.go"), want: ""},
		{name: "the parent of the root", input: filepath.Join(base, "a.go"), want: ""},
		{name: "a relative name with no module", input: "p/q.go", want: ""},
	}

	files := New(link)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range 2 {
				if got := files.Rel(testCase.input); got != testCase.want {
					t.Fatalf("Rel(%q) = %q, want %q", testCase.input, got, testCase.want)
				}
			}
		})
	}
}
