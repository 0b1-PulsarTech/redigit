package planning

import (
	"fmt"
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
