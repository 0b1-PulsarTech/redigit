package ordering

import (
	"maps"
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

func TestSeedFloored(t *testing.T) {
	t.Parallel()

	first := entities.Decl{Key: entities.DeclKey{File: "a.go", Name: "A"}}
	second := entities.Decl{Key: entities.DeclKey{File: "a.go", Name: "B"}}
	third := entities.Decl{Key: entities.DeclKey{File: "b.go", Name: "C"}}
	decls := []entities.Decl{first, second, third}

	snap := entities.Snapshot{
		History: []entities.SourceTouch{{Position: 4, Touched: []string{"a.go"}}},
	}

	testCases := []struct {
		name         string
		floor        map[entities.DeclKey]entities.Position
		wantBase     map[entities.DeclKey]entities.Position
		wantInserted map[entities.DeclKey]bool
	}{
		{
			"a nil floor is the file's first touch for everything",
			nil,
			map[entities.DeclKey]entities.Position{first.Key: 4, second.Key: 4, third.Key: 0},
			map[entities.DeclKey]bool{third.Key: true},
		},
		{
			"a floor replaces the first touch per declaration",
			map[entities.DeclKey]entities.Position{first.Key: 1, second.Key: 9, third.Key: 2},
			map[entities.DeclKey]entities.Position{first.Key: 1, second.Key: 9, third.Key: 2},
			map[entities.DeclKey]bool{},
		},
		{
			"a floor of zero is a real position, not an absent one",
			map[entities.DeclKey]entities.Position{first.Key: 0, second.Key: 2, third.Key: 0},
			map[entities.DeclKey]entities.Position{first.Key: 0, second.Key: 2, third.Key: 0},
			map[entities.DeclKey]bool{},
		},
		{
			"a key missing from the floor keeps its file's first touch",
			map[entities.DeclKey]entities.Position{first.Key: 3, third.Key: 1},
			map[entities.DeclKey]entities.Position{first.Key: 3, second.Key: 4, third.Key: 1},
			map[entities.DeclKey]bool{},
		},
		{
			"a key missing from the floor whose file has no history is inserted at 0",
			map[entities.DeclKey]entities.Position{first.Key: 3, second.Key: 3},
			map[entities.DeclKey]entities.Position{first.Key: 3, second.Key: 3, third.Key: 0},
			map[entities.DeclKey]bool{third.Key: true},
		},
		{
			"an empty floor is the first touch for everything",
			map[entities.DeclKey]entities.Position{},
			map[entities.DeclKey]entities.Position{first.Key: 4, second.Key: 4, third.Key: 0},
			map[entities.DeclKey]bool{third.Key: true},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base, inserted := seedFloored(decls, snap, testCase.floor)

			if !maps.Equal(base, testCase.wantBase) {
				t.Fatalf("base = %v, want %v", base, testCase.wantBase)
			}

			for _, decl := range decls {
				if got, want := inserted[decl.Key], testCase.wantInserted[decl.Key]; got != want {
					t.Fatalf("inserted[%s] = %v, want %v", decl.Key, got, want)
				}
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
