package ordering

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestNewEngineSeedsAssignFromBase(t *testing.T) {
	t.Parallel()

	touched := entities.Decl{Key: entities.DeclKey{File: "a.go", Name: "A"}, Unit: "pkg"}
	snap := entities.Snapshot{
		History: []entities.SourceTouch{{Position: 4, Touched: []string{"a.go"}}},
	}
	graph := entities.NewGraph([]entities.Decl{touched}, nil, nil)

	eng := newEngine(graph, snap, 8)

	if got := eng.assign[touched.Key]; got != 4 {
		t.Fatalf("assign seeded from base = %d, want 4", got)
	}

	if got := eng.limit; got != 12 {
		t.Fatalf("limit = %d, want 12", got)
	}
}

func TestEngineOrderingCopiesAssign(t *testing.T) {
	t.Parallel()

	key := entities.DeclKey{File: "a.go", Name: "A"}
	eng := &engine{assign: map[entities.DeclKey]entities.Position{key: 5}}

	result := eng.ordering(3, nil)
	result.At[key] = 99

	if got := eng.assign[key]; got != 5 {
		t.Fatalf("ordering() leaked its map, engine assign = %d, want 5", got)
	}

	if result.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3", result.Rounds)
	}
}

func TestEngineUnresolved(t *testing.T) {
	t.Parallel()

	dependent := entities.DeclKey{File: "b.go", Name: "B"}
	provider := entities.DeclKey{File: "a.go", Name: "A"}
	graph := entities.NewGraph(
		[]entities.Decl{{Key: dependent, Unit: "b"}, {Key: provider, Unit: "a"}},
		[]entities.Edge{
			{Dependent: dependent, Provider: provider, Reason: entities.ReasonReference},
		},
		nil,
	)

	testCases := []struct {
		name   string
		assign map[entities.DeclKey]entities.Position
		limit  entities.Position
		want   []entities.DeclKey
	}{
		{
			"within limit, nothing is unresolved",
			map[entities.DeclKey]entities.Position{provider: 2, dependent: 2},
			10,
			nil,
		},
		{
			"a bound that exceeds the limit is reported",
			map[entities.DeclKey]entities.Position{provider: 20, dependent: 20},
			5,
			[]entities.DeclKey{dependent},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			eng := &engine{
				graph:  graph,
				decls:  graph.Decls(),
				base:   map[entities.DeclKey]entities.Position{},
				assign: testCase.assign,
				limit:  testCase.limit,
			}

			got := eng.unresolved()
			if len(got) != len(testCase.want) {
				t.Fatalf("unresolved() = %v, want %v", got, testCase.want)
			}

			for i, key := range testCase.want {
				if got[i] != key {
					t.Fatalf("unresolved()[%d] = %v, want %v", i, got[i], key)
				}
			}
		})
	}
}
