package planning

import (
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
	own := assign(graph)

	for _, member := range []entities.Decl{first, second} {
		if got := own.owner[member.Key]; got != reach.SharedFeature {
			t.Fatalf(
				"%s owner = %q, want the shared bucket: a library has no capability",
				member.Key,
				got,
			)
		}
	}

	if got := own.owner[handler.Key]; got != "app/api" {
		t.Fatalf("handler owner = %q, want its own feature", got)
	}
}

func TestDeferLibraryEntries(t *testing.T) {
	t.Parallel()

	free := decl("lib/a.go", "A", "lib")
	owned := decl("lib/b.go", "B", "lib")
	own := ownership{
		owner:    map[entities.DeclKey]FeatureID{free.Key: reach.SharedFeature, owned.Key: "f"},
		deferred: reach.Wiring{},
		loose:    reach.Wiring{},
	}

	deferLibraryEntries(own, []entities.Decl{free, owned})

	if !own.deferred[free.Key] || !own.loose[free.Key] {
		t.Fatalf("the unowned entry %s is not deferred as loose", free.Key)
	}

	if own.deferred[owned.Key] {
		t.Fatalf("the owned entry %s was deferred", owned.Key)
	}
}
