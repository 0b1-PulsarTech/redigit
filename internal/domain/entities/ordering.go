package entities

import (
	"cmp"
	"slices"
	"strings"
)

// Position is an index into the commit order being built. It is not a commit
// count: the order carries spare slots past the source history's end, because
// the last commits introduce declarations that read each other back and every
// such turn needs a slot of its own.
type Position int

// NoPosition marks a declaration nothing has placed yet. Zero is a real
// position (the first commit), so absence needs a value of its own.
const NoPosition Position = -1

func (p Position) IsSet() bool { return p >= 0 }

// Cycle is a set of declarations that wait on each other. Naming it is the
// point: a cycle the tool hides becomes a declaration silently pinned at the
// last slot, which reads as convergence and is not.
type Cycle struct {
	Members []DeclKey
	Reasons []EdgeReason
}

// SpansFiles reports whether the cycle reaches across more than one file.
//
// One inside a single file is mutual recursion: the members share a position
// and the file is written with all of them. One that crosses files is the real
// hazard, and reporting both alike buries it - a real repository produced 240
// single-file cycles, every one a generated mock and its recorder.
func (c Cycle) SpansFiles() bool {
	for _, member := range c.Members[1:] {
		if member.File != c.Members[0].File {
			return true
		}
	}

	return false
}

// Ordering is where every declaration landed, plus what could not be resolved.
// Cycles are returned rather than logged so a caller can refuse to write a
// history that still holds one.
type Ordering struct {
	At     map[DeclKey]Position
	Cycles []Cycle
	Rounds int
}

func (o Ordering) Of(key DeclKey) Position {
	if at, found := o.At[key]; found {
		return at
	}

	return NoPosition
}

// Last reports the highest position any declaration reached, which bounds the
// commit range the writer has to emit.
func (o Ordering) Last() Position {
	last := NoPosition
	for _, at := range o.At {
		if at > last {
			last = at
		}
	}

	return last
}

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
