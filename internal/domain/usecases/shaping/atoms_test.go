package shaping

import (
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func atomDraft(rows [][]string, loose ...string) *Draft {
	var (
		decls []entities.Decl
		plan  entities.Plan
	)

	declare := func(spec string) entities.DeclKey {
		name, atom, _ := strings.Cut(spec, "@")
		decls = append(decls, entities.Decl{Key: key(name), Lines: 10, Atom: atom})

		return key(name)
	}

	for index, row := range rows {
		commit := entities.PlannedCommit{Position: entities.Position(index)}
		for _, spec := range row {
			commit.Decls = append(commit.Decls, declare(spec))
		}

		plan.Commits = append(plan.Commits, commit)
	}

	for _, spec := range loose {
		declare(spec)
	}

	return NewDraft(entities.NewGraph(decls, nil, nil), plan)
}

func TestDraftFitsAtoms(t *testing.T) {
	t.Parallel()

	together := [][]string{{"a"}, {"q@t", "g@t"}}
	spread := [][]string{{"q@t"}, {"a"}, {"g@t"}}
	pairedSpread := [][]string{{"q@t"}, {"a"}, {"g@t", "h@t"}}
	gOnly := []string{"g"}

	testCases := []struct {
		name  string
		rows  [][]string
		loose []string
		keys  []string
		after int
		want  bool
	}{
		{"one member away from its mate", together, nil, gOnly, 0, false},
		{"every member together", together, nil, []string{"q", "g"}, 0, true},
		{"mates in other slots", spread, nil, gOnly, 1, true},
		{"a mate left in the same slot", pairedSpread, nil, gOnly, 1, false},
		{"both mates leave together", pairedSpread, nil, []string{"g", "h"}, 1, true},
		{"an unplaced mate is ignored", [][]string{{"g@t"}}, []string{"q@t"}, gOnly, NoSlot, true},
		{"a loose key has no atom", [][]string{{"a", "b"}, {"q@t"}}, nil, []string{"b"}, 0, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := atomDraft(testCase.rows, testCase.loose...)

			keys := make([]entities.DeclKey, 0, len(testCase.keys))
			for _, name := range testCase.keys {
				keys = append(keys, key(name))
			}

			if got := draft.Fits(keys, testCase.after); got != testCase.want {
				t.Fatalf("Fits(%v) = %v, want %v", testCase.keys, got, testCase.want)
			}
		})
	}
}

func TestDraftAtomSplits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		rows  [][]string
		loose []string
		want  []entities.Edge
	}{
		{"no atoms", [][]string{{"a"}, {"b"}}, nil, nil},
		{"an atom in one slot", [][]string{{"q@t", "g@t"}, {"a"}}, nil, nil},
		{
			"an atom over two slots",
			[][]string{{"q@t"}, {"a"}, {"g@t"}},
			nil,
			[]entities.Edge{{Dependent: key("q"), Provider: key("g"), Reason: ReasonAtomSplit}},
		},
		{"an unplaced mate is no split", [][]string{{"q@t"}}, []string{"g@t"}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := atomDraft(testCase.rows, testCase.loose...).AtomSplits()
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("AtomSplits() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDraftReseatAtoms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		rows  [][]string
		id    int
		after int
		want  bool
	}{
		{"a lone member of a split atom", [][]string{{"q@t"}, {"a"}, {"g@t"}}, 2, NoSlot, true},
		{"a whole slot of an atom", [][]string{{"q@t", "g@t"}, {"a"}}, 0, 1, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			draft := atomDraft(testCase.rows)

			_, ok := draft.Reseat(testCase.id, testCase.after)
			if ok != testCase.want {
				t.Fatalf("Reseat(%d) = %v, want %v", testCase.id, ok, testCase.want)
			}
		})
	}
}
