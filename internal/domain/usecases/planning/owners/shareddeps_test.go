package owners

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestShareDependenciesPullsWhatASharedDeclarationNeeds(t *testing.T) {
	t.Parallel()

	entryA := decl("a/main.go", "A", "a")
	entryB := decl("b/main.go", "B", "b")
	converterA := decl("conv/gen.go", "ConvertA", "conv")
	converterB := decl("conv/gen.go", "ConvertB", "conv")
	converterA.Atom = "conv/gen.go"
	converterB.Atom = "conv/gen.go"
	scaffold := decl("conv/helper.go", "Seconds", "conv")
	deeper := decl("conv/deeper.go", "Scale", "conv")

	own := Assign(entities.NewGraph(
		[]entities.Decl{entryA, entryB, converterA, converterB, scaffold, deeper},
		[]entities.Edge{
			needs(entryA, converterA),
			needs(entryB, converterB),
			needs(converterA, scaffold),
			needs(scaffold, deeper),
		},
		nil,
	))

	for _, key := range []entities.DeclKey{converterA.Key, converterB.Key, scaffold.Key, deeper.Key} {
		if got := own.Owner[key]; got != reach.SharedFeature {
			t.Errorf("%s owner = %q, want the shared bucket", key, got)
		}
	}

	for _, key := range []entities.DeclKey{entryA.Key, entryB.Key} {
		if got := own.Owner[key]; got == reach.SharedFeature {
			t.Errorf("%s owner = shared, want its own feature: only providers are pulled", key)
		}
	}
}

func TestShareDependenciesLeavesAFeatureItsOwn(t *testing.T) {
	t.Parallel()

	entry := decl("a/main.go", "A", "a")
	scaffold := decl("a/helper.go", "Help", "a")

	own := Assign(entities.NewGraph(
		[]entities.Decl{entry, scaffold},
		[]entities.Edge{needs(entry, scaffold)},
		nil,
	))

	if got := own.Owner[scaffold.Key]; got != "a" {
		t.Fatalf("scaffold owner = %q, want %q", got, "a")
	}
}

func TestShareDependenciesMovesAnAtomWhole(t *testing.T) {
	t.Parallel()

	wired := decl("c/wire.go", "Wire", "c")
	gen := decl("conv/gen.go", "Gen", "conv")
	sibling := decl("conv/gen.go", "Sibling", "conv")
	gen.Atom = "conv/gen.go"
	sibling.Atom = "conv/gen.go"

	graph := entities.NewGraph(
		[]entities.Decl{wired, gen, sibling},
		[]entities.Edge{needs(wired, gen)},
		nil,
	)

	own := Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			wired.Key: reach.SharedFeature, gen.Key: "a", sibling.Key: "a",
		},
		Reach: map[entities.DeclKey]int{},
	}

	shareDependencies(graph, own)

	for _, key := range []entities.DeclKey{gen.Key, sibling.Key} {
		if got := own.Owner[key]; got != reach.SharedFeature {
			t.Errorf("%s owner = %q, want the shared bucket: an atom is not split", key, got)
		}
	}
}
