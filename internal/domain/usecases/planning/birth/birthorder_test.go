package birth

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSortForBirthOrdersKeysWhoseTextIsEqualWhateverTheArrival(t *testing.T) {
	t.Parallel()

	fileTie := entities.Decl{Key: entities.DeclKey{
		File: "x:func", Kind: entities.KindType, Name: "T",
	}}
	nameTie := entities.Decl{Key: entities.DeclKey{
		File: "x", Kind: entities.KindFunc, Name: "type:T",
	}}
	graph := entities.NewGraph([]entities.Decl{fileTie, nameTie}, nil, nil)

	testCases := []struct {
		name string
		keys []entities.DeclKey
	}{
		{"shorter file arrives first", []entities.DeclKey{nameTie.Key, fileTie.Key}},
		{"longer file arrives first", []entities.DeclKey{fileTie.Key, nameTie.Key}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := SortForBirth(graph, entities.Ordering{}, testCase.keys, nil)
			want := []entities.DeclKey{nameTie.Key, fileTie.Key}

			if !slices.Equal(got, want) {
				t.Fatalf("order = %v, want %v", got, want)
			}
		})
	}
}

func TestSortForBirthKeepsAnAtomContiguousWithinAPosition(t *testing.T) {
	t.Parallel()

	first := decl("gen/a.sql.go", "First", "gen")
	first.Atom = "gen"
	second := decl("gen/a.sql.go", "Second", "gen")
	second.Atom = "gen"
	looseA := decl("lib/a.go", "A", "lib")
	looseB := decl("lib/b.go", "B", "lib")

	graph := entities.NewGraph(
		[]entities.Decl{first, looseA, second, looseB},
		nil,
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, looseA.Key, second.Key, looseB.Key},
		nil,
	)

	at := map[entities.DeclKey]int{}
	for index, key := range got {
		at[key] = index
	}

	if gap := at[second.Key] - at[first.Key]; gap != 1 {
		t.Fatalf("atom members are %d apart in %v, want adjacent", gap, got)
	}
}

func TestSortForBirthWaitsForWhatAnAtomMemberNeeds(t *testing.T) {
	t.Parallel()

	first := decl("gen/a.sql.go", "First", "gen")
	first.Atom = "gen"
	second := decl("gen/a.sql.go", "Second", "gen")
	second.Atom = "gen"
	scaffold := decl("lib/a.go", "Helper", "lib")

	graph := entities.NewGraph(
		[]entities.Decl{first, second, scaffold},
		[]entities.Edge{needs(second, scaffold)},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, second.Key, scaffold.Key},
		nil,
	)

	if len(got) != 3 || got[0] != scaffold.Key {
		t.Fatalf("order = %v, want the scaffold before the atom", got)
	}
}

func TestSortForBirthIgnoresAGeneratedByEdgeThatClosesACycle(t *testing.T) {
	t.Parallel()

	scaffold := testDecl("pkg/a_test.go", "newUseCaseWithMocks", "pkg")
	mock := testDecl("pkg/z_mock_test.go", "NewMockStore", "pkg")

	graph := entities.NewGraph(
		[]entities.Decl{scaffold, mock},
		[]entities.Edge{
			needs(scaffold, mock),
			{Dependent: mock.Key, Provider: scaffold.Key, Reason: entities.ReasonGeneratedBy},
		},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{scaffold.Key, mock.Key},
		nil,
	)

	if len(got) != 2 || got[0] != mock.Key {
		t.Fatalf("order = %v, want the mock before the scaffold that reads it", got)
	}
}

func TestSortForBirthKeepsAHardCycleInArrivalOrder(t *testing.T) {
	t.Parallel()

	first := decl("pkg/a.go", "A", "pkg")
	second := decl("pkg/b.go", "B", "pkg")

	graph := entities.NewGraph(
		[]entities.Decl{first, second},
		[]entities.Edge{needs(first, second), needs(second, first)},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{first.Key, second.Key},
		nil,
	)

	if len(got) != 2 || got[0] != first.Key || got[1] != second.Key {
		t.Fatalf("order = %v, want the cycle kept in arrival order", got)
	}
}

func TestSortForBirthPutsWhatWaitsOnAHardCycleAfterIt(t *testing.T) {
	t.Parallel()

	dependent := decl("pkg/0.go", "Dependent", "pkg")
	first := decl("pkg/a.go", "A", "pkg")
	second := decl("pkg/b.go", "B", "pkg")

	graph := entities.NewGraph(
		[]entities.Decl{dependent, first, second},
		[]entities.Edge{needs(first, second), needs(second, first), needs(dependent, first)},
		nil,
	)

	got := SortForBirth(
		graph,
		entities.Ordering{},
		[]entities.DeclKey{dependent.Key, first.Key, second.Key},
		nil,
	)

	if len(got) != 3 || got[2] != dependent.Key {
		t.Fatalf("order = %v, want the dependent after the cycle it needs", got)
	}
}

func TestIsSoft(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		reason entities.EdgeReason
		want   bool
	}{
		{reason: entities.ReasonGeneratedBy, want: true},
		{reason: entities.ReasonMirrors, want: true},
		{reason: entities.ReasonE2E, want: true},
		{reason: entities.ReasonTestOf, want: false},
		{reason: entities.ReasonReference, want: false},
	}

	for _, testCase := range testCases {
		t.Run(string(testCase.reason), func(t *testing.T) {
			t.Parallel()

			if got := IsSoft(testCase.reason); got != testCase.want {
				t.Fatalf("IsSoft(%q) = %v, want %v", testCase.reason, got, testCase.want)
			}
		})
	}
}

func TestSortForBirthOrdersAnE2EEdgeByItsWaitAndBreaksItsCycle(t *testing.T) {
	t.Parallel()

	route := decl("app/routes.go", "Routes", "app")
	probe := testDecl("test/api_test.go", "TestHealth", "test")
	e2e := entities.Edge{Dependent: probe.Key, Provider: route.Key, Reason: entities.ReasonE2E}

	testCases := []struct {
		name    string
		edges   []entities.Edge
		arrival []entities.DeclKey
		want    []entities.DeclKey
	}{
		{
			name:    "the test waits for its route",
			edges:   []entities.Edge{e2e},
			arrival: []entities.DeclKey{probe.Key, route.Key},
			want:    []entities.DeclKey{route.Key, probe.Key},
		},
		{
			name:    "a cycle through the edge is broken",
			edges:   []entities.Edge{e2e, needs(route, probe)},
			arrival: []entities.DeclKey{route.Key, probe.Key},
			want:    []entities.DeclKey{probe.Key, route.Key},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(
				[]entities.Decl{route, probe}, testCase.edges, nil,
			)

			got := SortForBirth(graph, entities.Ordering{}, testCase.arrival, nil)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("order = %v, want %v", got, testCase.want)
			}
		})
	}
}
