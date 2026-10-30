package rewriting

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
)

var (
	ErrNoDates       = errors.New("no author dates to sample")
	ErrNegativeStamp = errors.New("negative author date")
)

type Stamp struct {
	Unix int64
	Zone string
}

func SampleDates(stamps []Stamp, count int) ([]Stamp, error) {
	if count == 0 {
		return nil, nil
	}

	if len(stamps) == 0 {
		return nil, fmt.Errorf("%w: the date source holds no commit", ErrNoDates)
	}

	if slices.ContainsFunc(stamps, func(stamp Stamp) bool { return stamp.Unix < 0 }) {
		return nil, fmt.Errorf("%w: fast-import takes unsigned dates", ErrNegativeStamp)
	}

	sorted := slices.Clone(stamps)
	slices.SortFunc(sorted, func(left, right Stamp) int {
		return cmp.Or(cmp.Compare(left.Unix, right.Unix), cmp.Compare(left.Zone, right.Zone))
	})

	if count == 1 {
		return []Stamp{sorted[len(sorted)-1]}, nil
	}

	sampled := make([]Stamp, 0, count)

	for idx := range count {
		// Half-even, as the reference's round(): math.Round would tip every .5 up.
		pick := int(math.RoundToEven(float64(idx*(len(sorted)-1)) / float64(count-1)))
		stamp := sorted[pick]

		if idx > 0 {
			stamp.Unix = max(stamp.Unix, sampled[idx-1].Unix+1)
		}

		sampled = append(sampled, stamp)
	}

	return sampled, nil
}
