package owners

import (
	"cmp"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestSortedKeysOrdersKeysWhoseTextIsEqual(t *testing.T) {
	t.Parallel()

	fileTie := entities.DeclKey{File: "x:func", Kind: entities.KindType, Name: "T"}
	nameTie := entities.DeclKey{File: "x", Kind: entities.KindFunc, Name: "type:T"}

	testCases := []struct {
		name string
		set  map[entities.DeclKey]bool
		want []entities.DeclKey
	}{
		{
			"tied keys order by file",
			map[entities.DeclKey]bool{fileTie: true, nameTie: true},
			[]entities.DeclKey{nameTie, fileTie},
		},
		{"an empty set yields nothing", map[entities.DeclKey]bool{}, []entities.DeclKey{}},
	}

	const runs = 256

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range runs {
				if got := SortedKeys(testCase.set); !slices.Equal(got, testCase.want) {
					t.Fatalf("SortedKeys() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key: "a",
		event.Key:  reach.SharedFeature,
		reason.Key: reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	want := []entities.DeclKey{event.Key, reason.Key}
	slices.SortFunc(
		want,
		func(first, second entities.DeclKey) int { return cmp.Compare(first.String(), second.String()) },
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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key: "a",
		typeA.Key:  reach.SharedFeature,
		typeB.Key:  reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key:    "a",
		event.Key:     reach.SharedFeature,
		eventTest.Key: reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key:    "a",
		event.Key:     reach.SharedFeature,
		late.Key:      "b",
		eventTest.Key: reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			member.Key: "a",
			event.Key:  reach.SharedFeature,
		},
	}
	born := map[entities.DeclKey]bool{event.Key: true}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, born)

	if len(got) != 0 {
		t.Fatalf("unbornProviders() = %v, want none: everything it needs is already born", got)
	}
}

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

	own := Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			member.Key: "app", manifest.Key: reach.SharedFeature,
			used.Key: reach.SharedFeature, unused.Key: reach.SharedFeature,
		},
	}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key:    "a",
		event.Key:     reach.SharedFeature,
		late.Key:      "b",
		support.Key:   reach.SharedFeature,
		eventTest.Key: reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key:  "a",
		mock.Key:    reach.SharedFeature,
		support.Key: reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			member.Key: "a",
			stmt.Key:   reach.SharedFeature,
			health.Key: "w",
			status.Key: "w",
		},
		Roots: reach.Wiring{stmt.Key: true},
	}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

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

	own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
		member.Key: "a",
		impl.Key:   reach.SharedFeature,
		method.Key: reach.SharedFeature,
		iface.Key:  reach.SharedFeature,
	}}

	got := UnbornProviders(graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{})

	want := []entities.DeclKey{impl.Key, method.Key}
	slices.SortFunc(
		want,
		func(first, second entities.DeclKey) int { return cmp.Compare(first.String(), second.String()) },
	)

	if !slices.Equal(got, want) {
		t.Fatalf("unbornProviders() = %v, want %v", got, want)
	}
}

func TestUnbornProvidersPullsASharedTestOnlyPastItsSoftEdges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		reason entities.EdgeReason
		want   bool
	}{
		{
			name:   "an e2e edge keeps the test behind its route",
			reason: entities.ReasonE2E, want: false,
		},
		{
			name:   "a generated-by edge is still skipped",
			reason: entities.ReasonGeneratedBy, want: true,
		},
		{
			name:   "a mirrors edge is still skipped",
			reason: entities.ReasonMirrors, want: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			member := decl("a/handler.go", "Handle", "a")
			norm := decl("shared/norm.go", "Norm", "shared")
			normTest := testDecl("shared/norm_test.go", "TestNorm", "shared")
			route := decl("users/routes.go", "Routes", "users")

			graph := entities.NewGraph(
				[]entities.Decl{member, norm, normTest, route},
				[]entities.Edge{
					needs(member, norm),
					edgeOf(normTest, route, testCase.reason),
				},
				[]entities.Anchor{
					{Decl: normTest.Key, To: norm.Key, Reason: entities.ReasonTestOf},
				},
			)

			own := Ownership{Owner: map[entities.DeclKey]reach.FeatureID{
				member.Key:   "a",
				norm.Key:     reach.SharedFeature,
				normTest.Key: reach.SharedFeature,
				route.Key:    "users",
			}}

			got := UnbornProviders(
				graph, own, []entities.DeclKey{member.Key}, map[entities.DeclKey]bool{},
			)

			if carried := slices.Contains(got, normTest.Key); carried != testCase.want {
				t.Fatalf("unbornProviders() = %v, carries %s = %v, want %v",
					got, normTest.Key, carried, testCase.want)
			}
		})
	}
}
