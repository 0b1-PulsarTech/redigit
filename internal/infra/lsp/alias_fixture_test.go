//go:build integration

package lsp

import (
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func aliasCases(dir string) []matchCase {
	at := func(line int) entities.Location {
		return entities.Location{File: filepath.Join(dir, "alias.go"), Line: line, Char: 5}
	}

	found := func(line int) []entities.Location {
		return []entities.Location{{File: "alias.go", Line: line, Char: 5}}
	}

	return []matchCase{
		{"an alias of an interface literal", false, at(2), found(8)},
		{"a generic alias of an interface literal", false, at(4), found(12)},
		{
			"an alias of a named interface, which meets itself",
			false, at(6),
			[]entities.Location{
				{File: "alias.go", Line: 16, Char: 5}, {File: "embed.go", Line: 2, Char: 5},
			},
		},
	}
}

const aliasSource = `package fixture

type Assistant = interface{ Assist() }

type Stasher[T any] = interface{ Stash(T) }

type Called = Namer

type assists struct{}

func (assists) Assist() {}

type stashes struct{}

func (stashes) Stash(int) {}

type callable struct{}

func (callable) String() string { return "" }
`
