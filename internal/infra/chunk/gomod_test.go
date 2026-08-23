package chunk

import (
	"context"
	"testing"
)

func TestGoModHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"root go.mod", "go.mod", true},
		{"nested go.mod", "modules/lead/go.mod", true},
		{"not go.mod", "modules/lead/go.sum", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := (GoMod{}).Handles(testCase.path); got != testCase.want {
				t.Fatalf("Handles(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestGoModChunks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		source    string
		wantNames []string
	}{
		{
			name:      "empty file",
			source:    "",
			wantNames: []string{fileDeclName},
		},
		{
			name:      "header only, no requires",
			source:    "module github.com/x/y\n\ngo 1.26\n",
			wantNames: []string{"github.com/x/y"},
		},
		{
			name: "single-line require",
			source: "module github.com/x/y\n\ngo 1.26\n\n" +
				"require github.com/a/a v1.0.0\n",
			wantNames: []string{"github.com/x/y", "require github.com/a/a"},
		},
		{
			name: "require block versus single-line require",
			source: "module github.com/x/y\n\ngo 1.26\n\n" +
				"require (\n\tgithub.com/a/a v1.0.0\n\tgithub.com/b/b v1.0.0\n)\n\n" +
				"require github.com/c/c v1.0.0\n",
			wantNames: []string{
				"github.com/x/y", "require github.com/a/a",
				"require github.com/b/b", "require github.com/c/c",
			},
		},
		{
			name: "replace with a local ../ path",
			source: "module github.com/x/y\n\ngo 1.26\n\n" +
				"require github.com/a/a v1.0.0\n\n" +
				"replace github.com/a/a => ../a\n",
			// The require and the replace of the same module must not share a
			// name: one DeclKey for both lines means only one of them is ever
			// emitted, and the file loses the other line's bytes.
			wantNames: []string{
				"github.com/x/y", "require github.com/a/a", "replace github.com/a/a",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := (GoMod{}).Chunks(context.Background(), "go.mod", []byte(testCase.source))
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)
			assertDeclNames(t, decls, testCase.wantNames)
		})
	}
}
