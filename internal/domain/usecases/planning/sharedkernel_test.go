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

// A library package is born with its first consumer, not whole: pulling every
// package of a library when one is read put agentichatter at commit 41 and its
// first importer at 787.
func TestUnbornProvidersBringsOnlyTheLibraryDeclarationsAConsumerNeeds(t *testing.T) {
	t.Parallel()

	member := decl("app/main.go", "Main", "app")
	manifest := decl("libs/kit/go.mod", "require", "kit")
	used := decl("libs/kit/used/used.go", "Used", "kit/used")
	unused := decl("libs/kit/other/other.go", "Other", "kit/other")

	graph := entities.NewGraph(
		[]entities.Decl{member, manifest, used, unused},
		[]entities.Edge{needs(member, manifest), needs(member, used)},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			member.Key: "app", manifest.Key: SharedFeature,
			used.Key: SharedFeature, unused.Key: SharedFeature,
		},
	}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	if len(got) != 2 || !slices.Contains(got, manifest.Key) || !slices.Contains(got, used.Key) {
		t.Fatalf("unbornProviders() = %v, want only the manifest and the package read", got)
	}
}

func TestUnbornProvidersLeavesATestBehindWhoseSharedSupportNeedsAFeature(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	event := decl("kernel/event.go", "Event", "kernel")
	late := decl("b/late.go", "Late", "b")
	support := testDecl("kernel/support_test.go", "runEvent", "kernel")
	eventTest := testDecl("kernel/event_test.go", "TestEvent", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{member, event, late, support, eventTest},
		[]entities.Edge{
			needs(member, event),
			needs(eventTest, support),
			needs(support, late),
		},
		[]entities.Anchor{{Decl: eventTest.Key, To: event.Key, Reason: entities.ReasonTestOf}},
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key:    "a",
		event.Key:     SharedFeature,
		late.Key:      "b",
		support.Key:   SharedFeature,
		eventTest.Key: SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	if slices.Contains(got, eventTest.Key) || slices.Contains(got, support.Key) {
		t.Fatalf(
			"unbornProviders() = %v, must leave the test and its support behind: the support needs %s",
			got,
			late.Key,
		)
	}
}

func TestUnbornProvidersIgnoresAGeneratedByEdgeIntoATest(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	mock := decl("kernel/store_mock.go", "NewMockStore", "kernel")
	support := testDecl("kernel/store_test.go", "newStore", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{member, mock, support},
		[]entities.Edge{
			needs(member, mock),
			needs(support, mock),
			{Dependent: mock.Key, Provider: support.Key, Reason: entities.ReasonGeneratedBy},
		},
		nil,
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key:  "a",
		mock.Key:    SharedFeature,
		support.Key: SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	if slices.Contains(got, support.Key) {
		t.Fatalf(
			"unbornProviders() = %v, must not follow the generator directive into %s",
			got,
			support.Key,
		)
	}
}

func TestUnbornProvidersBringsWhatAWalkedStatementReadsFromAFeature(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	stmt := decl("w/server.go", "NewServer#5", "w")
	stmt.Key.Kind = entities.KindStmt
	health := decl("w/server.go", "health", "w")
	status := decl("w/server.go", "statusKey", "w")

	graph := entities.NewGraph(
		[]entities.Decl{member, stmt, health, status},
		[]entities.Edge{needs(member, stmt), needs(stmt, health), needs(health, status)},
		nil,
	)

	own := ownership{
		owner: map[entities.DeclKey]FeatureID{
			member.Key: "a",
			stmt.Key:   SharedFeature,
			health.Key: "w",
			status.Key: "w",
		},
		roots: Wiring{stmt.Key: true},
	}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	for _, want := range []entities.DeclKey{stmt.Key, health.Key, status.Key} {
		if !slices.Contains(got, want) {
			t.Fatalf("unbornProviders() = %v, want it to carry %s", got, want)
		}
	}
}

func TestUnbornProvidersDoesNotFollowAnInterfaceATypeOnlyFills(t *testing.T) {
	t.Parallel()

	member := decl("a/handler.go", "Handle", "a")
	impl := decl("kernel/impl.go", "Impl", "kernel")
	method := decl("kernel/impl.go", "Impl.Do", "kernel")
	iface := decl("other/iface.go", "Iface", "other")
	iface.IsInterface = true

	graph := entities.NewGraph(
		[]entities.Decl{member, impl, method, iface},
		[]entities.Edge{
			needs(member, impl),
			{Dependent: member.Key, Provider: method.Key, Reason: entities.ReasonImplements},
			{Dependent: impl.Key, Provider: iface.Key, Reason: entities.ReasonImplements},
		},
		nil,
	)

	own := ownership{owner: map[entities.DeclKey]FeatureID{
		member.Key: "a",
		impl.Key:   SharedFeature,
		method.Key: SharedFeature,
		iface.Key:  SharedFeature,
	}}

	got := unbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	want := []entities.DeclKey{impl.Key, method.Key}
	slices.SortFunc(
		want,
		func(a, b entities.DeclKey) int { return cmp.Compare(a.String(), b.String()) },
	)

	if !slices.Equal(got, want) {
		t.Fatalf("unbornProviders() = %v, want %v", got, want)
	}
}
