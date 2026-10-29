package wiring

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

// receiverOfAssertion parses "_ I = (*T)(nil)" and returns T.
//
// The text is tried as written first and only then with a "var" prepended: a
// declaration's span carries the keyword AND its doc comment, while its name
// carries neither, and prepending unconditionally breaks the commented form.
func receiverOfAssertion(text string) (string, bool) {
	body := strings.TrimSpace(text)

	name, found := parseAssertion(body)
	if found {
		return name, true
	}

	return parseAssertion("var " + body)
}

func parseAssertion(body string) (string, bool) {
	file, err := parser.ParseFile(
		token.NewFileSet(), "assert.go", "package p\n"+body+"\n", parser.SkipObjectResolution,
	)
	if err != nil {
		return "", false
	}

	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			// The declared TYPE is what makes this an assertion. Without it,
			// "_ = compute()" is a blank assignment that names a function and
			// asserts nothing.
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || value.Type == nil || len(value.Values) != 1 {
				continue
			}

			if name, found := declsource.ConcreteTypeIn(value.Values[0]); found {
				return name, true
			}
		}
	}

	return "", false
}
