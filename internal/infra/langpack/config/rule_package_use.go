package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// packageUse is one in-repo package a file imports: the directory it lives in
// and each selector the file reaches it by.
type packageUse struct {
	dir       string
	sites     []useSite
	wholeFile bool
}

// useSite is one "pkg.Name" selector: where it sits and the name it reaches.
type useSite struct {
	offset int
	name   string
}

// uses lists the in-repo packages a parsed file imports, with the selectors
// reaching each, found the way a render finds them: syntactically.
func (index packageIndex) uses(parsed *ast.File, fset *token.FileSet) []*packageUse {
	byLocal := map[string]*packageUse{}

	var out []*packageUse

	for _, spec := range parsed.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		dir, inside := index.directoryOf(imported)
		if _, found := index.byDir[dir]; !inside || !found {
			continue
		}

		use := &packageUse{dir: dir}
		out = append(out, use)

		switch name := localName(spec); name {
		case "_", ".":
			use.wholeFile = true
		case "":
			byLocal[index.packageName(index.byDir[dir].Key.File)] = use
		default:
			byLocal[name] = use
		}
	}

	ast.Inspect(parsed, func(node ast.Node) bool {
		sel, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}

		if ident, isIdent := sel.X.(*ast.Ident); isIdent && byLocal[ident.Name] != nil {
			byLocal[ident.Name].sites = append(byLocal[ident.Name].sites, useSite{
				offset: fset.Position(sel.Pos()).Offset,
				name:   sel.Sel.Name,
			})
		}

		return true
	})

	return out
}

// edges makes each declaration of the file wait for what it takes from the
// package. A blank or dot import is kept by a render in every file it writes, so
// every declaration waits for the package to exist. Any other import is kept
// for the declarations that reach it by selector, and each waits for the
// declarations it names: never for an arbitrary one, because a consumer-declared
// port that mentions the package would then wait on an implementation of itself
// that the package declares first. Only a selector naming nothing the index
// holds falls back to the package's earliest declaration, which at least makes
// it exist.
func (index packageIndex) edges(use *packageUse, fileDecls []entities.Decl) []entities.Edge {
	var out []entities.Edge

	exists := index.byDir[use.dir].Key

	for _, decl := range fileDecls {
		providers := index.providersFor(use, decl)
		if use.wholeFile {
			providers = []entities.DeclKey{exists}
		}

		for _, provider := range providers {
			out = append(out, entities.Edge{
				Dependent: decl.Key,
				Provider:  provider,
				Reason:    entities.ReasonImport,
			})
		}
	}

	return out
}

func (index packageIndex) providersFor(use *packageUse, decl entities.Decl) []entities.DeclKey {
	var named []entities.DeclKey

	reached := false

	for _, site := range use.sites {
		if site.offset < decl.Span.Start || site.offset >= decl.Span.End {
			continue
		}

		reached = true

		named = append(named, index.byName[use.dir+"\x00"+site.name]...)
	}

	if reached && len(named) == 0 {
		return []entities.DeclKey{index.byDir[use.dir].Key}
	}

	return named
}

func localName(spec *ast.ImportSpec) string {
	if spec.Name == nil {
		return ""
	}

	return spec.Name.Name
}

// packageName reads the name a package declares, which is what code in the
// importing file calls it by. A file that cannot be read has no name, and an
// empty one matches no selector.
func (index packageIndex) packageName(file string) string {
	source, err := readSource(index.snap, file)
	if err != nil {
		return ""
	}

	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.PackageClauseOnly)
	if err != nil {
		return ""
	}

	return parsed.Name.Name
}
