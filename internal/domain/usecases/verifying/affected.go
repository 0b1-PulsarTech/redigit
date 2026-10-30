package verifying

import (
	"cmp"
	"slices"
)

func Resolve(modules []Module) []Unit {
	byPath := make(map[string][]Module, len(modules))
	byDir := make(map[string]Module, len(modules))

	for _, module := range modules {
		byPath[module.Path] = append(byPath[module.Path], module)
		byDir[module.Dir] = module
	}

	units := make([]Unit, 0, len(modules))

	for _, module := range modules {
		deps := withWorkspace(module, modules, closure(module, byPath, byDir))
		units = append(units, Unit{Module: module, Deps: deps})
	}

	slices.SortFunc(units, func(left, right Unit) int { return cmp.Compare(left.Dir, right.Dir) })

	return units
}

func withWorkspace(root Module, modules, deps []Module) []Module {
	if root.Workspace == "" {
		return deps
	}

	for _, module := range modules {
		isKnown := slices.ContainsFunc(deps, func(dep Module) bool { return dep.Dir == module.Dir })
		if module.Workspace == root.Workspace && module.Dir != root.Dir && !isKnown {
			deps = append(deps, module)
		}
	}

	slices.SortFunc(deps, func(left, right Module) int { return cmp.Compare(left.Dir, right.Dir) })

	return deps
}

func closure(root Module, byPath map[string][]Module, byDir map[string]Module) []Module {
	seen := map[string]bool{root.Dir: true}
	pending := []Module{root}
	deps := []Module{}

	visit := func(dep Module) {
		if seen[dep.Dir] {
			return
		}

		seen[dep.Dir] = true

		deps = append(deps, dep)
		pending = append(pending, dep)
	}

	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		for _, required := range current.Requires {
			// A path two modules share is ambiguous, so the requirer is keyed on all of them.
			for _, dep := range byPath[required] {
				visit(dep)
			}
		}

		for _, local := range current.Locals {
			dep, isModule := byDir[local.Dir]
			if !isModule {
				dep = Module{Dir: local.Dir, Path: local.Dir, Subtree: local.Subtree}
			}

			visit(dep)
		}
	}

	slices.SortFunc(deps, func(left, right Module) int { return cmp.Compare(left.Dir, right.Dir) })

	return deps
}
