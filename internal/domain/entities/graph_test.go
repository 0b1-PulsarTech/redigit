package entities

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func keysOf(edges []Edge) []string {
	out := make([]string, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edge.Dependent.String())
	}

	slices.Sort(out)

	return out
}

func sampleGraph() Graph {
	repoInsert := Decl{
		Key:  DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"},
		Unit: "repo",
	}
	repoHelper := Decl{
		Key:  DeclKey{File: "repo/lead.go", Kind: KindFunc, Name: "helper"},
		Unit: "repo",
	}
	port := Decl{
		Key:  DeclKey{File: "usecases/lead.go", Kind: KindType, Name: "Repository"},
		Unit: "usecases",
	}

	return NewGraph(
		[]Decl{repoInsert, repoHelper, port},
		[]Edge{
			{Dependent: repoInsert.Key, Provider: port.Key, Reason: ReasonImplements},
			{Dependent: repoHelper.Key, Provider: repoInsert.Key, Reason: ReasonReference},
			{Dependent: repoInsert.Key, Provider: repoHelper.Key, Reason: ReasonReference},
		},
		nil,
	)
}

func TestGraphNeeds(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()

	testCases := []struct {
		name string
		key  DeclKey
		want int
	}{
		{
			"a declaration that reads two things waits for both",
			DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"},
			2,
		},
		{
			"a provider nobody in this graph feeds waits for nothing",
			DeclKey{File: "usecases/lead.go", Kind: KindType, Name: "Repository"},
			0,
		},
		{"unknown key", DeclKey{File: "nope.go", Kind: KindFunc, Name: "X"}, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := len(graph.Needs(testCase.key)); got != testCase.want {
				t.Fatalf("Needs() = %d edges, want %d", got, testCase.want)
			}
		})
	}
}

func TestGraphExternalConsumers(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()
	insert := DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"}
	port := DeclKey{File: "usecases/lead.go", Kind: KindType, Name: "Repository"}

	testCases := []struct {
		name string
		key  DeclKey
		want []string
	}{
		{
			"a unit's own declarations are not outside readers",
			insert,
			nil,
		},
		{
			"a reader in another unit counts",
			port,
			[]string{insert.String()},
		},
		{"unknown key", DeclKey{File: "nope.go"}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := keysOf(graph.ExternalConsumers(testCase.key))
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ExternalConsumers() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestGraphConsumersCountsInsiders(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()
	insert := DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"}

	// Consumers is the unfiltered count on purpose. Using it where the
	// external one belongs is what pinned every package to the instant the
	// source history happened to create it.
	if got := len(graph.Consumers(insert)); got != 1 {
		t.Fatalf("Consumers() = %d, want 1", got)
	}

	if got := len(graph.ExternalConsumers(insert)); got != 0 {
		t.Fatalf("ExternalConsumers() = %d, want 0", got)
	}
}

func TestGraphDecl(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()

	testCases := []struct {
		name      string
		key       DeclKey
		wantFound bool
		wantUnit  UnitID
	}{
		{
			"known",
			DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"},
			true,
			"repo",
		},
		{"unknown", DeclKey{File: "nope.go", Kind: KindFunc, Name: "X"}, false, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl, found := graph.Decl(testCase.key)
			if found != testCase.wantFound {
				t.Fatalf("Decl() found = %v, want %v", found, testCase.wantFound)
			}

			if decl.Unit != testCase.wantUnit {
				t.Fatalf("Decl() unit = %q, want %q", decl.Unit, testCase.wantUnit)
			}
		})
	}
}

func permutedGraphs(withDangling bool) (Graph, Graph) {
	decls := make([]Decl, 0, 6)
	edges := make([]Edge, 0, 13)
	anchors := make([]Anchor, 0, 6)

	for index := range 6 {
		key := DeclKey{File: "f.go", Kind: KindFunc, Name: string(rune('a' + index))}
		next := DeclKey{File: "g.go", Kind: KindType, Name: string(rune('a' + (index+1)%6))}

		decls = append(decls, Decl{Key: key, Unit: "u"})
		edges = append(edges,
			Edge{Dependent: key, Provider: next, Reason: ReasonReference},
			Edge{Dependent: key, Provider: next, Reason: ReasonImplements},
		)

		anchors = append(anchors, Anchor{Decl: key, To: next, Reason: ReasonReference})
	}

	if withDangling {
		for _, name := range []string{"gone", "away"} {
			edges = append(edges, Edge{
				Dependent: DeclKey{File: "f.go", Kind: KindFunc, Name: "b"},
				Provider:  DeclKey{File: "outside.go", Kind: KindFunc, Name: name},
				Reason:    ReasonReference,
			})
		}
	}

	forward := NewGraph(decls, edges, anchors)

	slices.Reverse(decls)
	slices.Reverse(edges)
	slices.Reverse(anchors)

	return forward, NewGraph(decls, edges, anchors)
}

func TestNewGraphIgnoresInputOrder(t *testing.T) {
	t.Parallel()

	key := DeclKey{File: "g.go", Kind: KindType, Name: "c"}
	dependent := DeclKey{File: "f.go", Kind: KindFunc, Name: "b"}

	var testCases []struct {
		name string
		got  any
		want any
	}

	for name, withDangling := range map[string]bool{
		"every edge ends at a known declaration":    false,
		"an edge ends at a key no declaration owns": true,
	} {
		forward, backward := permutedGraphs(withDangling)

		for field, pair := range map[string][2]any{
			"decls":     {backward.Decls(), forward.Decls()},
			"edges":     {backward.Edges(), forward.Edges()},
			"anchors":   {backward.Anchors(), forward.Anchors()},
			"needs":     {backward.Needs(dependent), forward.Needs(dependent)},
			"consumers": {backward.Consumers(key), forward.Consumers(key)},
		} {
			testCases = append(testCases, struct {
				name string
				got  any
				want any
			}{name + "/" + field, pair[0], pair[1]})
		}
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !reflect.DeepEqual(testCase.got, testCase.want) {
				t.Fatalf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
			}
		})
	}
}

func TestNewGraphKeepsEdgesInKeyOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		withDangling bool
	}{
		{"every edge ends at a known declaration", false},
		{"an edge ends at a key no declaration owns", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, _ := permutedGraphs(testCase.withDangling)
			byDependent := func(left, right Edge) int {
				return cmp.Or(
					CompareKeys(left.Dependent, right.Dependent),
					CompareKeys(left.Provider, right.Provider),
					cmp.Compare(left.Reason, right.Reason),
				)
			}

			if !slices.IsSortedFunc(graph.Edges(), byDependent) {
				t.Fatalf("Edges() is not in key order: %v", graph.Edges())
			}

			for _, edge := range graph.Edges() {
				consumers := graph.Consumers(edge.Provider)
				if !slices.IsSortedFunc(consumers, byDependent) ||
					!slices.Contains(consumers, edge) {
					t.Fatalf(
						"Consumers(%v) = %v, want key order holding %v",
						edge.Provider,
						consumers,
						edge,
					)
				}
			}
		})
	}
}

func TestNewGraphLeavesInputsAlone(t *testing.T) {
	t.Parallel()

	low := DeclKey{File: "a.go", Kind: KindFunc, Name: "A"}
	high := DeclKey{File: "b.go", Kind: KindFunc, Name: "B"}

	testCases := []struct {
		name  string
		check func(graph Graph, decls []Decl, edges []Edge, anchors []Anchor) bool
	}{
		{"decls keep their order", func(_ Graph, decls []Decl, _ []Edge, _ []Anchor) bool {
			return decls[0].Key == high
		}},
		{"edges keep their order", func(_ Graph, _ []Decl, edges []Edge, _ []Anchor) bool {
			return edges[0].Dependent == high
		}},
		{"anchors keep their order", func(_ Graph, _ []Decl, _ []Edge, anchors []Anchor) bool {
			return anchors[0].Decl == high
		}},
		{
			"Decls shares no storage with the graph",
			func(graph Graph, _ []Decl, _ []Edge, _ []Anchor) bool {
				graph.Decls()[0].Unit = "changed"

				return graph.Decls()[0].Unit == ""
			},
		},
		{
			"Decls shares no storage with the input",
			func(graph Graph, decls []Decl, _ []Edge, _ []Anchor) bool {
				graph.Decls()[0].Unit = "changed"

				return decls[0].Unit == ""
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls := []Decl{{Key: high}, {Key: low}}
			edges := []Edge{{Dependent: high, Provider: low}, {Dependent: low, Provider: high}}
			anchors := []Anchor{{Decl: high, To: low}, {Decl: low, To: high}}

			if !testCase.check(NewGraph(decls, edges, anchors), decls, edges, anchors) {
				t.Fatal("NewGraph() disturbed its input")
			}
		})
	}
}

