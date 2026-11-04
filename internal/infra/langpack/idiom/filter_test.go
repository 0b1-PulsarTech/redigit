package idiom

import (
	"bytes"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func linked(dependent, provider string, reason entities.EdgeReason) entities.Edge {
	return entities.Edge{
		Dependent: goDecl(dependent).Key,
		Provider:  goDecl(provider).Key,
		Reason:    reason,
	}
}

func loggedPack() (Pack, *bytes.Buffer) {
	logs := &bytes.Buffer{}
	handler := slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn})

	return NewPack(nil, nil).WithLogger(slog.New(handler)), logs
}

func filterTwice(pack Pack, edges []entities.Edge) []entities.Edge {
	var got []entities.Edge

	for range 2 {
		got = pack.FilterEdges(entities.Snapshot{}, edges)
	}

	return got
}

func TestPackFilterEdgesKeeps(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		edges []entities.Edge
	}{
		{name: "no edges"},
		{
			name: "a cycle with no DI edge is left to the orderer",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonReference),
				linked("B", "A", entities.ReasonReference),
			},
		},
		{
			name: "a DI edge whose provider cannot reach its dependent",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("B", "C", entities.ReasonReference),
			},
		},
		{
			name: "a DI edge between two components beside a cycle elsewhere",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("B", "C", entities.ReasonReference),
				linked("C", "B", entities.ReasonReference),
			},
		},
		{
			name: "a registration in a helper that package main hosts",
			edges: []entities.Edge{
				linked("setup", "main", entities.ReasonHosts),
				linked("main", "setup", entities.ReasonReference),
				linked("main", "Run", entities.ReasonReference),
				linked("Run", "setup", entities.ReasonRegistered),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pack, logs := loggedPack()
			got := filterTwice(pack, testCase.edges)
			logged := strings.Count(logs.String(), "droppedEdges=")

			if !slices.Equal(got, testCase.edges) || logged != 0 {
				t.Fatalf("FilterEdges() = %v, logged %d, want %v and no log",
					got, logged, testCase.edges)
			}
		})
	}
}

type dropCase struct {
	name     string
	edges    []entities.Edge
	want     []entities.Edge
	isLogged bool
}

func requireDropped(t *testing.T, testCases []dropCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			wantLogged := 0
			if testCase.isLogged {
				wantLogged = 1
			}

			pack, logs := loggedPack()
			if !testCase.isLogged {
				pack = NewPack(nil, nil)
			}

			got := filterTwice(pack, testCase.edges)
			logged := strings.Count(logs.String(), "droppedEdges=")

			if !slices.Equal(got, testCase.want) || logged != wantLogged {
				t.Fatalf("FilterEdges() = %v, logged %d, want %v, logged %d",
					got, logged, testCase.want, wantLogged)
			}
		})
	}
}

func TestPackFilterEdgesDropsThroughOtherEdges(t *testing.T) {
	t.Parallel()

	manifest := entities.DeclKey{File: "go.mod", Kind: entities.KindOther, Name: "module"}

	requireDropped(t, []dropCase{
		{
			name: "a provider reaching the dependent by references",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("B", "C", entities.ReasonReference),
				linked("C", "A", entities.ReasonReference),
			},
			want: []entities.Edge{
				linked("B", "C", entities.ReasonReference),
				linked("C", "A", entities.ReasonReference),
			},
			isLogged: true,
		},
		{
			name: "a registered edge through implements and method sets",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonRegistered),
				linked("B", "C", entities.ReasonImplements),
				linked("C", "A", entities.ReasonMethodSet),
			},
			want: []entities.Edge{
				linked("B", "C", entities.ReasonImplements),
				linked("C", "A", entities.ReasonMethodSet),
			},
			isLogged: true,
		},
		{
			name: "a DI edge between two ends of a cycle of other edges",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("A", "B", entities.ReasonReference),
				linked("B", "A", entities.ReasonReference),
			},
			want: []entities.Edge{
				linked("A", "B", entities.ReasonReference),
				linked("B", "A", entities.ReasonReference),
			},
			isLogged: true,
		},
		{
			name: "a provider reaching the dependent through the manifest that hosts it",
			edges: []entities.Edge{
				{Dependent: goDecl("A").Key, Provider: manifest, Reason: entities.ReasonHosts},
				{Dependent: manifest, Provider: goDecl("B").Key, Reason: entities.ReasonReference},
				linked("B", "A", entities.ReasonRegistered),
			},
			want: []entities.Edge{
				{Dependent: goDecl("A").Key, Provider: manifest, Reason: entities.ReasonHosts},
				{Dependent: manifest, Provider: goDecl("B").Key, Reason: entities.ReasonReference},
			},
			isLogged: true,
		},
	})
}

