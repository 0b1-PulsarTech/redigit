package apperr

import "errors"

var (
	// ErrIncomplete is a result the tool refuses to hand on: a measure with no
	// stated blind spot, a plan with unplaced declarations, an ordering that
	// still holds a cycle.
	ErrIncomplete = errors.New("incomplete result")
	// ErrUnresolved is a question the resolver could not answer exactly.
	// Guessing here is what identifier matching did, and it put 24 of 53
	// destinations in the wrong place.
	ErrUnresolved = errors.New("unresolved reference")
	// ErrInvariant is the final tree not matching the target.
	ErrInvariant = errors.New("final tree invariant failed")
)
