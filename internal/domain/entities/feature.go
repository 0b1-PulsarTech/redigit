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

	// At is a SET, not a range. A capability whose schema was published at 17,
	// its domain at 258 and its use case at 575 is one slice at three distant
	// positions, and a range spanning them would swallow the hundreds in
	// between that belong to other capabilities.
	At []Position
}

// Holds reports whether at is one of the feature's positions.
func (f Feature) Holds(at Position) bool { return slices.Contains(f.At, at) }

// First reports the earliest position the feature covers, or NoPosition when
// it covers none.
func (f Feature) First() Position {
	first := NoPosition

	for _, at := range f.At {
		if !first.IsSet() || at < first {
			first = at
		}
	}

	return first
}

// Features is a partition of the published history into vertical slices.
type Features struct {
	ordered []Feature
	index   map[Position]int
	clashes [][2]Feature
}

// NewFeatures sorts features by their earliest position and builds the
// position lookup. A position two features both claim is recorded as a clash
// rather than silently resolved: the first claimant keeps it, and Overlaps
// reports the pair so a caller can refuse the whole set.
func NewFeatures(features []Feature) Features {
	sorted := slices.Clone(features)
	slices.SortFunc(sorted, func(a, b Feature) int { return cmp.Compare(a.First(), b.First()) })

	index := map[Position]int{}
	reported := map[[2]int]bool{}

	var clashes [][2]Feature

	for position, feature := range sorted {
		for _, at := range feature.At {
			claimed, taken := index[at]
			if !taken {
				index[at] = position

				continue
			}

			// One pair, however many positions they share: a caller reads this
			// to name the two slices, and repeating them per position buries
			// the second clashing pair under the first one's positions.
			if pair := [2]int{claimed, position}; !reported[pair] {
				reported[pair] = true

				clashes = append(clashes, [2]Feature{sorted[claimed], feature})
			}
		}
	}

	return Features{ordered: sorted, index: index, clashes: clashes}
}

// Empty reports whether no feature was given, which is how a run says it has
// no vertical slicing to respect.
func (f Features) Empty() bool { return len(f.ordered) == 0 }

// All returns the features ordered by their earliest position.
func (f Features) All() []Feature { return slices.Clone(f.ordered) }

// IndexAt reports which feature holds a position, or -1 for a position no
// feature covers. The index, rather than the Feature, is what a caller groups
// by: two features may share a name and still be different slices.
func (f Features) IndexAt(at Position) int {
	if index, found := f.index[at]; found {
		return index
	}

	return -1
}

// Covered reports how many distinct positions the features cover, which is
// what a caller compares against the history's length to size the gap.
func (f Features) Covered() int { return len(f.index) }

// Overlaps reports the pairs of features that claim a shared position. A
// position belonging to two slices makes the grouping ambiguous, and IndexAt
// would silently answer with whichever sorted first.
func (f Features) Overlaps() [][2]Feature { return slices.Clone(f.clashes) }
