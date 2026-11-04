package auditing

import "github.com/wrapped-owls/redigit/internal/domain/entities"

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=gate.go -destination=gate_mock_test.go -package=auditing

type Gate interface {
	Name() string
	Measure(graph entities.Graph, order entities.Ordering) entities.Measure
}

func (auditor Auditor) gateMeasures(
	graph entities.Graph,
	order entities.Ordering,
) []entities.Measure {
	measures := make([]entities.Measure, 0, len(auditor.cfg.Gates))

	for _, gate := range auditor.cfg.Gates {
		measures = append(measures, gate.Measure(graph, order))
	}

	return measures
}
