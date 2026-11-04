package typed

import (
	"context"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
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

func TestFactsKeyAProviderSeenThroughExportDataByItsName(t *testing.T) {
	t.Parallel()

	padded := "package a\n\nfunc Big() {\n" + strings.Repeat("\t_ = 1\n", 40) + "}\n\n"
	big := testkit.DeclKey("a/a.go", entities.KindFunc, "Big")
	use := testkit.DeclKey("b/b.go", entities.KindFunc, "Use")

	testCases := []struct {
		name       string
		provided   string
		dependent  string
		wantUse    entities.DeclKey
		wantMethod entities.DeclKey
	}{
		{
			"a called func",
			"func Target() {}\n",
			"func Use() {\n\ta.Target()\n}\n",
			testkit.DeclKey("a/a.go", entities.KindFunc, "Target"),
			entities.DeclKey{},
		},
		{
			"a converted type and its method",
			"type T struct{}\n\nfunc (T) Read() {}\n",
			"type Reader interface{ Read() }\n\nfunc Use() {\n\tvar _ Reader = a.T{}\n}\n",
			testkit.DeclKey("a/a.go", entities.KindType, "T"),
			testkit.DeclKey("a/a.go", entities.KindMethod, "T.Read"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{
				"a/a.go": padded + testCase.provided,
				"b/b.go": "package b\n\nimport \"example.test/fixture/a\"\n\n" + testCase.dependent,
			})

			pkgs, err := typeCheck(
				context.Background(),
				snap.Root,
				loadPass{patterns: []string{"example.test/fixture/b"}},
			)
			if err != nil {
				t.Fatalf("typeCheck() error = %v", err)
			}

			facts := collectFacts(pkgs, snap.Root, NewDeclLocator(decls), dynamicNames(pkgs))

			if !slices.Contains(facts.Uses, Use{Dependent: use, Provider: testCase.wantUse}) {
				t.Errorf("Uses = %v, want Use to use %v", facts.Uses, testCase.wantUse)
			}

			atBig := func(found Use) bool { return found.Provider == big }

			if slices.ContainsFunc(facts.Uses, atBig) {
				t.Errorf("Uses = %v, want no use of the decl at the fake offset", facts.Uses)
			}

			if testCase.wantMethod != (entities.DeclKey{}) &&
				!slices.ContainsFunc(facts.Converted, func(req Requirement) bool {
					return slices.Contains(req.Methods, testCase.wantMethod)
				}) {
				t.Errorf("Converted = %v, want the method %v", facts.Converted, testCase.wantMethod)
			}
		})
	}
}

func TestFactsKeyAParsedProviderByItsPosition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		source       string
		wantProvider entities.DeclKey
	}{
		{
			name: "a method of an interface literal inside a type",
			source: "package b\n\ntype S struct{ F interface{ Fld() } }\n\nvar Global S\n\n" +
				"func Use() {\n\tGlobal.F.Fld()\n}\n",
			wantProvider: testkit.DeclKey("b/b.go", entities.KindType, "S"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{"b/b.go": testCase.source})

			pkgs, err := typeCheck(
				context.Background(),
				snap.Root,
				loadPass{patterns: []string{"example.test/fixture/b"}},
			)
			if err != nil {
				t.Fatalf("typeCheck() error = %v", err)
			}

			facts := collectFacts(pkgs, snap.Root, NewDeclLocator(decls), dynamicNames(pkgs))
			want := Use{
				Dependent: testkit.DeclKey("b/b.go", entities.KindFunc, "Use"),
				Provider:  testCase.wantProvider,
			}

			if !slices.Contains(facts.Uses, want) {
				t.Fatalf("Uses = %v, want %v", facts.Uses, want)
			}
		})
	}
}
