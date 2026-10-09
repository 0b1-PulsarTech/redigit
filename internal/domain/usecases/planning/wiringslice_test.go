package planning

import (
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

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
