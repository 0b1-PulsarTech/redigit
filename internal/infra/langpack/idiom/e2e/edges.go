package e2e

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type tally struct {
	listening int
	blind     int
	unknown   map[string]bool
}

func (rule *Rule) Edges(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
	known idiom.Facts,
) ([]entities.Edge, error) {
	view, err := rule.router.Routes(decls, known)
	if err != nil {
		return nil, fmt.Errorf("routes: %w", err)
	}

	var (
		edges []entities.Edge
		count = tally{unknown: map[string]bool{}}
		rows  = newTable(view, newTopology(decls, known, rule.mount))
	)

	for _, decl := range decls {
		if !rule.listens(decl) {
			continue
		}

		calls := known.Calls(decl.Key)

		count.listening++
		if len(calls) == 0 {
			count.blind++
		}

		edges = append(edges, rule.edgesOf(decl.Key, calls, rows, count.unknown)...)
	}

	slices.SortFunc(edges, func(left, right entities.Edge) int {
		return cmp.Or(
			entities.CompareKeys(left.Dependent, right.Dependent),
			entities.CompareKeys(left.Provider, right.Provider),
		)
	})

	edges = slices.Compact(edges)

	rule.reported.Do(func() { rule.report(count, len(view.Routes), len(edges)) })

	return edges, nil
}

func (rule *Rule) edgesOf(
	dependent entities.DeclKey,
	calls []facts.Call,
	rows table,
	unknown map[string]bool,
) []entities.Edge {
	var edges []entities.Edge

	for _, call := range calls {
		if !rule.canRequest(call) {
			continue
		}

		for _, literal := range call.Literals {
			method, url, found := urlOf(literal)
			if !found {
				continue
			}

			providers := rows.waitsFor(method, url)
			if len(providers) == 0 {
				unknown[literal] = true
			}

			for _, provider := range providers {
				edges = append(edges, entities.Edge{
					Dependent: dependent, Provider: provider, Reason: entities.ReasonE2E,
				})
			}
		}
	}

	return edges
}

func (rule *Rule) listens(decl entities.Decl) bool {
	switch decl.Key.Kind {
	case entities.KindFunc, entities.KindMethod, entities.KindStmt:
	default:
		return false
	}

	return entities.IsTestSide(decl) && slices.ContainsFunc(rule.dirs, func(dir string) bool {
		return strings.HasPrefix(decl.Key.File, strings.TrimSuffix(dir, "/")+"/")
	})
}
