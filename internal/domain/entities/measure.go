package entities

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
)

// Measure is a measurement and what it did NOT look at. Excluded is required,
// not decorative: rebuilding a history by hand produced eleven bad numbers, and
// every one of them came from the measurer rather than the tree. A sweep that
// compiled only what one commit's workspace file declared, a vet run without the
// integration tag that left 194 files outside every proof, a comparison of
// commit hashes where trees were what had to match. Each looked clean.
type Measure struct {
	Name     string
	Value    int
	Total    int
	Excluded []string
}

// Validate refuses a measure that does not say what it is blind to. A measure
// that genuinely looks at everything says so explicitly.
func (m Measure) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("%w: measure has no name", apperr.ErrIncomplete)
	}

	if len(m.Excluded) == 0 {
		return fmt.Errorf(
			"%w: measure %q does not say what it excludes",
			apperr.ErrIncomplete,
			m.Name,
		)
	}

	if m.Total < 0 || m.Value < 0 || m.Value > m.Total {
		return fmt.Errorf(
			"%w: measure %q has value %d of %d",
			apperr.ErrIncomplete,
			m.Name,
			m.Value,
			m.Total,
		)
	}

	return nil
}

func (m Measure) Percent() int {
	if m.Total == 0 {
		return 0
	}

	return m.Value * 100 / m.Total
}

// Report is the two kinds of number kept apart on purpose. A rebuild can replay
// perfectly and still group badly: measured across real repositories, agents
// reached replay validity above 0.988 while grouping quality sat between 0.03
// and 0.46. Reporting only the tree invariant hides exactly that failure.
type Report struct {
	Integrity []Measure
	Shape     []Measure
}

func (r Report) Validate() error {
	for _, measure := range append(append([]Measure{}, r.Integrity...), r.Shape...) {
		if err := measure.Validate(); err != nil {
			return err
		}
	}

	return nil
}
