package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestPlaceTests(t *testing.T) {
	t.Parallel()

	one := decl("a/one.go", "One", "a")
	two := decl("a/two.go", "Two", "a")
	other := decl("b/other.go", "Other", "b")
	testOne := testDecl("a/one_test.go", "TestOne", "a")
	helper := testDecl("a/helper_test.go", "newHelper", "a")
	useHelper := testDecl("a/two_test.go", "TestTwo", "a")
	orphan := testDecl("c/orphan_test.go", "TestOrphan", "c")
	sameAtom := decl("a/gen.go", "GenA", "a")
	sameAtom.Atom = "gen"
	sameAtomB := decl("a/gen.go", "GenB", "a")
	sameAtomB.Atom = "gen"
	testGen := testDecl("a/gen_test.go", "TestGen", "a")

	graph := entities.NewGraph(
		[]entities.Decl{
			one, two, other, testOne, helper, useHelper, orphan, sameAtom, sameAtomB, testGen,
		},
		[]entities.Edge{
			needs(testOne, one),
			needs(useHelper, two),
			needs(useHelper, helper),
			needs(helper, two),
			needs(testGen, sameAtom),
		},
		nil,
	)

	testCases := []struct {
		name string
		keys []entities.Decl
		want []entities.Decl
	}{
		{
			"a test moves up behind the code it exercises",
			[]entities.Decl{one, two, other, testOne},
			[]entities.Decl{one, testOne, two, other},
		},
		{
			"a test waits for the helper it uses, which waits for its own provider",
			[]entities.Decl{one, two, other, helper, useHelper},
			[]entities.Decl{one, two, helper, useHelper, other},
		},
		{
			"a test with nothing in the group to follow stays where it is",
			[]entities.Decl{one, orphan, other},
			[]entities.Decl{one, orphan, other},
		},
		{
			"a test never lands inside an atom",
			[]entities.Decl{sameAtom, one, sameAtomB, other, testGen},
			[]entities.Decl{sameAtom, one, sameAtomB, testGen, other},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := placeTests(graph, keysOf(testCase.keys))
			if want := keysOf(testCase.want); !slices.Equal(got, want) {
				t.Fatalf("placeTests() = %v, want %v", got, want)
			}
		})
	}
}

func keysOf(decls []entities.Decl) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(decls))
	for _, item := range decls {
		out = append(out, item.Key)
	}

	return out
}
