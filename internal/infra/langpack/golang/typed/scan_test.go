package typed

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestFactsSortedIsStableUnderEveryPermutation(t *testing.T) {
	t.Parallel()

	left := entities.DeclKey{File: "x:func", Kind: entities.KindType, Name: "T"}
	right := entities.DeclKey{File: "x", Kind: entities.KindFunc, Name: "type:T"}
	first := entities.DeclKey{File: "a.go", Kind: entities.KindMethod, Name: "T.A"}
	second := entities.DeclKey{File: "a.go", Kind: entities.KindMethod, Name: "T.B"}
	third := entities.DeclKey{File: "b.go", Kind: entities.KindMethod, Name: "U.A"}
	owner := entities.DeclKey{File: "a.go", Kind: entities.KindType, Name: "T"}
	dependent := entities.DeclKey{File: "c.go", Kind: entities.KindFunc, Name: "Use"}

	testCases := []struct {
		name  string
		facts Facts
	}{
		{
			"requirements tied on dependent and owner differ by their methods",
			Facts{Forced: []Requirement{
				{Dependent: dependent, Owner: owner, Methods: []entities.DeclKey{first}},
				{Dependent: dependent, Owner: owner, Methods: []entities.DeclKey{second}},
				{Dependent: dependent, Owner: owner, Methods: []entities.DeclKey{first, second}},
			}},
		},
		{
			"keys with equal text and different parts keep one order",
			Facts{Converted: []Requirement{
				{Dependent: left, Owner: owner, Methods: []entities.DeclKey{first}},
				{Dependent: right, Owner: owner, Methods: []entities.DeclKey{first}},
			}},
		},
		{
			"the methods of one requirement come out in key order",
			Facts{Escaped: []Requirement{{
				Dependent: dependent,
				Owner:     owner,
				Methods:   []entities.DeclKey{third, second, left, first, right},
			}}},
		},
		{
			"uses with equal text and different parts keep one order",
			Facts{Uses: []Use{
				{Dependent: left, Provider: owner},
				{Dependent: right, Provider: owner},
				{Dependent: dependent, Provider: left},
				{Dependent: dependent, Provider: right},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rng := rand.New(rand.NewPCG(7, 11))
			want := permuted(testCase.facts, rng).sorted()

			for run := range 256 {
				got := permuted(testCase.facts, rng).sorted()
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("run %d: sorted() = %+v, want %+v", run, got, want)
				}
			}
		})
	}
}

func permuted(facts Facts, rng *rand.Rand) Facts {
	return Facts{
		Forced:    shuffledRequirements(facts.Forced, rng),
		Converted: shuffledRequirements(facts.Converted, rng),
		Escaped:   shuffledRequirements(facts.Escaped, rng),
		Uses:      shuffled(facts.Uses, rng),
	}
}

func shuffledRequirements(all []Requirement, rng *rand.Rand) []Requirement {
	out := make([]Requirement, len(all))

	for index, req := range all {
		req.Methods = shuffled(req.Methods, rng)
		out[index] = req
	}

	rng.Shuffle(len(out), func(from, to int) { out[from], out[to] = out[to], out[from] })

	return out
}

func shuffled[Item any](all []Item, rng *rand.Rand) []Item {
	out := slices.Clone(all)
	rng.Shuffle(len(out), func(from, to int) { out[from], out[to] = out[to], out[from] })

	return out
}
