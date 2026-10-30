package typed

import (
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type DeclLocator struct {
	byFile map[string][]entities.Decl
}

func NewDeclLocator(decls []entities.Decl) DeclLocator {
	byFile := declsource.GroupByFile(decls)

	for _, group := range byFile {
		slices.SortFunc(
			group,
			func(left, right entities.Decl) int { return left.Span.Start - right.Span.Start },
		)
	}

	return DeclLocator{byFile: byFile}
}

func (locator DeclLocator) enclosing(file string, offset int) (entities.DeclKey, bool) {
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

type Positions struct {
	root    string
	locator DeclLocator
	rel     *sync.Map
}

func NewPositions(root string, locator DeclLocator) Positions {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	return Positions{root: root, locator: locator, rel: &sync.Map{}}
}

func (where Positions) DeclAt(fset *token.FileSet, pos token.Pos) (entities.DeclKey, bool) {
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

func (where Positions) relative(name string) string {
	if cached, known := where.rel.Load(name); known {
		return cached.(string)
	}

	resolved := name
	if target, err := filepath.EvalSymlinks(name); err == nil {
		resolved = target
	}

	rel, err := filepath.Rel(where.root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = ""
	}

	// a miss race recomputes the same value, so a plain Store is enough
	slashed := filepath.ToSlash(rel)
	where.rel.Store(name, slashed)

	return slashed
}