func TestPackFilterEdgesDropsAmongDIEdges(t *testing.T) {
	t.Parallel()

	self := []entities.Edge{linked("A", "A", entities.ReasonProvides)}

	requireDropped(t, []dropCase{
		{
			name: "the second of two DI edges that close each other",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("B", "A", entities.ReasonProvides),
			},
			want:     []entities.Edge{linked("A", "B", entities.ReasonProvides)},
			isLogged: true,
		},
		{
			name: "a DI edge kept earlier closes a later one",
			edges: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("B", "C", entities.ReasonRegistered),
				linked("C", "A", entities.ReasonBody),
			},
			want: []entities.Edge{
				linked("A", "B", entities.ReasonProvides),
				linked("C", "A", entities.ReasonBody),
			},
			isLogged: true,
		},
		{name: "a declaration to itself", edges: self, isLogged: true},
		{name: "a drop with no logger", edges: self},
	})
}

func TestPackFilterEdgesReportsTheCountOfEachPass(t *testing.T) {
	t.Parallel()

	one := []entities.Edge{linked("A", "A", entities.ReasonProvides)}
	two := []entities.Edge{
		linked("A", "A", entities.ReasonProvides), linked("B", "B", entities.ReasonProvides),
	}

	testCases := []struct {
		name   string
		passes [][]entities.Edge
		want   []string
	}{
		{
			name:   "a count that grows is reported again",
			passes: [][]entities.Edge{one, two},
			want:   []string{"droppedEdges=1", "droppedEdges=2"},
		},
		{
			name:   "a cut that clears the drop is said to be gone",
			passes: [][]entities.Edge{one, nil},
			want:   []string{"droppedEdges=1", "no edge dropped on the latest pass"},
		},
		{
			name:   "the same count is reported once",
			passes: [][]entities.Edge{one, one},
			want:   []string{"droppedEdges=1"},
		},
		{
			name:   "a graph that never drops says nothing",
			passes: [][]entities.Edge{nil, nil},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pack, logs := loggedPack()

			for _, edges := range testCase.passes {
				pack.FilterEdges(entities.Snapshot{}, edges)
			}

			var lines []string
			if logs.Len() > 0 {
				lines = strings.Split(strings.TrimSpace(logs.String()), "\n")
			}

			if len(lines) != len(testCase.want) {
				t.Fatalf("logged %q, want %d lines", logs.String(), len(testCase.want))
			}

			for at, want := range testCase.want {
				if !strings.Contains(lines[at], want) {
					t.Fatalf("line %d = %q, want it to hold %q", at, lines[at], want)
				}
			}
		})
	}
}

func BenchmarkFilterEdges(b *testing.B) {
	const (
		plainEdges = 20000
		diEdges    = 1000
	)

	for _, shape := range []struct {
		name    string
		nodes   int
		isRing  bool
		isChain bool
	}{
		{name: "dag10k", nodes: 10000},
		{name: "scc2k", nodes: 2000, isRing: true},
		{name: "scc10k", nodes: 10000, isRing: true},
		{name: "cycle10kThroughDI", nodes: 10000, isChain: true},
	} {
		b.Run(shape.name, func(b *testing.B) {
			rng := rand.New(rand.NewPCG(7, 11))
			key := func(at int) entities.DeclKey {
				name := strconv.Itoa(at)

				return entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: name}
			}
			edges := make([]entities.Edge, 0, plainEdges+diEdges)

			for at := range shape.nodes {
				next := at + 1
				if shape.isRing {
					next %= shape.nodes
				}

				if (shape.isRing || shape.isChain) && next < shape.nodes {
					edges = append(edges, entities.Edge{
						Dependent: key(at), Provider: key(next), Reason: entities.ReasonReference,
					})
				}
			}

			for len(edges) < plainEdges {
				from, to := rng.IntN(shape.nodes), rng.IntN(shape.nodes)
				if !shape.isRing && from >= to {
					continue
				}

				edges = append(edges, entities.Edge{
					Dependent: key(from), Provider: key(to), Reason: entities.ReasonReference,
				})
			}

			for range diEdges {
				edges = append(edges, entities.Edge{
					Dependent: key(rng.IntN(shape.nodes)), Provider: key(rng.IntN(shape.nodes)),
					Reason: entities.ReasonRegistered,
				})
			}

			pack := NewPack(nil, nil)

			b.ReportAllocs()

			for b.Loop() {
				pack.FilterEdges(entities.Snapshot{}, edges)
			}

			b.ReportMetric(float64(b.Elapsed().Milliseconds())/float64(b.N), "ms/filter")
		})
	}
}
