package auditing

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// nearTheirReader is the measure the whole rewrite serves. It counts only
// declarations something outside their unit actually reads: one nothing reads
// has no reader to be near, and counting it as a success would inflate the
// number with declarations the measure never examined.
func (a Auditor) nearTheirReader(
	graph entities.Graph,
	order entities.Ordering,
	found []Drift,
) entities.Measure {
	total := 0

	for _, decl := range graph.Decls() {
		if !order.Of(decl.Key).IsSet() {
			continue
		}

		if _, _, hasReader := firstReader(graph, order, decl.Key); hasReader {
			total++
		}
	}

	return entities.Measure{
		Name:  fmt.Sprintf("born within %d positions of the first outside reader", a.cfg.FarApart),
		Value: max(total-len(found), 0),
		Total: total,
		Excluded: []string{
			"declarations nothing outside their unit reads",
			"declarations the ordering never placed",
			"readers inside the same unit, which is the whole point",
		},
	}
}

// testsWithSubject measures the rule on the FUNCTION, not on the file. Asking
// whether a test file lands near its twin called a package clean while every
// one of its tests waited for the last function anybody added.
func (a Auditor) testsWithSubject(graph entities.Graph, order entities.Ordering) entities.Measure {
	var together, total int

	for _, anchor := range graph.Anchors() {
		if anchor.Reason != entities.ReasonTestOf {
			continue
		}

		test, subject := order.Of(anchor.Decl), order.Of(anchor.To)
		if !test.IsSet() || !subject.IsSet() {
			continue
		}

		total++

		if test == subject {
			together++
		}
	}

	return entities.Measure{
		Name:  "tests born in the same commit as their subject",
		Value: together,
		Total: total,
		Excluded: []string{
			"tests whose subject no rule could identify",
			"tests the ordering never placed",
			"integration suites, which boot a harness and belong with it",
		},
	}
}
