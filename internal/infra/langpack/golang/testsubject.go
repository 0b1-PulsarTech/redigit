package golang

import (
	"go/ast"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// testSubject prefers a call the test body actually makes - a fact - over a
// guess from its name, and tries the name only when the body names none or
// several.
func testSubject(
	d entities.Decl,
	scope []entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) (entities.DeclKey, bool) {
	if to, ok := calledSubject(d, scope, snap, sources); ok {
		return to, true
	}

	prefix, ok := testSubjectPrefix(d.Key.Name)
	if !ok {
		return entities.DeclKey{}, false
	}

	return bestSubject(scope, camelOrSnakeTokens(prefix))
}

// calledSubject reports the one package-level function the test body calls
// directly by name, ignoring anything qualified (an import, a local var's
// method) or ambiguous (more than one candidate, or none).
func calledSubject(
	d entities.Decl,
	scope []entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) (entities.DeclKey, bool) {
	source, cached := sources[d.Key.File]
	if !cached {
		read, err := ReadSource(snap, d.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[d.Key.File] = source
	}

	fn := ParseFunc(SliceSpan(source, d.Span))
	if fn == nil || fn.Body == nil {
		return entities.DeclKey{}, false
	}

	called := map[entities.DeclKey]bool{}

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		ident, isIdent := call.Fun.(*ast.Ident)
		if !isIdent {
			return true
		}

		if key, ok := findFuncNamed(scope, ident.Name); ok {
			called[key] = true
		}

		return true
	})

	if len(called) != 1 {
		return entities.DeclKey{}, false
	}

	for key := range called {
		return key, true
	}

	return entities.DeclKey{}, false
}

func findFuncNamed(scope []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, d := range scope {
		if d.Key.Kind == entities.KindFunc && !d.IsTest &&
			!strings.HasSuffix(d.Key.File, "_test.go") && d.Key.Name == name {
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}

// bestSubject walks the token boundaries left to right. At each cut, the
// growing left side is checked against a type name and the shrinking right
// side against a bare method name; either match overwrites the previous
// best, so the last (most specific) match along the walk wins.
func bestSubject(scope []entities.Decl, tokens []string) (entities.DeclKey, bool) {
	var best entities.DeclKey

	found := false

	for cut := 1; cut < len(tokens); cut++ {
		left := strings.Join(tokens[:cut], "")
		right := strings.Join(tokens[cut:], "")

		if key, ok := FindTypeNamed(scope, left); ok {
			best, found = key, true
		}

		if key, ok := findMethodNamed(scope, right); ok {
			best, found = key, true
		}
	}

	if found {
		return best, true
	}

	return longestBareMatch(scope, tokens)
}

// longestBareMatch tries the whole token run as a bare declaration name, then
// each shorter leading run, so a trailing descriptive word ("...Drains",
// "...ReturnsError") drops off before giving up rather than after.
func longestBareMatch(scope []entities.Decl, tokens []string) (entities.DeclKey, bool) {
	for length := len(tokens); length >= 1; length-- {
		if key, ok := findDeclNamed(scope, strings.Join(tokens[:length], "")); ok {
			return key, true
		}
	}

	return entities.DeclKey{}, false
}

// findDeclNamed looks up a type, function, or value by name alone - the
// bare-match fallback bestSubject's walk covers a method through, but a
// plain function or package-level value never had a receiver to split on.
func findDeclNamed(scope []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, d := range scope {
		if d.IsTest || d.Key.Name != name {
			continue
		}

		switch d.Key.Kind {
		case entities.KindType, entities.KindFunc, entities.KindValue:
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}

// findMethodNamed looks up a method by its bare method part (after the
// receiver's "."), the shape a test name's suffix names.
func findMethodNamed(decls []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, d := range decls {
		if d.Key.Kind != entities.KindMethod {
			continue
		}

		if _, method, ok := strings.Cut(d.Key.Name, "."); ok && method == name {
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}
