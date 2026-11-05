package chunk

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
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

func TestGoWorkHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want bool
	}{
		{"root go.work", "go.work", true},
		{"nested go.work", "sub/go.work", true},
		{"not go.work", "go.work.sum", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := (GoWork{}).Handles(testCase.path); got != testCase.want {
				t.Fatalf("Handles(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestGoWorkChunks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		source    string
		wantNames []string
	}{
		{"empty file", "", []string{fileDeclName}},
		{"header only", "go 1.26\n", []string{fileDeclName}},
		{
			"single use line",
			"go 1.26\n\nuse ./modules/lead\n",
			[]string{fileDeclName, "use ./modules/lead"},
		},
		{
			"use block with a commented-out line",
			"go 1.26\n\nuse (\n\t./modules/lead\n\t// ./modules/nothere\n\t./modules/contact\n)\n",
			[]string{fileDeclName, "use ./modules/lead", "use ./modules/contact"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := (GoWork{}).Chunks(
				context.Background(),
				"go.work",
				[]byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)
			assertDeclNames(t, decls, testCase.wantNames)
		})
	}
}

// A use line and the go.mod header of the module it names are born together:
// a go.work listing a module before its go.mod exists, or missing one that
// does, stops every go command.
func TestGoWorkUseLineSharesAnAtomWithTheModulesHeader(t *testing.T) {
	t.Parallel()

	source := "go 1.26\n\nuse (\n\t./modules/lead\n\t.\n)\n"

	workDecls, err := (GoWork{}).Chunks(context.Background(), "go.work", []byte(source))
	if err != nil {
		t.Fatalf("GoWork.Chunks() error = %v", err)
	}

	modDecls, err := (GoMod{}).Chunks(
		context.Background(), "modules/lead/go.mod", []byte("module github.com/org/lead\n"),
	)
	if err != nil {
		t.Fatalf("GoMod.Chunks() error = %v", err)
	}

	rootDecls, err := (GoMod{}).Chunks(
		context.Background(), "go.mod", []byte("module github.com/org/root\n"),
	)
	if err != nil {
		t.Fatalf("GoMod.Chunks() error = %v", err)
	}

	testCases := []struct {
		name string
		decl entities.Decl
		want string
	}{
		{"the go.work header belongs to no module", workDecls[0], ""},
		{"a use line shares its module's atom", workDecls[1], modDecls[0].Atom},
		{"use . shares the root module's atom", workDecls[2], rootDecls[0].Atom},
	}

	for _, testCase := range testCases {
		if testCase.decl.Atom != testCase.want {
			t.Errorf("%s: Atom = %q, want %q", testCase.name, testCase.decl.Atom, testCase.want)
		}
	}

	if modDecls[0].Atom != "workspace:modules/lead" || rootDecls[0].Atom != "workspace:." {
		t.Errorf("header atoms = %q, %q", modDecls[0].Atom, rootDecls[0].Atom)
	}
}
