package shaping

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type reseatCase struct {
	name        string
	rows        [][]string
	merge       []int
	id          int
	after       int
	wantMoves   int
	wantSubject []string
}

func reseatCases() []reseatCase {
	rows := [][]string{{"a"}, {"b"}, {"c", "d"}}
	same := []string{"row a", "row b", "row c"}
	swapped := []string{"row b", "row a", "row c"}
	merged := [][]string{{"a"}, {"b"}, {"c"}, {"d"}}
	holed := [][]string{{"a"}, {}, {"c"}}
	viaAlias := []string{"row a", "row d", "row c"}
	provided := []string{"row a", "row c", "row b"}

	return []reseatCase{
		{"moves a commit after its provider", rows, nil, 2, 0, 2, provided},
		{"stays when already right after the target", rows, nil, 1, 0, 0, same},
		{"refused when a consumer would precede", rows, nil, 0, 2, 0, same},
		{"refused for the same slot", rows, nil, 1, 1, 0, same},
		{"refused for an unknown slot", rows, nil, 9, 0, 0, same},
		{"refused after an unknown slot", rows, nil, 2, 9, 0, same},
		{"moves to the front for NoSlot", rows, nil, 1, NoSlot, 1, swapped},
		{"stays when already at the front", rows, nil, 0, NoSlot, 0, same},
		{"refused at the front when a provider would follow", rows, nil, 2, NoSlot, 0, same},
		{"moves forward past the slot after its target", rows, nil, 0, 1, 1, swapped},
		{"follows an alias from an earlier merge", merged, []int{1, 0}, 3, 1, 1, viaAlias},
		{"refused for an emptied slot", holed, nil, 1, NoSlot, 0, []string{"row a", "row c"}},
	}
}

func TestDraftReseat(t *testing.T) {
	t.Parallel()

	specs := []edgeSpec{{"c", "a", entities.ReasonReference}}

	for _, testCase := range reseatCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := buildDraft(testCase.rows, specs)
			if testCase.merge != nil {
				draft.Merge(testCase.merge[0], testCase.merge[1])
			}

			moves, ok := draft.Reseat(testCase.id, testCase.after)
			if ok != (testCase.wantMoves > 0) || len(moves) != testCase.wantMoves {
				t.Fatalf("Reseat() = %d moves, %v, want %d", len(moves), ok, testCase.wantMoves)
			}

			for _, move := range moves {
				want := Move{Decl: move.Decl, From: testCase.id, To: testCase.after, Reseat: true}
				if testCase.merge != nil {
					want.To = testCase.merge[1]
				}

				if move != want {
					t.Fatalf("Reseat() move = %+v, want %+v", move, want)
				}
			}

			plan := draft.Plan()

			got := make([]string, 0, len(plan.Commits))
			for _, commit := range plan.Commits {
				got = append(got, commit.Subject)
			}

			if !slices.Equal(got, testCase.wantSubject) {
				t.Fatalf("subjects = %v, want %v", got, testCase.wantSubject)
			}

			if len(draft.Violations()) != 0 {
				t.Fatalf("Reseat() left violations: %v", draft.Violations())
			}
		})
	}
}
