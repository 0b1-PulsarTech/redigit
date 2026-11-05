package config

import (
	"go/ast"
	"go/token"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// importUse is where a file reaches one import: the selectors written through
// its local name. A render keeps an import only for the declarations that use
// it, so a declaration waits for the require of the imports it reaches and no
// others. A name that cannot be told, or that no selector uses, leaves every
// declaration of the file depending on the import, the safe reading.
type importUse struct {
	sites     []int
	wholeFile bool
}

// reaches is whether a declaration holds a selector through the import.
func (use importUse) reaches(span declSpan) bool {
	if use.wholeFile {
		return true
	}

	for _, offset := range use.sites {
		if offset >= span.start && offset < span.end {
			return true
		}
	}

	return false
}

type declSpan struct{ start, end int }

// importUses maps each import path of a parsed file to where the file reaches
// it.
func importUses(parsed *ast.File, fset *token.FileSet) map[string]importUse {
	selected := map[string][]int{}

	ast.Inspect(parsed, func(node ast.Node) bool {
		sel, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}

		if base, isIdent := sel.X.(*ast.Ident); isIdent {
			selected[base.Name] = append(selected[base.Name], fset.Position(sel.Pos()).Offset)
		}

		return true
	})

	out := map[string]importUse{}

	for _, spec := range parsed.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		name := localName(spec)
		if name == "" {
			name = guessedName(imported)
		}

		sites := selected[name]
		blank := name == "_" || name == "."

		out[imported] = importUse{sites: sites, wholeFile: blank || name == "" || len(sites) == 0}
	}

	return out
}

// guessedName is the name a package is most likely imported by: the last
// element of its path, or the one before it when that is a major version. A
// path whose last element is not an identifier says nothing, and the empty
// name sends the import to the whole file.
func guessedName(imported string) string {
	name := path.Base(imported)

	if isMajorVersion(name) {
		name = path.Base(path.Dir(imported))
	}

	for index, char := range name {
		if char != '_' && !unicode.IsLetter(char) && (index == 0 || !unicode.IsDigit(char)) {
			return ""
		}
	}

	return name
}

func isMajorVersion(element string) bool {
	if len(element) < 2 || element[0] != 'v' {
		return false
	}

	for _, char := range element[1:] {
		if !unicode.IsDigit(char) {
			return false
		}
	}

	return true
}

// importsOf lists the import paths of a parsed file.
func importsOf(parsed *ast.File) []string {
	out := make([]string, 0, len(parsed.Imports))

	for _, spec := range parsed.Imports {
		if imported, err := strconv.Unquote(spec.Path.Value); err == nil {
			out = append(out, imported)
		}
	}

	return out
}

// providedRequires lists the require pieces of one go.mod that provide a
// package among imports, each once.
func providedRequires(imports []string, required map[string]entities.DeclKey) []entities.DeclKey {
	seen := map[string]bool{}

	var out []entities.DeclKey

	for _, imported := range imports {
		module, found := owningModule(imported, required)
		if !found || seen[module] {
			continue
		}

		seen[module] = true

		out = append(out, required[module])
	}

	return out
}

// owningModule is the longest required module path that is imported, or
// prefixes it at a path boundary. Longest, not first: "example.com/a" and
// "example.com/a/b" can both be required, and the import belongs to the
// deeper one.
func owningModule(imported string, required map[string]entities.DeclKey) (string, bool) {
	best := ""

	for module := range required {
		if imported != module && !strings.HasPrefix(imported, module+"/") {
			continue
		}

		if len(module) > len(best) {
			best = module
		}
	}

	return best, best != ""
}
