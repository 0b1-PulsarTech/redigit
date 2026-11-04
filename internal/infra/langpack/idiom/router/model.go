package router

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type View struct {
	Routes  []Route
	Mounts  []Mount
	nonPath int
	Unmet   int
}

func (view View) Blind() int {
	return countRoutes(view.Routes, func(route Route) bool { return route.Path == "" })
}

func (view View) Inline() int {
	return countRoutes(view.Routes, func(route Route) bool { return route.Handler.IsZero() })
}

func countRoutes(routes []Route, pick func(Route) bool) int {
	count := 0

	for _, route := range routes {
		if pick(route) {
			count++
		}
	}

	return count
}

func build(cfg Config, decls []entities.Decl, known idiom.Facts) (View, error) {
	var (
		view    View
		mounts  []facts.Call
		routers = map[string]bool{}
		sites   = sourceOrder(cfg, decls)
	)

	for _, site := range sites {
		for _, call := range known.Calls(site.Key) {
			handles, err := matchesAny(call, cfg.Handle)
			if err != nil {
				return View{}, fmt.Errorf("calls of %s: %w", site.Key, err)
			}

			if handles {
				route := routeOf(call)
				if route.Path == "" && len(call.Literals) > 0 {
					view.nonPath++
				}

				route.Path = joinPath(prefixOf(cfg, call), route.Path)
				view.Routes = append(view.Routes, route)
				routers[call.Receiver.Name] = true

				continue
			}

			mounted, err := matchesAny(call, cfg.Mount)
			if err != nil {
				return View{}, fmt.Errorf("calls of %s: %w", site.Key, err)
			}

			if mounted {
				mounts = append(mounts, call)
			}
		}
	}

	stitchedFacts := newStitched(known, decls)
	view.Mounts = findMounts(cfg, mounts, stitchedFacts, routers)
	view.Unmet = unmetMounts(cfg, mounts, stitchedFacts, routers, view.Mounts)

	return view, nil
}

func sourceOrder(cfg Config, decls []entities.Decl) []entities.Decl {
	sites := make([]entities.Decl, 0, len(decls))

	for _, decl := range decls {
		switch decl.Key.Kind {
		case entities.KindFunc, entities.KindMethod, entities.KindStmt:
			if !entities.IsTestSide(decl) && cfg.within(decl.Key.File) {
				sites = append(sites, decl)
			}
		default:
		}
	}

	slices.SortStableFunc(sites, func(left, right entities.Decl) int {
		return cmp.Or(
			cmp.Compare(left.Key.File, right.Key.File),
			cmp.Compare(left.Span.Start, right.Span.Start),
		)
	})

	return sites
}
