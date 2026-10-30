package indexing

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// Blind is one question the resolver could not answer. It is data, not a log
// line: a graph missing a tenth of its edges looks exactly like a graph whose
// code has a tenth fewer edges, and only a count kept alongside the graph
// tells them apart.
type Blind struct {
	Decl   entities.DeclKey
	Query  string
	Reason string
}

// Result is the graph plus what building it could not see.
type Result struct {
	Graph   entities.Graph
	Blind   []Blind
	Partial []Partial
}

// BlindRatio is how much of the graph rests on an unanswered question.
func (r Result) BlindRatio() float64 {
	if r.Graph.Len() == 0 {
		return 0
	}

	return float64(len(r.Blind)) / float64(r.Graph.Len())
}

type (
	Partial struct {
		Broken int
		Of     int
		Tags   []string
	}

	Shortfall interface {
		Partial() []Partial
	}
)

func (indexer Indexer) partials() []Partial {
	// Asked last: the rules load their type facts lazily while the graph is built.
	var partial []Partial

	for _, lang := range indexer.langs {
		if shortfall, isShortfall := lang.Rules.(Shortfall); isShortfall {
			partial = append(partial, shortfall.Partial()...)
		}
	}

	return partial
}
