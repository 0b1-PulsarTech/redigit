package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestAssign(t *testing.T) {
	t.Parallel()

	graph, _, named := twoFeatures()
	own := assign(graph)

	testCases := []struct {
		name      string
		decl      entities.Decl
		wantOwner FeatureID
		wantReach int
	}{
		{"reached by both features, so owned by neither", named["kernel"], SharedFeature, 2},
		{
			"a use case reached only through its handler travels with that slice",
			named["leadUse"],
			"lead/api",
			1,
		},
		{"so does the repository below it", named["leadRepo"], "lead/api", 1},
		{"an entry point owns its own feature", named["leadAPI"], "lead/api", 1},
		{"the other slice is named after its own tip", named["billUse"], "bill/api", 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := own.owner[testCase.decl.Key]; got != testCase.wantOwner {
				t.Fatalf("owner = %q, want %q", got, testCase.wantOwner)
			}

			if got := own.reach[testCase.decl.Key]; got != testCase.wantReach {
				t.Fatalf("reach = %d, want %d", got, testCase.wantReach)
			}
		})
	}
}

func TestAssignKeepsUnreachedDeclarations(t *testing.T) {
	t.Parallel()

	entry := decl("a/main.go", "Main", "a")
	// A pair that only consumes each other has no tip, so no entry-point walk
	// ever reaches it, even though both are consumed.
	loopA := decl("z/one.go", "One", "z")
	loopB := decl("z/two.go", "Two", "z")

	own := assign(entities.NewGraph(
		[]entities.Decl{entry, loopA, loopB},
		[]entities.Edge{needs(loopA, loopB), needs(loopB, loopA)},
		nil,
	))

	for _, key := range []entities.DeclKey{loopA.Key, loopB.Key} {
		// Dropping it would leave a hole in the final tree that the invariant
		// only catches once every explanation for it is gone.
		if _, found := own.reach[key]; !found {
			t.Fatalf("%s was dropped instead of marked shared", key)
		}

		if got := own.owner[key]; got != SharedFeature {
			t.Fatalf("%s owner = %q, want the shared bucket", key, got)
		}
	}
}

// TestAssignHasNoSharingThreshold pins that reach is not a tunable number.
// Reached by one feature means owned by it; reached by more means owned by
// none. There is no third answer to expose as configuration.
func TestAssignHasNoSharingThreshold(t *testing.T) {
	t.Parallel()

	graph, _, named := twoFeatures()

	if got := assign(graph).owner[named["kernel"].Key]; got != SharedFeature {
		t.Fatalf("owner = %q, want the shared bucket", got)
	}
}

func TestAssignMergesAnAtomIntoOneOwner(t *testing.T) {
	t.Parallel()

	newAtom := func() (typeA, typeB entities.Decl) {
		typeA = decl("dbgen/models.go", "TypeA", "dbgen")
		typeA.Atom = "dbgen/models.go"
		typeB = decl("dbgen/models.go", "TypeB", "dbgen")
		typeB.Atom = "dbgen/models.go"

		return typeA, typeB
	}

	t.Run("read by two features through either half, so neither owns it alone", func(t *testing.T) {
		t.Parallel()

		typeA, typeB := newAtom()
		readerA := decl("a/h.go", "A", "a")
		readerB := decl("b/h.go", "B", "b")

		own := assign(entities.NewGraph(
			[]entities.Decl{typeA, typeB, readerA, readerB},
			[]entities.Edge{needs(readerA, typeA), needs(readerB, typeB)},
			nil,
		))

		if got := own.owner[typeA.Key]; got != SharedFeature {
			t.Fatalf("TypeA owner = %q, want the shared bucket", got)
		}

		if got := own.owner[typeB.Key]; got != SharedFeature {
			t.Fatalf("TypeB owner = %q, want the shared bucket", got)
		}
	})

	t.Run("read by one feature through either half, so that feature owns both", func(t *testing.T) {
		t.Parallel()

		typeA, typeB := newAtom()
		readerA := decl("a/one.go", "One", "a")
		readerA2 := decl("a/two.go", "Two", "a")

		own := assign(entities.NewGraph(
			[]entities.Decl{typeA, typeB, readerA, readerA2},
			[]entities.Edge{needs(readerA, typeA), needs(readerA2, typeB)},
			nil,
		))

		if got := own.owner[typeA.Key]; got != "a" {
			t.Fatalf("TypeA owner = %q, want %q", got, "a")
		}

		if got := own.owner[typeB.Key]; got != "a" {
			t.Fatalf("TypeB owner = %q, want %q", got, "a")
		}
	})
}

func TestAssignOrdersFeaturesDeterministically(t *testing.T) {
	t.Parallel()

	graph, _, _ := twoFeatures()

	first := assign(graph).order
	second := assign(graph).order

	if !slices.Equal(first, second) {
		t.Fatalf("feature order is not stable: %v then %v", first, second)
	}

	if !slices.IsSorted(first) {
		t.Fatalf("feature order %v is not sorted", first)
	}
}
