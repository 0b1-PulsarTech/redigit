package ordering

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// compareKeys orders two declaration keys by their stable text form, which is
// what every deterministic sort in this package sorts on: a map iterates in
// random order, and a rewrite of a repository's history has to be
// reproducible run to run.
func compareKeys(a, b entities.DeclKey) int {
	return strings.Compare(a.String(), b.String())
}
