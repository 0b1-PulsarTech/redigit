package linting

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	edgeSpec struct {
		dependent string
		provider  string
		reason    entities.EdgeReason
	}

	structureSpec struct {
		placed    map[string]int
		unplaced  []string
		edges     []edgeSpec
		tests     []string
		generated []string
	}
)

func declOf(name string) entities.Decl {
	kind, base := entities.KindFunc, name

	if head, tail, found := strings.Cut(name, ":"); found {
		kind, base = entities.DeclKind(head), tail
	}

	if head, index, isStmt := strings.Cut(base, "#"); isStmt {
		position, _ := strconv.Atoi(index)

		return entities.Decl{
			Key:  entities.DeclKey{File: "a.go", Kind: entities.KindStmt, Name: base},
			Unit: "a",
			Span: entities.Span{Start: position * 10, End: position*10 + 10},
			Atom: head,
		}
	}

	return entities.Decl{
		Key:  entities.DeclKey{File: "a.go", Kind: kind, Name: base},
		Unit: "a",
		Span: entities.Span{Start: 0, End: 5},
	}
}

func structureOf(spec structureSpec, syntax Syntax) Structure {
	var edges []entities.Edge

	born := map[entities.DeclKey]int{}
	subjects := map[int]string{}

	names := slices.Concat(slices.Collect(maps.Keys(spec.placed)), spec.unplaced)
	decls := make([]entities.Decl, 0, len(names))

	for _, name := range names {
		decl := declOf(name)
		decl.IsTest = slices.Contains(spec.tests, name)
		decl.Generated = slices.Contains(spec.generated, name)
		decls = append(decls, decl)

		if row, isPlaced := spec.placed[name]; isPlaced {
			born[decl.Key] = row
			subjects[row] = "row " + strconv.Itoa(row)
		}

		if decl.Key.Kind == entities.KindStmt {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: decl.Atom},
				Reason:    entities.ReasonBody,
			})
		}
	}

	for _, edge := range spec.edges {
		reason := edge.reason
		if reason == "" {
			reason = entities.ReasonReference
		}

		edges = append(edges, entities.Edge{
			Dependent: declOf(edge.dependent).Key,
			Provider:  declOf(edge.provider).Key,
			Reason:    reason,
		})
	}

	return Structure{
		Graph: entities.NewGraph(decls, edges, nil), Born: born, Subjects: subjects, Syntax: syntax,
	}
}

func findingsOf(report Report, measure string) []Finding {
	var out []Finding

	for _, finding := range report.Findings {
		if finding.Measure == measure {
			out = append(out, finding)
		}
	}

	return out
}

func measureOf(report Report, name string) Measure {
	for _, measure := range report.Measures {
		if measure.Name == name {
			return measure
		}
	}

	return Measure{}
}

func equalFindings(tb testing.TB, got, want []Finding) {
	tb.Helper()

	if len(got) != len(want) {
		tb.Fatalf("findings = %+v, want %+v", got, want)
	}

	for index := range got {
		if got[index] != want[index] {
			tb.Fatalf("finding %d = %+v, want %+v", index, got[index], want[index])
		}
	}
}
