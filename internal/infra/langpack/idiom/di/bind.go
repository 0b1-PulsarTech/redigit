package di

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

func boundRef(call facts.Call) (facts.TypeRef, bool) {
	// Without generic methods every type argument of a method call is its receiver's.
	if !call.Receiver.IsZero() || len(call.TypeArgs) == 0 || typeID(call.TypeArgs[0]) == "" {
		return facts.TypeRef{}, false
	}

	return call.TypeArgs[0], true
}

func bindKey(ref facts.TypeRef) entities.DeclKey {
	// A slice, map, func or generic instance is not the declaration its key names.
	if strings.ContainsAny(ref.Name, "[ (") {
		return entities.DeclKey{}
	}

	return ref.Key
}

func constructorNeeds(
	cfg Config,
	call facts.Call,
	known idiom.Facts,
) ([]facts.TypeRef, entities.DeclKey) {
	// Wrappers and options may stand around the constructor, so the last known function wins.
	for _, fn := range slices.Backward(call.FuncArgs) {
		args, err := known.CtorArgTypes(fn)
		if err != nil {
			continue
		}

		var needs []facts.TypeRef

		for _, arg := range args {
			if id := typeID(arg); id != "" && !cfg.isInjector(id) {
				needs = append(needs, arg)
			}
		}

		return needs, fn
	}

	return nil, entities.DeclKey{}
}

func (built *model) askImplementors(known idiom.Facts) {
	registered := map[string]bool{}
	for _, entry := range built.registrations {
		registered[entry.typ] = true
	}

	asked := map[entities.DeclKey]bool{}

	for _, use := range built.uses() {
		if use.key.IsZero() || (registered[use.typ] && !use.all) || asked[use.key] {
			continue
		}

		asked[use.key] = true

		if found := known.Implementors(use.key); len(found) > 0 {
			if built.impls == nil {
				built.impls = map[entities.DeclKey][]entities.DeclKey{}
			}

			built.impls[use.key] = found
		}
	}
}

func asUse(ref facts.TypeRef) resolve {
	return resolve{typ: typeID(ref), key: bindKey(ref)}
}

func (built model) uses() []resolve {
	out := slices.Clone(built.resolves)

	for _, entry := range built.registrations {
		out = append(out, entry.pulls...)

		for _, need := range entry.needs {
			out = append(out, asUse(need))
		}
	}

	return out
}

type catalog struct {
	byType map[string][]registration
	byKey  map[entities.DeclKey][]registration
	impls  map[entities.DeclKey][]entities.DeclKey
}

func (built model) catalog() catalog {
	view := catalog{
		byType: map[string][]registration{},
		byKey:  map[entities.DeclKey][]registration{},
		impls:  built.impls,
	}

	for idx, entry := range built.registrations {
		entry.id = idx

		view.byType[entry.typ] = append(view.byType[entry.typ], entry)

		if !entry.key.IsZero() {
			view.byKey[entry.key] = append(view.byKey[entry.key], entry)
		}
	}

	return view
}

func (built model) ambiguous() int {
	var (
		seen = map[resolve]bool{}
		view = built.catalog()
	)

	for _, use := range built.uses() {
		if _, isAmbiguous := view.serving(use); isAmbiguous {
			seen[use] = true
		}
	}

	return len(seen)
}

func (built model) summary() string {
	switch {
	case built.untyped > 0:
		return "calls with no static bound type"
	case len(built.registrations) == 0:
		return "0 register sites"
	default:
		return "bindings left out"
	}
}

func (view catalog) serving(use resolve) ([]registration, bool) {
	exact := view.byType[use.typ]

	// GetAll lists every bind of the type, the implementors beside the exact registrations.
	if (len(exact) > 0 && !use.all) || use.key.IsZero() {
		return exact, false
	}

	var (
		via   []registration
		kinds = map[string]bool{}
	)

	for _, impl := range view.impls[use.key] {
		for _, entry := range view.byKey[impl] {
			if entry.typ != use.typ {
				via = append(via, entry)
				kinds[entry.typ] = true
			}
		}
	}

	// One Get cannot choose between several implementors; GetAll takes them all.
	if !use.all && len(kinds) > 1 {
		return nil, true
	}

	return slices.Concat(exact, via), false
}
