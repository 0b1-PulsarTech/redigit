package syntax

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSyntaxDeclarationsSelectorIsTheName(t *testing.T) {
	t.Parallel()

	const src = "package sample\n\nfunc Greet() {}\n"

	decls, err := Declarations(context.Background(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	want := entities.Location{File: "sample.go", Line: 2, Char: len("func ")}
	if decls[0].Selector != want {
		t.Fatalf("Selector = %+v, want %+v", decls[0].Selector, want)
	}
}

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

			got := selectorOf(t, testCase.source, testCase.declName)
			if got.Line != testCase.wantLine || got.Char != testCase.wantChar {
				t.Fatalf(
					"selector = %d:%d, want %d:%d",
					got.Line, got.Char, testCase.wantLine, testCase.wantChar,
				)
			}
		})
	}
}

func TestSelectorIgnoresLineDirectives(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		source   string
		declName string
		wantLine int
		wantChar int
	}{
		{
			"a directive with no column",
			"package a\n\n//line other.go:100\nfunc Add() {}\n",
			"Add",
			3,
			5,
		},
		{
			"a directive with a column",
			"package a\n\nfunc Z() {}\n\n/*line other.go:7:30*/ func Add() {}\n",
			"Add",
			4,
			28,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := selectorOf(t, testCase.source, testCase.declName)
			if got.Line != testCase.wantLine || got.Char != testCase.wantChar {
				t.Fatalf(
					"selector = %d:%d, want %d:%d",
					got.Line, got.Char, testCase.wantLine, testCase.wantChar,
				)
			}
		})
	}
}

func selectorOf(t *testing.T, source, declName string) entities.Location {
	t.Helper()

	decls, err := Declarations(context.Background(), "a.go", []byte(source))
	if err != nil {
		t.Fatalf("Declarations() = %v", err)
	}

	for _, decl := range decls {
		if decl.Key.Name == declName {
			return decl.Selector
		}
	}

	t.Fatalf("no declaration named %q among %d", declName, len(decls))

	return entities.Location{}
}
