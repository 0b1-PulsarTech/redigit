package shaping

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func subjects(draft *Draft) []string {
	out := make([]string, 0, len(draft.Slots()))
	for _, slot := range draft.Slots() {
		out = append(out, slot.Commit.Subject)
	}

	return out
}

func TestDraftRetitle(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		id   int
		want []string
	}{
		{name: "changes the row of that id", id: 1, want: []string{"row a", "new", "row c"}},
		{name: "ignores an unknown id", id: 9, want: []string{"row a", "row b", "row c"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := buildDraft([][]string{{"a"}, {"b"}, {"c"}}, nil)
			draft.Retitle(testCase.id, "new")

			if got := subjects(draft); !slices.Equal(got, testCase.want) {
				t.Fatalf("subjects = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDraftSpawn(t *testing.T) {
	t.Parallel()

	rows := [][]string{{"a"}, {"b", "c"}, {"d"}}
	specs := []edgeSpec{
		{"c", "a", entities.ReasonReference},
		{"d", "c", entities.ReasonReference},
	}
	same := []string{"row a", "row b", "row c"}

	testCases := []struct {
		name        string
		keys        []string
		after       int
		wantOK      bool
		wantMoves   int
		wantSubject []string
	}{
		{
			"opens a commit after its provider",
			[]string{"c"},
			0, true, 1,
			[]string{"row a", "new", "row b", "row c"},
		},
		{
			"opens it at the front",
			[]string{"b"},
			-1, true, 1,
			[]string{"new", "row a", "row b", "row c"},
		},
		{"refused when a consumer would precede", []string{"c"}, 2, false, 0, same},
		{"refused for an unplaced key", []string{"ghost"}, 0, false, 0, same},
		{"refused for no keys", nil, 0, false, 0, same},
		{"refused after an unknown slot", []string{"b"}, 9, false, 0, same},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := buildDraft(rows, specs)

			keys := make([]entities.DeclKey, 0, len(testCase.keys))
			for _, name := range testCase.keys {
				keys = append(keys, key(name))
			}

			moves, ok := draft.Spawn(testCase.after, entities.PlannedCommit{Subject: "new"}, keys)
			if ok != testCase.wantOK || len(moves) != testCase.wantMoves {
				t.Fatalf(
					"Spawn() = %d moves, %v, want %d, %v",
					len(moves),
					ok,
					testCase.wantMoves,
					testCase.wantOK,
				)
			}

			if got := subjects(draft); !slices.Equal(got, testCase.wantSubject) {
				t.Fatalf("subjects = %v, want %v", got, testCase.wantSubject)
			}

			if len(draft.Violations()) != 0 {
				t.Fatalf("Spawn() left violations: %v", draft.Violations())
			}
		})
	}
}

func TestDraftMerge(t *testing.T) {
	t.Parallel()

	rows := [][]string{{"a"}, {"b"}, {"c"}}
	specs := []edgeSpec{{"a", "c", entities.ReasonReference}}

	testCases := []struct {
		name       string
		from, into int
		wantOK     bool
		wantSlots  int
		wantViol   int
		wantOfA    int
	}{
		{"merges the dependent forward", 0, 2, true, 2, 0, 2},
		{"merges the provider back", 2, 0, true, 2, 0, 0},
		{"merge into itself is refused", 1, 1, false, 3, 1, 0},
		{"merge from an unknown slot is refused", 7, 1, false, 3, 1, 0},
		{"merge into an unknown slot is refused", 1, 7, false, 3, 1, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := buildDraft(rows, specs)

			_, ok := draft.Merge(testCase.from, testCase.into)
			if ok != testCase.wantOK {
				t.Fatalf("Merge() ok = %v, want %v", ok, testCase.wantOK)
			}

			if got := len(draft.Slots()); got != testCase.wantSlots {
				t.Fatalf("slots = %d, want %d", got, testCase.wantSlots)
			}

			if got := len(draft.Violations()); got != testCase.wantViol {
				t.Fatalf("violations = %d, want %d", got, testCase.wantViol)
			}

			if id, _ := draft.Of(key("a")); id != testCase.wantOfA {
				t.Fatalf("Of(a) = %d, want %d", id, testCase.wantOfA)
			}
		})
	}
}
