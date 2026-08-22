package auditing

import (
	"cmp"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Drift is one declaration born far from whatever needs it, and the reason the
// graph gives for the wait. The reason is what separates a defect from a
// dependency nobody named: guessing between the two from outside cost the
// previous rebuild a whole round.
type Drift struct {
	Decl      entities.DeclKey
	BornAt    entities.Position
	FirstRead entities.Position
	Reader    entities.DeclKey
	Reason    entities.EdgeReason
}

// Gap is how many positions separate a declaration from its first reader.
func (d Drift) Gap() int { return int(d.FirstRead - d.BornAt) }

// firstReader is the earliest reader of a declaration, and the edge that made
// it one. Readers inside the same unit are skipped: counting them made a
// package look consumed the instant the source history created it, which is
// the largest single correction the previous rebuild made.
func firstReader(
	graph entities.Graph,
	order entities.Ordering,
	key entities.DeclKey,
) (entities.Edge, entities.Position, bool) {
	var (
		best  entities.Edge
		at    = entities.NoPosition
		found bool
	)

	edges := slices.Clone(graph.ExternalConsumers(key))
	slices.SortFunc(edges, func(a, b entities.Edge) int {
		return cmp.Compare(a.Dependent.String(), b.Dependent.String())
	})

	for _, edge := range edges {
		reader := order.Of(edge.Dependent)
		if !reader.IsSet() {
			continue
		}

		if !found || reader < at {
			best, at, found = edge, reader, true
		}
	}

	return best, at, found
}

// drifts lists every declaration whose first outside reader lands more than
// farApart positions later, worst first.
func drifts(graph entities.Graph, order entities.Ordering, farApart int) []Drift {
	var out []Drift

	for _, decl := range graph.Decls() {
		born := order.Of(decl.Key)
		if !born.IsSet() {
			continue
		}

		edge, at, found := firstReader(graph, order, decl.Key)
		if !found || int(at-born) <= farApart {
			continue
		}

		out = append(out, Drift{
			Decl:      decl.Key,
			BornAt:    born,
			FirstRead: at,
			Reader:    edge.Dependent,
			Reason:    edge.Reason,
		})
	}

	slices.SortFunc(out, func(a, b Drift) int {
		if by := cmp.Compare(b.Gap(), a.Gap()); by != 0 {
			return by
		}

		return cmp.Compare(a.Decl.String(), b.Decl.String())
	})

	return out
}
