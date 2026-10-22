package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func fragmentCommit(feature string, decls ...entities.Decl) entities.PlannedCommit {
	return entities.PlannedCommit{
		Shape:   entities.ShapeFeature,
		Feature: feature,
		Decls:   keysOf(decls),
	}
}

type fragmentDecls struct {
	port, impl, assertion, body, scrap, big, other entities.Decl
}

func newFragmentDecls() fragmentDecls {
	port := decl("a/port.go", "Port", "a")
	impl := decl("a/impl.go", "Impl", "a")
	assertion := decl("a/impl.go", "_ Port = Impl{}", "a")
	assertion.Key.Kind = entities.KindValue
	assertion.Lines = 1
	body := decl("b/body.go", "Body", "b")
	body.Lines = 400
	scrap := decl("c/scrap.go", "Scrap", "c")
	scrap.Lines = 3
	big := decl("d/big.go", "Big", "d")
	big.Lines = 899
	other := decl("e/other.go", "Other", "e")
	other.Lines = 899

	return fragmentDecls{port, impl, assertion, body, scrap, big, other}
}

func TestMergeFragments(t *testing.T) {
	t.Parallel()

	fixtures := newFragmentDecls()
	port, impl, assertion := fixtures.port, fixtures.impl, fixtures.assertion
	body, scrap, big, other := fixtures.body, fixtures.scrap, fixtures.big, fixtures.other
	graph := entities.NewGraph(
		[]entities.Decl{port, impl, assertion, body, scrap, big, other},
		nil,
		nil,
	)
	planner := New(Config{MinCommitLines: 15})

	testCases := []struct {
		name    string
		planner Planner
		commits []entities.PlannedCommit
		want    []int
	}{
		{
			"a lone assertion folds into the commit before it",
			planner,
			[]entities.PlannedCommit{
				fragmentCommit("a", port, impl), fragmentCommit("a", assertion),
			},
			[]int{3},
		},
		{
			"a scrap folds into the commit after it when none is before",
			planner,
			[]entities.PlannedCommit{
				fragmentCommit("c", scrap), fragmentCommit("b", body),
			},
			[]int{2},
		},
		{
			"a scrap stays when the cap leaves no room",
			planner,
			[]entities.PlannedCommit{
				fragmentCommit("d", big), fragmentCommit("c", scrap), fragmentCommit("e", other),
			},
			[]int{1, 1, 1},
		},
		{
			"nothing merges when the knob is off",
			New(Config{}),
			[]entities.PlannedCommit{
				fragmentCommit("a", port, impl), fragmentCommit("a", assertion),
			},
			[]int{2, 1},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.planner.mergeFragments(graph, testCase.commits)

			sizes := make([]int, 0, len(got))
			for _, commit := range got {
				sizes = append(sizes, len(commit.Decls))
			}

			if len(sizes) != len(testCase.want) {
				t.Fatalf("commit sizes = %v, want %v", sizes, testCase.want)
			}

			for index := range sizes {
				if sizes[index] != testCase.want[index] {
					t.Fatalf("commit sizes = %v, want %v", sizes, testCase.want)
				}
			}
		})
	}
}
