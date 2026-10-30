package shaping

import (
	"slices"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type edgeSpec struct {
	dependent, provider string
	reason              entities.EdgeReason
}

func buildDraft(rows [][]string, specs []edgeSpec) *Draft {
	var (
		decls []entities.Decl
		plan  entities.Plan
	)

	edges := make([]entities.Edge, 0, len(specs))

	for index, row := range rows {
		commit := entities.PlannedCommit{
			Position: entities.Position(index), Subject: "row " + string(rune('a'+index)),
		}

		for _, name := range row {
			decls = append(decls, entities.Decl{Key: key(name), Lines: 10})
			commit.Decls = append(commit.Decls, key(name))
		}

		plan.Commits = append(plan.Commits, commit)
	}

	for _, spec := range specs {
		edges = append(edges, entities.Edge{
			Dependent: key(spec.dependent), Provider: key(spec.provider), Reason: spec.reason,
		})
	}

	return NewDraft(entities.NewGraph(decls, edges, nil), plan)
}

func TestDraftViolations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		rows [][]string
		spec []edgeSpec
		want []string
	}{
		{"no edges", [][]string{{"a"}, {"b"}}, nil, nil},
		{
			"provider first",
			[][]string{{"a"}, {"b"}},
			[]edgeSpec{{"b", "a", entities.ReasonReference}},
			nil,
		},
		{
			"provider in the same commit",
			[][]string{{"a", "b"}},
			[]edgeSpec{{"b", "a", entities.ReasonReference}},
			nil,
		},
		{
			"provider later",
			[][]string{{"a"}, {"b"}},
			[]edgeSpec{{"a", "b", entities.ReasonReference}},
			[]string{"a<-b"},
		},
		{
			"soft edge is ignored",
			[][]string{{"a"}, {"b"}},
			[]edgeSpec{{"a", "b", entities.ReasonTestOf}},
			nil,
		},
		{
			"unplaced endpoint is ignored",
			[][]string{{"a"}},
			[]edgeSpec{{"a", "ghost", entities.ReasonReference}},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for _, edge := range buildDraft(testCase.rows, testCase.spec).Violations() {
				got = append(got, edge.Dependent.Name+"<-"+edge.Provider.Name)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Violations() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDraftFits(t *testing.T) {
	t.Parallel()

	rows := [][]string{{"a"}, {"b"}, {"c"}, {"d"}}
	specs := []edgeSpec{
		{"b", "a", entities.ReasonReference},
		{"d", "c", entities.ReasonReference},
		{"d", "b", entities.ReasonTestOf},
	}

	testCases := []struct {
		name  string
		keys  []string
		after int
		want  bool
	}{
		{"after its provider", []string{"c"}, 0, true},
		{"before its provider", []string{"b"}, -1, false},
		{"past its consumer", []string{"c"}, 3, false},
		{"soft consumer is free", []string{"b"}, 2, true},
		{"consumer inside the set", []string{"c", "d"}, 3, true},
		{"after the last slot", []string{"a"}, 3, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := buildDraft(rows, specs)

			keys := make([]entities.DeclKey, 0, len(testCase.keys))
			for _, name := range testCase.keys {
				keys = append(keys, key(name))
			}

			if got := draft.Fits(keys, testCase.after); got != testCase.want {
				t.Fatalf(
					"Fits(%v, %d) = %v, want %v",
					testCase.keys,
					testCase.after,
					got,
					testCase.want,
				)
			}
		})
	}
}

func TestDraftClone(t *testing.T) {
	t.Parallel()

	draft := buildDraft([][]string{{"a"}, {"b"}}, nil)
	clone := draft.Clone()

	if _, ok := clone.Spawn(0, entities.PlannedCommit{}, []entities.DeclKey{key("b")}); !ok {
		t.Fatal("Spawn() on the clone was refused")
	}

	if got := len(draft.Slots()); got != 2 {
		t.Fatalf("original has %d slots after the clone changed, want 2", got)
	}

	if id, _ := draft.Of(key("b")); id != 1 {
		t.Fatalf("Of(b) on the original = %d, want 1", id)
	}
}

func benchDraft(decls, edges, commits int) *Draft {
	names := make([]string, decls)
	rows := make([][]string, commits)

	for index := range names {
		names[index] = "d" + strconv.Itoa(index)
		rows[index%commits] = append(rows[index%commits], names[index])
	}

	specs := make([]edgeSpec, edges)
	for index := range specs {
		dependent := (index*7 + 3) % decls
		provider := names[(dependent*13+index)%decls]
		specs[index] = edgeSpec{names[dependent], provider, entities.ReasonReference}
	}

	return buildDraft(rows, specs)
}

func BenchmarkViolations(b *testing.B) {
	const decls, edges = 2000, 20000

	draft := benchDraft(decls, edges, 300)

	b.ReportAllocs()
	b.ResetTimer()

	var found int
	for b.Loop() {
		found = len(draft.Violations())
	}

	b.ReportMetric(float64(edges), "edges")
	b.ReportMetric(float64(found), "violations")
}
