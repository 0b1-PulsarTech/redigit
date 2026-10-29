package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestAnchorTests(t *testing.T) {
	t.Parallel()

	source := decl("a/store.go", "Put", "a")
	other := decl("a/other.go", "Other", "a")
	kernel := decl("k/k.go", "Kernel", "k")
	helper := testDecl("a/store_test.go", "errBoom", "a")
	named := testDecl("a/store_test.go", "TestPut", "a")
	loose := testDecl("a/util_test.go", "fixture", "a")
	alone := testDecl("z/alone_test.go", "TestAlone", "z")
	bare := testDecl("z/helper_test.go", "helper", "z")

	graph := entities.NewGraph(
		[]entities.Decl{source, other, kernel, helper, named, loose, alone, bare},
		nil,
		[]entities.Anchor{{Decl: named.Key, To: other.Key, Reason: entities.ReasonTestOf}},
	)

	anchored, _ := anchorTests(graph, entities.Ordering{})
	got := reach.TestSubjects(anchored)

	testCases := []struct {
		name string
		test entities.Decl
		want []entities.DeclKey
	}{
		{
			name: "helper waits for the twin file",
			test: helper,
			want: []entities.DeclKey{source.Key},
		},
		{
			name: "a test with a subject keeps it and waits for the twin",
			test: named,
			want: []entities.DeclKey{other.Key, source.Key},
		},
		{
			name: "a file without a twin waits for the package",
			test: loose,
			want: []entities.DeclKey{source.Key, other.Key},
		},
		{name: "a package without code waits for nothing", test: alone, want: nil},
		{name: "a helper there waits for nothing", test: bare, want: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			subjects := slices.Clone(got[testCase.test.Key])
			slices.SortFunc(subjects, func(a, b entities.DeclKey) int {
				return slices.Index(testCase.want, a) - slices.Index(testCase.want, b)
			})

			if !slices.Equal(subjects, testCase.want) {
				t.Fatalf("subjects = %v, want %v", subjects, testCase.want)
			}
		})
	}
}

func TestOpeningTests(t *testing.T) {
	t.Parallel()

	setup := testDecl("e2e/main_test.go", "TestMain", "e2e")
	early := testDecl("e2e/b_test.go", "TestEarly", "e2e")
	late := testDecl("e2e/a_test.go", "TestLate", "e2e")
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		early.Key: 3, late.Key: 9,
	}}

	got := openingTests([]entities.Decl{setup, early, late}, order)

	if got.atoms[setup.Key] == "" || got.atoms[setup.Key] != got.atoms[early.Key] {
		t.Fatalf("atoms = %v, want TestMain tied to the test settled first", got.atoms)
	}

	if _, tied := got.atoms[late.Key]; tied {
		t.Fatalf("atoms = %v, the later test must stay out of the atom", got.atoms)
	}

	want := []entities.Anchor{
		{Decl: setup.Key, To: early.Key, Reason: entities.ReasonTestOf},
		{Decl: late.Key, To: early.Key, Reason: entities.ReasonTestOf},
	}
	if !slices.Equal(got.anchors, want) {
		t.Fatalf("anchors = %v, want %v", got.anchors, want)
	}
}

func TestProductionEdges(t *testing.T) {
	t.Parallel()

	kernel := decl("k/errors.go", "badRequestError", "k")
	double := testDecl("a/a_test.go", "problemError", "a")
	suite := testDecl("a/a_test.go", "TestA", "a")
	graph := entities.NewGraph(
		[]entities.Decl{kernel, double, suite},
		[]entities.Edge{needs(kernel, double), needs(suite, double), needs(suite, kernel)},
		nil,
	)

	got := productionEdges(graph)
	want := []entities.Edge{needs(suite, double), needs(suite, kernel)}

	if !slices.Equal(got, want) {
		t.Fatalf(
			"productionEdges() = %v, want %v: only production waiting on a test goes",
			got,
			want,
		)
	}
}

// A helper is born with the first piece of its twin file, so a late method of
// the same file does not carry the suite past its source.
func TestAnchorTestsWaitsForTheEarliestOfATwin(t *testing.T) {
	t.Parallel()

	early := decl("a/store.go", "Put", "a")
	late := decl("a/store.go", "Close", "a")
	helper := testDecl("a/store_test.go", "errBoom", "a")

	graph := entities.NewGraph([]entities.Decl{early, late, helper}, nil, nil)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		early.Key: 3, late.Key: 90, helper.Key: 3,
	}}

	anchored, raised := anchorTests(graph, order)
	got := reach.TestSubjects(anchored)[helper.Key]

	if !slices.Equal(got, []entities.DeclKey{early.Key}) {
		t.Fatalf("subjects = %v, want only %v", got, early.Key)
	}

	if raised.Of(helper.Key) != 3 {
		t.Errorf("helper positioned at %d, want 3", raised.Of(helper.Key))
	}
}

// A test that calls a helper is lifted with it, or the cut sorts the test ahead
// of the helper it cannot compile without.
func TestAnchorTestsLiftsAReaderOfALiftedHelper(t *testing.T) {
	t.Parallel()

	source := decl("a/store.go", "Put", "a")
	helper := testDecl("a/store_test.go", "newStore", "a")
	reader := testDecl("a/other_test.go", "TestOther", "a")

	graph := entities.NewGraph(
		[]entities.Decl{source, helper, reader},
		[]entities.Edge{
			{Dependent: reader.Key, Provider: helper.Key, Reason: entities.ReasonReference},
		},
		nil,
	)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		source.Key: 40, helper.Key: 2, reader.Key: 5,
	}}

	_, raised := anchorTests(graph, order)

	if raised.Of(helper.Key) != 40 || raised.Of(reader.Key) != 40 {
		t.Fatalf(
			"helper at %d and reader at %d, want both at 40",
			raised.Of(helper.Key), raised.Of(reader.Key),
		)
	}
}
