package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestOwnTests(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	later := decl("b/later.go", "Later", "b")
	root := decl("cmd/main.go", "Run", "cmd")
	testCode := testDecl("a/code_test.go", "TestCode", "a")
	testLater := testDecl("b/later_test.go", "TestLater", "b")
	testRoot := testDecl("cmd/main_test.go", "TestRun", "cmd")

	graph := entities.NewGraph(
		[]entities.Decl{code, later, root, testCode, testLater, testRoot},
		[]entities.Edge{
			needs(root, code),
			needs(testCode, code),
			needs(testLater, later),
			needs(testRoot, root),
			needs(testRoot, code),
		},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			code.Key: "a", later.Key: "b", root.Key: SharedFeature,
			testCode.Key: SharedFeature, testLater.Key: SharedFeature, testRoot.Key: SharedFeature,
		},
		deferred: Wiring{
			root.Key:      true,
			testCode.Key:  true,
			testLater.Key: true,
			testRoot.Key:  true,
		},
		roots: Wiring{root.Key: true},
	}

	born := map[entities.DeclKey]bool{code.Key: true}
	fresh := map[entities.DeclKey]bool{code.Key: true}

	got := ownTests(graph, own, born, fresh)

	if want := []entities.DeclKey{testCode.Key}; !slices.Equal(got, want) {
		t.Fatalf(
			"ownTests() = %v, want only the test whose subject the feature completed: "+
				"the test of a feature still to come and the one that needs the root wait",
			got,
		)
	}
}
