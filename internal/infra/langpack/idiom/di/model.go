package di

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

//go:generate go tool -modfile=../../../../../tools/go.mod mockgen -source=../facts.go -destination=facts_mock_test.go -package=di

type (
	registration struct {
		site  entities.DeclKey
		typ   string
		key   entities.DeclKey
		needs []facts.TypeRef
		pulls []resolve // resolves made inside the constructor, needs of this registration
		id    int       // position in the model, set by the catalog
	}

	resolve struct {
		site entities.DeclKey
		typ  string
		key  entities.DeclKey
		all  bool
	}

	model struct {
		registrations []registration
		resolves      []resolve
		impls         map[entities.DeclKey][]entities.DeclKey
		untyped       int              // calls with no static bound type, counted so a gate can say so
		opaque        int              // constructors outside the loaded packages
		shared        map[resolve]bool // pulls whose helper other code also runs
	}
)

func buildModel(cfg Config, decls []entities.Decl, known idiom.Facts) (model, error) {
	return buildSeeded(cfg, decls, known, nil)
}

func buildSeeded(
	cfg Config,
	decls []entities.Decl,
	known idiom.Facts,
	seed func(entities.Decl) bool,
) (model, error) {
	var built model

	owners := map[entities.DeclKey][]int{}

	for _, decl := range decls {
		if !isSite(decl) || !cfg.within(decl.Key.File) {
			continue
		}

		for _, call := range known.Calls(decl.Key) {
			if err := built.add(cfg, call, known, owners); err != nil {
				return model{}, fmt.Errorf("calls of %s: %w", decl.Key, err)
			}
		}
	}

	built.fold(cfg, decls, owners, known, seed)
	built.askImplementors(known)

	return built, nil
}

func isSite(decl entities.Decl) bool {
	switch decl.Key.Kind {
	case entities.KindFunc, entities.KindMethod, entities.KindStmt:
		return !entities.IsTestSide(decl)
	default:
		return false
	}
}

func (built *model) add(
	cfg Config,
	call facts.Call,
	known idiom.Facts,
	owners map[entities.DeclKey][]int,
) error {
	if !cfg.injects(call) {
		return nil
	}

	registers, err := matchesAny(call, cfg.RegisterFuncs)
	if err != nil {
		return err
	}

	resolves, err := matchesAny(call, cfg.ResolveFuncs)
	if err != nil {
		return err
	}

	if !registers && !resolves {
		return nil
	}

	// A bound type is the first type argument, explicit or inferred; nothing else is guessed.
	ref, typed := boundRef(call)
	if !typed {
		built.untyped++

		return nil
	}

	bound := typeID(ref)

	if !registers {
		every := strings.Contains(call.Short(), "All")
		built.resolves = append(built.resolves,
			resolve{site: call.Site, typ: bound, key: bindKey(ref), all: every},
		)

		return nil
	}

	needs, ctor := constructorNeeds(cfg, call, known)
	if ctor.IsZero() && len(call.FuncArgs) > 0 {
		built.opaque++
	}

	owners[ctor] = append(owners[ctor], len(built.registrations))
	built.registrations = append(built.registrations,
		registration{site: call.Site, typ: bound, key: bindKey(ref), needs: needs},
	)

	return nil
}

func (built model) unbound() []string {
	missing, view := map[string]bool{}, built.catalog()

	for _, use := range built.uses() {
		serving, ambiguous := view.serving(use)
		missing[use.typ] = missing[use.typ] || (len(serving) == 0 && !ambiguous)
	}

	var out []string

	for typ, absent := range missing {
		if absent {
			out = append(out, typ)
		}
	}

	slices.Sort(out)

	return out
}
