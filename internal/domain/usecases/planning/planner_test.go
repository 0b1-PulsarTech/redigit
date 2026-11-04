package planning

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func decl(file, name string, unit entities.UnitID) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: unit,
	}
}

func needs(dependent, provider entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  provider.Key,
		Reason:    entities.ReasonReference,
	}
}

// twoFeatures builds the shape the whole planner exists for: two vertical
// slices that each reach their own use case and repository, plus one kernel
// declaration both of them read.
func twoFeatures() (entities.Graph, entities.Ordering, map[string]entities.Decl) {
	kernel := decl("kernel/id.go", "NewID", "kernel")

	leadUse := decl("lead/usecases/create.go", "Create", "lead/usecases")
	leadRepo := decl("lead/repo/lead.go", "Insert", "lead/repo")
	leadAPI := decl("lead/api/handler.go", "Handle", "lead/api")

	billUse := decl("bill/usecases/charge.go", "Charge", "bill/usecases")
	billAPI := decl("bill/api/handler.go", "Handle", "bill/api")

	decls := []entities.Decl{kernel, leadUse, leadRepo, leadAPI, billUse, billAPI}
	edges := []entities.Edge{
		needs(leadAPI, leadUse),
		needs(leadUse, leadRepo),
		needs(leadRepo, kernel),
		needs(billAPI, billUse),
		needs(billUse, kernel),
	}

	at := map[entities.DeclKey]entities.Position{
		kernel.Key:   0,
		leadRepo.Key: 1,
		leadUse.Key:  2,
		leadAPI.Key:  3,
		billUse.Key:  4,
		billAPI.Key:  5,
	}

	return entities.NewGraph(decls, edges, nil),
		entities.Ordering{At: at},
		map[string]entities.Decl{
			"kernel": kernel, "leadUse": leadUse, "leadRepo": leadRepo,
			"leadAPI": leadAPI, "billUse": billUse, "billAPI": billAPI,
		}
}

func TestPlannerVerticalCommitCrossesDirectories(t *testing.T) {
	t.Parallel()

	graph, order, _ := twoFeatures()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	widest := 0

	for _, commit := range plan.Commits {
		if commit.Shape != entities.ShapeFeature {
			continue
		}

		dirs := map[string]bool{}
		for _, key := range commit.Decls {
			dirs[reach.ParentDir(key.File)] = true
		}

		widest = max(widest, len(dirs))
	}

	// A layer-ordered history would give one directory per commit. The point
	// of the feature cut is that a slice arrives whole.
	if widest < 2 {
		t.Fatalf("widest feature commit spans %d directories, want at least 2", widest)
	}
}

func TestPlannerKeepsAnAtomInOneCommit(t *testing.T) {
	t.Parallel()

	typeA := decl("dbgen/models.go", "TypeA", "dbgen")
	typeA.Atom = "dbgen/models.go"
	typeB := decl("dbgen/models.go", "TypeB", "dbgen")
	typeB.Atom = "dbgen/models.go"

	consumerA := decl("consumera/handler.go", "HandleA", "consumera")
	consumerB := decl("consumerb/handler.go", "HandleB", "consumerb")

	graph := entities.NewGraph(
		[]entities.Decl{typeA, typeB, consumerA, consumerB},
		[]entities.Edge{needs(consumerA, typeA), needs(consumerB, typeB)},
		nil,
	)

	plan, err := New(Config{}).Plan(graph, entities.Ordering{})
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for index, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = index
		}
	}

	if bornAt[typeA.Key] != bornAt[typeB.Key] {
		t.Fatalf(
			"atom split across commits %d and %d, want the same one",
			bornAt[typeA.Key], bornAt[typeB.Key],
		)
	}
}

