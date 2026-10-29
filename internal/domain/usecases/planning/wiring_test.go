package planning

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// rooted is twoFeatures plus a composition root that wires both handlers and
// enough registrations of its own to read as the program's assembly.
func rooted() (entities.Graph, entities.Ordering, map[string]entities.Decl) {
	base, order, named := twoFeatures()
	root := decl("cmd/app/main.go", "Run", "cmd/app")

	decls := base.Decls()
	edges := base.Edges()
	at := map[entities.DeclKey]entities.Position{}

	for _, d := range decls {
		at[d.Key] = order.Of(d.Key)
	}

	decls = append(decls, root)
	edges = append(
		edges,
		needs(root, named["leadAPI"]),
		needs(root, named["billAPI"]),
	)
	at[root.Key] = entities.Position(len(at))

	for index := range 20 {
		ballast := decl("cmd/app/bind.go", fmt.Sprintf("Bind%d", index), "cmd/app")
		decls = append(decls, ballast)
		edges = append(edges, needs(root, ballast))
		at[ballast.Key] = entities.Position(len(at))
	}

	named["root"] = root

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}, named
}

func TestFindWiring(t *testing.T) {
	t.Parallel()

	graph, _, named := rooted()
	roots := entities.FindWiring(graph)

	testCases := []struct {
		name string
		decl entities.Decl
		want bool
	}{
		{"the root needs a twentieth of the program", named["root"], true},
		{"a handler needs a few declarations", named["leadAPI"], false},
		{"a leaf needs nothing", named["kernel"], false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := roots[testCase.decl.Key]; got != testCase.want {
				t.Fatalf("wiring = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestFindWiringIgnoresASmallProgram(t *testing.T) {
	t.Parallel()

	graph, _, _ := twoFeatures()

	if roots := entities.FindWiring(graph); len(roots) != 0 {
		t.Fatalf("wiring = %v, want none: the program is smaller than any root", roots)
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

func statement(function, file string, index int) entities.Decl {
	return entities.Decl{
		Key: entities.DeclKey{
			File: file,
			Kind: entities.KindStmt,
			Name: function + "#" + strconv.Itoa(index),
		},
		Unit: "cmd/app",
	}
}

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

func TestPlannerBearsTheTitleWithTheFirstEntry(t *testing.T) {
	t.Parallel()

	graph, order, parts := twoFeatures()
	title := prose("<file>")
	lead := prose("<file>#2")
	bill := prose("<file>#3")

	decls := append(graph.Decls(), title, lead, bill)
	edges := append(
		graph.Edges(),
		edgeOf(lead, parts["leadAPI"], entities.ReasonProse),
		edgeOf(lead, title, entities.ReasonProse),
		edgeOf(bill, parts["billAPI"], entities.ReasonProse),
		edgeOf(bill, title, entities.ReasonProse),
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

func prose(name string) entities.Decl {
	found := decl("CHANGELOG.md", name, "")
	found.Key.Kind = entities.KindOther

	return found
}
