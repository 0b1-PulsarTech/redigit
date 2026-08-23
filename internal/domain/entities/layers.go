package entities

import (
	"cmp"
	"slices"
	"strings"
)

// Layer is how early a path's declarations should be born when the dependency
// graph is indifferent. Lower comes first. It is a tiebreak and never a
// constraint: the graph decides what MUST come before what, and a layer only
// chooses among placements the graph already allows.
type Layer int

// NoLayer is the layer of a path no rule matches. Such a path keeps whatever
// the history gave it.
const NoLayer Layer = -1

// LayerRule assigns a Layer to every path under Prefix.
type LayerRule struct {
	Prefix string
	Layer  Layer
}

// Layers answers which layer a path belongs to. The longest matching prefix
// wins, so "libs/infra" can sit late while "libs/engines" sits early even
// though both start with "libs".
type Layers struct {
	rules []LayerRule
}

// NewLayers sorts rules longest-prefix-first so LayerOf can return the first
// match instead of scanning for the best one.
func NewLayers(rules []LayerRule) Layers {
	sorted := slices.Clone(rules)
	slices.SortFunc(sorted, func(a, b LayerRule) int {
		if byLength := cmp.Compare(len(b.Prefix), len(a.Prefix)); byLength != 0 {
			return byLength
		}

		return strings.Compare(a.Prefix, b.Prefix)
	})

	return Layers{rules: sorted}
}

// Empty reports whether no rule was given, which is how a run says it wants
// no layer tiebreak at all.
func (l Layers) Empty() bool { return len(l.rules) == 0 }

// Rules returns the rules in longest-prefix-first order.
func (l Layers) Rules() []LayerRule { return slices.Clone(l.rules) }

// LayerOf reports path's layer, or NoLayer when no rule matches.
//
// A prefix matches a whole path element, never half of one: "libs/eng" does
// not claim "libs/engines". Matching on raw string prefix would make one rule
// silently swallow a sibling directory whose name it happens to start.
func (l Layers) LayerOf(path string) Layer {
	for _, rule := range l.rules {
		if path == rule.Prefix || strings.HasPrefix(path, rule.Prefix+"/") {
			return rule.Layer
		}
	}

	return NoLayer
}
