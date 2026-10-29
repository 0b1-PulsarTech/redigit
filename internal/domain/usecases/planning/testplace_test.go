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
	support := testDecl("a/support_test.go", "newSupport", "a")
	useSupport := testDecl("a/two_test.go", "TestTwo", "a")
	orphan := testDecl("c/orphan_test.go", "TestOrphan", "c")
	sameAtom := decl("a/gen.go", "GenA", "a")
	sameAtom.Atom = "gen"
	sameAtomB := decl("a/gen.go", "GenB", "a")
	sameAtomB.Atom = "gen"
	testGen := testDecl("a/gen_test.go", "TestGen", "a")
	mockOne := testDecl("a/store_mock_test.go", "MockOne", "a")
	mockTwo := testDecl("a/store_mock_test.go", "MockTwo", "a")
	mockOne.Atom, mockTwo.Atom = "store-mock", "store-mock"
	useMock := testDecl("a/use_mock_test.go", "TestUseMock", "a")

	graph := entities.NewGraph(
		[]entities.Decl{
			one, two, other, testOne, support, useSupport, orphan, sameAtom, sameAtomB, testGen,
			mockOne, mockTwo, useMock,
		},
		[]entities.Edge{
			needs(testOne, one),
			needs(useSupport, two),
			needs(useSupport, support),
			needs(support, two),
			needs(testGen, sameAtom),
			needs(mockOne, one),
			needs(mockTwo, two),
			needs(useMock, mockOne),
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
			"a test waits for the support it uses, which waits for its own provider",
			[]entities.Decl{one, two, other, support, useSupport},
			[]entities.Decl{one, two, support, useSupport, other},
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
		{
			"a generated test file moves as one, behind the last thing any member needs",
			[]entities.Decl{one, two, other, mockOne, useMock, mockTwo},
			[]entities.Decl{one, two, mockOne, mockTwo, other, useMock},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := PlaceTests(graph, keysOf(testCase.keys))
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

func TestGatherTestAtoms(t *testing.T) {
	t.Parallel()

	setup := testDecl("a/main_test.go", "TestMain", "a")
	first := testDecl("a/first_test.go", "TestFirst", "a")
	middle := testDecl("a/middle_test.go", "TestMiddle", "a")
	user := testDecl("a/user_test.go", "TestUser", "a")
	prod := decl("a/code.go", "Code", "a")

	for _, member := range []*entities.Decl{&setup, &first} {
		member.Atom = "test-main:a"
	}

	graph := entities.NewGraph(
		[]entities.Decl{setup, first, middle, user, prod},
		[]entities.Edge{needs(user, setup)},
		nil,
	)

	testCases := []struct {
		name string
		keys []entities.Decl
		want []entities.Decl
	}{
		{
			"the early member moves up to the last one",
			[]entities.Decl{setup, middle, prod, first},
			[]entities.Decl{middle, prod, setup, first},
		},
		{
			"a member something between needs stays",
			[]entities.Decl{setup, user, first},
			[]entities.Decl{setup, user, first},
		},
		{
			"an adjacent atom is untouched",
			[]entities.Decl{middle, setup, first},
			[]entities.Decl{middle, setup, first},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := gatherTestAtoms(graph, keysOf(testCase.keys))
			if want := keysOf(testCase.want); !slices.Equal(got, want) {
				t.Fatalf("gatherTestAtoms() = %v, want %v", got, want)
			}
		})
	}
}
