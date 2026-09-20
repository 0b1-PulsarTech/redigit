package planning

import (
	"cmp"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestUnbornProvidersClosesTransitivelyOverTheSharedKernel(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	event := decl("kernel/event.go", "Event", "kernel")
	reason := decl("kernel/reason.go", "Reason", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{member, event, reason},
		[]entities.Edge{needs(member, event), needs(event, reason)},
		nil,
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key: "a",
		event.Key:  SharedFeature,
		reason.Key: SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	want := []entities.DeclKey{event.Key, reason.Key}
	slices.SortFunc(
		want,
		func(a, b entities.DeclKey) int { return cmp.Compare(a.String(), b.String()) },
	)

	if !slices.Equal(got, want) {
		t.Fatalf("unbornProviders() = %v, want %v", got, want)
	}
}

func TestUnbornProvidersPullsAnAtomSiblingAlong(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	typeA := decl("dbgen/models.go", "TypeA", "dbgen")
	typeA.Atom = "dbgen/models.go"
	typeB := decl("dbgen/models.go", "TypeB", "dbgen")
	typeB.Atom = "dbgen/models.go"

	graph := entities.NewGraph(
		[]entities.Decl{member, typeA, typeB},
		[]entities.Edge{needs(member, typeA)},
		nil,
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key: "a",
		typeA.Key:  SharedFeature,
		typeB.Key:  SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	if !slices.Contains(got, typeB.Key) {
		t.Fatalf(
			"unbornProviders() = %v, want it to also carry the atom sibling %s",
			got,
			typeB.Key,
		)
	}
}

func TestUnbornProvidersPullsAnAnchoredTestAlong(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	event := decl("kernel/event.go", "Event", "kernel")
	eventTest := testDecl("kernel/event_test.go", "TestEvent", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{member, event, eventTest},
		[]entities.Edge{needs(member, event)},
		[]entities.Anchor{{Decl: eventTest.Key, To: event.Key, Reason: entities.ReasonTestOf}},
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key:    "a",
		event.Key:     SharedFeature,
		eventTest.Key: SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	if !slices.Contains(got, eventTest.Key) {
		t.Fatalf(
			"unbornProviders() = %v, want it to also carry the anchored test %s",
			got,
			eventTest.Key,
		)
	}
}

func TestUnbornProvidersLeavesAnAnchoredTestBehindWhenItsOwnNeedIsNotReady(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	event := decl("kernel/event.go", "Event", "kernel")
	late := decl("b/late.go", "Late", "b")
	eventTest := testDecl("kernel/event_test.go", "TestEvent", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{member, event, late, eventTest},
		[]entities.Edge{needs(member, event), needs(eventTest, late)},
		[]entities.Anchor{{Decl: eventTest.Key, To: event.Key, Reason: entities.ReasonTestOf}},
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key:    "a",
		event.Key:     SharedFeature,
		late.Key:      "b",
		eventTest.Key: SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	if slices.Contains(got, eventTest.Key) {
		t.Fatalf(
			"unbornProviders() = %v, must not carry %s ahead of the feature b need it has not satisfied",
			got,
			eventTest.Key,
		)
	}
}

func TestUnbornProvidersSkipsWhatIsAlreadyBorn(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	event := decl("kernel/event.go", "Event", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{member, event},
		[]entities.Edge{needs(member, event)},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{member.Key: "a", event.Key: SharedFeature},
	}
	born := map[entities.DeclKey]bool{event.Key: true}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, born)

	if len(got) != 0 {
		t.Fatalf("unbornProviders() = %v, want none: everything it needs is already born", got)
	}
}
