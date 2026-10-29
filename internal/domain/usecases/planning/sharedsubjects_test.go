package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSharedSubjectsDropsWhatOnlyAFeatureReads(t *testing.T) {
	t.Parallel()

	test := testDecl("kernel/facet_test.go", "TestEntries", "kernel")
	entries := decl("kernel/facet.go", "Entries", "kernel")
	declares := decl("kernel/facet.go", "DeclaresType", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{test, entries, declares},
		nil,
		[]entities.Anchor{
			{Decl: test.Key, To: entries.Key, Reason: entities.ReasonTestOf},
			{Decl: test.Key, To: declares.Key, Reason: entities.ReasonTestOf},
		},
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		test.Key:     SharedFeature,
		entries.Key:  SharedFeature,
		declares.Key: "webrouters",
	}}

	got := sharedSubjects(graph, own)[test.Key]

	if want := []entities.DeclKey{entries.Key}; !slices.Equal(got, want) {
		t.Fatalf("sharedSubjects() = %v, want %v", got, want)
	}
}

func TestReadyDeferredDoesNotWaitForASubjectOnlyAFeatureReads(t *testing.T) {
	t.Parallel()

	test := testDecl("kernel/facet_test.go", "TestEntries", "kernel")
	entries := decl("kernel/facet.go", "Entries", "kernel")
	declares := decl("kernel/facet.go", "DeclaresType", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{test, entries, declares},
		[]entities.Edge{needs(test, entries)},
		[]entities.Anchor{
			{Decl: test.Key, To: entries.Key, Reason: entities.ReasonTestOf},
			{Decl: test.Key, To: declares.Key, Reason: entities.ReasonTestOf},
		},
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			test.Key: SharedFeature, entries.Key: SharedFeature, declares.Key: "webrouters",
		},
		deferred: Wiring{test.Key: true},
		roots:    Wiring{},
	}

	born := map[entities.DeclKey]bool{entries.Key: true}
	fresh := map[entities.DeclKey]bool{entries.Key: true}

	if got, want := readyDeferred(
		graph,
		own,
		born,
		fresh,
	), []entities.DeclKey{
		test.Key,
	}; !slices.Equal(
		got,
		want,
	) {
		t.Fatalf("readyDeferred() = %v, want %v", got, want)
	}
}
