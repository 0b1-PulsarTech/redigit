package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// TestSelectorCountsUTF16Units guards the seam between this package and the
// language server. token.Position.Column counts BYTES; entities.Location.Char
// counts UTF-16 code units. They agree on an ASCII line and diverge everywhere
// else, and a server queried at the wrong column answers about a different
// symbol without reporting anything amiss.
func TestSelectorCountsUTF16Units(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		source   string
		declName string
		wantLine int
		wantChar int
	}{
		{
			"an ascii line is the case where bytes and units agree",
			"package a\n\nfunc Add() {}\n",
			"Add",
			2,
			5,
		},
		{
			"a two-byte receiver name shifts the byte column but not the unit column",
			"package a\n\ntype Ação struct{}\n\nfunc (a Ação) Run() {}\n",
			"Ação.Run",
			4,
			14,
		},
		{
			"an emoji in a comment does not move a later line",
			"package a\n\n// \U0001F600 note\nfunc Add() {}\n",
			"Add",
			3,
			5,
		},
		{
			"a value declared after a two-byte name on the same line",
			"package a\n\nvar café, x = 1, 2\n",
			"x",
			2,
			10,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := NewSyntax().Declarations(
				context.Background(),
				"a.go",
				[]byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			var found entities.Decl

			for _, decl := range decls {
				if decl.Key.Name == testCase.declName {
					found = decl
				}
			}

			if found.Key.Name == "" {
				t.Fatalf("no declaration named %q among %d", testCase.declName, len(decls))
			}

			if found.Selector.Line != testCase.wantLine {
				t.Fatalf("selector line = %d, want %d", found.Selector.Line, testCase.wantLine)
			}

			if found.Selector.Char != testCase.wantChar {
				t.Fatalf("selector char = %d, want %d", found.Selector.Char, testCase.wantChar)
			}
		})
	}
}

// TestDeclarationlessFileIsCarried pins the case real source found and no
// fixture had: a file holding a package clause and imports and nothing else.
// With no declaration to stand for it, the file belongs to no commit, is never
// written, and vanishes from the rebuilt tree without the tree invariant ever
// naming it.
func TestDeclarationlessFileIsCarried(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{
			"a file that only pins blank imports",
			"package infra\n\nimport (\n\t_ \"example.com/a\"\n\t_ \"example.com/b\"\n)\n",
		},
		{
			"a package doc file with no imports at all",
			"// Package thing does a thing.\npackage thing\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := NewSyntax().Declarations(context.Background(), "deps.go", source)
			if err != nil {
				t.Fatalf("Declarations() = %v", err)
			}

			if len(decls) != 1 {
				t.Fatalf("got %d declarations, want the one standing for the file", len(decls))
			}

			if decls[0].Key.Kind != entities.KindOther {
				t.Fatalf("kind = %q, want %q", decls[0].Key.Kind, entities.KindOther)
			}

			rendered, err := NewSyntax().Render(
				context.Background(),
				source,
				[]entities.DeclKey{decls[0].Key},
			)
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}

			if string(rendered) != testCase.source {
				t.Fatalf("Render() = %q, want the file verbatim", rendered)
			}
		})
	}
}

// TestDeclarationlessFileRendersEmptyBeforeItIsBorn pins the other half: until
// its declaration is born, the file is not written at all, rather than being
// born holding only a package clause.
func TestDeclarationlessFileRendersEmptyBeforeItIsBorn(t *testing.T) {
	t.Parallel()

	source := []byte("package infra\n\nimport _ \"example.com/a\"\n")

	rendered, err := NewSyntax().Render(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}

	if len(rendered) != 0 {
		t.Fatalf("Render() = %q, want nothing", rendered)
	}
}
