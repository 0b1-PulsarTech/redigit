package emitting

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var ErrSplice = errors.New("pre-image splice")

type (
	Piece struct {
		Span    entities.Span
		Text    string
		Imports []string
	}

	Layout struct {
		File   string
		Source []byte
		Pieces []Piece
		Keep   []entities.DeclKey
	}

	Admitter interface {
		Admit(ctx context.Context, file string, source []byte, specs []string) ([]byte, error)
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

func (run *versioning) pieces(born []entities.DeclKey) ([]Piece, error) {
	if run == nil {
		return nil, nil
	}

	var layers []Piece

	for _, key := range born {
		version, isActive := run.plan.VersionAt(key, run.at)
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

		layers = append(
			layers,
			Piece{Span: decl.Span, Text: version.Text, Imports: version.Imports},
		)
	}

	return layers, nil
}

func Compose(ctx context.Context, renderer Renderer, layout Layout) ([]byte, error) {
	spliced, err := splice(layout.Source, layout.Pieces)
	if err != nil {
		return nil, fmt.Errorf("splice %s: %w", layout.File, err)
	}

	var imports []string
	for _, piece := range layout.Pieces {
		imports = append(imports, piece.Imports...)
	}

	imports = slices.Compact(slices.Sorted(slices.Values(imports)))

	if admitter, isAdmitter := renderer.(Admitter); isAdmitter && len(imports) > 0 {
		if spliced, err = admitter.Admit(ctx, layout.File, spliced, imports); err != nil {
			return nil, fmt.Errorf("splice %s: admit imports: %w", layout.File, err)
		}
	}

	rendered, err := renderer.Render(ctx, spliced, layout.Keep)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", layout.File, err)
	}

	return rendered, nil
}

func splice(source []byte, pieces []Piece) ([]byte, error) {
	if len(pieces) == 0 {
		return source, nil
	}

	ordered := slices.SortedFunc(slices.Values(pieces), func(left, right Piece) int {
		return cmp.Compare(left.Span.Start, right.Span.Start)
	})
	ordered = slices.CompactFunc(
		ordered,
		func(left, right Piece) bool { return left.Span == right.Span && left.Text == right.Text },
	)
	out := make([]byte, 0, len(source))
	cursor := 0

	for _, one := range ordered {
		if one.Span.Start < cursor || one.Span.End < one.Span.Start ||
			one.Span.End > len(source) {
			return nil, fmt.Errorf(
				"%w: span %d-%d overlaps an earlier one or leaves the %d-byte file",
				ErrSplice, one.Span.Start, one.Span.End, len(source),
			)
		}

		out = append(out, source[cursor:one.Span.Start]...)
		out = append(out, one.Text...)
		cursor = one.Span.End
	}

	return append(out, source[cursor:]...), nil
}
