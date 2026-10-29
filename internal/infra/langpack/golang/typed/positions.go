package typed

import (
	"go/token"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type declLocator struct {
	byFile map[string][]entities.Decl
}

func newDeclLocator(decls []entities.Decl) declLocator {
	byFile := declsource.GroupByFile(decls)

	for _, group := range byFile {
		slices.SortFunc(group, func(left, right entities.Decl) int { return left.Span.Start - right.Span.Start })
	}

	return declLocator{byFile: byFile}
}

func (locator declLocator) enclosing(file string, offset int) (entities.DeclKey, bool) {
	group := locator.byFile[file]

	index, exact := slices.BinarySearchFunc(group, offset, func(decl entities.Decl, at int) int {
		return decl.Span.Start - at
	})
	if !exact {
		index--
	}

	if index < 0 || index >= len(group) || offset >= group[index].Span.End {
		return entities.DeclKey{}, false
	}

	return group[index].Key, true
}

type positions struct {
	root    string
	locator declLocator
	rel     map[string]string
}

func newPositions(root string, locator declLocator) positions {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	return positions{root: root, locator: locator, rel: map[string]string{}}
}

func (where positions) relative(name string) string {
	if cached, known := where.rel[name]; known {
		return cached
	}

	resolved := name
	if target, err := filepath.EvalSymlinks(name); err == nil {
		resolved = target
	}

	rel, err := filepath.Rel(where.root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = ""
	}

	where.rel[name] = filepath.ToSlash(rel)

	return where.rel[name]
}

func (where positions) declAt(fset *token.FileSet, pos token.Pos) (entities.DeclKey, bool) {
	if !pos.IsValid() {
		return entities.DeclKey{}, false
	}

	at := fset.Position(pos)

	rel := where.relative(at.Filename)
	if rel == "" {
		return entities.DeclKey{}, false
	}

	return where.locator.enclosing(rel, at.Offset)
}
