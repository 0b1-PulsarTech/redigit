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
		slices.SortFunc(group, func(a, b entities.Decl) int { return a.Span.Start - b.Span.Start })
	}

	return declLocator{byFile: byFile}
}

func (l declLocator) enclosing(file string, offset int) (entities.DeclKey, bool) {
	group := l.byFile[file]

	index, exact := slices.BinarySearchFunc(group, offset, func(d entities.Decl, at int) int {
		return d.Span.Start - at
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

func (p positions) relative(name string) string {
	if cached, known := p.rel[name]; known {
		return cached
	}

	resolved := name
	if target, err := filepath.EvalSymlinks(name); err == nil {
		resolved = target
	}

	rel, err := filepath.Rel(p.root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = ""
	}

	p.rel[name] = filepath.ToSlash(rel)

	return p.rel[name]
}

func (p positions) declAt(fset *token.FileSet, pos token.Pos) (entities.DeclKey, bool) {
	if !pos.IsValid() {
		return entities.DeclKey{}, false
	}

	at := fset.Position(pos)

	rel := p.relative(at.Filename)
	if rel == "" {
		return entities.DeclKey{}, false
	}

	return p.locator.enclosing(rel, at.Offset)
}
