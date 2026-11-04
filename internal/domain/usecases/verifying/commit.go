package verifying

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type pending struct {
	unit Unit
	key  string
}

func (sweep Sweep) logger() *slog.Logger {
	if sweep.Logger == nil {
		return slog.Default()
	}

	return sweep.Logger
}

func (sweep Sweep) checkCommit(
	ctx context.Context,
	slot int,
	commit Commit,
	exclude []string,
) (commitResult, error) {
	started := time.Now()

	listing, err := sweep.Modules.List(ctx, commit)
	if err != nil {
		return commitResult{}, fmt.Errorf("list modules: %w", err)
	}

	base := Row{
		Position: commit.Position, Commit: commit.Oid, Subject: commit.Subject,
		IsWip: isWip(commit.Subject),
	}

	var rows []Row

	var misses []pending

	// Resolved before the filter, so an excluded dependency still reaches a module that needs it.
	for _, unit := range Resolve(listing.Modules) {
		if IsExcluded(unit.Dir, exclude) {
			continue
		}

		row, miss, lookupErr := sweep.lookup(ctx, base, unit)
		if lookupErr != nil {
			return commitResult{}, lookupErr
		}

		rows = append(rows, row)

		if miss != nil {
			misses = append(misses, *miss)
		}
	}

	if stray, hasStray := strayRow(base, listing.Strays, exclude); hasStray {
		rows = append(rows, stray)
	}

	if len(rows) == 0 {
		base.Status = StatusEmpty

		return commitResult{rows: []Row{base}, overhead: time.Since(started)}, nil
	}

	spent, err := sweep.build(ctx, slot, commit, rows, misses)
	if err != nil {
		return commitResult{}, err
	}

	return commitResult{rows: rows, overhead: time.Since(started) - spent}, nil
}

func (sweep Sweep) lookup(ctx context.Context, base Row, unit Unit) (Row, *pending, error) {
	base.Module, base.Language = unit.Dir, unit.Language

	identity, err := sweep.Builder.Identity(ctx, unit)
	if err != nil {
		return Row{}, nil, fmt.Errorf("%w: identity: %w", ErrBuilder, err)
	}

	key := Key(unit, identity)

	outcome, isHit, err := sweep.Cache.Get(ctx, key)
	if err != nil {
		sweep.logger().Warn("read verify cache", slog.String("error", err.Error()))

		isHit = false
	}

	if !isHit {
		return base, &pending{unit: unit, key: key}, nil
	}

	base.IsCached = true

	return withOutcome(base, outcome), nil, nil
}

func withOutcome(row Row, outcome Outcome) Row {
	row.Status, row.Step, row.Tail = StatusPass, outcome.Step, outcome.Tail

	if outcome.IsFailed {
		row.Status = StatusFail
	}

	return row
}

func (sweep Sweep) build(
	ctx context.Context,
	slot int,
	commit Commit,
	rows []Row,
	misses []pending,
) (time.Duration, error) {
	if len(misses) == 0 {
		return 0, nil
	}

	dir, err := sweep.Trees.Materialise(ctx, slot, commit)
	if err != nil {
		return 0, fmt.Errorf("materialise commit: %w", err)
	}

	var spent time.Duration

	for _, miss := range misses {
		began := time.Now()

		outcome, checkErr := sweep.Builder.Check(ctx, dir, miss.unit)
		ctxErr := ctx.Err()

		if ctxErr != nil {
			return 0, fmt.Errorf("check stopped: %w", ctxErr)
		}

		if checkErr != nil {
			return 0, fmt.Errorf("%w: check: %w", ErrBuilder, checkErr)
		}

		took := time.Since(began)
		spent += took

		for idx := range rows {
			if rows[idx].Module == miss.unit.Dir {
				rows[idx] = withOutcome(rows[idx], outcome)
				rows[idx].Millis = took.Milliseconds()
			}
		}

		if outcome.IsFailed {
			continue
		}

		if putErr := sweep.Cache.Put(ctx, miss.key, outcome); putErr != nil {
			sweep.logger().Warn("write verify cache", slog.String("error", putErr.Error()))
		}
	}

	return spent, nil
}
