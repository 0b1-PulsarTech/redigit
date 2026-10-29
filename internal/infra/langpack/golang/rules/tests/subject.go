package tests

import (
	"go/ast"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

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

func calledSubject(
	d entities.Decl,
	scope []entities.Decl,
	snap entities.Snapshot,
	sources map[string][]byte,
) (entities.DeclKey, bool) {
	source, cached := sources[d.Key.File]
	if !cached {
		read, err := declsource.ReadSource(snap, d.Key.File)
		if err != nil {
			read = nil
		}

		source = read
		sources[d.Key.File] = source
	}

	fn := declsource.ParseFunc(declsource.SliceSpan(source, d.Span))
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

func bestSubject(scope []entities.Decl, tokens []string) (entities.DeclKey, bool) {
	var best entities.DeclKey

	found := false

	for cut := 1; cut < len(tokens); cut++ {
		left := strings.Join(tokens[:cut], "")
		right := strings.Join(tokens[cut:], "")

		if key, ok := declsource.FindTypeNamed(scope, left); ok {
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

func longestBareMatch(scope []entities.Decl, tokens []string) (entities.DeclKey, bool) {
	for length := len(tokens); length >= 1; length-- {
		if key, ok := findDeclNamed(scope, strings.Join(tokens[:length], "")); ok {
			return key, true
		}
	}

	return entities.DeclKey{}, false
}

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
