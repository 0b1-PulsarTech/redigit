package resolve

import (
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type covering struct {
	textFile

	incomplete bool
}

func indexHidden(passes [][]*packages.Package, where *places, built []index) []index {
	owners, out := slices.Clone(built), []index{}

	// A hidden pass re-checks whole packages, so it answers only for the files no earlier group
	// parsed. Its complete files lend what they read of the owners' declarations in the packages
	// it re-checked; an incomplete file lends nothing, and no loss is settled onto the owners.
	for _, pass := range passes {
		for _, group := range indexGroups([][]*packages.Package{pass}, where) {
			for rel := range group.covered {
				if slices.ContainsFunc(owners, func(owner index) bool {
					_, parsed := owner.covered[rel]

					return parsed
				}) {
					delete(group.covered, rel)
				}
			}

			group.crossed = map[site]bool{}

			for at := range group.implementors {
				_, group.crossed[at] = group.covered[at.file]
			}

			for _, owner := range owners {
				lendTo(owner.refs, group.refs, owner.covered, group.covered)
				lendTo(owner.implementors, group.implementors, owner.covered, group.covered)
			}

			owners, out = append(owners, group), append(out, group)
		}
	}

	return out
}

func lendTo(to, from map[site][]entities.Location, owns, hidden map[string]covering) {
	for def, readers := range from {
		if _, owned := owns[def.file]; !owned {
			continue
		}

		for _, reader := range readers {
			file, inHidden := hidden[reader.File]
			if inHidden && !file.incomplete && !slices.Contains(to[def], reader) {
				to[def] = append(to[def], reader)
			}
		}

		slices.SortFunc(to[def], compareLocations)
	}
}

func rewritten(pkg *packages.Package) bool {
	// cgo compiles generated files, so the positions its types carry are not the listed files'.
	return !slices.Equal(pkg.GoFiles, pkg.CompiledGoFiles)
}

func cgoReach(pkgs []*packages.Package) map[string]bool {
	// A cgo package is not read, so a declaration it can reach may have readers nobody sees.
	reached := map[string]bool{}

	for _, pkg := range pkgs {
		if rewritten(pkg) {
			packages.Visit([]*packages.Package{pkg}, func(imported *packages.Package) bool {
				reached[imported.ID] = true

				return true
			}, nil)
		}
	}

	return reached
}

func variantsOf(pkgs []*packages.Package) (parsed []*packages.Package, bare map[string]bool) {
	// A package recompiled for another package's test is no root, yet that test reads it. One with
	// no type-check cannot be compared, so the package it recompiles is left unsettled.
	roots, paths := map[string]bool{}, map[string]bool{}
	for _, pkg := range pkgs {
		roots[pkg.ID], paths[pkg.PkgPath] = true, true
	}

	bare = map[string]bool{}

	packages.Visit(pkgs, func(dep *packages.Package) bool {
		switch {
		case roots[dep.ID] || dep.ID == dep.PkgPath || !paths[dep.PkgPath]:
		case dep.TypesInfo != nil:
			parsed = append(parsed, dep)
		default:
			bare[dep.PkgPath] = true
		}

		return true
	}, nil)

	return parsed, bare
}

func unseen(pkgs []*packages.Package) bool {
	_, bare := variantsOf(pkgs)

	return len(bare) > 0 || slices.ContainsFunc(pkgs, rewritten)
}

func settleUnread(groups []index) {
	// A group that met a file only as export data lost that file's readers, so every group that
	// parses it answers for the declarations without them.
	for idx := range groups {
		groups[idx].partial = map[row]bool{}
	}

	for _, from := range groups {
		for rel := range from.unread {
			for _, group := range groups {
				if file, parses := group.covered[rel]; parses {
					file.incomplete = true
					group.covered[rel] = file
				}
			}
		}

		for line := range from.lost {
			for _, group := range groups {
				if _, parses := group.covered[line.file]; parses {
					group.partial[line] = true
				}
			}
		}
	}
}

func (where *places) covered(pkgs []*packages.Package) map[string]covering {
	seen, skipped, incomplete := map[string]bool{}, map[string]bool{}, map[string]bool{}
	reached := cgoReach(pkgs)
	_, bare := variantsOf(pkgs)

	for _, pkg := range pkgs {
		for _, name := range pkg.GoFiles {
			rel := where.relative(name)
			if rel == "" {
				continue
			}

			seen[rel] = true
			skipped[rel] = skipped[rel] || rewritten(pkg)
			incomplete[rel] = incomplete[rel] || len(pkg.Errors) > 0 || reached[pkg.ID] ||
				bare[pkg.PkgPath]
		}
	}

	out := make(map[string]covering, len(seen))

	for rel := range seen {
		if file, readable := where.text(rel); readable && !skipped[rel] {
			out[rel] = covering{textFile: file, incomplete: incomplete[rel]}
		}
	}

	return out
}
