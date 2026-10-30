package passes

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

const TidyName = "tidy-test-after"

type (
	TidyConfig struct {
		MinTestLines int `json:"min_test_lines"`
		MaxProdLines int `json:"max_prod_lines"`
		TailWindow   int `json:"tail_window"`
	}

	Tidy struct{ cfg TidyConfig }
)

func NewTidy(raw []byte) (Tidy, error) {
	const (
		defaultMinTestLines = 200
		defaultMaxProdLines = 40
		defaultTailWindow   = 12
	)

	spec := struct {
		TidyConfig

		Type string `json:"type"`
	}{TidyConfig: TidyConfig{
		MinTestLines: defaultMinTestLines,
		MaxProdLines: defaultMaxProdLines,
		TailWindow:   defaultTailWindow,
	}}

	if err := shaping.DecodeStrict(raw, &spec); err != nil {
		return Tidy{}, fmt.Errorf("%s: %w", TidyName, err)
	}

	cfg := spec.TidyConfig
	if cfg.MaxProdLines < 0 || cfg.TailWindow < 0 {
		return Tidy{}, fmt.Errorf(
			"%w: %s knobs must not be negative", shaping.ErrMalformed, TidyName,
		)
	}

	// A block with no test line is production code; it must never travel under a test subject.
	if cfg.MinTestLines < 1 {
		return Tidy{}, fmt.Errorf(
			"%w: %s min_test_lines must be at least 1", shaping.ErrMalformed, TidyName,
		)
	}

	return Tidy{cfg: cfg}, nil
}

func (tidy Tidy) Name() string { return TidyName }

func (tidy Tidy) Apply(ctx context.Context, draft *shaping.Draft) (shaping.Moves, error) {
	rounds, known := heldCount(draft)+1, originsOf(draft)

	// A tail move can bring a soft provider ahead of a foreign block's slot, so both
	// rules repeat together until neither moves.
	return fixpoint(rounds, func() (shaping.Moves, error) {
		moves, err := tidy.foreign(ctx, draft, known)
		if err != nil {
			return moves, err
		}

		tail, err := tidy.tail(ctx, draft, known)

		return append(moves, tail...), err
	})
}
