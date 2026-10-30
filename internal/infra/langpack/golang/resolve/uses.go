package resolve

import (
	"cmp"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type usage struct {
	refs map[site][]entities.Location
	defs map[site]entities.Location

	unread map[string]bool
	lost   map[row]bool
}

type collector struct {
	usage

	where  *places
	parsed map[*token.File]bool
	cgo    map[string]bool
	deps   map[string][]*packages.Package
	whole  map[string]bool
}

func collectUses(pkgs []*packages.Package, where *places) usage {
	gather := collector{
		usage: usage{
			refs: map[site][]entities.Location{},
			defs: map[site]entities.Location{},

			unread: map[string]bool{},
			lost:   map[row]bool{},
		},
		where:  where,
		parsed: map[*token.File]bool{},
		cgo:    map[string]bool{},
		deps:   depsOf(pkgs),
		whole:  map[string]bool{},
	}

	for _, pkg := range pkgs {
		gather.cgo[pkg.PkgPath] = gather.cgo[pkg.PkgPath] || rewritten(pkg)

		for _, file := range pkg.Syntax {
			gather.parsed[pkg.Fset.File(file.Pos())] = true
		}
	}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil || rewritten(pkg) {
			continue
		}

		for ident, obj := range pkg.TypesInfo.Uses {
			gather.add(pkg.Fset, ident.Pos(), originOf(obj))
		}
	}

	gather.addImports(pkgs, where)

	for _, group := range gather.deps {
		for _, dep := range group {
			gather.leaveDirected(dep, where)
		}
	}

	for _, readers := range gather.refs {
		slices.SortFunc(readers, compareLocations)
	}

	return gather.usage
}

func (gather collector) add(fset *token.FileSet, usePos token.Pos, obj types.Object) {
	// A test variant re-checks the same file, so the same use appears once per variant; the file
	// position, not the object, is what names it.
	useAt, useLoc, written := gather.where.at(fset, usePos)
	if !written {
		return
	}

	if _, dup := gather.defs[useAt]; dup {
		return
	}

	defAt, defLoc, defined := gather.where.at(fset, obj.Pos())
	if !defined {
		// This group read the package as export data, whose position names no tree line.
		if pkg := obj.Pkg(); pkg != nil && !gather.whole[pkg.Path()] {
			gather.whole[pkg.Path()] = true

			for _, dep := range gather.deps[pkg.Path()] {
				gather.leaveWhole(dep, gather.where)
			}
		}

		if far, known := gather.outside(fset, obj); known {
			gather.defs[useAt] = far
		}

		return
	}

	// Export data keeps lines, not offsets, so a position of a file nobody parsed is only a guess.
	if !gather.parsed[fset.File(obj.Pos())] {
		// The load pattern skips testdata and underscore directories, so this use is lost here
		// and the group that does parse the file must not answer for its readers. The line is all
		// that data holds, so the declarations sharing it are the ones refused.
		line := fset.PositionFor(obj.Pos(), false).Line
		gather.lost[row{file: defAt.file, line: line}] = true

		return
	}

	gather.defs[useAt] = defLoc
	gather.refs[defAt] = append(gather.refs[defAt], useLoc)
}

func (gather collector) outside(fset *token.FileSet, obj types.Object) (entities.Location, bool) {
	// cgo writes its types to the cache and points //line directives back into the tree, so a
	// position there is not an answer.
	if obj.Pkg() != nil && gather.cgo[obj.Pkg().Path()] {
		return entities.Location{}, false
	}

	pos := obj.Pos()
	if pos.IsValid() && gather.where.relative(fset.PositionFor(pos, true).Filename) != "" {
		return entities.Location{}, false
	}

	return outsideTree(fset, pos), true
}

func outsideTree(fset *token.FileSet, pos token.Pos) entities.Location {
	if !pos.IsValid() {
		return entities.Location{File: "builtin"}
	}

	// A definition in the standard library, a dependency or the universe is an answer, not a miss:
	// it names a file no declaration selector can match, as the language server does. Export data
	// gives a path relative to the root, so it is made absolute.
	name := fset.PositionFor(pos, false).Filename
	if !filepath.IsAbs(name) {
		name = string(filepath.Separator) + name
	}

	// Unlike gopls this is not the file on disk, only a name no tree file shares. The graph is
	// unaffected: directCallsOnly just compares it with the declaration's file, a miss either way.

	return entities.Location{File: name}
}

func originOf(obj types.Object) types.Object {
	switch found := obj.(type) {
	case *types.Func:
		return found.Origin()
	case *types.Var:
		return found.Origin()
	}

	return obj
}

func compareLocations(left, right entities.Location) int {
	return cmp.Or(
		cmp.Compare(left.File, right.File),
		cmp.Compare(left.Line, right.Line),
		cmp.Compare(left.Char, right.Char),
	)
}
