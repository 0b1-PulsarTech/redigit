package birth

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSortForBirthKeepsAnAtomContiguousWithinAPosition(t *testing.T) {
	t.Parallel()

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

	got := SortForBirth(
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

	first := decl("gen/a.sql.go", "First", "gen")
	first.Atom = "gen"
	second := decl("gen/a.sql.go", "Second", "gen")
	second.Atom = "gen"
	scaffold := decl("lib/a.go", "Helper", "lib")

	graph := entities.NewGraph(
		[]entities.Decl{first, second, scaffold},
		[]entities.Edge{needs(second, scaffold)},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, second.Key, scaffold.Key},
		nil,
	)

	if len(got) != 3 || got[0] != scaffold.Key {
		t.Fatalf("order = %v, want the helper before the atom", got)
	}
}

func TestSortForBirthIgnoresAGeneratedByEdgeThatClosesACycle(t *testing.T) {
	t.Parallel()

	scaffold := testDecl("pkg/a_test.go", "newUseCaseWithMocks", "pkg")
	mock := testDecl("pkg/z_mock_test.go", "NewMockStore", "pkg")

	graph := entities.NewGraph(
		[]entities.Decl{scaffold, mock},
		[]entities.Edge{
			needs(scaffold, mock),
			{Dependent: mock.Key, Provider: scaffold.Key, Reason: entities.ReasonGeneratedBy},
		},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{scaffold.Key, mock.Key},
		nil,
	)

	if len(got) != 2 || got[0] != mock.Key {
		t.Fatalf("order = %v, want the mock before the helper that reads it", got)
	}
}

func TestSortForBirthKeepsAHardCycleInArrivalOrder(t *testing.T) {
	t.Parallel()

	first := decl("pkg/a.go", "A", "pkg")
	second := decl("pkg/b.go", "B", "pkg")

	graph := entities.NewGraph(
		[]entities.Decl{first, second},
		[]entities.Edge{needs(first, second), needs(second, first)},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, second.Key},
		nil,
	)

	if len(got) != 2 || got[0] != first.Key || got[1] != second.Key {
		t.Fatalf("order = %v, want the cycle kept in arrival order", got)
	}
}

func TestSortForBirthPutsWhatWaitsOnAHardCycleAfterIt(t *testing.T) {
	t.Parallel()

	dependent := decl("pkg/0.go", "Dependent", "pkg")
	first := decl("pkg/a.go", "A", "pkg")
	second := decl("pkg/b.go", "B", "pkg")

	graph := entities.NewGraph(
		[]entities.Decl{dependent, first, second},
		[]entities.Edge{needs(first, second), needs(second, first), needs(dependent, first)},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{dependent.Key, first.Key, second.Key},
		nil,
	)

	if len(got) != 3 || got[2] != dependent.Key {
		t.Fatalf("order = %v, want the dependent after the cycle it needs", got)
	}
}
