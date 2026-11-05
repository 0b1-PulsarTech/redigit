package linting

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type errPairs struct {
	pairs  int
	unread int
	split  map[AssignForm]int
}

func errcheckSplits(
	ctx context.Context,
	structure Structure,
	cuts []cutFunction,
) ([]Finding, Measure, error) {
	byFile := map[string][]cutFunction{}
	for _, cut := range cuts {
		byFile[cut.head.File] = append(byFile[cut.head.File], cut)
	}

	var found []Finding

	tally := errPairs{split: map[AssignForm]int{}}

	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		spans := map[entities.DeclKey]entities.Span{}

		for _, cut := range byFile[file] {
			for _, stmt := range cut.stmts {
				spans[stmt.Key] = stmt.Span
			}
		}

		facts, err := structure.Syntax.Statements(ctx, file, spans)
		if err != nil {
			return nil, Measure{}, fmt.Errorf("read the statements of %s: %w", file, err)
		}

		// A nil map is a language with no reader; a missing key in a map is a span unread.
		if facts != nil {
			tally.unread += len(spans) - len(facts)
		}

		for _, cut := range byFile[file] {
			found = append(found, splitPairs(cut, facts, structure.Subjects, &tally)...)
		}
	}

	return found, describeErrcheck(tally), nil
}

func splitPairs(
	cut cutFunction,
	facts map[entities.DeclKey]Statement,
	subjects map[int]string,
	tally *errPairs,
) []Finding {
	var found []Finding

	for assign := range cut.stmts {
		form := bindingForm(cut, facts, assign)
		if form == FormNone {
			continue
		}

		check := checkAfter(cut, facts, assign)
		if check < 0 {
			continue
		}

		tally.pairs++

		from, to := cut.rows[assign], cut.rows[check]
		if from == to {
			continue
		}

		tally.split[form]++

		found = append(found, Finding{
			Measure: MeasureErrcheckSplit, Position: from, Subject: subjects[from],
			Value: max(to-from, from-to),
			Detail: fmt.Sprintf(
				"%s: %s at statement %d in row %d, checked at statement %d in row %d",
				cut.head, form, assign, from, check, to,
			),
		})
	}

	return found
}

func bindingForm(cut cutFunction, facts map[entities.DeclKey]Statement, assign int) AssignForm {
	form := facts[cut.stmts[assign].Key].Assign
	if form != FormDefine && form != FormReuse {
		return form
	}

	// A := whose err no earlier top-level statement bound introduces it; the rest reuse it.
	for earlier := range assign {
		bound := facts[cut.stmts[earlier].Key]
		if bound.Assign != FormNone || bound.Binds {
			return FormReuse
		}
	}

	return FormDefine
}

func checkAfter(cut cutFunction, facts map[entities.DeclKey]Statement, assign int) int {
	for next := assign + 1; next < len(cut.stmts); next++ {
		later := facts[cut.stmts[next].Key]
		if later.Checks {
			return next
		}

		if later.Assign != FormNone {
			return -1
		}
	}

	return -1
}

func describeErrcheck(tally errPairs) Measure {
	split := tally.split[FormAssign] + tally.split[FormReuse] + tally.split[FormDefine]

	return Measure{
		Name: MeasureErrcheckSplit,
		Definition: "an assignment to a variable named err (err = ..., := or var with a value) " +
			"and the if err != nil that checks it, in different rows; the check is looked for " +
			"past statements that do not assign err, and the next assignment of err ends the " +
			"search. Pairs are counted apart by what the statement does to err: " +
			"plain err =; err newly bound here (no earlier statement of the body bound it); " +
			"err already bound, with other names defined (an earlier statement bound it, " +
			"including var err T)",
		Detail: fmt.Sprintf(
			"pairs %d, split %d (%s: %d; %s: %d; %s: %d); statements unread %d",
			tally.pairs, split, FormAssign, tally.split[FormAssign],
			FormReuse, tally.split[FormReuse], FormDefine, tally.split[FormDefine], tally.unread,
		),
		Excluded: []string{
			"the three forms were taken from the review-s10 oracle table after seeing it; " +
				"the spec says only that errcheck pairs are split",
			"a variable not named err: the name is read, not the type",
			"err already bound is read from earlier statements of the body, not from types: " +
				"a parameter or named result named err reads as newly bound",
			"statements the reader returns no fact for are counted as unread, not paired",
			"statements that are not at the top level of the body: a nested block is not seen",
			"a check that is not an if with err != nil (a switch, a return err) pairs with nothing",
			"an assignment overwritten, or never checked: it pairs with nothing",
			"if err := f(); err != nil is one statement and cannot be split",
			"functions the slicer did not cut, and files of a language with no statement reader",
		},
	}
}