func TestPlannerErrors(t *testing.T) {
	t.Parallel()

	left := decl("a/one.go", "One", "a")
	right := decl("b/two.go", "Two", "b")

	testCases := []struct {
		name    string
		graph   entities.Graph
		wantErr error
	}{
		{
			"nothing to plan",
			entities.NewGraph(nil, nil, nil),
			apperr.ErrIncomplete,
		},
		{
			"a graph with no unconsumed declaration has no entry point to walk back from",
			entities.NewGraph(
				[]entities.Decl{left, right},
				[]entities.Edge{needs(left, right), needs(right, left)},
				nil,
			),
			apperr.ErrIncomplete,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(Config{}).Plan(testCase.graph, entities.Ordering{})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Plan() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestPlannerSplitsWideCommits(t *testing.T) {
	t.Parallel()

	const leaves = 9

	decls := make([]entities.Decl, 0, leaves+1)
	edges := make([]entities.Edge, 0, leaves)

	entry := decl("app/main.go", "Main", "app")
	decls = append(decls, entry)

	for index := range leaves {
		leaf := decl("lib/f"+string(rune('a'+index))+".go", "F"+string(rune('A'+index)), "lib")
		decls = append(decls, leaf)
		edges = append(edges, needs(entry, leaf))
	}

	plan, err := New(Config{MaxFiles: 3}).Plan(
		entities.NewGraph(decls, edges, nil),
		entities.Ordering{},
	)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		files := map[string]bool{}
		for _, key := range commit.Decls {
			files[key.File] = true
		}

		if len(files) > 3 {
			t.Fatalf("commit %q touches %d files, want at most 3", commit.Subject, len(files))
		}
	}

	if len(plan.Commits) < 4 {
		t.Fatalf("got %d commits, want the 10 files split across at least 4", len(plan.Commits))
	}
}

func TestPlannerMarksEverySubjectAsDraft(t *testing.T) {
	t.Parallel()

	graph, order, _ := twoFeatures()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		if !entities.IsDraft(commit.Subject) {
			t.Fatalf("subject %q is not marked as a draft", commit.Subject)
		}
	}
}

func TestPlannerPositionsAreDense(t *testing.T) {
	t.Parallel()

	graph, order, _ := twoFeatures()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	want := make([]entities.Position, len(plan.Commits))
	got := make([]entities.Position, len(plan.Commits))

	for index, commit := range plan.Commits {
		want[index] = entities.Position(index)
		got[index] = commit.Position
	}

	if !slices.Equal(got, want) {
		t.Fatalf("positions = %v, want %v", got, want)
	}
}

func testDecl(file, name string, unit entities.UnitID) entities.Decl {
	out := decl(file, name, unit)
	out.IsTest = true

	return out
}

func TestPlannerBearsTheTitleWithTheFirstEntry(t *testing.T) {
	t.Parallel()

	graph, order, parts := twoFeatures()
	title := prose("<file>")
	lead := prose("<file>#2")
	bill := prose("<file>#3")

	decls := append(graph.Decls(), title, lead, bill)
	edges := append(
		graph.Edges(),
		proseEdge(lead, parts["leadAPI"]),
		proseEdge(lead, title),
		proseEdge(bill, parts["billAPI"]),
		proseEdge(bill, title),
	)

	order.At[title.Key], order.At[lead.Key], order.At[bill.Key] = 9, 7, 8

	plan, err := New(Config{}).Plan(entities.NewGraph(decls, edges, nil), order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for index, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = index
		}
	}

	first := min(bornAt[lead.Key], bornAt[bill.Key])
	if bornAt[title.Key] != first {
		t.Fatalf("the title is at %d, the first entry at %d", bornAt[title.Key], first)
	}

	for _, entry := range []struct{ entry, route entities.Decl }{
		{lead, parts["leadAPI"]}, {bill, parts["billAPI"]},
	} {
		if bornAt[entry.entry.Key] < bornAt[entry.route.Key] {
			t.Fatalf("%s is born before the route it records", entry.entry.Key)
		}
	}
}

func reversedGraph(graph entities.Graph) entities.Graph {
	decls := graph.Decls()
	edges := slices.Clone(graph.Edges())
	anchors := slices.Clone(graph.Anchors())

	slices.Reverse(decls)
	slices.Reverse(edges)
	slices.Reverse(anchors)

	return entities.NewGraph(decls, edges, anchors)
}

func TestPlanIgnoresInputOrder(t *testing.T) {
	t.Parallel()

	synthetic, syntheticOrder := syntheticGraph(8, 12)
	two, twoOrder, _ := twoFeatures()
	root, rootOrder, _ := rooted()
	tied, tiedOrder := tiedFeatures()

	testCases := []struct {
		name  string
		graph entities.Graph
		order entities.Ordering
	}{
		{"a synthetic graph of many features", synthetic, syntheticOrder},
		{"two vertical slices and a kernel", two, twoOrder},
		{"a composition root over both", root, rootOrder},
		{"features sharing a position and a helper", tied, tiedOrder},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			forward, err := New(Config{}).Plan(testCase.graph, testCase.order)
			if err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			for run := range 32 {
				backward, err := New(Config{}).Plan(reversedGraph(testCase.graph), testCase.order)
				if err != nil {
					t.Fatalf("Plan() reversed = %v", err)
				}

				if !reflect.DeepEqual(forward, backward) {
					t.Fatalf("run %d: reversed graph gave plan %v, want %v", run, backward, forward)
				}
			}
		})
	}
}

