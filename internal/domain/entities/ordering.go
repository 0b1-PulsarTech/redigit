package entities

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
