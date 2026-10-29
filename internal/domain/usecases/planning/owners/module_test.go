package owners

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func inModule(dependent, manifest entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  manifest.Key,
		Reason:    entities.ReasonModule,
	}
}

func TestAssignSharesALibraryWhole(t *testing.T) {
	t.Parallel()

	libMod := entities.Decl{
		Key: entities.DeclKey{File: "libs/k/go.mod", Kind: entities.KindOther, Name: "module"},
	}
	appMod := entities.Decl{
		Key: entities.DeclKey{File: "app/go.mod", Kind: entities.KindOther, Name: "module"},
	}
	first := decl("libs/k/a.go", "First", "libs/k")
	second := decl("libs/k/b.go", "Second", "libs/k")
	handler := decl("app/api/h.go", "Handle", "app/api")

	graph := entities.NewGraph(
		[]entities.Decl{libMod, appMod, first, second, handler},
		[]entities.Edge{
			inModule(first, libMod),
			inModule(second, libMod),
			inModule(handler, appMod),
			needs(handler, first),
		},
		nil,
	)
	own := Assign(graph)

	for _, member := range []entities.Decl{first, second} {
		if got := own.Owner[member.Key]; got != reach.SharedFeature {
			t.Fatalf(
				"%s owner = %q, want the shared bucket: a library has no capability",
				member.Key,
				got,
			)
		}
	}

	if got := own.Owner[handler.Key]; got != "app/api" {
		t.Fatalf("handler owner = %q, want its own feature", got)
	}
}

func TestDeferLibraryEntries(t *testing.T) {
	t.Parallel()

	free := decl("lib/a.go", "A", "lib")
	owned := decl("lib/b.go", "B", "lib")
	own := Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			free.Key:  reach.SharedFeature,
			owned.Key: "f",
		},
		Deferred: reach.Wiring{},
		Loose:    reach.Wiring{},
	}

	deferLibraryEntries(own, []entities.Decl{free, owned})

	if !own.Deferred[free.Key] || !own.Loose[free.Key] {
		t.Fatalf("the unowned entry %s is not deferred as loose", free.Key)
	}

	if own.Deferred[owned.Key] {
		t.Fatalf("the owned entry %s was deferred", owned.Key)
	}
}

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
		Owner: map[entities.DeclKey]reach.FeatureID{
			plain.Key:     reach.SharedFeature,
			sibling.Key:   reach.SharedFeature,
			generated.Key: reach.SharedFeature,
			table.Key:     reach.SharedFeature,
			owned.Key:     "f",
			held.Key:      reach.SharedFeature,
		},
		Deferred: reach.Wiring{held.Key: true},
	}

	got := sharedPackages(graph, own)["lib"]

	if len(got) != 2 || !slices.Contains(got, plain.Key) || !slices.Contains(got, sibling.Key) {
		t.Fatalf("sharedPackages()[lib] = %v, want only %s and %s", got, plain.Key, sibling.Key)
	}
}
