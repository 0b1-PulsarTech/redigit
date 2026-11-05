package linting

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type cutFunction struct {
	head    entities.DeclKey
	headRow int
	stmts   []entities.Decl
	rows    []int
}

func CheckStructure(ctx context.Context, structure Structure, limits Limits) (Report, error) {
	cuts, tier, err := cutFunctions(structure)
	if err != nil {
		return Report{}, err
	}

	splits, errMeasure, err := errcheckSplits(ctx, structure, cuts)
	if err != nil {
		return Report{}, err
	}

	empty, tail, firstAway := shells(cuts, structure.Subjects)
	unused, unusedMeasure := unusedAtBirth(structure, limits)

	report := Report{Graph: &tier, Measures: []Measure{
		describeBornEmpty(),
		describeWithoutTail(len(cuts), firstAway),
		describeStubGap(limits),
		errMeasure,
		unusedMeasure,
	}}

	for _, group := range [][]Finding{
		empty, tail, stubGaps(cuts, structure.Subjects, limits), splits, unused,
	} {
		report.Findings = append(report.Findings, sortFindings(group)...)
	}

	for index := range report.Measures {
		for _, finding := range report.Findings {
			if finding.Measure == report.Measures[index].Name {
				report.Measures[index].Count++
			}
		}
	}

	return report, nil
}

func sortFindings(findings []Finding) []Finding {
	slices.SortStableFunc(findings, func(left, right Finding) int {
		return cmp.Or(
			cmp.Compare(left.Position, right.Position), cmp.Compare(left.Detail, right.Detail),
		)
	})

	return findings
}

func cutFunctions(structure Structure) ([]cutFunction, GraphTier, error) {
	graph := structure.Graph
	pieces := map[entities.DeclKey][]entities.Decl{}

	for _, decl := range graph.Decls() {
		if head, found := bodyHead(graph, decl); found {
			pieces[head] = append(pieces[head], decl)
		}
	}

	var cuts []cutFunction

	for _, head := range slices.SortedFunc(maps.Keys(pieces), entities.CompareKeys) {
		cut, err := placeCut(head, pieces[head], structure.Born)
		if err != nil {
			return nil, GraphTier{}, err
		}

		cuts = append(cuts, cut)
	}

	return cuts, GraphTier{CutFunctions: len(cuts)}, nil
}

func placeCut(
	head entities.DeclKey,
	stmts []entities.Decl,
	born map[entities.DeclKey]int,
) (cutFunction, error) {
	slices.SortFunc(stmts, func(left, right entities.Decl) int {
		return cmp.Or(
			cmp.Compare(left.Span.Start, right.Span.Start),
			entities.CompareKeys(left.Key, right.Key),
		)
	})

	cut := cutFunction{head: head, stmts: stmts}
	members := append([]entities.DeclKey{head}, keysOf(stmts)...)
	placed := 0

	for _, member := range members {
		if _, isBorn := born[member]; isBorn {
			placed++
		}
	}

	if placed < len(members) {
		missing := slices.IndexFunc(members, func(member entities.DeclKey) bool {
			_, isBorn := born[member]

			return !isBorn
		})

		return cutFunction{}, fmt.Errorf(
			"%w: %s is cut in the graph but %s is born in no row",
			ErrMismatch, head, members[missing],
		)
	}

	cut.headRow = born[head]
	for _, stmt := range stmts {
		cut.rows = append(cut.rows, born[stmt.Key])
	}

	return cut, nil
}

func keysOf(decls []entities.Decl) []entities.DeclKey {
	keys := make([]entities.DeclKey, len(decls))
	for index, decl := range decls {
		keys[index] = decl.Key
	}

	return keys
}

func bodyHead(graph entities.Graph, decl entities.Decl) (entities.DeclKey, bool) {
	if decl.Key.Kind != entities.KindStmt {
		return entities.DeclKey{}, false
	}

	for _, edge := range graph.Needs(decl.Key) {
		isCallable := edge.Provider.Kind == entities.KindFunc ||
			edge.Provider.Kind == entities.KindMethod
		if edge.Reason == entities.ReasonBody && isCallable && edge.Provider.File == decl.Key.File {
			return edge.Provider, true
		}
	}

	return entities.DeclKey{}, false
}
