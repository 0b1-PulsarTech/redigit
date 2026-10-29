package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestSharedPackagesSkipsWhatIsBornElsewhere(t *testing.T) {
	t.Parallel()

	plain := decl("lib/a.go", "A", "lib")
	sibling := decl("lib/b.go", "B", "lib")
	generated := decl("lib/gen.go", "G", "lib")
	generated.Generated = true
	table := decl("lib/q.go", "Q", "lib")
	query := decl("db/q.sql", "q", "db")
	owned := decl("lib/c.go", "C", "lib")
	held := decl("lib/d.go", "D", "lib")

	graph := entities.NewGraph(
		[]entities.Decl{plain, sibling, generated, table, query, owned, held},
		[]entities.Edge{edgeOf(query, table, entities.ReasonQuery)},
		nil,
	)

	own := Ownership{
		Owner: map[entities.DeclKey]FeatureID{
			plain.Key: reach.SharedFeature, sibling.Key: reach.SharedFeature, generated.Key: reach.SharedFeature,
			table.Key: reach.SharedFeature, owned.Key: "f", held.Key: reach.SharedFeature,
		},
		Deferred: reach.Wiring{held.Key: true},
	}

	got := sharedPackages(graph, own)["lib"]

	if len(got) != 2 || !slices.Contains(got, plain.Key) || !slices.Contains(got, sibling.Key) {
		t.Fatalf("sharedPackages()[lib] = %v, want only %s and %s", got, plain.Key, sibling.Key)
	}
}
