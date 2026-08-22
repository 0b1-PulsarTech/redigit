package auditing

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func decl(file, name string, unit entities.UnitID) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: unit,
	}
}

func needs(dependent, provider entities.Decl, reason entities.EdgeReason) entities.Edge {
	return entities.Edge{Dependent: dependent.Key, Provider: provider.Key, Reason: reason}
}

// wiring is the shape the whole audit exists to catch: configuration born
// early and read only at the composition root, hundreds of commits later.
func wiring() (entities.Graph, entities.Ordering, map[string]entities.Decl) {
	config := decl("apps/molecwire/config.go", "Environment", "apps/molecwire")
	local := decl("apps/molecwire/config.go", "defaults", "apps/molecwire")
	root := decl("cmd/app/main.go", "main", "cmd/app")
	near := decl("libs/kit/id.go", "NewID", "libs/kit")
	nearReader := decl("libs/lead/lead.go", "New", "libs/lead")

	decls := []entities.Decl{config, local, root, near, nearReader}
	edges := []entities.Edge{
		needs(local, config, entities.ReasonReference),
		needs(root, config, entities.ReasonReference),
		needs(nearReader, near, entities.ReasonReference),
	}

	at := map[entities.DeclKey]entities.Position{
		config.Key:     129,
		local.Key:      130,
		near.Key:       400,
		nearReader.Key: 405,
		root.Key:       738,
	}

	return entities.NewGraph(decls, edges, nil),
		entities.Ordering{At: at},
		map[string]entities.Decl{
			"config": config, "local": local, "root": root,
			"near": near, "nearReader": nearReader,
		}
}

func TestDrifts(t *testing.T) {
	t.Parallel()

	graph, order, named := wiring()
	found := drifts(graph, order, 25)

	if len(found) != 1 {
		t.Fatalf("found %d drifts, want 1: %v", len(found), found)
	}

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{"the drifting declaration", found[0].Decl, named["config"].Key},
		{"born where the ordering put it", found[0].BornAt, entities.Position(129)},
		{
			"first read at the composition root, not by its own package",
			found[0].FirstRead,
			entities.Position(738),
		},
		{"and it names which reader", found[0].Reader, named["root"].Key},
		{"and why that reader waits", found[0].Reason, entities.ReasonReference},
		{"the gap is reported, not just the fact", found[0].Gap(), 609},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.got != testCase.want {
				t.Fatalf("got %v, want %v", testCase.got, testCase.want)
			}
		})
	}
}

func TestDriftsIgnoresInsiders(t *testing.T) {
	t.Parallel()

	graph, order, named := wiring()

	// The declaration's own package reads it at 130, one position later. If
	// insiders counted, the gap would read as 1 and the real 609 would vanish.
	edge, at, found := firstReader(graph, order, named["config"].Key)
	if !found {
		t.Fatal("no outside reader found")
	}

	if at != 738 {
		t.Fatalf("first reader at %d, want 738; an insider was counted", at)
	}

	if edge.Dependent != named["root"].Key {
		t.Fatalf("reader = %v, want the composition root", edge.Dependent)
	}
}

func TestDriftsThreshold(t *testing.T) {
	t.Parallel()

	graph, order, _ := wiring()

	testCases := []struct {
		name     string
		farApart int
		want     int
	}{
		{"a wide threshold reports only the worst", 600, 1},
		{"wider than the worst gap reports nothing", 700, 0},
		{"a narrow threshold also catches the five-position pair", 2, 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := len(drifts(graph, order, testCase.farApart)); got != testCase.want {
				t.Fatalf("drifts = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestFirstReaderWithoutReaders(t *testing.T) {
	t.Parallel()

	lonely := decl("z/dead.go", "Unused", "z")
	graph := entities.NewGraph([]entities.Decl{lonely}, nil, nil)
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{lonely.Key: 1}}

	if _, _, found := firstReader(graph, order, lonely.Key); found {
		t.Fatal("a declaration nothing reads reported a first reader")
	}

	if got := drifts(graph, order, 1); len(got) != 0 {
		t.Fatalf("drifts = %v, want none; nothing to be far from", got)
	}
}
