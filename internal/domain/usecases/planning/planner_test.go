package planning

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
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

func TestPlannerBornWithTheFirstFeatureThatNeedsIt(t *testing.T) {
	t.Parallel()

	graph, order, named := twoFeatures()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	if len(plan.Unplaced) != 0 {
		t.Fatalf("Unplaced = %v, want none", plan.Unplaced)
	}

	features := map[entities.DeclKey]string{}

	for _, commit := range plan.Commits {
		if commit.Shape != entities.ShapeFeature {
			t.Fatalf(
				"commit %q is shape %q, want only feature commits",
				commit.Subject,
				commit.Shape,
			)
		}

		for _, key := range commit.Decls {
			features[key] = commit.Feature
		}
	}

	testCases := []struct {
		name string
		decl entities.Decl
		want string
	}{
		{
			"a declaration two features reach is born by the first, the cheaper one",
			named["kernel"],
			"bill/api",
		},
		{"a use case only its own feature reaches travels with it", named["leadUse"], "lead/api"},
		{"so does its repository", named["leadRepo"], "lead/api"},
		{"and the other feature's use case", named["billUse"], "bill/api"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := features[testCase.decl.Key]; got != testCase.want {
				t.Fatalf("feature = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestPlannerBornBeforeUse(t *testing.T) {
	t.Parallel()

	graph, order, _ := twoFeatures()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for index, commit := range plan.Commits {
		for _, key := range commit.Decls {
			if _, already := bornAt[key]; already {
				t.Fatalf("%s is born twice", key)
			}

			bornAt[key] = index
		}
	}

	// The invariant the whole tool serves: no commit carries a declaration
	// whose provider is written later. Inside one commit is fine, which is
	// exactly what makes a vertical slice legal.
	for _, edge := range graph.Edges() {
		dependent, provider := bornAt[edge.Dependent], bornAt[edge.Provider]
		if provider > dependent {
			t.Fatalf(
				"%s (commit %d) is born before %s (commit %d), which it needs",
				edge.Dependent, dependent, edge.Provider, provider,
			)
		}
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
			dirs[parentDir(key.File)] = true
		}

		widest = max(widest, len(dirs))
	}

	// A layer-ordered history would give one directory per commit. The point
	// of the feature cut is that a slice arrives whole.
	if widest < 2 {
		t.Fatalf("widest feature commit spans %d directories, want at least 2", widest)
	}
}

// sharedKernelWithLateProvider: two features read shared event directly,
// event needs shared reason, and one reader also owns an early declaration
// that wins the chronology tie-break ahead of what event itself needed.
func sharedKernelWithLateProvider() (entities.Graph, entities.Ordering) {
	early := decl("consumera/early.go", "Early", "consumera")
	consumerA := decl("consumera/handler.go", "HandleA", "consumera")
	consumerB := decl("consumerb/handler.go", "HandleB", "consumerb")
	event := decl("kernel/event.go", "Event", "kernel")
	reason := decl("kernel/reason.go", "Reason", "kernel")

	decls := []entities.Decl{early, consumerA, consumerB, event, reason}
	edges := []entities.Edge{
		needs(consumerA, event),
		needs(consumerB, event),
		needs(event, reason),
	}

	at := map[entities.DeclKey]entities.Position{
		early.Key:     0,
		consumerB.Key: 2,
		reason.Key:    50,
		consumerA.Key: 60,
		event.Key:     61,
	}

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}
}

func TestPlannerSharedDeclarationWaitsForItsOwnProvider(t *testing.T) {
	t.Parallel()

	graph, order := sharedKernelWithLateProvider()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for index, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = index
		}
	}

	for _, edge := range graph.Edges() {
		dependent, provider := bornAt[edge.Dependent], bornAt[edge.Provider]
		if provider > dependent {
			t.Fatalf(
				"%s (commit %d) is born before %s (commit %d), which it needs",
				edge.Dependent, dependent, edge.Provider, provider,
			)
		}
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

func syntheticGraph(features, declsPer int) (entities.Graph, entities.Ordering) {
	layers := []string{"api", "usecases", "repo"}
	kernel := []entities.Decl{
		decl("kernel/id.go", "NewID", "kernel"),
		decl("kernel/clock.go", "Now", "kernel"),
	}

	decls := append([]entities.Decl{}, kernel...)

	var edges []entities.Edge

	at := map[entities.DeclKey]entities.Position{}

	position := entities.Position(0)
	for _, item := range kernel {
		at[item.Key] = position
		position++
	}

	for feature := range features {
		var previous entities.Decl

		for index := range declsPer {
			layer := layers[index*len(layers)/declsPer]
			item := decl(
				fmt.Sprintf("f%d/%s/file%d.go", feature, layer, index),
				fmt.Sprintf("Fn%d", index),
				entities.UnitID(fmt.Sprintf("f%d/%s", feature, layer)),
			)

			decls = append(decls, item)
			at[item.Key] = position
			position++

			if index == 0 {
				edges = append(edges, needs(item, kernel[feature%len(kernel)]))
			} else {
				edges = append(edges, needs(previous, item))
			}

			previous = item
		}
	}

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}
}

func BenchmarkPlan(b *testing.B) {
	graph, order := syntheticGraph(50, 40)

	b.ReportAllocs()

	var commits int

	for range b.N {
		plan, err := New(Config{}).Plan(graph, order)
		if err != nil {
			b.Fatalf("Plan() = %v", err)
		}

		commits = len(plan.Commits)
	}

	b.ReportMetric(float64(commits), "commits")
	b.ReportMetric(float64(graph.Len()), "decls")
}
