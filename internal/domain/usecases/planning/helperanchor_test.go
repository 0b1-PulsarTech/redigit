package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestAnchorHelpersKeepsAHelperOnlyWithAChosenReader(t *testing.T) {
	t.Parallel()

	prod := decl("a/code.go", "Code", "a")
	chosenTest := testDecl("a/code_test.go", "TestCode", "a")
	otherTest := testDecl("a/other_test.go", "TestOther", "a")
	read := decl("a/helpers_test.go", "newFake", "a")
	unread := decl("a/helpers_test.go", "newSpy", "a")
	nested := decl("a/helpers_test.go", "fakeClock", "a")

	graph := entities.NewGraph(
		[]entities.Decl{prod, chosenTest, otherTest, read, unread, nested},
		[]entities.Edge{
			needs(chosenTest, read), needs(read, nested), needs(otherTest, unread),
		},
		nil,
	)
	own := Ownership{Owner: map[entities.DeclKey]FeatureID{}}
	pool := map[entities.DeclKey]bool{
		prod.Key: true, chosenTest.Key: true, read.Key: true, unread.Key: true, nested.Key: true,
	}
	chosen := map[entities.DeclKey]bool{
		prod.Key: true, chosenTest.Key: true, read.Key: true, unread.Key: true,
	}

	got := anchorHelpers(graph, own, chosen, pool)

	for _, want := range []entities.Decl{prod, chosenTest, read, nested} {
		if !got[want.Key] {
			t.Errorf("anchorHelpers() dropped %v, want it kept", want.Key)
		}
	}

	if got[unread.Key] {
		t.Errorf("anchorHelpers() kept %v, want it left for a reader", unread.Key)
	}
}
