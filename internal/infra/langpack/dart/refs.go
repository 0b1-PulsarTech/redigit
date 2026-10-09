package dart

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// reference is one name a declaration reads. through is the import it came
// in by, or ownLibrary; key is zero when the import is known to be used but
// its library lies outside the tree.
type reference struct {
	provider

	through int
}

// prefixTail is the tokens a prefixed name spans past the prefix: "." and the name.
const prefixTail = 2

var memberAccess = []string{".", "?.", "..", "?.."}

// references scans a declaration's tokens for the names it reads. It is a
// scope-aware text scan, not a type check: an identifier is matched against
// what the library declares and imports, so a local variable that shadows an
// imported name still counts as reading it. That errs toward an extra edge,
// never toward a missing one.
func (s scope) references(toks []token, self entities.DeclKey) []reference {
	var out []reference

	for index := 0; index < len(toks); index++ {
		tok := toks[index]

		if tok.kind == tokString {
			out = append(out, s.references(tok.inner, self)...)

			continue
		}

		if tok.kind != tokIdent {
			continue
		}

		found, skip := s.resolveAt(toks, index)
		index += skip

		for _, each := range found {
			if each.key != self {
				out = append(out, each)
			}
		}
	}

	return out
}

// resolveAt resolves the identifier at index, reporting how many tokens after
// it the resolution consumed.
func (s scope) resolveAt(toks []token, index int) ([]reference, int) {
	name := toks[index].text
	prev, next := textAt(toks, index-1), textAt(toks, index+1)

	switch {
	case slices.Contains(memberAccess, prev):
		return s.member(name), 0
	case next == "." && index+prefixTail < len(toks) && toks[index+prefixTail].kind == tokIdent && s.hasPrefix(name):
		return s.prefixed(name, toks[index+prefixTail].text), prefixTail
	case next == ":" && (prev == "(" || prev == ","):
		return nil, 0
	default:
		return s.lookup(name), 0
	}
}

func textAt(toks []token, index int) string {
	if index < 0 || index >= len(toks) {
		return ""
	}

	return toks[index].text
}
