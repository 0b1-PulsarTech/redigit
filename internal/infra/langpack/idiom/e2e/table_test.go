package e2e

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestRuleEdgesPickTheMostSpecificRoute(t *testing.T) {
	t.Parallel()

	var (
		set  = newRouteSet()
		me   = suiteKey("test", "TestMe")
		one  = suiteKey("test", "TestOne")
		hold = suiteKey("test", "TestHolder")
		bare = suiteKey("test", "TestBare")
		verb = suiteKey("test", "TestVerb")
		okay = suiteKey("test", "TestTyped")
		miss = suiteKey("test", "TestMiss")
	)

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:     "a literal route beats a parameter route",
			suite:    entities.Decl{Key: me},
			literals: []string{"/items/me"},
			want:     []entities.Edge{edgeTo(me, set.itemMe)},
		},
		{
			name:     "a value matches the parameter route",
			suite:    entities.Decl{Key: one},
			literals: []string{"/items/7"},
			want:     []entities.Edge{edgeTo(one, set.itemParam)},
		},
		{
			name:     "a placeholder matches the parameter route",
			suite:    entities.Decl{Key: hold},
			literals: []string{"/items/{other}"},
			want:     []entities.Edge{edgeTo(hold, set.itemParam)},
		},
		{
			name:     "a route call with no method word answers a bare path",
			suite:    entities.Decl{Key: bare},
			literals: []string{"/plain"},
			want:     []entities.Edge{edgeTo(bare, set.plain)},
		},
		{
			name:     "a route call with no method word answers any method",
			suite:    entities.Decl{Key: verb},
			literals: []string{"GET /plain"},
			want:     []entities.Edge{edgeTo(verb, set.plain)},
		},
		{
			name:     "a typed route answers its own method",
			suite:    entities.Decl{Key: okay},
			literals: []string{"GET /typed"},
			want:     []entities.Edge{edgeTo(okay, set.typed)},
		},
		{
			name:     "a typed route does not answer another method",
			suite:    entities.Decl{Key: miss},
			literals: []string{"POST /typed"},
			wantLog:  "POST /typed",
		},
	})
}
