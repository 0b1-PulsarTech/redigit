package reach

import (
	"cmp"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestAtomGroups(t *testing.T) {
	t.Parallel()

	typeA := decl("dbgen/models.go", "TypeA", "dbgen")
	typeA.Atom = "dbgen/models.go"
	typeB := decl("dbgen/models.go", "TypeB", "dbgen")
	typeB.Atom = "dbgen/models.go"
	loose := decl("dbgen/other.go", "Loose", "dbgen")

	graph := entities.NewGraph([]entities.Decl{typeA, typeB, loose}, nil, nil)

	groups := AtomGroups(graph)

	if _, found := groups[""]; found {
		t.Fatalf("groups carries an entry for the empty atom, want none")
	}

	got := groups["dbgen/models.go"]
	slices.SortFunc(got, func(a, b entities.DeclKey) int { return cmp.Compare(a.Name, b.Name) })

	want := []entities.DeclKey{typeA.Key, typeB.Key}
	if !slices.Equal(got, want) {
		t.Fatalf("atomGroups()[%q] = %v, want %v", "dbgen/models.go", got, want)
	}
}
