package emitting

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var ErrSplice = errors.New("pre-image splice")

type (
	edit struct {
		span entities.Span
		text string
	}

	versioning struct {
		plan  entities.Plan
		graph entities.Graph
		at    entities.Position
	}
)

func newVersioning(plan entities.Plan, graph entities.Graph) *versioning {
	if len(plan.Versions) == 0 {
		return nil
	}

	return &versioning{plan: plan, graph: graph}
}

func (run *versioning) advance(position entities.Position) {
	// A plan with no anchor has no versioning at all (nil).
	if run != nil {
		run.at = position
	}
}

func (run *versioning) settled(planned entities.PlannedCommit) entities.PlannedCommit {
	if run == nil {
		return planned
	}

	// A pre-image is not the final text, so the graph's edges say nothing about it.
	planned.Decls = slices.DeleteFunc(slices.Clone(planned.Decls), func(key entities.DeclKey) bool {
		_, isHeld := run.plan.PreimageAt(key, planned.Position)

		return isHeld
	})

	return planned
}

func (run *versioning) apply(source []byte, born []entities.DeclKey) ([]byte, error) {
	if run == nil {
		return source, nil
	}

	var edits []edit

	for _, key := range born {
		text, isActive := run.plan.PreimageAt(key, run.at)
		if !isActive {
			continue
		}

		decl, found := run.graph.Decl(key)
		if !found {
			return nil, fmt.Errorf(
				"%w: declaration %s has a pre-image but no entry in the graph",
				apperr.ErrIncomplete, key,
			)
		}

		edits = append(edits, edit{span: decl.Span, text: text})
	}

	return splice(source, edits)
}

func splice(source []byte, edits []edit) ([]byte, error) {
	if len(edits) == 0 {
		return source, nil
	}

	ordered := slices.SortedFunc(slices.Values(edits), func(left, right edit) int {
		return cmp.Compare(left.span.Start, right.span.Start)
	})
	ordered = slices.CompactFunc(
		ordered,
		func(left, right edit) bool { return left.span == right.span },
	)
	out := make([]byte, 0, len(source))
	cursor := 0

	for _, one := range ordered {
		if one.span.Start < cursor || one.span.End < one.span.Start || one.span.End > len(source) {
			return nil, fmt.Errorf(
				"%w: span %d-%d overlaps an earlier one or leaves the %d-byte file",
				ErrSplice, one.span.Start, one.span.End, len(source),
			)
		}

		out = append(out, source[cursor:one.span.Start]...)
		out = append(out, one.text...)
		cursor = one.span.End
	}

	return append(out, source[cursor:]...), nil
}
