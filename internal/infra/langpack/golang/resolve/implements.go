package resolve

import (
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type namedType struct {
	at     site
	where  entities.Location
	named  *types.Named
	target *types.Interface
	fset   *token.FileSet
	pkg    string
	local  bool
	alias  bool
}

func collectImplementors(all []namedType) implemented {
	out := map[site][]entities.Location{}

	for _, iface := range all {
		out[iface.at] = implementorsOf(iface, all, out[iface.at])
	}

	for _, found := range out {
		slices.SortFunc(found, compareLocations)
	}

	return implemented{implementors: out}
}

func namedTypes(pkgs []*packages.Package, where *places) []namedType {
	var all []namedType

	// A recompiled package types against the test variants its readers name, so it is a candidate.
	variants, _ := variantsOf(pkgs)

	for _, pkg := range slices.Concat(pkgs, variants) {
		if pkg.TypesInfo == nil {
			continue
		}

		for ident, obj := range pkg.TypesInfo.Defs {
			name, isName := obj.(*types.TypeName)
			if !isName || name.Pkg() == nil {
				continue
			}

			entry, usable := declared(name)
			if !usable {
				continue
			}

			// Unlike gopls, an implementor in the standard library or a dependency is dropped here;
			// the graph is unaffected because edgesFrom drops locations outside the tree.
			if at, loc, found := where.at(pkg.Fset, ident.Pos()); found {
				entry.at, entry.where, entry.fset = at, loc, pkg.Fset
				entry.pkg, entry.local = name.Pkg().Path(), name.Parent() != name.Pkg().Scope()
				all = append(all, entry)
			}
		}
	}

	return all
}

func declared(name *types.TypeName) (namedType, bool) {
	if name.IsAlias() {
		// An alias of a non-interface names no relationship; one of a named interface is asked
		// at its own site, where gopls counts the interface it stands for among the answers.
		// That interface is the target: an alias names one without being a candidate.
		target, isInterface := types.Unalias(name.Type()).Underlying().(*types.Interface)

		return namedType{target: target, alias: true}, isInterface
	}

	named, isNamed := name.Type().(*types.Named)
	if !isNamed {
		return namedType{}, false
	}

	target, _ := named.Underlying().(*types.Interface)

	return namedType{named: named, target: target}, true
}

func implementorsOf(
	iface namedType,
	all []namedType,
	already []entities.Location,
) []entities.Location {
	// Variants of one package are separate type-checks, so a pair is tried in each; the position
	// keeps one entry per type, and the interface never answers for itself.
	target := iface.target
	// An empty interface is satisfied by everything, which names no relationship; gopls answers
	// none for it.
	if target == nil || target.NumMethods() == 0 {
		return already
	}

	for _, candidate := range all {
		// gopls looks inside function bodies only within the interface's own package.
		if candidate.alias || candidate.at == iface.at ||
			slices.Contains(already, candidate.where) ||
			(candidate.local && candidate.pkg != iface.pkg) {
			continue
		}

		if satisfies(candidate, target) {
			already = append(already, candidate.where)
		}
	}

	return already
}

func satisfies(candidate namedType, target *types.Interface) bool {
	// Every pair goes through the unifier: a type from another test variant is a different object
	// for the same declaration, which types.Implements would not match. A constraint interface
	// is read by its methods alone, as gopls does, so its type-set terms are ignored.
	receiver := types.Type(types.NewPointer(candidate.named))
	// A pointer to an interface has no methods, so an interface candidate is looked into itself.
	if types.IsInterface(candidate.named) {
		receiver = candidate.named
	}

	for want := range target.Methods() {
		found, _, _ := types.LookupFieldOrMethod(receiver, true, want.Pkg(), want.Name())

		// A type parameter is held to what it met within one method; the next starts afresh.
		have, isFunc := found.(*types.Func)
		if !isFunc || !newUnifier(candidate.fset).match(want.Type(), have.Type()) {
			return false
		}
	}

	return true
}
