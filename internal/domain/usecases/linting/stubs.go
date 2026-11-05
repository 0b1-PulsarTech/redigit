package linting

import (
	"fmt"
	"slices"
)

func stubGaps(cuts []cutFunction, subjects map[int]string, limits Limits) []Finding {
	var found []Finding

	for _, cut := range cuts {
		rows := append([]int{cut.headRow}, cut.rows...)
		first, last := slices.Min(rows), slices.Max(rows)

		if last-first > limits.StubGap {
			found = append(found, Finding{
				Measure: MeasureStubGap, Position: first, Subject: subjects[first],
				Value: last - first, Limit: limits.StubGap,
				Detail: fmt.Sprintf(
					"%s: rows %d to %d, %d statements", cut.head, first, last, len(cut.rows),
				),
			})
		}
	}

	return found
}

func describeStubGap(limits Limits) Measure {
	return Measure{
		Name: MeasureStubGap,
		Definition: "a cut function whose head and statements land in rows more than the " +
			"limit apart, counted as the last row minus the first",
		Limit:    limits.StubGap,
		Excluded: cutCaveats(),
	}
}
