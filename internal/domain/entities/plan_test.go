package entities

import (
	"slices"
	"testing"
)

func TestPlanFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		plan Plan
		want []string
	}{
		{"empty", Plan{}, nil},
		{
			"one feature commit spanning directories reports each path once",
			Plan{Commits: []PlannedCommit{{
				Shape: ShapeFeature,
				Decls: []DeclKey{
					{File: "entities/lead.go", Kind: KindType, Name: "Lead"},
					{File: "usecases/create.go", Kind: KindFunc, Name: "Create"},
					{File: "usecases/create.go", Kind: KindType, Name: "Input"},
				},
			}}},
			[]string{"entities/lead.go", "usecases/create.go"},
		},
		{
			"a path touched by two commits is still one file",
			Plan{Commits: []PlannedCommit{
				{Decls: []DeclKey{{File: "a.go", Kind: KindType, Name: "A"}}},
				{Decls: []DeclKey{{File: "a.go", Kind: KindFunc, Name: "New"}}},
			}},
			[]string{"a.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.plan.Files()
			slices.Sort(got)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Files() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestPlanLen(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		plan Plan
		want int
	}{
		{"empty", Plan{}, 0},
		{
			"unplaced declarations do not count as commits",
			Plan{
				Commits:  []PlannedCommit{{Subject: "one"}},
				Unplaced: []DeclKey{{File: "orphan.go", Kind: KindFunc, Name: "Lost"}},
			},
			1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.plan.Len(); got != testCase.want {
				t.Fatalf("Len() = %d, want %d", got, testCase.want)
			}
		})
	}
}
