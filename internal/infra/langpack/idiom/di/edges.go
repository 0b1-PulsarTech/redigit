package di

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (built model) edges() []entities.Edge {
	var (
		out  []entities.Edge
		view = built.catalog()
	)

	link := func(site entities.DeclKey, use resolve, reason entities.EdgeReason) {
		serving, _ := view.serving(use)

		for _, provider := range serving {
			// A function-level site registers and resolves in one declaration.
			if provider.site != site {
				out = append(
					out,
					entities.Edge{Dependent: site, Provider: provider.site, Reason: reason},
				)
			}
		}
	}

	for _, entry := range built.registrations {
		for _, need := range entry.needs {
			link(entry.site, asUse(need), entities.ReasonProvides)
		}

		for _, pull := range entry.pulls {
			link(entry.site, pull, entities.ReasonProvides)
		}
	}

	for _, use := range built.resolves {
		link(use.site, use, entities.ReasonRegistered)
	}

	slices.SortFunc(out, func(left, right entities.Edge) int {
		return cmp.Or(
			entities.CompareKeys(left.Dependent, right.Dependent),
			entities.CompareKeys(left.Provider, right.Provider),
			cmp.Compare(left.Reason, right.Reason),
		)
	})

	return slices.Compact(out)
}
