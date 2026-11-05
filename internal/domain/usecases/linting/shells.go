package linting

import (
	"fmt"
	"slices"
)

func shells(cuts []cutFunction, subjects map[int]string) (empty, tail []Finding, firstAway int) {
	for _, cut := range cuts {
		last := cut.rows[len(cut.rows)-1]

		if cut.rows[0] != cut.headRow {
			firstAway++
		}

		if !slices.Contains(cut.rows, cut.headRow) {
			empty = append(empty, Finding{
				Measure: MeasureShellsBornEmpty, Position: cut.headRow,
				Subject: subjects[cut.headRow], Value: len(cut.rows),
				Detail: fmt.Sprintf(
					"%s: head in row %d, %d statements in rows %d to %d",
					cut.head,
					cut.headRow,
					len(cut.rows),
					slices.Min(cut.rows),
					slices.Max(cut.rows),
				),
			})
		}

		if last != cut.headRow {
			tail = append(tail, Finding{
				Measure: MeasureShellsWithoutTail, Position: cut.headRow,
				Subject: subjects[cut.headRow], Value: max(last-cut.headRow, cut.headRow-last),
				Detail: fmt.Sprintf(
					"%s: head in row %d, last statement in row %d of %d",
					cut.head, cut.headRow, last, len(cut.rows),
				),
			})
		}
	}

	return empty, tail, firstAway
}

func describeBornEmpty() Measure {
	return Measure{
		Name: MeasureShellsBornEmpty,
		Definition: "a cut function whose head is born in a row that holds none of its " +
			"statements; the slicer ties the head to one statement, so a count above 0 means " +
			"a later pass or a config rule separated them",
		Excluded: cutCaveats(),
	}
}

func describeWithoutTail(cuts, firstAway int) Measure {
	return Measure{
		Name: MeasureShellsWithoutTail,
		Definition: "a cut function whose last piece (a statement, or an element of a returned " +
			"literal cut by element) is not in the row of its head; it finds void functions, " +
			"main-like functions that assemble a program, and functions with results whose " +
			"returned literal is cut by element, the head tied to element 0 only",
		Detail: fmt.Sprintf(
			"cut functions %d; first statement not in the head's row: %d of %d",
			cuts, firstAway, cuts,
		),
		Excluded: append(
			cutCaveats(),
			"elements of a returned literal are told from statements by nothing here: a "+
				"function with results can be a finding, and Detail calls its elements statements",
		),
	}
}

func cutCaveats() []string {
	return []string{
		"functions the slicer did not cut: only a wiring root or a slice.functions match with " +
			"two statements or more (or a returned literal of two elements or more, cut by " +
			"element), no early return before the last and no label or goto",
		"test and generated functions: they are never cut",
		"a language with no slicer: its files hold no cut function, and none is read as one",
		"a cut function the manifest places no piece of is refused as a mismatch, not counted",
	}
}