func BenchmarkNewGraph(b *testing.B) {
	const (
		declCount = 2000
		edgeCount = 20000
	)

	testCases := []struct {
		name         string
		danglingStep int
	}{
		{"known", 0},
		{"dangling", 5},
	}

	for _, testCase := range testCases {
		decls := make([]Decl, declCount)
		edges := make([]Edge, edgeCount)

		for index := range decls {
			decls[index] = Decl{Key: DeclKey{
				File: fmt.Sprintf("p%d/f.go", (declCount-index)%97),
				Kind: KindFunc,
				Name: fmt.Sprintf("N%d", index),
			}}
		}

		for index := range edges {
			edges[index] = Edge{
				Dependent: decls[(index*7919)%declCount].Key,
				Provider:  decls[(index*104729+1)%declCount].Key,
				Reason:    ReasonReference,
			}

			if testCase.danglingStep > 0 && index%testCase.danglingStep == 0 {
				edges[index].Provider = DeclKey{
					File: "gone.go",
					Kind: KindFunc,
					Name: fmt.Sprint("G", index%900),
				}
			}
		}

		b.Run(testCase.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				NewGraph(decls, edges, nil)
			}
		})
	}
}

func TestNewGraphCollidingStrings(t *testing.T) {
	t.Parallel()

	first := DeclKey{File: "x:func", Kind: KindType, Name: "T"}
	second := DeclKey{File: "x", Kind: KindFunc, Name: "type:T"}

	testCases := []struct {
		name      string
		decls     []Decl
		edges     []Edge
		wantLen   int
		wantFirst string
	}{
		{
			"edge from the later key to the earlier",
			[]Decl{{Key: first, Atom: "1"}, {Key: second, Atom: "2"}, {Key: first, Atom: "3"}},
			[]Edge{{Dependent: second, Provider: first, Reason: ReasonReference}},
			2,
			"1",
		},
		{
			"edges both ways",
			[]Decl{{Key: second, Atom: "2"}, {Key: first, Atom: "1"}},
			[]Edge{
				{Dependent: first, Provider: second, Reason: ReasonReference},
				{Dependent: second, Provider: first, Reason: ReasonReference},
			},
			2,
			"1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := NewGraph(slices.Clone(testCase.decls), slices.Clone(testCase.edges), nil)
			if graph.Len() != testCase.wantLen {
				t.Fatalf("Len() = %d, want %d", graph.Len(), testCase.wantLen)
			}

			if kept, _ := graph.Decl(first); kept.Atom != testCase.wantFirst {
				t.Fatalf("Decl(first).Atom = %q, want %q", kept.Atom, testCase.wantFirst)
			}

			decls, edges := slices.Clone(testCase.decls), slices.Clone(testCase.edges)
			slices.Reverse(decls)
			slices.Reverse(edges)

			reversed := NewGraph(decls, edges, nil)
			if !reflect.DeepEqual(reversed.Edges(), graph.Edges()) ||
				!reflect.DeepEqual(reversed.Decls(), graph.Decls()) {
				t.Fatalf(
					"reversed Decls() = %v, Edges() = %v; want %v, %v",
					reversed.Decls(), reversed.Edges(), graph.Decls(), graph.Edges(),
				)
			}
		})
	}
}
