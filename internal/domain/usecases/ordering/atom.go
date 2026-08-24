package ordering

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// atomFloor is the position every member of an atom shares: the LATEST any of
// them reached. A file its tool wrote in one pass cannot arrive in pieces, and
// the earliest would put a declaration before something it needs while the
// latest only delays the ones that were ready.
type atomFloor map[string]entities.Position

// recomputeAtoms raises every atom member to the atom's own floor and reports
// whether anything moved.
func (e *engine) recomputeAtoms() bool {
	floors := atomFloor{}

	for _, decl := range e.decls {
		if decl.Atom == "" {
			continue
		}

		if at := e.assign[decl.Key]; at > floors[decl.Atom] {
			floors[decl.Atom] = at
		}
	}

	if len(floors) == 0 {
		return false
	}

	changed := false

	for _, decl := range e.decls {
		if decl.Atom == "" {
			continue
		}

		if e.raise(decl.Key, floors[decl.Atom]) {
			changed = true
		}
	}

	return changed
}
