package shaping

import (
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type Pipeline struct {
	observer Observer
	passes   []Pass
}

func NewPipeline(observer Observer, passes ...Pass) *Pipeline {
	if observer == nil {
		observer = nopObserver{}
	}

	return &Pipeline{observer: observer, passes: passes}
}

func (pipeline *Pipeline) Name() string { return "pipeline" }

func (pipeline *Pipeline) Apply(ctx context.Context, draft *Draft) (Moves, error) {
	if len(pipeline.passes) == 0 {
		return nil, nil
	}

	var all Moves

	standing, spread := draft.Violations(), draft.atomSpread()

	for index, pass := range pipeline.passes {
		if err := ctx.Err(); err != nil {
			return all, fmt.Errorf("pass %d (%s): %w", index, pass.Name(), err)
		}

		saved := draft.Clone()

		moves, err := pass.Apply(ctx, draft)
		if err != nil {
			*draft = *saved

			return all, fmt.Errorf("pass %d (%s): %w", index, pass.Name(), err)
		}

		after := draft.Violations()

		added := slices.Concat(newViolations(standing, after), draft.wideningSplits(spread))
		if len(added) > 0 {
			*draft = *saved

			pipeline.observer.Refused(pass.Name(), added)

			continue
		}

		for _, move := range moves {
			pipeline.observer.Moved(pass.Name(), move)
		}

		all = append(all, moves...)
		standing, spread = after, draft.atomSpread()
	}

	return all, nil
}

func newViolations(before, after []entities.Edge) []entities.Edge {
	known := make(map[entities.Edge]bool, len(before))
	for _, edge := range before {
		known[edge] = true
	}

	var added []entities.Edge

	for _, edge := range after {
		if !known[edge] {
			added = append(added, edge)
		}
	}

	return added
}
