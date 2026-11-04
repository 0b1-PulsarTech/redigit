package di

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

func (built *model) fold(
	cfg Config,
	decls []entities.Decl,
	owners map[entities.DeclKey][]int,
	known idiom.Facts,
	seed func(entities.Decl) bool,
) {
	if parts := stitch(decls); parts != nil {
		known = stitched{Facts: known, parts: parts}
	}

	inside := map[entities.DeclKey][]entities.DeclKey{}

	for ctor := range owners {
		if ctor.IsZero() {
			continue
		}

		for _, body := range bodies(cfg, ctor, known) {
			inside[body] = append(inside[body], ctor)
		}
	}

	if len(inside) == 0 {
		return
	}

	var shared map[entities.DeclKey]bool
	if seed != nil {
		shared = runByOthers(cfg, decls, owners, known, seed)
	}

	for _, use := range built.resolves {
		var found []entities.DeclKey

		for _, head := range heads(use.site) {
			found = append(found, inside[head]...)

			if shared[head] {
				built.share(use)
			}
		}

		slices.SortFunc(found, entities.CompareKeys)
		found = slices.Compact(found)

		// The resolve stays: a helper shared with boot code is still a site that needs its type.
		for _, ctor := range found {
			for _, idx := range owners[ctor] {
				built.registrations[idx].pulls = append(built.registrations[idx].pulls, use)
			}
		}
	}
}

func bodies(cfg Config, ctor entities.DeclKey, known idiom.Facts) []entities.DeclKey {
	seen := map[entities.DeclKey]bool{ctor: true}
	queue := []entities.DeclKey{ctor}

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]

		for _, call := range known.Calls(key) {
			// A registration runs, but it hands its constructors to the container, which runs those.
			if registers(cfg, call) {
				call.FuncArgs = nil
			}

			for _, target := range targets(call) {
				if seen[target] || !takesInjector(cfg, target, known) {
					continue
				}

				seen[target] = true

				queue = append(queue, target)
			}
		}
	}

	out := make([]entities.DeclKey, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}

	return out
}

type stitched struct {
	idiom.Facts

	parts map[entities.DeclKey][]entities.DeclKey
}

func stitch(decls []entities.Decl) map[entities.DeclKey][]entities.DeclKey {
	var parts map[entities.DeclKey][]entities.DeclKey

	for _, decl := range decls {
		if decl.Key.Kind != entities.KindStmt {
			continue
		}

		if parts == nil {
			parts = map[entities.DeclKey][]entities.DeclKey{}
		}

		// A function cut into statements records its calls under Name#i, never at its head.
		for _, head := range heads(decl.Key)[1:] {
			parts[head] = append(parts[head], decl.Key)
		}
	}

	return parts
}

func (view stitched) Calls(site entities.DeclKey) []facts.Call {
	out := view.Facts.Calls(site)

	for _, part := range view.parts[site] {
		out = append(slices.Clip(out), view.Facts.Calls(part)...)
	}

	return out
}

func targets(call facts.Call) []entities.DeclKey {
	var out []entities.DeclKey

	for _, key := range append([]entities.DeclKey{call.CalleeKey}, call.FuncArgs...) {
		if !key.IsZero() {
			out = append(out, key)
		}
	}

	return out
}

func takesInjector(cfg Config, fn entities.DeclKey, known idiom.Facts) bool {
	args, err := known.CtorArgTypes(fn)

	return err == nil && slices.ContainsFunc(args, func(arg facts.TypeRef) bool {
		return cfg.isInjector(typeID(arg))
	})
}

func heads(site entities.DeclKey) []entities.DeclKey {
	if site.Kind != entities.KindStmt {
		return []entities.DeclKey{site}
	}

	name := site.Name[:lastHash(site.Name)]

	return []entities.DeclKey{
		site,
		{File: site.File, Kind: entities.KindFunc, Name: name},
		{File: site.File, Kind: entities.KindMethod, Name: name},
	}
}

func (built *model) share(use resolve) {
	if built.shared == nil {
		built.shared = map[resolve]bool{}
	}

	built.shared[use] = true
}
