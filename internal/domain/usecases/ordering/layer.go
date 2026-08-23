package ordering

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// groupKey identifies one layer inside one feature. The feature index is part
// of the key because a layer applied across the history orders every kernel
// declaration before every engine: that builds the repository horizontally,
// with no working product until the end. A layer bounds only its own slice.
type groupKey struct {
	feature int
	layer   entities.Layer
}

// wholeHistory is the feature index every declaration gets when a run supplies
// no features at all.
const wholeHistory = 0

// applyLayers pulls an early layer's seed forward, in place: inside one
// feature, no declaration of a layer is seeded LATER than the earliest seed
// of any later layer of that same feature. See groupKey for why the scoping
// to a feature is the whole point.
func applyLayers(
	decls []entities.Decl,
	base map[entities.DeclKey]entities.Position,
	layers entities.Layers,
	features entities.Features,
) {
	if layers.Empty() {
		return
	}

	ceilings := layerCeilings(decls, base, layers, features)

	// Only ever LOWER a seed. Every dependency is enforced by bound taking a
	// max over the edges, so a seed that starts too early is raised back to
	// legality by the fixed point. Raising one instead could push a
	// declaration past something that needs it, uncaught.
	for _, decl := range decls {
		key, ok := groupOf(decl, base, layers, features)
		if !ok {
			continue
		}

		if ceiling, found := ceilings[key]; found {
			base[decl.Key] = min(base[decl.Key], ceiling)
		}
	}
}

// groupOf places a declaration in one feature and one layer. It reports false
// for a declaration no layer rule claims, and for one whose seed falls outside
// every feature: neither has a slice to be ordered inside of.
func groupOf(
	decl entities.Decl,
	base map[entities.DeclKey]entities.Position,
	layers entities.Layers,
	features entities.Features,
) (groupKey, bool) {
	layer := layers.LayerOf(decl.Key.File)
	if layer == entities.NoLayer {
		return groupKey{}, false
	}

	if features.Empty() {
		return groupKey{feature: wholeHistory, layer: layer}, true
	}

	index := features.IndexAt(base[decl.Key])
	if index < 0 {
		return groupKey{}, false
	}

	return groupKey{feature: index, layer: layer}, true
}

// layerCeilings maps each (feature, layer) to the earliest seed held by a
// LATER layer of the SAME feature. A layer with nothing above it inside its
// own feature gets no entry, because nothing there bounds it.
func layerCeilings(
	decls []entities.Decl,
	base map[entities.DeclKey]entities.Position,
	layers entities.Layers,
	features entities.Features,
) map[groupKey]entities.Position {
	earliest := earliestPerGroup(decls, base, layers, features)

	ceilings := make(map[groupKey]entities.Position, len(earliest))

	for key := range earliest {
		for other, at := range earliest {
			if other.feature != key.feature || other.layer <= key.layer {
				continue
			}

			if current, found := ceilings[key]; !found || at < current {
				ceilings[key] = at
			}
		}
	}

	return ceilings
}

func earliestPerGroup(
	decls []entities.Decl,
	base map[entities.DeclKey]entities.Position,
	layers entities.Layers,
	features entities.Features,
) map[groupKey]entities.Position {
	earliest := map[groupKey]entities.Position{}

	for _, decl := range decls {
		key, ok := groupOf(decl, base, layers, features)
		if !ok {
			continue
		}

		at := base[decl.Key]
		if current, found := earliest[key]; !found || at < current {
			earliest[key] = at
		}
	}

	return earliest
}
