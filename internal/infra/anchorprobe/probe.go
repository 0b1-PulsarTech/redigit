package anchorprobe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

const filePerm = 0o600

var (
	ErrNoRevision = errors.New("revision has no commit")

	_ anchoring.Probe  = (*Probe)(nil)
	_ anchoring.Reader = (*Probe)(nil)
)

type (
	Config struct {
		Commits  Commits
		Checkout Checkout
		Modules  Modules
		Builder  Builder
		Renderer Renderer
		Rev      string
	}

	Probe struct {
		cfg     Config
		setup   func() (prepared, error)
		release func(context.Context) error
		healthy baselines
	}

	prepared struct {
		dir   string
		units []verifying.Unit
	}
)

func New(ctx context.Context, cfg Config) *Probe {
	probe := &Probe{cfg: cfg}
	probe.setup = sync.OnceValues(func() (prepared, error) { return probe.prepare(ctx) })

	return probe
}

func (probe *Probe) Builds(
	ctx context.Context,
	edits []anchoring.Edit,
) (anchoring.Verdict, error) {
	ready, err := probe.setup()

	if ctx.Err() != nil {
		return anchoring.Verdict{}, fmt.Errorf("prepare the tree: %w", ctx.Err())
	}

	// A tree that cannot be prepared gives every anchor the same unchecked verdict.
	if err != nil {
		return anchoring.Verdict{
			IsUnchecked: true,
			Detail:      "tree not prepared: " + err.Error(),
		}, nil
	}

	files := make([]string, 0, len(edits))
	for _, edit := range edits {
		files = append(files, edit.File)
	}

	units := affectedBy(ready.units, files)

	if err = probe.baseline(ctx, ready.dir, units); err != nil {
		return anchoring.Verdict{}, err
	}

	restore, err := apply(ctx, probe.cfg.Renderer, ready.dir, edits)
	if errors.Is(err, ErrUnfit) {
		return anchoring.Verdict{IsUnfit: true, Detail: err.Error()}, nil
	}

	if err != nil {
		return anchoring.Verdict{}, err
	}

	verdict, checkErr := probe.check(ctx, ready.dir, units)

	return verdict, errors.Join(checkErr, restore())
}

func (probe *Probe) prepare(ctx context.Context) (prepared, error) {
	commits, err := probe.cfg.Commits.List(ctx, probe.cfg.Rev)
	if err != nil {
		return prepared{}, fmt.Errorf("list commits of %s: %w", probe.cfg.Rev, err)
	}

	if len(commits) == 0 {
		return prepared{}, fmt.Errorf("%w: %s", ErrNoRevision, probe.cfg.Rev)
	}

	last := commits[len(commits)-1]

	dir, err := probe.cfg.Checkout.Materialise(ctx, 0, last)
	if err != nil {
		return prepared{}, fmt.Errorf("check out %s: %w", probe.cfg.Rev, err)
	}

	listing, err := probe.cfg.Modules.List(ctx, last)
	if err != nil {
		return prepared{}, fmt.Errorf("list modules of %s: %w", probe.cfg.Rev, err)
	}

	return prepared{dir: dir, units: verifying.Resolve(listing.Modules)}, nil
}

func (probe *Probe) check(
	ctx context.Context,
	dir string,
	units []verifying.Unit,
) (anchoring.Verdict, error) {
	if len(units) == 0 {
		return anchoring.Verdict{IsUnchecked: true, Detail: "no unit covers the edits"}, nil
	}

	var unchecked []string

	for _, unit := range units {
		found, err := probe.trial(ctx, dir, unit)
		if err != nil {
			return anchoring.Verdict{}, err
		}

		switch {
		case found.isFailed:
			return anchoring.Verdict{Detail: found.detail}, nil
		case found.isUnchecked:
			unchecked = append(unchecked, found.detail)
		}
	}

	if len(unchecked) > 0 {
		return anchoring.Verdict{IsUnchecked: true, Detail: strings.Join(unchecked, "; ")}, nil
	}

	return anchoring.Verdict{IsBuilt: true}, nil
}
