package auditing

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// defaultFarApart is how many positions a declaration may sit ahead of its
// first outside reader before it is reported. The previous rebuild used the
// same threshold when deciding which commits were worth moving.
const defaultFarApart = 25

// Config tunes what counts as far.
type Config struct {
	FarApart int
	// Unseen names the files a build constraint hid from the reference
	// queries. It is supplied by the composition root because deciding which
	// constraints a run satisfies is a per-language question, and this package
	// knows no language.
	Unseen []Unseen
	// Unclaimed names the final-tree paths no configured language pack's
	// extension list matches. Supplied by the composition root for the same
	// reason as Unseen: which extensions a language owns is a per-language
	// question, and this package knows no language.
	Unclaimed []Unclaimed
}

func (c Config) withDefaults() Config {
	if c.FarApart < 1 {
		c.FarApart = defaultFarApart
	}

	return c
}

// Result is what an audit produces: numbers that each declare their own blind
// spot, and the list of declarations worth looking at.
type Result struct {
	Report    entities.Report
	Drifts    []Drift
	Unseen    []Unseen
	Unclaimed []Unclaimed
}

// Auditor measures a rebuilt ordering. Every measure it produces names what it
// does NOT look at, because that is where the previous rebuild's bad numbers
// came from: eleven times the measurer was wrong and the tree was fine.
type Auditor struct {
	cfg Config
}

func New(cfg Config) Auditor { return Auditor{cfg: cfg.withDefaults()} }

// Audit reports integrity and shape separately, and never merges them.
// Measured on real repositories, a rebuild can replay almost perfectly while
// grouping badly: replay validity above 0.988 alongside grouping quality
// between 0.03 and 0.46. A single combined number would hide exactly that.
func (a Auditor) Audit(graph entities.Graph, order entities.Ordering) (Result, error) {
	if graph.Len() == 0 {
		return Result{}, fmt.Errorf("%w: nothing to audit", apperr.ErrIncomplete)
	}

	found := drifts(graph, order, a.cfg.FarApart)

	result := Result{
		Report: entities.Report{
			Integrity: []entities.Measure{
				a.placed(graph, order),
				a.cyclesNamed(graph, order),
				a.resolverCouldSee(graph),
			},
			Shape: []entities.Measure{
				a.nearTheirReader(graph, order, found),
				a.testsWithSubject(graph, order),
			},
		},
		Drifts:    found,
		Unseen:    a.cfg.Unseen,
		Unclaimed: a.cfg.Unclaimed,
	}

	// A measure that does not name its blind spot never leaves this package.
	if err := result.Report.Validate(); err != nil {
		return Result{}, fmt.Errorf("audit report: %w", err)
	}

	return result, nil
}

func (a Auditor) placed(graph entities.Graph, order entities.Ordering) entities.Measure {
	placed := 0

	for _, decl := range graph.Decls() {
		if order.Of(decl.Key).IsSet() {
			placed++
		}
	}

	return entities.Measure{
		Name:  "declarations placed",
		Value: placed,
		Total: graph.Len(),
		// This one really does look at every declaration, and says so rather
		// than leaving a reader to assume it.
		Excluded: []string{"nothing"},
	}
}

// cyclesNamed counts against the graph's declarations, the same population
// every other measure here uses. Counting against the ordering's own map made
// the total drift above the declaration count whenever a stray key crept in,
// and two measures whose denominators disagree cannot be read together.
func (a Auditor) cyclesNamed(graph entities.Graph, order entities.Ordering) entities.Measure {
	members := map[entities.DeclKey]bool{}

	for _, cycle := range order.Cycles {
		for _, key := range cycle.Members {
			members[key] = true
		}
	}

	return entities.Measure{
		Name:  "declarations free of a cycle",
		Value: max(graph.Len()-len(members), 0),
		Total: graph.Len(),
		Excluded: []string{
			"declarations the ordering never placed",
			"a cycle the graph does not represent, which is the defect this cannot see",
		},
	}
}

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
