package gofacts

import (
	"go/token"
	"go/types"
	"slices"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/resolve"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type typeEntry struct {
	key   entities.DeclKey
	named *types.Named
	iface *types.Interface
	fset  *token.FileSet
	alias bool
}

type implementors struct {
	pkgs  []*packages.Package
	where typed.Positions

	guard   sync.RWMutex
	entries []typeEntry
	loaded  bool
	memo    map[entities.DeclKey][]entities.DeclKey
}

func newImplementors(pkgs []*packages.Package, where typed.Positions) *implementors {
	return &implementors{pkgs: pkgs, where: where, memo: map[entities.DeclKey][]entities.DeclKey{}}
}

func (impl *implementors) of(iface entities.DeclKey) []entities.DeclKey {
	impl.guard.RLock()
	answered, known := impl.memo[iface]
	impl.guard.RUnlock()

	if known {
		return answered
	}

	impl.guard.Lock()
	defer impl.guard.Unlock()

	if answered, known = impl.memo[iface]; known {
		return answered
	}

	if !impl.loaded {
		impl.entries, impl.loaded = impl.collect(), true
	}

	impl.memo[iface] = impl.compute(iface)

	return impl.memo[iface]
}

func (impl *implementors) compute(iface entities.DeclKey) []entities.DeclKey {
	found := map[entities.DeclKey]bool{}

	for _, target := range impl.entries {
		// An empty interface is satisfied by everything, which names no relationship.
		if target.key == iface && target.iface != nil && target.iface.NumMethods() > 0 {
			impl.satisfying(target, found)
		}
	}

	out := make([]entities.DeclKey, 0, len(found))
	for key := range found {
		out = append(out, key)
	}

	slices.SortFunc(out, entities.CompareKeys)

	return out
}

func (impl *implementors) satisfying(target typeEntry, found map[entities.DeclKey]bool) {
	for _, candidate := range impl.entries {
		if candidate.alias || candidate.key == target.key || found[candidate.key] {
			continue
		}

		// Every pair goes through resolve's unifier, so a generic or recompiled type matches
		// as it does for the resolver the graph is built from.
		if resolve.Satisfies(candidate.named, candidate.fset, target.iface) {
			found[candidate.key] = true
		}
	}
}

func (impl *implementors) collect() []typeEntry {
	var entries []typeEntry

	// Only pass-0 packages are walked, by design: a type seen solely under hidden run tags is not
	// an implementor here.
	for _, pkg := range impl.pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		for ident, obj := range pkg.TypesInfo.Defs {
			name, isName := obj.(*types.TypeName)
			if !isName || name.Pkg() == nil {
				continue
			}

			entry, usable := entryOf(name)
			// A function-local type is no declaration; DeclAt would key its enclosing func.
			if !usable || name.Parent() != name.Pkg().Scope() {
				continue
			}

			if key, inTree := impl.where.DeclAt(pkg.Fset, ident.Pos()); inTree {
				entry.key, entry.fset = key, pkg.Fset
				entries = append(entries, entry)
			}
		}
	}

	return entries
}

func entryOf(name *types.TypeName) (typeEntry, bool) {
	if name.IsAlias() {
		iface, isInterface := types.Unalias(name.Type()).Underlying().(*types.Interface)

		return typeEntry{iface: iface, alias: true}, isInterface
	}

	named, isNamed := name.Type().(*types.Named)
	if !isNamed {
		return typeEntry{}, false
	}

	iface, _ := named.Underlying().(*types.Interface)

	return typeEntry{named: named, iface: iface}, true
}
