package dart

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// keywords never name a declaration, so they never reach an open export.
var keywords = strings.Fields(`
	abstract as assert async await base break case catch class const continue
	covariant default deferred do dynamic else enum export extends extension
	external factory false final finally for get hide if implements import in
	interface is late library mixin new null of on operator part required rethrow
	return sealed set show static super switch sync this throw true try type typedef
	var void when while with yield
`)

// openThrough is what an import or export passes on of a namespace's open
// exports: everything, unless a show list names what it passes.
func openThrough(passed namespace, rule directive) map[entities.DeclKey]provider {
	if len(rule.show) > 0 {
		return map[entities.DeclKey]provider{}
	}

	return passed.open
}

// unlisted resolves a name nothing in the tree declares to every import, under
// prefix, whose exports leave the tree - a generated file the repository
// ignores, an outside package. A local variable lands here too, so this errs
// toward keeping an import, never toward dropping one.
func (s scope) unlisted(name, prefix string) []reference {
	if slices.Contains(keywords, name) {
		return nil
	}

	var out []reference

	for index, each := range s.imports {
		if each.directive.prefix == prefix {
			out = append(out, openReferences(each, index)...)
		}
	}

	return out
}

func openReferences(each importScope, index int) []reference {
	exports := slices.SortedFunc(maps.Values(each.offer.open), func(a, b provider) int {
		return cmp.Compare(a.key.File+"\x00"+a.key.Name, b.key.File+"\x00"+b.key.Name)
	})

	out := make([]reference, 0, len(exports))

	for _, found := range exports {
		out = append(out, reference{provider: found, through: index})
	}

	return out
}
