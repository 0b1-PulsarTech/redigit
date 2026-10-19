package routes

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"strconv"
	"strings"
	"unicode"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Reader returns the text of a file of the snapshot.
type Reader func(file string) ([]byte, error)

// Groups maps each registered route group to the declarations that name it. A
// group is the string a method called GroupName returns. An end-to-end suite and
// a changelog entry both name a route by its URL, and no reference ties either
// to the handler that serves it, so each was born hundreds of commits before the
// route it talks about.
type Groups map[string][]entities.DeclKey

// Find lists the groups of decls.
func Find(read Reader, decls []entities.Decl) Groups {
	out := Groups{}
	sources := map[string][]byte{}

	for _, decl := range decls {
		if decl.IsTest || decl.Key.Kind != entities.KindMethod ||
			!strings.HasSuffix(decl.Key.Name, "GroupName") ||
			strings.HasSuffix(decl.Key.File, "_test.go") {
			continue
		}

		if group, found := returnedString(Text(read, sources, decl)); found {
			out[group] = append(out[group], decl.Key)
		}
	}

	return out
}

// Of is the declarations of the longest group that is a whole-segment prefix of
// the path, so "/settings/appearance" finds its own group before "/settings".
func (g Groups) Of(route string) []entities.DeclKey {
	segments := strings.Split(strings.Trim(route, "/"), "/")

	for cut := len(segments); cut >= 1; cut-- {
		if keys, found := g["/"+strings.Join(segments[:cut], "/")]; found {
			return keys
		}
	}

	return nil
}

// Text is the source a declaration spans, reading each file once into sources.
func Text(read Reader, sources map[string][]byte, decl entities.Decl) string {
	source, cached := sources[decl.Key.File]
	if !cached {
		loaded, err := read(decl.Key.File)
		if err != nil {
			loaded = nil
		}

		source = loaded
		sources[decl.Key.File] = source
	}

	if decl.Span.Start < 0 || decl.Span.End > len(source) || decl.Span.Start > decl.Span.End {
		return ""
	}

	return string(source[decl.Span.Start:decl.Span.End])
}

func returnedString(text string) (string, bool) {
	file, err := parser.ParseFile(
		token.NewFileSet(), "fn.go", "package p\n"+strings.TrimSpace(text)+"\n",
		parser.SkipObjectResolution,
	)
	if err != nil {
		return "", false
	}

	value, found := "", false

	ast.Inspect(file, func(node ast.Node) bool {
		ret, isRet := node.(*ast.ReturnStmt)
		if !isRet || len(ret.Results) != 1 {
			return true
		}

		lit, isLit := ret.Results[0].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}

		if unquoted, unquoteErr := strconv.Unquote(lit.Value); unquoteErr == nil {
			value, found = unquoted, true
		}

		return false
	})

	return value, found
}

// InGo lists the URL paths among the string literals of a piece of Go.
func InGo(text string) []string {
	var (
		lex  scanner.Scanner
		out  []string
		file = token.NewFileSet().AddFile("", -1, len(text))
	)

	lex.Init(file, []byte(text), nil, 0)

	for {
		_, tok, lit := lex.Scan()
		if tok == token.EOF {
			return out
		}

		if tok != token.STRING {
			continue
		}

		if value, err := strconv.Unquote(lit); err == nil {
			if route, ok := asPath(value); ok {
				out = append(out, route)
			}
		}
	}
}

// InProse lists the URL paths a document names: the words that start with a
// slash, between the quotes and brackets prose puts around them.
func InProse(text string) []string {
	var out []string

	for _, word := range strings.FieldsFunc(text, isDelimiter) {
		if route, ok := asPath(word); ok {
			out = append(out, route)
		}
	}

	return out
}

func isDelimiter(char rune) bool {
	return unicode.IsSpace(char) || strings.ContainsRune("`'\"(),;:*[]<>", char)
}

// asPath reports a URL path with its query dropped: a leading slash and segments
// of letters, digits, braces, hyphens and underscores.
func asPath(text string) (string, bool) {
	route, _, _ := strings.Cut(text, "?")
	route = strings.TrimRight(route, ".")

	if len(route) < 2 || route[0] != '/' {
		return "", false
	}

	for _, char := range route[1:] {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) &&
			!strings.ContainsRune("/{}-_", char) {
			return "", false
		}
	}

	return route, true
}
