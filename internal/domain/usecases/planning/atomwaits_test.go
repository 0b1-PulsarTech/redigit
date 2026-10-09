package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestReadyDeferredWaitsForTheWholeAtom(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	later := decl("b/later.go", "Later", "b")
	setup := testDecl("a/main_test.go", "TestMain", "a")
	first := testDecl("a/first_test.go", "TestFirst", "a")
	plain := testDecl("a/plain_test.go", "TestPlain", "a")

	for _, member := range []*entities.Decl{&setup, &first} {
		member.Atom = "test-main:a"
	}

	graph := entities.NewGraph(
		[]entities.Decl{code, later, setup, first, plain},
		[]entities.Edge{needs(setup, code), needs(first, later), needs(plain, code)},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			code.Key: "a", later.Key: "b",
			setup.Key: SharedFeature, first.Key: SharedFeature, plain.Key: SharedFeature,
		},
		deferred: wiring{setup.Key: true, first.Key: true, plain.Key: true},
		roots:    wiring{},
	}

	born := map[entities.DeclKey]bool{code.Key: true}
	fresh := map[entities.DeclKey]bool{code.Key: true}

	if got, want := readyDeferred(
		graph,
		own,
		born,
		fresh,
	), []entities.DeclKey{
		plain.Key,
	}; !slices.Equal(
		got,
		want,
	) {
		t.Fatalf("readyDeferred() = %v, want %v: TestMain waits for its atom's test", got, want)
	}

	born[later.Key] = true
	fresh[later.Key] = true

	if got := readyDeferred(graph, own, born, fresh); len(got) != 3 {
		t.Fatalf("readyDeferred() = %v, want the whole atom once its test can be born", got)
	}
}
