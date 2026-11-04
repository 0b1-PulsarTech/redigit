package idiom

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
)

type labelled struct {
	inner auditing.Gate
	label string
}

var _ auditing.Gate = labelled{}

func (gate labelled) Name() string { return gate.inner.Name() }

func (gate labelled) Measure(graph entities.Graph, order entities.Ordering) entities.Measure {
	measured := gate.inner.Measure(graph, order)
	measured.Name = gate.label + ": " + measured.Name

	return measured
}
