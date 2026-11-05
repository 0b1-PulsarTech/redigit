package deferral

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestOwnTests(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	later := decl("b/later.go", "Later", "b")
	root := decl("cmd/main.go", "Run", "cmd")
	testCode := testDecl("a/code_test.go", "TestCode", "a")
	testLater := testDecl("b/later_test.go", "TestLater", "b")
	testRoot := testDecl("cmd/main_test.go", "TestRun", "cmd")

	graph := entities.NewGraph(
		[]entities.Decl{code, later, root, testCode, testLater, testRoot},
		[]entities.Edge{
			needs(root, code),
			needs(testCode, code),
			needs(testLater, later),
			needs(testRoot, root),
			needs(testRoot, code),
		},
		nil,
	)

	own := owners.Ownership{
		Owner: map[entities.DeclKey]reach.FeatureID{
			code.Key:      "a",
			later.Key:     "b",
			root.Key:      reach.SharedFeature,
			testCode.Key:  reach.SharedFeature,
			testLater.Key: reach.SharedFeature,
			testRoot.Key:  reach.SharedFeature,
		},
		Deferred: reach.Wiring{
			root.Key:      true,
			testCode.Key:  true,
			testLater.Key: true,
			testRoot.Key:  true,
		},
		Roots: reach.Wiring{root.Key: true},
	}

	born := map[entities.DeclKey]bool{code.Key: true}
	fresh := map[entities.DeclKey]bool{code.Key: true}

	got := OwnTests(graph, own, born, fresh)

	if want := []entities.DeclKey{testCode.Key}; !slices.Equal(got, want) {
		t.Fatalf(
			"ownTests() = %v, want only the test whose subject the feature completed: "+
				"the test of a feature still to come and the one that needs the root wait",
			got,
		)
	}
}

