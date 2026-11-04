package di

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

const measureName = "DI resolve sites born no earlier than a registration of their type"

type gate struct {
	name    string
	cfg     Config
	factsOf idiom.FactsOf
}

var _ auditing.Gate = gate{}

func (probe gate) Name() string { return probe.name }

func (probe gate) Measure(graph entities.Graph, order entities.Ordering) entities.Measure {
	decls := graph.Decls()

	known, err := probe.factsOf(decls)
	if err != nil {
		return notMeasured(err)
	}

	reach := reachable(graph, probe.cfg.Roots)

	built, err := buildSeeded(probe.cfg, decls, known, func(decl entities.Decl) bool {
		return reach == nil || reach[decl.Key]
	})
	if err != nil {
		return notMeasured(err)
	}

	seen, view := map[resolve]bool{}, built.catalog()
	check := newReadiness(view, order)

	var value, total, unboundSites, offRoot, unplaced, inCtors int

	pulled := built.pulled(reach)

	for _, use := range built.resolves {
		if seen[use] {
			continue
		}

		seen[use] = true

		serving, ambiguous := view.serving(use)

		switch {
		case reach != nil && !reach[use.site]:
			offRoot++
		case pulled[use]:
			inCtors++
		case len(serving) == 0 && !ambiguous:
			unboundSites++
		case !order.Of(use.site).IsSet():
			unplaced++
		default:
			total++

			if check.readyAt(use) <= order.Of(use.site) {
				value++
			}
		}
	}

	return entities.Measure{
		Name:  measureName,
		Value: value,
		Total: total,
		Excluded: append([]string{
			fmt.Sprintf(
				"%d resolve sites whose type no registration binds (%d required types are unbound)",
				unboundSites,
				len(built.unbound()),
			),
			fmt.Sprintf(
				"%d resolve sites outside the code reachable from the configured roots",
				offRoot,
			),
			fmt.Sprintf(
				"%d resolve sites inside constructors, folded into the needs of their registration",
				inCtors,
			),
			fmt.Sprintf("resolve sites the ordering never placed: %d", unplaced),
			fmt.Sprintf("dynamic registration: %d register or resolve calls with no static type, "+
				"%d constructors outside the loaded packages, and any binding made at run time",
				built.untyped, built.opaque),
		}, limits()...),
	}
}

func notMeasured(err error) entities.Measure {
	return entities.Measure{
		Name:     measureName,
		Excluded: []string{fmt.Sprintf("not measured: %v", err)},
	}
}

func (built model) pulled(reach map[entities.DeclKey]bool) map[resolve]bool {
	out := map[resolve]bool{}

	for _, entry := range built.registrations {
		// A registration that never runs does not build its constructor, so it pulls nothing.
		if reach != nil && !reach[entry.site] {
			continue
		}

		for _, pull := range entry.pulls {
			if !built.shared[pull] {
				out[pull] = true
			}
		}
	}

	return out
}

func limits() []string {
	return []string{
		"calls through a local func value, a local method value or an interface have no Call fact, " +
			"so a window through them stays hidden",
		"a registration whose constructor is a func literal has no needs or pulls, " +
			"so a resolve under what it calls reads as standalone and its readiness is not followed",
		"a wrapper that is no configured register function reads as a direct call of what it was handed, " +
			"so a constructor passed through it is taken to run by hand",
		"a func literal handed to a registration is taken to run when the container builds, " +
			"so a hand call inside it is not seen as boot code",
		"a named function a constructor hands on as a value is taken to run inside it, " +
			"so one stored for boot code to run later reads as folded",
		"under configured roots a package var or init that runs when its package loads " +
			"counts as boot only when a root reaches it, so a constructor it calls by hand reads as folded",
		"a root in a test file opens nothing, so a resolve only a test reaches counts as outside the roots",
		"a func converted to no func type, as any(helper), hands nothing on, " +
			"so what runs it is not followed",
		"a root glob also opens a plain function a constructor calls, " +
			"so a resolve under it stays a window",
	}
}
