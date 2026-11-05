package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func reportLines(graph entities.Graph, births indexing.Births) []reportLine {
	lines := make([]reportLine, 0, graph.Len())
	subjects := subjectsOf(graph, births)
	satisfying := satisfyingTypes(graph)

	for _, decl := range graph.Decls() {
		born, dated := births.At[decl.Key]
		if !dated {
			born = entities.NoPosition
		}

		readers := readersOf(graph, births, decl.Key)
		inside, byTests := insideAndTestFanout(graph, decl)

		line := reportLine{
			Decl:         decl.Key.String(),
			Unit:         string(decl.Unit),
			IsTest:       decl.IsTest,
			InTestFile:   strings.HasSuffix(decl.Key.File, "_test.go"),
			BornAt:       int(born),
			Fanout:       len(readers),
			InsideFanout: inside,
			TestFanout:   byTests,
			InInterface:  inInterface(satisfying, decl),
			GroupSize:    decl.GroupSize,
			Readers:      readers,
			Gap:          0,
		}

		if len(readers) > 0 {
			first := readers[0]
			line.FirstRead = &first
			line.Gap = first.At - int(born)
		}

		if subject, anchored := subjects[decl.Key]; anchored {
			line.Subject = &subject
			line.SubjectGap = int(born) - subject.At
		}

		lines = append(lines, line)
	}

	slices.SortFunc(lines, func(a, b reportLine) int {
		if by := cmp.Compare(b.Gap, a.Gap); by != 0 {
			return by
		}

		return cmp.Compare(a.Decl, b.Decl)
	})

	return lines
}

// readersOf lists every reader outside the declaration's own unit, nearest
// first. Readers inside the unit are left out on purpose: counting them makes
// a package look consumed the instant its own file was created, which hides
// the real distance to whoever actually needs it.
func readersOf(
	graph entities.Graph,
	births indexing.Births,
	key entities.DeclKey,
) []readerDTO {
	edges := graph.ExternalConsumers(key)
	out := make([]readerDTO, 0, len(edges))

	for _, edge := range edges {
		at, dated := births.At[edge.Dependent]
		if !dated {
			at = entities.NoPosition
		}

		unit := ""
		if reader, known := graph.Decl(edge.Dependent); known {
			unit = string(reader.Unit)
		}

		out = append(out, readerDTO{
			Decl:   edge.Dependent.String(),
			At:     int(at),
			Reason: string(edge.Reason),
			Unit:   unit,
		})
	}

	slices.SortFunc(out, func(a, b readerDTO) int {
		if by := cmp.Compare(a.At, b.At); by != 0 {
			return by
		}

		return cmp.Compare(a.Decl, b.Decl)
	})

	return out
}

// insideAndTestFanout counts, separately, the readers in a declaration's own
// unit and the readers that are tests. A declaration with no reader of any
// kind is a different finding from one its own package leans on, and neither
// is visible from the external count alone.
func insideAndTestFanout(graph entities.Graph, decl entities.Decl) (inside, byTests int) {
	seen := map[entities.DeclKey]bool{}

	for _, edge := range graph.Consumers(decl.Key) {
		if seen[edge.Dependent] {
			continue
		}

		seen[edge.Dependent] = true

		reader, known := graph.Decl(edge.Dependent)
		if !known {
			continue
		}

		if reader.Unit == decl.Unit {
			inside++
		}

		if reader.IsTest {
			byTests++
		}
	}

	return inside, byTests
}