func TestOwnTestsKeepsAnAtomWhole(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	root := decl("cmd/main.go", "Run", "cmd")
	first := testDecl("a/gen_test.go", "First", "a")
	second := testDecl("a/gen_test.go", "Second", "a")
	third := testDecl("a/gen_test.go", "Third", "a")
	free, atomRoot := third, root

	for _, member := range []*entities.Decl{&atomRoot, &first, &second, &third} {
		member.Atom = "mock:a"
	}

	owner := map[entities.DeclKey]reach.FeatureID{
		code.Key: "a", root.Key: reach.SharedFeature, first.Key: reach.SharedFeature,
		second.Key: reach.SharedFeature, third.Key: reach.SharedFeature,
	}
	deferred := reach.Wiring{root.Key: true, first.Key: true, second.Key: true, third.Key: true}
	allRead := []entities.Edge{
		needs(root, code), needs(first, code), needs(second, code), needs(third, code),
	}

	testCases := []struct {
		name  string
		decls []entities.Decl
		edges []entities.Edge
		want  []entities.DeclKey
	}{
		{
			name:  "a member held back takes its mates with it",
			decls: []entities.Decl{code, root, first, second, third},
			edges: append([]entities.Edge{needs(third, root)}, allRead...),
			want:  nil,
		},
		{
			name:  "every member free",
			decls: []entities.Decl{code, root, first, second, third},
			edges: allRead,
			want:  []entities.DeclKey{first.Key, second.Key, third.Key},
		},
		{
			name:  "a root among the members holds the atom back while a free test stays",
			decls: []entities.Decl{code, atomRoot, first, second, free},
			edges: allRead,
			want:  []entities.DeclKey{free.Key},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := OwnTests(
				entities.NewGraph(testCase.decls, testCase.edges, nil),
				owners.Ownership{
					Owner: owner, Deferred: deferred, Roots: reach.Wiring{root.Key: true},
				},
				map[entities.DeclKey]bool{code.Key: true},
				map[entities.DeclKey]bool{code.Key: true},
			)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ownTests() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestOwnTestsHoldsAnAtomForAMateNotYetBuilt(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	first := testDecl("a/gen_test.go", "First", "a")
	second := testDecl("a/gen_test.go", "Second", "a")
	mock := decl("a/mock.go", "Mock", "a")
	shared := reach.SharedFeature

	for _, member := range []*entities.Decl{&first, &second, &mock} {
		member.Atom = "mock:a"
	}

	testCases := []struct {
		name     string
		decls    []entities.Decl
		owner    map[entities.DeclKey]reach.FeatureID
		deferred reach.Wiring
		born     map[entities.DeclKey]bool
		want     []entities.DeclKey
	}{
		{
			name:  "a mate no test defers and nobody has built holds the atom back",
			decls: []entities.Decl{code, first, mock},
			owner: map[entities.DeclKey]reach.FeatureID{
				code.Key: "a", first.Key: shared, mock.Key: shared,
			},
			deferred: reach.Wiring{first.Key: true},
			born:     map[entities.DeclKey]bool{code.Key: true},
			want:     nil,
		},
		{
			name:  "a mate that is already built lets the rest go",
			decls: []entities.Decl{code, first, second},
			owner: map[entities.DeclKey]reach.FeatureID{
				code.Key: "a", first.Key: shared, second.Key: shared,
			},
			deferred: reach.Wiring{first.Key: true, second.Key: true},
			born:     map[entities.DeclKey]bool{code.Key: true, second.Key: true},
			want:     []entities.DeclKey{first.Key},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			edges := []entities.Edge{needs(first, code), needs(second, code), needs(mock, code)}
			got := OwnTests(
				entities.NewGraph(testCase.decls, edges, nil),
				owners.Ownership{
					Owner: testCase.owner, Deferred: testCase.deferred, Roots: reach.Wiring{},
				},
				testCase.born,
				map[entities.DeclKey]bool{code.Key: true},
			)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ownTests() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestOwnTestsStaysWithWhatItReads(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	later := decl("a/code.go", "Later", "a")
	probe := testDecl("a/code_test.go", "TestCode", "a")
	owner := map[entities.DeclKey]reach.FeatureID{
		code.Key: "a", later.Key: "b", probe.Key: reach.SharedFeature,
	}

	testCases := []struct {
		name     string
		anchors  []entities.Anchor
		deferred reach.Wiring
		want     []entities.DeclKey
	}{
		{
			name: "a test stays with what it reads and does not wait for a twin it does not read",
			anchors: []entities.Anchor{
				{Decl: probe.Key, To: later.Key, Reason: entities.ReasonTestOf},
			},
			deferred: reach.Wiring{probe.Key: true},
			want:     []entities.DeclKey{probe.Key},
		},
		{
			name: "a test waits for a twin a feature still defers",
			anchors: []entities.Anchor{
				{Decl: probe.Key, To: later.Key, Reason: entities.ReasonTestOf},
			},
			deferred: reach.Wiring{probe.Key: true, later.Key: true},
			want:     nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := OwnTests(
				entities.NewGraph(
					[]entities.Decl{code, later, probe},
					[]entities.Edge{needs(probe, code)},
					testCase.anchors,
				),
				owners.Ownership{Owner: owner, Deferred: testCase.deferred, Roots: reach.Wiring{}},
				map[entities.DeclKey]bool{code.Key: true},
				map[entities.DeclKey]bool{code.Key: true},
			)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ownTests() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestOwnTestsHoldsAChainBehindItsHead(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	unborn := decl("b/unborn.go", "Unborn", "b")
	linkA := testDecl("a/chain_test.go", "LinkA", "a")
	linkB := testDecl("a/chain_test.go", "LinkB", "a")
	linkC := testDecl("a/chain_test.go", "LinkC", "a")
	linkD := testDecl("a/chain_test.go", "LinkD", "a")
	linkE := testDecl("a/chain_test.go", "LinkE", "a")
	linkF := testDecl("a/chain_test.go", "LinkF", "a")
	shared := reach.SharedFeature
	owner := map[entities.DeclKey]reach.FeatureID{
		code.Key: "a", unborn.Key: shared, linkA.Key: shared, linkB.Key: shared,
		linkC.Key: shared, linkD.Key: shared, linkE.Key: shared, linkF.Key: shared,
	}
	deferred := reach.Wiring{
		linkA.Key: true, linkB.Key: true, linkC.Key: true,
		linkD.Key: true, linkE.Key: true, linkF.Key: true,
	}
	edges := []entities.Edge{
		needs(linkF, code), needs(linkF, unborn), needs(linkE, code), needs(linkE, linkF),
		needs(linkD, code), needs(linkD, linkE), needs(linkC, code), needs(linkC, linkD),
		needs(linkB, code), needs(linkB, linkC), needs(linkA, code), needs(linkA, linkB),
	}

	testCases := []struct {
		name string
		born map[entities.DeclKey]bool
		want []entities.DeclKey
	}{
		{
			name: "every link waits on what the head waits on",
			born: map[entities.DeclKey]bool{code.Key: true},
			want: nil,
		},
		{
			name: "the whole chain lands once the head can",
			born: map[entities.DeclKey]bool{code.Key: true, unborn.Key: true},
			want: []entities.DeclKey{
				linkA.Key, linkB.Key, linkC.Key, linkD.Key, linkE.Key, linkF.Key,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := OwnTests(
				entities.NewGraph(
					[]entities.Decl{code, unborn, linkA, linkB, linkC, linkD, linkE, linkF},
					edges,
					nil,
				),
				owners.Ownership{Owner: owner, Deferred: deferred, Roots: reach.Wiring{}},
				testCase.born,
				map[entities.DeclKey]bool{code.Key: true},
			)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ownTests() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestOwnTestsWaitsForAnUnbornProvider(t *testing.T) {
	t.Parallel()

	code := decl("a/code.go", "Code", "a")
	unborn := decl("b/unborn.go", "Unborn", "b")
	aid := testDecl("a/helper_test.go", "Helper", "a")
	probe := testDecl("a/code_test.go", "TestCode", "a")
	shared := reach.SharedFeature

	testCases := []struct {
		name  string
		decls []entities.Decl
		edges []entities.Edge
		owner map[entities.DeclKey]reach.FeatureID
		born  map[entities.DeclKey]bool
		want  []entities.DeclKey
	}{
		{
			name:  "a test reading a reach 0 declaration nobody has built waits for it",
			decls: []entities.Decl{code, unborn, probe},
			edges: []entities.Edge{needs(probe, code), needs(probe, unborn)},
			owner: map[entities.DeclKey]reach.FeatureID{
				code.Key: "a", unborn.Key: shared, probe.Key: shared,
			},
			born: map[entities.DeclKey]bool{code.Key: true},
			want: nil,
		},
		{
			name:  "the same test lands at once when that declaration is built",
			decls: []entities.Decl{code, unborn, probe},
			edges: []entities.Edge{needs(probe, code), needs(probe, unborn)},
			owner: map[entities.DeclKey]reach.FeatureID{
				code.Key: "a", unborn.Key: shared, probe.Key: shared,
			},
			born: map[entities.DeclKey]bool{code.Key: true, unborn.Key: true},
			want: []entities.DeclKey{probe.Key},
		},
		{
			name:  "a test reading a test helper that lands with it stays",
			decls: []entities.Decl{code, aid, probe},
			edges: []entities.Edge{needs(probe, code), needs(probe, aid), needs(aid, code)},
			owner: map[entities.DeclKey]reach.FeatureID{
				code.Key: "a", aid.Key: shared, probe.Key: shared,
			},
			born: map[entities.DeclKey]bool{code.Key: true},
			want: []entities.DeclKey{probe.Key, aid.Key},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			deferred := reach.Wiring{}

			for _, member := range testCase.decls {
				if member.IsTest {
					deferred[member.Key] = true
				}
			}

			got := OwnTests(
				entities.NewGraph(testCase.decls, testCase.edges, nil),
				owners.Ownership{
					Owner: testCase.owner, Deferred: deferred, Roots: reach.Wiring{},
				},
				testCase.born,
				map[entities.DeclKey]bool{code.Key: true},
			)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ownTests() = %v, want %v", got, testCase.want)
			}
		})
	}
}
