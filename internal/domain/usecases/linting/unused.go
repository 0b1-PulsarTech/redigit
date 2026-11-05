package linting

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type birthTally struct {
	measured int
	never    int
	stub     int
	gaps     []int
	excluded map[string]int
	unplaced int
}

func unusedAtBirth(structure Structure, limits Limits) ([]Finding, Measure) {
	graph := structure.Graph
	tally := birthTally{excluded: map[string]int{}}

	var found []Finding

	for _, decl := range graph.Decls() {
		birth, isBorn := structure.Born[decl.Key]
		if !isBorn {
			tally.unplaced++

			continue
		}

		if reason := excludedFromBirth(structure, decl); reason != "" {
			tally.excluded[reason]++

			continue
		}

		tally.measured++

		first, isRead, isUsed := firstRead(graph, structure.Born, decl.Key, birth)
		if isUsed {
			continue
		}

		finding := Finding{
			Measure: MeasureUnusedAtBirth, Position: birth, Subject: structure.Subjects[birth],
			Detail: decl.Key.String() + ": never read",
		}

		if isRead {
			tally.gaps = append(tally.gaps, first-birth)
			finding.Value = first - birth
			finding.Detail = fmt.Sprintf("%s: first read in row %d", decl.Key, first)
			finding.NotCounted = first-birth <= limits.StubGap

			if !finding.NotCounted {
				tally.stub++
			}
		} else {
			tally.never++
		}

		found = append(found, finding)
	}

	return found, describeUnused(tally, len(found), limits.StubGap)
}

func excludedFromBirth(structure Structure, decl entities.Decl) string {
	switch {
	case decl.Key.Kind == entities.KindStmt:
		return "statement"
	case entities.IsTestSide(decl):
		return "test"
	case decl.Generated:
		return "generated"
	case !decl.Key.Kind.IsCode():
		return "not code"
	case decl.Key.Kind == entities.KindMethod:
		return "method"
	case hasImplements(structure.Graph, decl.Key):
		return "implements"
	}

	return structure.Syntax.Exempt(decl)
}

func hasImplements(graph entities.Graph, key entities.DeclKey) bool {
	isImplements := func(edge entities.Edge) bool { return edge.Reason == entities.ReasonImplements }

	return slices.ContainsFunc(graph.Needs(key), isImplements) ||
		slices.ContainsFunc(graph.Consumers(key), isImplements)
}

func firstRead(
	graph entities.Graph,
	born map[entities.DeclKey]int,
	key entities.DeclKey,
	birth int,
) (first int, isRead, isUsed bool) {
	for _, edge := range graph.Consumers(key) {
		if edge.Dependent == key || edge.Reason == entities.ReasonBody ||
			entities.IsStructural(edge.Reason) {
			continue
		}

		row, isBorn := born[edge.Dependent]
		if !isBorn {
			continue
		}

		if row <= birth {
			return birth, true, true
		}

		if !isRead || row < first {
			first, isRead = row, true
		}
	}

	return first, isRead, false
}

func describeUnused(tally birthTally, unused, stubGap int) Measure {
	gap := "none"

	if len(tally.gaps) > 0 {
		slices.Sort(tally.gaps)
		gap = fmt.Sprintf(
			"median %d, max %d", tally.gaps[len(tally.gaps)/2], tally.gaps[len(tally.gaps)-1],
		)
	}

	detail := fmt.Sprintf(
		"measured %d, unused at birth %d (never read %d; gap of the rest: %s); "+
			"counted by --fail-over %d",
		tally.measured, unused, tally.never, gap, tally.never+tally.stub,
	)

	if len(tally.excluded) > 0 {
		parts := make([]string, 0, len(tally.excluded))
		for _, reason := range slices.Sorted(maps.Keys(tally.excluded)) {
			parts = append(parts, fmt.Sprintf("%s %d", reason, tally.excluded[reason]))
		}

		detail += "; excluded: " + strings.Join(parts, ", ")
	}

	if tally.unplaced > 0 {
		detail += fmt.Sprintf("; unplaced %d", tally.unplaced)
	}

	return Measure{
		Name: MeasureUnusedAtBirth,
		Definition: "a production declaration that, in the row it is born, is read by nothing " +
			"born in that row or earlier; the finding's value is the rows until its first reader. " +
			fmt.Sprintf(
				"--fail-over counts only a declaration never read or first read more than %d rows "+
					"later (the stub-gap limit); the rest is reported with notCounted, because a "+
					"reader a few rows on is how code is usually built, not a stub",
				stubGap,
			),
		Detail: detail,
		Excluded: []string{
			"tests, anything declared in a test file, and generated declarations",
			"roots: main and init",
			"exported declarations of a package other than main",
			"methods, and any declaration the graph holds an implements edge for",
			"statements of a cut function, and non-code declarations (docs, config, data)",
			"files of a language whose exports the language pack cannot tell",
			"readers that are tests count as readers; a module, host, workspace or checksum " +
				"edge, and a statement's edge to its own head, do not",
			"declarations or readers the manifest never births (counted as unplaced)",
		},
	}
}
