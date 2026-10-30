package shaping

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestDraftPlan(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		spawn        string
		subject      string
		wantSubjects []string
	}{
		{"untouched draft round-trips", "", "", []string{"row a", "row b", "row c"}},
		{
			"added commit gets a draft subject",
			"c", "test: cover c",
			[]string{"row a", "row b", "DRAFT: test: cover c"},
		},
		{
			"added commit keeps a draft subject once",
			"b", "DRAFT: test: cover b",
			[]string{"row a", "row c", "DRAFT: test: cover b"},
		},
		{
			"emptied commit is dropped and positions stay dense",
			"a", "test: cover a",
			[]string{"row b", "row c", "DRAFT: test: cover a"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := buildDraft([][]string{{"a"}, {"b"}, {"c"}}, nil)
			draft.unplaced = []entities.DeclKey{key("lost")}

			if testCase.spawn != "" {
				commit := entities.PlannedCommit{Subject: testCase.subject}
				if _, ok := draft.Spawn(2, commit, []entities.DeclKey{key(testCase.spawn)}); !ok {
					t.Fatal("Spawn() refused")
				}
			}

			plan := draft.Plan()

			gotSubjects := make([]string, 0, len(plan.Commits))

			for index, commit := range plan.Commits {
				gotSubjects = append(gotSubjects, commit.Subject)

				if int(commit.Position) != index {
					t.Fatalf("commit %d has position %d", index, commit.Position)
				}
			}

			if !slices.Equal(gotSubjects, testCase.wantSubjects) {
				t.Fatalf("subjects = %v, want %v", gotSubjects, testCase.wantSubjects)
			}

			if len(plan.Unplaced) != 1 || plan.Unplaced[0] != key("lost") {
				t.Fatalf("Unplaced = %v, want the one kept", plan.Unplaced)
			}
		})
	}
}
