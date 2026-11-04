package typed

import (
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"

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

func (locator DeclLocator) has(key entities.DeclKey) bool {
	group := locator.byFile[key.File]
	same := func(decl entities.Decl) bool { return decl.Key == key }
	first := slices.IndexFunc(group, same)

	return first >= 0 && !slices.ContainsFunc(group[first+1:], same)
}

type Positions struct {
	root    string
	locator DeclLocator
	rel     *sync.Map
	parsed  map[*token.File]bool
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

func (where Positions) Parsing(pkgs []*packages.Package) Positions {
	where.parsed = map[*token.File]bool{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			where.parsed[pkg.Fset.File(file.FileStart)] = true
		}
	}

	return where
}

func (where Positions) ObjectAt(fset *token.FileSet, obj types.Object) (entities.DeclKey, bool) {
	// Export data keeps lines, not offsets, so only a parsed file maps by position.
	if where.parsed[fset.File(obj.Pos())] {
		return where.DeclAt(fset, obj.Pos())
	}

	key, named := declKeyOf(obj)
	key.File = where.relative(fset.Position(obj.Pos()).Filename)

	if named && where.locator.has(key) {
		return key, true
	}

	return entities.DeclKey{}, false
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

func declKeyOf(obj types.Object) (entities.DeclKey, bool) {
	if fn, isFunc := obj.(*types.Func); isFunc {
		return funcKeyOf(fn)
	}

	kind := entities.KindValue
	if _, isType := obj.(*types.TypeName); isType {
		kind = entities.KindType
	}

	return entities.DeclKey{Kind: kind, Name: obj.Name()}, isPackageLevel(obj)
}

func isPackageLevel(obj types.Object) bool {
	return obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope()
}

func funcKeyOf(fn *types.Func) (entities.DeclKey, bool) {
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return entities.DeclKey{Kind: entities.KindFunc, Name: fn.Name()}, isPackageLevel(fn)
	}

	holder := recv.Type()
	if pointer, isPointer := holder.(*types.Pointer); isPointer {
		holder = pointer.Elem()
	}

	named, isNamed := holder.(interface{ Obj() *types.TypeName })
	if !isNamed {
		return entities.DeclKey{}, false
	}

	owner := named.Obj().Name()

	// An abstract method has no body; DeclAt answers the interface it sits in.
	if types.IsInterface(holder) {
		return entities.DeclKey{Kind: entities.KindType, Name: owner}, true
	}

	return entities.DeclKey{Kind: entities.KindMethod, Name: owner + "." + fn.Name()}, true
}
