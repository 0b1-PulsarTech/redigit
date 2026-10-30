//go:build integration

package lsp

import (
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func unifyCases(dir string) []matchCase {
	return slices.Concat(importerCases(dir), shapeCases(dir))
}

func importerCases(dir string) []matchCase {
	importers := []entities.Location{
		{File: "pkuser/user.go", Line: 2, Char: 7}, {File: "pkuser/x_test.go", Line: 2, Char: 7},
	}

	return []matchCase{
		{
			"the package clause of a test file is read by importers, not by the package's own " +
				"external test",
			true,
			entities.Location{File: filepath.Join(dir, "pk/in_test.go"), Line: 0, Char: 9},
			importers,
		},
		{
			"the package clause of a plain file is read by the same importers",
			true,
			entities.Location{File: filepath.Join(dir, "pk/pk.go"), Line: 0, Char: 9},
			importers,
		},
		{
			"a reader in a package with type errors is still a reference",
			true,
			entities.Location{File: filepath.Join(dir, "sub/mark.go"), Line: 2, Char: 5},
			[]entities.Location{{File: "bad/bad.go", Line: 4, Char: 24}},
		},
		{
			"a function read by the external test of a package its package imports",
			true,
			entities.Location{File: filepath.Join(dir, "rchelp/help.go"), Line: 4, Char: 5},
			[]entities.Location{{File: "rc/x_test.go", Line: 7, Char: 20}},
		},
		{
			"an interface met by a type of an external test that names the recompiled package",
			false,
			entities.Location{File: filepath.Join(dir, "rchelp/help.go"), Line: 6, Char: 5},
			[]entities.Location{{File: "rc/x_test.go", Line: 9, Char: 5}},
		},
		{
			"an interface literal embedding another, a type parameter in a method of it",
			false,
			entities.Location{File: filepath.Join(dir, "embed.go"), Line: 4, Char: 5},
			[]entities.Location{{File: "embed.go", Line: 6, Char: 5}},
		},
		{
			"an interface only a test variant holds is met by a package no test of it recompiles",
			false,
			entities.Location{File: filepath.Join(dir, "ta/ta_test.go"), Line: 2, Char: 5},
			[]entities.Location{
				{File: "cell.go", Line: 2, Char: 5},
				{File: "tar/r.go", Line: 4, Char: 5},
				{File: "tas/s.go", Line: 4, Char: 5},
			},
		},
		{
			"a type only a test variant holds meets an interface of a package outside it",
			false,
			entities.Location{File: filepath.Join(dir, "tbp/port.go"), Line: 4, Char: 5},
			[]entities.Location{
				{File: "cell.go", Line: 2, Char: 5}, {File: "tb/tb_test.go", Line: 2, Char: 5},
			},
		},
	}
}

func shapeCases(dir string) []matchCase {
	at := func(line int) entities.Location {
		return entities.Location{File: filepath.Join(dir, "unify.go"), Line: line, Char: 5}
	}
	found := func(lines ...int) []entities.Location {
		out := make([]entities.Location, 0, len(lines))

		for _, line := range lines {
			out = append(out, entities.Location{File: "unify.go", Line: line, Char: 5})
		}

		return out
	}

	return []matchCase{
		{"a parameter bound to a type that contains it, with a result", false, at(2), nil},
		{"a parameter bound to a type that contains it, without a result", false, at(8), nil},
		{
			"a type parameter bound differently by two methods",
			false, at(14), found(16, 22, 24),
		},
		{
			"a concrete interface against a type parameter bound per method",
			false, at(22), found(14, 16, 24),
		},
		{"a struct literal in a signature", false, at(30), found(32)},
		{"a struct literal with a tag", false, at(36), found(38)},
		{"an interface literal in a signature", false, at(46), found(48)},
		{"a constraint interface with methods", false, at(52), found(57, 61)},
	}
}

const unifySource = `package fixture

type Folder[T any] interface{ Fold([]T, T) T }

type Summer[S any] struct{}

func (Summer[S]) Fold(S, S) S { var zero S; return zero }

type Folder2[T any] interface{ Fold([]T, T) }

type Summer2[S any] struct{}

func (Summer2[S]) Fold(S, S) {}

type Multi[T any] interface{ A(T); B() T }

type split struct{}

func (split) A(int) {}

func (split) B() string { return "" }

type Two interface{ A(int); B() string }

type Gen[S any] struct{}

func (Gen[S]) A(S) {}

func (Gen[S]) B() S { var zero S; return zero }

type Shape[T any] interface{ Form() struct{ V T } }

type shaper struct{}

func (shaper) Form() struct{ V int } { return struct{ V int }{} }

type Tagged[T any] interface{ Tag() struct{ V T "k" } }

type tagger struct{}

func (tagger) Tag() struct{ V int "k" } { return struct{ V int "k" }{} }

type retagger struct{}

func (retagger) Tag() struct{ V int "x" } { return struct{ V int "x" }{} }

type Taker[T any] interface{ Take(interface{ Label() T }) }

type taker struct{}

func (taker) Take(interface{ Label() int }) {}

type Stringish interface {
	~string
	Text() string
}

type MyInt int

func (MyInt) Text() string { return "" }

type MyStr string

func (MyStr) Text() string { return "" }
`
