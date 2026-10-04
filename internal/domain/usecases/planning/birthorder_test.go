package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSortForBirthKeepsAnAtomContiguousWithinAPosition(t *testing.T) {
	t.Parallel()

	// Both halves of the atom and two loose declarations tie at one position.
	// The ready queue alone yields first, loose, second, loose.
	first := decl("gen/a.sql.go", "First", "gen")
	first.Atom = "gen"
	second := decl("gen/a.sql.go", "Second", "gen")
	second.Atom = "gen"
	looseA := decl("lib/a.go", "A", "lib")
	looseB := decl("lib/b.go", "B", "lib")

	graph := entities.NewGraph(
		[]entities.Decl{first, looseA, second, looseB},
		nil,
		nil,
	)

	got := sortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, looseA.Key, second.Key, looseB.Key},
		nil,
	)

	at := map[entities.DeclKey]int{}
	for index, key := range got {
		at[key] = index
	}

	if gap := at[second.Key] - at[first.Key]; gap != 1 {
		t.Fatalf("atom members are %d apart in %v, want adjacent", gap, got)
	}
}

func TestSortForBirthWaitsForWhatAnAtomMemberNeeds(t *testing.T) {
	t.Parallel()

	// The second member needs a loose declaration at the same position, so the
	// whole atom comes after it.
	first := decl("gen/a.sql.go", "First", "gen")
	first.Atom = "gen"
	second := decl("gen/a.sql.go", "Second", "gen")
	second.Atom = "gen"
	helper := decl("lib/a.go", "Helper", "lib")

	graph := entities.NewGraph(
		[]entities.Decl{first, second, helper},
		[]entities.Edge{needs(second, helper)},
		nil,
	)

	got := sortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, second.Key, helper.Key},
		nil,
	)

	if len(got) != 3 || got[0] != helper.Key {
		t.Fatalf("order = %v, want the helper before the atom", got)
	}
}
