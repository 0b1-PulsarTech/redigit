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

// A registration waits for the shared code a feature also uses: bringing it
// early would pull that code's whole closure into the first registration.
func TestReadyDeferredWaitsForSharedCodeAFeatureAlsoUses(t *testing.T) {
	t.Parallel()

	registration := decl("wire/wire.go", "register", "wire")
	feature := decl("a/handler.go", "Handle", "a")
	both := decl("kernel/both.go", "Both", "kernel")
	onlyWiring := decl("kernel/only.go", "Only", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{registration, feature, both, onlyWiring},
		[]entities.Edge{
			needs(registration, both), needs(registration, onlyWiring), needs(feature, both),
		},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			registration.Key: SharedFeature, feature.Key: "a",
			both.Key: SharedFeature, onlyWiring.Key: SharedFeature,
		},
		reach: map[entities.DeclKey]int{
			both.Key: 2, onlyWiring.Key: 1,
		},
		deferred: wiring{registration.Key: true},
		roots:    wiring{registration.Key: true},
	}

	fresh := map[entities.DeclKey]bool{onlyWiring.Key: true}

	if got := readyDeferred(graph, own, map[entities.DeclKey]bool{}, fresh); len(got) != 0 {
		t.Fatalf("readyDeferred() = %v, want none: Both is born with the feature that uses it", got)
	}

	born := map[entities.DeclKey]bool{both.Key: true}

	if got := readyDeferred(graph, own, born, fresh); len(got) != 1 {
		t.Fatalf("readyDeferred() = %v, want the registration once Both is born", got)
	}
}
