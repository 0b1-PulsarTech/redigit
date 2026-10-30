package passes

import (
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

func sharedProviderCases() []tidyCase {
	provider := declSpec{file: "pkg/b/b.go", name: "B", lines: 20}
	filler := declSpec{file: "pkg/c/c.go", name: "F", lines: 30}
	other := declSpec{file: "pkg/c/x.go", name: "X", lines: 30}
	testOf := func(name string) declSpec {
		return declSpec{file: "pkg/b/" + name + "_test.go", name: name, lines: 90, test: true}
	}
	twoTests := []edgeSpec{{"T1", "B"}, {"T2", "B"}}
	threeTests := append(slices.Clone(twoTests), edgeSpec{"T3", "B"})

	return []tidyCase{
		{
			name:         "two tests sharing a provider settle in order",
			rows:         [][]declSpec{{provider}, {other}, {testOf("T1")}, {testOf("T2")}},
			edges:        twoTests,
			wantSubjects: []string{"row a", "row c", "row d", "row b"},
		},
		{
			name: "two tests sharing a provider past a filler settle in order",
			rows: [][]declSpec{
				{provider}, {filler}, {other}, {testOf("T1")}, {testOf("T2")},
			},
			edges:        twoTests,
			wantSubjects: []string{"row a", "row d", "row e", "row b", "row c"},
		},
		{
			name: "three tests sharing a provider settle in order",
			rows: [][]declSpec{
				{provider}, {other}, {testOf("T1")}, {testOf("T2")}, {testOf("T3")},
			},
			edges:        threeTests,
			wantSubjects: []string{"row a", "row c", "row d", "row e", "row b"},
		},
		{
			name: "three tests sharing a provider past a filler settle in order",
			rows: [][]declSpec{
				{provider}, {filler}, {other}, {testOf("T1")}, {testOf("T2")}, {testOf("T3")},
			},
			edges:        threeTests,
			wantSubjects: []string{"row a", "row d", "row e", "row f", "row b", "row c"},
		},
		{
			name: "a multi-block all-test commit splits into one DRAFT piece per block",
			rows: [][]declSpec{
				{provider},
				{{file: "pkg/c/c.go", name: "C", lines: 20}},
				{filler},
				{
					testOf("T1"),
					{file: "pkg/c/c_test.go", name: "T2", lines: 90, test: true},
				},
			},
			edges: []edgeSpec{{"T1", "B"}, {"T2", "C"}},
			wantSubjects: []string{
				"row a", "DRAFT: test(b): T1_test.go", "row b",
				"DRAFT: test(c): c_test.go", "row c",
			},
		},
	}
}

func TestFixpoint(t *testing.T) {
	t.Parallel()

	failure := errors.New("round failed")
	one := shaping.Moves{{}}

	testCases := []struct {
		name      string
		rounds    int
		script    []shaping.Moves
		scriptErr error
		wantRuns  int
		wantMoves int
		wantErr   error
	}{
		{"stops at the first quiet round", 5, []shaping.Moves{one, one, nil}, nil, 3, 2, nil},
		{"a round error stops it", 5, []shaping.Moves{one}, failure, 1, 1, failure},
		{"the bound is an error", 2, []shaping.Moves{one, one, one}, nil, 2, 2, ErrUnsettled},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runs := 0
			moves, err := fixpoint(testCase.rounds, func() (shaping.Moves, error) {
				runs++
				if testCase.scriptErr != nil && runs == len(testCase.script) {
					return testCase.script[runs-1], testCase.scriptErr
				}

				return testCase.script[runs-1], nil
			})

			if !errors.Is(err, testCase.wantErr) || runs != testCase.wantRuns ||
				len(moves) != testCase.wantMoves {
				t.Fatalf(
					"fixpoint() = %d moves, %v after %d runs, want %d, %v, %d",
					len(moves), err, runs, testCase.wantMoves, testCase.wantErr, testCase.wantRuns,
				)
			}
		})
	}
}

func TestTidyTailIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, testCase := range sharedProviderCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, _ := applyTwice(t, testCase)
			if !slices.Equal(got, testCase.wantSubjects) {
				t.Fatalf("subjects = %v, want %v", got, testCase.wantSubjects)
			}
		})
	}
}
