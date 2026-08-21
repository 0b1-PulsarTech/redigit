package ordering

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// applyAnchors lowers base toward each Anchor's target before the loop
// starts. An anchor pulls backward: "not before the function" is a floor the
// loop still enforces through edges, but with only that floor a package's
// whole test suite once arrived six hundred commits late, under someone
// else's commit subject.
//
// It repeats the pass until nothing changes, bounded by len(anchors), because
// anchors can chain: a test can anchor to a helper that itself anchors to
// something else, and a single unordered pass would leave the chain's start
// seeing its middle link's stale, pre-anchor base depending on slice
// iteration order. This stays a small, finite, pre-loop-only fixed point; it
// is not the monotone round loop and never runs inside it.
func applyAnchors(graph entities.Graph, base map[entities.DeclKey]entities.Position) {
	anchors := graph.Anchors()

	for range anchors {
		changed := false

		for _, anchor := range anchors {
			to, found := base[anchor.To]
			if !found {
				continue
			}

			if current, known := base[anchor.Decl]; !known || to < current {
				base[anchor.Decl] = to
				changed = true
			}
		}

		if !changed {
			return
		}
	}
}
