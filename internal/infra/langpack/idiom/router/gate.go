package router

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

const measureName = "route registrations: mounts placed with their first caller, " +
	"routes registered once and placed after their handler"

type gate struct {
	name    string
	cfg     Config
	factsOf idiom.FactsOf
}

type tally struct {
	value, total                                    int
	mounts, apart                                   int
	routes, repeats, early                          int
	unplacedMounts, unplacedRoutes                  int
	blind, nonPath, inline, matched, crossed, unmet int
}

var _ auditing.Gate = gate{}

func (probe gate) Name() string { return probe.name }

func (probe gate) Measure(graph entities.Graph, order entities.Ordering) entities.Measure {
	decls := graph.Decls()

	known, err := probe.factsOf(decls)
	if err != nil {
		return notMeasured(err)
	}

	view, err := build(probe.cfg, decls, known)
	if err != nil {
		return notMeasured(err)
	}

	count := tally{
		blind:   view.Blind() - view.nonPath,
		nonPath: view.nonPath,
		inline:  view.Inline(),
		matched: len(view.Routes),
		unmet:   view.Unmet,
	}
	count.mountsOf(view, order)
	count.routesOf(view, order)

	return entities.Measure{
		Name:     measureName,
		Value:    count.value,
		Total:    count.total,
		Excluded: count.excluded(),
	}
}

func (count *tally) mountsOf(view View, order entities.Ordering) {
	for _, mount := range view.Mounts {
		if !order.Of(mount.Fn).IsSet() || !order.Of(mount.Caller).IsSet() {
			count.unplacedMounts++

			continue
		}

		count.mounts++
		count.total++

		if order.Of(mount.Fn) == order.Of(mount.Caller) {
			count.value++

			continue
		}

		count.apart++
	}
}

func (count *tally) routesOf(view View, order entities.Ordering) {
	seen := map[string]map[entities.DeclKey]bool{}

	for _, route := range view.Routes {
		// A route with no path or no handler cannot be judged, so it is left out of the total.
		if route.Path == "" || route.Handler.IsZero() {
			continue
		}

		key, owner := route.Method+" "+route.Path, enclosing(route.Site)
		if seen[key] == nil {
			seen[key] = map[entities.DeclKey]bool{}
		}

		repeated := seen[key][owner]
		crossed := !repeated && len(seen[key]) > 0
		seen[key][owner] = true

		if !order.Of(route.Site).IsSet() {
			count.unplacedRoutes++

			continue
		}

		count.routes++
		count.total++

		early := order.Of(route.Handler) > order.Of(route.Site)

		if crossed {
			count.crossed++
		}

		if repeated {
			count.repeats++
		}

		if early {
			count.early++
		}

		if !repeated && !early {
			count.value++
		}
	}
}
