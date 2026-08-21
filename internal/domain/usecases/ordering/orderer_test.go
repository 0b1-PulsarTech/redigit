package ordering

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestOrderUnitFloorUsesExternalReadersOnly(t *testing.T) {
	t.Parallel()

	sibling := entities.DeclKey{File: "repo/helper.go", Name: "helper"}
	target := entities.DeclKey{File: "repo/lead.go", Name: "New"}
	internalReader := entities.DeclKey{File: "repo/init.go", Name: "init"}
	externalReader := entities.DeclKey{File: "usecase/create.go", Name: "Create"}

	testCases := []struct {
		name  string
		edges []entities.Edge
		want  entities.Position
	}{
		{
			"a reader inside the unit does not pin the unit early",
			[]entities.Edge{
				{Dependent: internalReader, Provider: target, Reason: entities.ReasonReference},
			},
			// sibling's own base: internalReader touches its own file at 900,
			// clearly later than sibling's base of 1, so this only stays 1
			// when the unit floor truly ignores internal readers.
			1,
		},
		{
			"a reader outside the unit pins the whole unit at its position",
			[]entities.Edge{
				{Dependent: internalReader, Provider: target, Reason: entities.ReasonReference},
				{Dependent: externalReader, Provider: target, Reason: entities.ReasonReference},
			},
			487,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls := []entities.Decl{
				{Key: sibling, Unit: "repo"},
				{Key: target, Unit: "repo"},
				{Key: internalReader, Unit: "repo"},
				{Key: externalReader, Unit: "usecase"},
			}

			snap := entities.Snapshot{History: []entities.SourceTouch{
				{Position: 1, Touched: []string{"repo/helper.go", "repo/lead.go"}},
				{Position: 487, Touched: []string{"usecase/create.go"}},
				{Position: 900, Touched: []string{"repo/init.go"}},
			}}

			graph := entities.NewGraph(decls, testCase.edges, nil)

			result, err := New(Config{Slack: 8, RoundLimit: 2000}).Order(graph, snap)
			if err != nil {
				t.Fatalf("Order() err = %v, want nil", err)
			}

			if got := result.Of(sibling); got != testCase.want {
				t.Fatalf("sibling position = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestOrderAnchorPullsTestBackButStillRespectsWhatItReads(t *testing.T) {
	t.Parallel()

	subject := entities.DeclKey{File: "lead.go", Name: "Insert"}
	helper := entities.DeclKey{File: "helper.go", Name: "Validate"}
	test := entities.DeclKey{File: "lead_test.go", Name: "TestInsert"}

	decls := []entities.Decl{
		{Key: subject, Unit: "pkg"},
		{Key: helper, Unit: "pkg"},
		{Key: test, Unit: "pkg", IsTest: true},
	}
	edges := []entities.Edge{{Dependent: test, Provider: helper, Reason: entities.ReasonReference}}
	anchors := []entities.Anchor{{Decl: test, To: subject, Reason: entities.ReasonTestOf}}

	snap := entities.Snapshot{History: []entities.SourceTouch{
		{Position: 2, Touched: []string{"lead.go"}},
		{Position: 50, Touched: []string{"helper.go"}},
		{Position: 600, Touched: []string{"lead_test.go"}},
	}}

	graph := entities.NewGraph(decls, edges, anchors)

	result, err := New(Config{Slack: 8, RoundLimit: 2000}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() err = %v, want nil", err)
	}

	if got := result.Of(test); got != 50 {
		t.Fatalf(
			"test position = %d, want 50: pulled back from 600, still after what it reads",
			got,
		)
	}
}

func TestOrderNamesTwoNodeCycle(t *testing.T) {
	t.Parallel()

	a := entities.DeclKey{File: "a.go", Name: "A"}
	b := entities.DeclKey{File: "b.go", Name: "B"}

	graph := entities.NewGraph(
		[]entities.Decl{{Key: a}, {Key: b}},
		[]entities.Edge{
			{Dependent: a, Provider: b, Reason: entities.ReasonReference},
			{Dependent: b, Provider: a, Reason: entities.ReasonImplements},
		},
		nil,
	)

	result, err := New(Config{Slack: 8, RoundLimit: 2000}).Order(graph, entities.Snapshot{})
	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("Order() err = %v, want wrapping %v", err, apperr.ErrIncomplete)
	}

	if len(result.Cycles) != 1 {
		t.Fatalf("Cycles = %v, want exactly one", result.Cycles)
	}

	if len(result.Cycles[0].Members) != 2 || len(result.Cycles[0].Reasons) != 2 {
		t.Fatalf("cycle = %+v, want 2 members and 2 reasons named", result.Cycles[0])
	}
}

func TestOrderReportsUnresolvedAtLimit(t *testing.T) {
	t.Parallel()

	root := entities.DeclKey{File: "root.go", Name: "Root"}
	l1 := entities.DeclKey{File: "l1.go", Name: "L1"}
	l2 := entities.DeclKey{File: "l2.go", Name: "L2"}
	l3 := entities.DeclKey{File: "l3.go", Name: "L3"}

	decls := []entities.Decl{{Key: root}, {Key: l1}, {Key: l2}, {Key: l3}}
	edges := []entities.Edge{
		{Dependent: l1, Provider: root, Reason: entities.ReasonReference},
		{Dependent: l2, Provider: l1, Reason: entities.ReasonReference},
		{Dependent: l3, Provider: l2, Reason: entities.ReasonReference},
	}
	graph := entities.NewGraph(decls, edges, nil)

	// root and l2 are original history, at position 0; l1 and l3 are newly
	// inserted, so each hop from an original decl to an inserted one costs a
	// slot. A slack of 1 leaves no room for the second such hop.
	snap := entities.Snapshot{History: []entities.SourceTouch{
		{Position: 0, Touched: []string{"root.go", "l2.go"}},
	}}

	result, err := New(Config{Slack: 1, RoundLimit: 2000}).Order(graph, snap)
	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("Order() err = %v, want wrapping %v", err, apperr.ErrIncomplete)
	}

	var unresolvedErr *UnresolvedError
	if !errors.As(err, &unresolvedErr) {
		t.Fatalf("Order() err = %v, want wrapping *UnresolvedError", err)
	}

	want := []entities.DeclKey{l3}
	if !slices.Equal(unresolvedErr.Declarations, want) {
		t.Fatalf("UnresolvedError.Declarations = %v, want %v", unresolvedErr.Declarations, want)
	}

	if got := result.Of(l3); got != 1 {
		t.Fatalf("l3 pinned at = %d, want 1 (the limit)", got)
	}
}

func TestOrderWithNoHistorySucceedsFromDependencyAlone(t *testing.T) {
	t.Parallel()

	provider := entities.DeclKey{File: "a.go", Name: "A"}
	dependent := entities.DeclKey{File: "b.go", Name: "B"}

	graph := entities.NewGraph(
		[]entities.Decl{{Key: provider}, {Key: dependent}},
		[]entities.Edge{
			{Dependent: dependent, Provider: provider, Reason: entities.ReasonReference},
		},
		nil,
	)

	result, err := New(Config{Slack: 8, RoundLimit: 2000}).Order(graph, entities.Snapshot{})
	if err != nil {
		t.Fatalf("Order() err = %v, want nil", err)
	}

	if result.Of(dependent) < result.Of(provider) {
		t.Fatalf(
			"dependent = %d before provider = %d, with no history to blame",
			result.Of(dependent),
			result.Of(provider),
		)
	}
}

func TestOrderRoundGuardReturnsError(t *testing.T) {
	t.Parallel()

	const depth = 6

	root := entities.DeclKey{File: "z.go", Name: "root"}
	decls := []entities.Decl{{Key: root}}

	var edges []entities.Edge

	provider := root

	for i := 1; i <= depth; i++ {
		file := fmt.Sprintf("l%c.go", 'a'+depth-i)
		key := entities.DeclKey{File: file, Name: fmt.Sprintf("L%d", i)}
		decls = append(decls, entities.Decl{Key: key})
		edges = append(
			[]entities.Edge{{Dependent: key, Provider: provider, Reason: entities.ReasonReference}},
			edges...,
		)

		provider = key
	}

	graph := entities.NewGraph(decls, edges, nil)
	snap := entities.Snapshot{
		History: []entities.SourceTouch{{Position: 0, Touched: []string{root.File}}},
	}

	_, err := New(Config{Slack: 100, RoundLimit: 1}).Order(graph, snap)
	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("Order() err = %v, want wrapping %v", err, apperr.ErrIncomplete)
	}
}

func TestOrderIsMonotoneAboveBase(t *testing.T) {
	t.Parallel()

	a := entities.DeclKey{File: "a.go", Name: "A"}
	b := entities.DeclKey{File: "b.go", Name: "B"}
	c := entities.DeclKey{File: "c.go", Name: "C"}

	decls := []entities.Decl{{Key: a, Unit: "x"}, {Key: b, Unit: "y"}, {Key: c, Unit: "y"}}
	edges := []entities.Edge{
		{Dependent: b, Provider: a, Reason: entities.ReasonReference},
		{Dependent: c, Provider: b, Reason: entities.ReasonReference},
	}
	graph := entities.NewGraph(decls, edges, nil)

	snap := entities.Snapshot{History: []entities.SourceTouch{
		{Position: 0, Touched: []string{"a.go"}},
		{Position: 1, Touched: []string{"b.go"}},
		{Position: 2, Touched: []string{"c.go"}},
	}}

	result, err := New(Config{Slack: 8, RoundLimit: 2000}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() err = %v, want nil", err)
	}

	base, _ := seed(decls, snap)

	for _, decl := range decls {
		if got, want := result.Of(decl.Key), base[decl.Key]; got < want {
			t.Fatalf("%s ended at %d, before its base %d", decl.Key, got, want)
		}
	}
}