func TestPlannerBirthsAWholeAtomWithTheTestThatPullsIt(t *testing.T) {
	t.Parallel()

	base, _, named := twoFeatures()
	charge, create, handle := named["billUse"], named["leadUse"], named["leadAPI"]
	first := testDecl("lead/mocks/gen_test.go", "First", "lead/mocks")
	second := decl("lead/mocks/gen_test.go", "Second", "lead/mocks")
	third := decl("lead/mocks/gen_test.go", "Third", "lead/mocks")
	rig := decl("lead/mocks/rig_test.go", "rig", "lead/mocks")
	later := testDecl("lead/api/later_test.go", "TestLater", "lead/api")
	kept := testDecl("bill/api/kept_test.go", "TestKept", "bill/api")
	port := decl("mocks/mock.go", "NewMockPort", "mocks")
	get := decl("mocks/mock.go", "MockPort.Get", "mocks")
	kind := decl("mocks/mock.go", "MockPort", "mocks")
	rearm := decl("mocks/mock.go", "rearm", "mocks")
	settle := decl("mocks/mock.go", "settle", "mocks")
	testCharge := testDecl("bill/api/charge_test.go", "TestCharge", "bill/api")
	testCreate := testDecl("lead/api/create_test.go", "TestCreate", "lead/api")

	first.Atom, second.Atom, third.Atom = "gen", "gen", "gen"

	for _, member := range []*entities.Decl{&port, &get, &kind, &rearm, &settle} {
		member.Atom = "mock"
	}

	testCases := []struct {
		name     string
		extra    []entities.Decl
		edges    []entities.Edge
		together []entities.Decl
		with     entities.Decl
		after    []entities.Decl
	}{
		{
			name:  "a mate that needs a rig only a later test reads",
			extra: []entities.Decl{first, second, rig, later},
			edges: []entities.Edge{
				needs(first, charge), needs(second, rig),
				needs(later, second), needs(later, handle),
			},
			together: []entities.Decl{first, second, rig},
			after:    []entities.Decl{later},
			with:     charge,
		},
		{
			name:  "a kept test reads one mate while another mate needs the rig",
			extra: []entities.Decl{second, third, rig, kept},
			edges: []entities.Edge{
				needs(kept, second), needs(kept, charge), needs(third, rig),
			},
			together: []entities.Decl{second, third, rig, kept},
			with:     charge,
		},
		{
			name:  "a mate no feature reaches does not keep the atom from the test that pulls it",
			extra: []entities.Decl{port, get, kind, rearm, settle, testCharge, testCreate},
			edges: []entities.Edge{
				needs(port, kind), needs(port, get), needs(get, kind), needs(get, rearm),
				needs(rearm, settle), needs(settle, rearm), needs(kind, charge),
				needs(testCharge, port), needs(testCharge, charge), needs(testCreate, port),
				needs(testCreate, create),
			},
			together: []entities.Decl{port, get, kind, rearm, settle},
			after:    []entities.Decl{testCreate},
			with:     testCharge,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			bornAt := bornAtOf(
				t,
				append(base.Decls(), testCase.extra...),
				append(base.Edges(), testCase.edges...),
			)
			assertBornTogether(t, bornAt, testCase.with, testCase.together, testCase.after)
		})
	}
}

func assertBornTogether(
	t testing.TB,
	bornAt map[entities.DeclKey]int,
	with entities.Decl,
	together, after []entities.Decl,
) {
	t.Helper()

	want := bornAt[with.Key]

	for _, member := range together {
		if got := bornAt[member.Key]; got != want {
			t.Fatalf("%v is born at %d, want %d with %v", member.Key, got, want, with.Key)
		}
	}

	for _, member := range after {
		if got := bornAt[member.Key]; got <= want {
			t.Fatalf("%v is born at %d, want after %d", member.Key, got, want)
		}
	}
}
