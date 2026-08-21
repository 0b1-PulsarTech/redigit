package ordering

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSeed(t *testing.T) {
	t.Parallel()

	touched := entities.Decl{Key: entities.DeclKey{File: "a.go", Name: "A"}}
	untouched := entities.Decl{Key: entities.DeclKey{File: "b.go", Name: "B"}}

	snap := entities.Snapshot{
		History: []entities.SourceTouch{{Position: 4, Touched: []string{"a.go"}}},
	}

	testCases := []struct {
		name         string
		decls        []entities.Decl
		snap         entities.Snapshot
		wantBase     entities.Position
		wantInserted bool
		decl         entities.Decl
	}{
		{
			"with history, a touched file inherits its first touch",
			[]entities.Decl{touched},
			snap,
			4,
			false,
			touched,
		},
		{
			"with history, an untouched file seeds at 0 and counts as inserted",
			[]entities.Decl{untouched},
			snap, 0, true, untouched,
		},
		{
			"with no history at all, everything seeds at 0 and nothing is inserted",
			[]entities.Decl{touched},
			entities.Snapshot{},
			0, false, touched,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base, inserted := seed(testCase.decls, testCase.snap)

			if got := base[testCase.decl.Key]; got != testCase.wantBase {
				t.Fatalf("base = %d, want %d", got, testCase.wantBase)
			}

			if got := inserted[testCase.decl.Key]; got != testCase.wantInserted {
				t.Fatalf("inserted = %v, want %v", got, testCase.wantInserted)
			}
		})
	}
}

func TestLimitFor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		snap  entities.Snapshot
		slack int
		want  entities.Position
	}{
		{"no history, limit is pure slack", entities.Snapshot{}, 8, 8},
		{
			"limit sits slack positions past the latest touch",
			entities.Snapshot{
				History: []entities.SourceTouch{{Position: 3}, {Position: 41}, {Position: 7}},
			},
			8,
			49,
		},
		{
			"zero slack pins the limit to the latest touch",
			entities.Snapshot{History: []entities.SourceTouch{{Position: 5}}},
			0,
			5,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := limitFor(testCase.snap, testCase.slack); got != testCase.want {
				t.Fatalf("limitFor() = %d, want %d", got, testCase.want)
			}
		})
	}
}
