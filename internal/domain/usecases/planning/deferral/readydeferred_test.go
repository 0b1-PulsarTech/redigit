package deferral

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestReadyDeferredIgnoresAnInterfaceATypeOnlyFills(t *testing.T) {
	t.Parallel()

	subject := decl("a/create.go", "Create", "a")
	port := decl("b/port.go", "Port", "b")
	port.IsInterface = true
	double := testDecl("a/create_test.go", "stub", "a")
	graph := entities.NewGraph(
		[]entities.Decl{subject, port, double},
		[]entities.Edge{
			needs(double, subject),
			{Dependent: double.Key, Provider: port.Key, Reason: entities.ReasonImplements},
		},
		nil,
	)
	own := owners.Ownership{
		Owner:    map[entities.DeclKey]reach.FeatureID{subject.Key: "a", port.Key: "b"},
		Deferred: reach.Wiring{double.Key: true},
		Roots:    reach.Wiring{},
	}
	born := map[entities.DeclKey]bool{subject.Key: true}
	fresh := map[entities.DeclKey]bool{subject.Key: true}

	got := ReadyDeferred(graph, own, born, fresh)

	if !slices.Contains(got, double.Key) {
		t.Fatalf("readyDeferred() = %v, want the double born before the interface it fills", got)
	}
}

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

	own := owners.Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			code.Key: "a", later.Key: "b",
			setup.Key: reach.SharedFeature, first.Key: reach.SharedFeature, plain.Key: reach.SharedFeature,
		},
		Deferred: reach.Wiring{setup.Key: true, first.Key: true, plain.Key: true},
		Roots:    reach.Wiring{},
	}

	born := map[entities.DeclKey]bool{code.Key: true}
	fresh := map[entities.DeclKey]bool{code.Key: true}

	if got, want := ReadyDeferred(
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

	if got := ReadyDeferred(graph, own, born, fresh); len(got) != 3 {
		t.Fatalf("readyDeferred() = %v, want the whole atom once its test can be born", got)
	}
}

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

	own := owners.Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			registration.Key: reach.SharedFeature, feature.Key: "a",
			both.Key: reach.SharedFeature, onlyWiring.Key: reach.SharedFeature,
		},
		Reach: map[entities.DeclKey]int{
			both.Key: 2, onlyWiring.Key: 1,
		},
		Deferred: reach.Wiring{registration.Key: true},
		Roots:    reach.Wiring{registration.Key: true},
	}

	fresh := map[entities.DeclKey]bool{onlyWiring.Key: true}

	if got := ReadyDeferred(graph, own, map[entities.DeclKey]bool{}, fresh); len(got) != 0 {
		t.Fatalf("readyDeferred() = %v, want none: Both is born with the feature that uses it", got)
	}

	born := map[entities.DeclKey]bool{both.Key: true}

	if got := ReadyDeferred(graph, own, born, fresh); len(got) != 1 {
		t.Fatalf("readyDeferred() = %v, want the registration once Both is born", got)
	}
}

func TestReadyDeferredBringsWhatNoBuiltFeatureReaches(t *testing.T) {
	t.Parallel()

	suite := testDecl("e2e/flow_test.go", "TestFlow", "e2e")
	require := decl("e2e/go.mod", "require x/y", "e2e")
	require.Key.Kind = entities.KindOther
	indirect := decl("e2e/go.mod", "require x/z // indirect", "e2e")
	indirect.Key.Kind = entities.KindOther
	query := decl("db/q.sql", "Insert", "db")
	query.Key.Kind = entities.KindOther
	feature := decl("a/handler.go", "Handle", "a")
	counted := decl("a/count.go", "Count", "a")

	graph := entities.NewGraph(
		[]entities.Decl{suite, require, indirect, query, feature, counted},
		[]entities.Edge{
			needs(suite, require), needs(suite, query), needs(suite, counted),
			{Dependent: suite.Key, Provider: require.Key, Reason: entities.ReasonImport},
			needs(feature, counted),
			{Dependent: require.Key, Provider: indirect.Key, Reason: entities.ReasonImport},
		},
		nil,
	)

	testCases := []struct {
		name  string
		reach map[entities.DeclKey]int
		owner map[entities.DeclKey]reach.FeatureID
		want  bool
	}{
		{
			"a require nothing but the suite imports",
			map[entities.DeclKey]int{require.Key: 1, indirect.Key: 1, query.Key: 0, counted.Key: 1},
			map[entities.DeclKey]reach.FeatureID{counted.Key: reach.SharedFeature},
			true,
		},
		{
			"a query no built feature reaches",
			map[entities.DeclKey]int{require.Key: 0, query.Key: 0, counted.Key: 1},
			map[entities.DeclKey]reach.FeatureID{counted.Key: reach.SharedFeature},
			true,
		},
		{
			"shared code a feature also reads still waits for its feature",
			map[entities.DeclKey]int{require.Key: 0, query.Key: 2, counted.Key: 1},
			map[entities.DeclKey]reach.FeatureID{counted.Key: reach.SharedFeature},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			owner := map[entities.DeclKey]reach.FeatureID{
				suite.Key: reach.SharedFeature, require.Key: reach.SharedFeature, indirect.Key: reach.SharedFeature,
				query.Key:   reach.SharedFeature,
				counted.Key: reach.SharedFeature, feature.Key: "a",
			}
			own := owners.Ownership{
				Owner:    owner,
				Reach:    testCase.reach,
				Deferred: reach.Wiring{suite.Key: true},
				Roots:    reach.Wiring{},
			}
			born := map[entities.DeclKey]bool{counted.Key: true}
			fresh := map[entities.DeclKey]bool{counted.Key: true}

			got := slices.Contains(ReadyDeferred(graph, own, born, fresh), suite.Key)
			if got != testCase.want {
				t.Fatalf("suite ready = %v, want %v", got, testCase.want)
			}
		})
	}
}
