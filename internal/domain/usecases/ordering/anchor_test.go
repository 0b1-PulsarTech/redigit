package ordering

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestApplyAnchors(t *testing.T) {
	t.Parallel()

	subject := entities.DeclKey{File: "lead.go", Kind: entities.KindFunc, Name: "Insert"}
	test := entities.DeclKey{File: "lead_test.go", Kind: entities.KindFunc, Name: "TestInsert"}
	orphan := entities.Anchor{
		Decl: entities.DeclKey{File: "gone.go"},
		To:   entities.DeclKey{File: "nope.go"},
	}

	x := entities.DeclKey{File: "x.go", Kind: entities.KindFunc, Name: "X"}
	y := entities.DeclKey{File: "y.go", Kind: entities.KindFunc, Name: "Y"}
	z := entities.DeclKey{File: "z.go", Kind: entities.KindFunc, Name: "Z"}

	testCases := []struct {
		name    string
		anchors []entities.Anchor
		base    map[entities.DeclKey]entities.Position
		want    entities.Position
	}{
		{
			"an anchor pulls a late test back to its subject",
			[]entities.Anchor{{Decl: test, To: subject, Reason: entities.ReasonTestOf}},
			map[entities.DeclKey]entities.Position{subject: 3, test: 600},
			3,
		},
		{
			"an anchor never raises the base, only lowers it",
			[]entities.Anchor{{Decl: test, To: subject, Reason: entities.ReasonTestOf}},
			map[entities.DeclKey]entities.Position{subject: 900, test: 3},
			3,
		},
		{
			"an anchor to a declaration outside the graph is a no-op",
			[]entities.Anchor{orphan},
			map[entities.DeclKey]entities.Position{},
			entities.NoPosition,
		},
		{
			"a two-hop anchor chain resolves through both links",
			[]entities.Anchor{
				{Decl: x, To: y, Reason: entities.ReasonTestOf},
				{Decl: y, To: z, Reason: entities.ReasonTestOf},
			},
			map[entities.DeclKey]entities.Position{x: 900, y: 5, z: 2},
			2, // x inherits z's base, not y's stale pre-anchor base of 5
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(nil, nil, testCase.anchors)
			applyAnchors(graph, testCase.base)

			var target entities.DeclKey
			if len(testCase.anchors) > 0 {
				target = testCase.anchors[0].Decl
			}

			got, found := testCase.base[target]
			if !found {
				got = entities.NoPosition
			}

			if got != testCase.want {
				t.Fatalf("base[%v] = %d, want %d", target, got, testCase.want)
			}
		})
	}
}
