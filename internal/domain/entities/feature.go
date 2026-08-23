package entities

import (
	"cmp"
	"slices"
)

// Feature is one vertical slice of the published history: the positions that
// together built one capability end to end. It is what makes a layer local -
// the feature says what gets built, the layer says in which order that
// feature's own pieces are born.
type Feature struct {
	Name       string
	Capability string

	// From and To are inclusive positions of the published history.
	From, To Position
}

// Holds reports whether at falls inside the feature.
func (f Feature) Holds(at Position) bool { return at >= f.From && at <= f.To }

// Features is a partition of the published history into vertical slices,
// ordered by From.
type Features struct {
	ordered []Feature
}

// NewFeatures sorts features by their first position so At can binary search.
func NewFeatures(features []Feature) Features {
	sorted := slices.Clone(features)
	slices.SortFunc(sorted, func(a, b Feature) int { return cmp.Compare(a.From, b.From) })

	return Features{ordered: sorted}
}

// Empty reports whether no feature was given, which is how a run says it has
// no vertical slicing to respect.
func (f Features) Empty() bool { return len(f.ordered) == 0 }

// All returns the features in position order.
func (f Features) All() []Feature { return slices.Clone(f.ordered) }

// IndexAt reports which feature holds a position, or -1 for a position no
// feature covers. The index, rather than the Feature, is what a caller groups
// by: two features may share a name and still be different slices.
func (f Features) IndexAt(at Position) int {
	for index, feature := range f.ordered {
		if feature.Holds(at) {
			return index
		}
	}

	return -1
}

// Overlaps reports the pairs of features that claim a shared position. A
// position belonging to two slices makes the grouping ambiguous, and IndexAt
// would silently answer with whichever sorted first.
func (f Features) Overlaps() [][2]Feature {
	var pairs [][2]Feature

	for index := 1; index < len(f.ordered); index++ {
		if previous, current := f.ordered[index-1], f.ordered[index]; current.From <= previous.To {
			pairs = append(pairs, [2]Feature{previous, current})
		}
	}

	return pairs
}
