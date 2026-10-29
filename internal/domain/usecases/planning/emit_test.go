package planning

import (
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A function cut into statements grows with the program: the program starts
// with its entry, and each feature adds the registration that wires it. Held
// whole it waits for the last feature and `main` lands at the end.
func TestPlannerBirthsEachRegistrationWithItsFeature(t *testing.T) {
	t.Parallel()

	run := decl("cmd/app/run.go", "Run", "cmd/app")
	main := decl("cmd/app/main.go", "Main", "cmd/app")
	first := statement("Run", "cmd/app/run.go", 0)
	lead := statement("Run", "cmd/app/run.go", 1)
	bill := statement("Run", "cmd/app/run.go", 2)

	first.Atom = "run"
	run.Atom = "run"

	base, _, named := twoFeatures()

	decls := append(base.Decls(), run, main, first, lead, bill)
	edges := append(
		base.Edges(),
		needs(main, run),
		needs(first, run),
		needs(lead, run),
		needs(bill, run),
		needs(lead, named["leadAPI"]),
		needs(bill, named["billAPI"]),
	)

	at := map[entities.DeclKey]entities.Position{}
	for index, d := range decls {
		at[d.Key] = entities.Position(index)
	}

	plan, err := New(Config{}).Plan(
		entities.NewGraph(decls, edges, nil),
		entities.Ordering{At: at},
	)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for index, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = index
		}
	}

	if len(plan.Unplaced) != 0 {
		t.Fatalf("Unplaced = %v, want none", plan.Unplaced)
	}

	testCases := []struct {
		name         string
		born, before entities.Decl
	}{
		{"main is born before the first feature", main, named["leadAPI"]},
		{"the first registration comes with the signature", first, named["leadAPI"]},
		{"a registration comes after the feature it wires", named["leadAPI"], lead},
		{"the other registration comes after its own", named["billAPI"], bill},
		{"a registration is not held for the last feature", bill, named["leadAPI"]},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if bornAt[testCase.born.Key] >= bornAt[testCase.before.Key] {
				t.Fatalf("%v is born at %d, want before %v at %d",
					testCase.born.Key, bornAt[testCase.born.Key],
					testCase.before.Key, bornAt[testCase.before.Key])
			}
		})
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

// A test waits for the member of its atom that needs the last feature. The
// feature freshens that member alone, and nothing is left to freshen the test:
// it is born once the last feature is built, not in the tail of what no entry
// point reached.
func TestPlannerBirthsATestWaitingOnItsAtomAfterTheLastFeature(t *testing.T) {
	t.Parallel()

	handler := decl("a/handler.go", "Handle", "a")
	counted := decl("a/count.go", "Count", "a")
	late := decl("b/handler.go", "Serve", "b")
	early := testDecl("b/early_test.go", "TestEarly", "b")
	last := testDecl("b/last_test.go", "TestLast", "b")
	early.Atom, last.Atom = "e2e-main", "e2e-main"

	graph := entities.NewGraph(
		[]entities.Decl{handler, counted, late, early, last},
		[]entities.Edge{needs(handler, counted), needs(early, counted), needs(last, late)},
		nil,
	)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		handler.Key: 0, counted.Key: 1, late.Key: 2, early.Key: 3, last.Key: 4,
	}}

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	for _, commit := range plan.Commits {
		if strings.HasPrefix(commit.Reason, "reached by no entry point") {
			t.Fatalf("commit %q holds what no feature reached", commit.Subject)
		}
	}
}

func TestPlannerBuildsEachHandlerAsItsOwnArc(t *testing.T) {
	t.Parallel()

	graph, order, named := rooted()

	plan, err := New(Config{}).Plan(graph, order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	if len(plan.Unplaced) != 0 {
		t.Fatalf("Unplaced = %v, want none", plan.Unplaced)
	}

	bornAt := map[entities.DeclKey]int{}
	runs := []string{}

	for index, commit := range plan.Commits {
		if len(runs) == 0 || runs[len(runs)-1] != commit.Feature {
			runs = append(runs, commit.Feature)
		}

		for _, key := range commit.Decls {
			bornAt[key] = index
		}
	}

	seen := map[string]bool{}

	for _, feature := range runs {
		if seen[feature] {
			t.Fatalf("feature runs %v: %q is split in two", runs, feature)
		}

		seen[feature] = true
	}

	if !seen["lead/api"] || !seen["bill/api"] {
		t.Fatalf("feature runs %v, want one arc for each handler", runs)
	}

	root := bornAt[named["root"].Key]
	for _, handler := range []string{"leadAPI", "billAPI"} {
		if bornAt[named[handler].Key] >= root {
			t.Fatalf(
				"%s is born at %d, want before the root at %d",
				handler,
				bornAt[named[handler].Key],
				root,
			)
		}
	}

	first := plan.Commits[bornAt[named["kernel"].Key]].Feature
	if first != "bill/api" {
		t.Fatalf("the shared declaration is born by %q, want the first feature built", first)
	}
}
