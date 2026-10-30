package verifying

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"
)

type (
	Sweep struct {
		Commits Commits
		Trees   Trees
		Modules Modules
		Builder Builder
		Cache   Cache
		Logger  *slog.Logger
	}

	commitResult struct {
		rows     []Row
		overhead time.Duration
	}
)

func (sweep Sweep) Run(ctx context.Context, opts Options, emit func(Row) error) (Summary, error) {
	summary, err := sweep.run(ctx, opts, emit)

	// Cleanup must survive a cancelled run, which is when slots are most likely left behind.
	if closeErr := sweep.Trees.Close(context.WithoutCancel(ctx)); closeErr != nil {
		sweep.logger().Warn("remove checkout slots", slog.String("error", closeErr.Error()))
	}

	return summary, err
}

func (sweep Sweep) run(ctx context.Context, opts Options, emit func(Row) error) (Summary, error) {
	commits, err := sweep.Commits.List(ctx, opts.Ref)
	if err != nil {
		return Summary{}, fmt.Errorf("list commits: %w", err)
	}

	picked, err := Select(commits, opts)
	if err != nil {
		return Summary{}, err
	}

	results := make([]commitResult, len(picked))
	done := make([]chan struct{}, len(picked))

	for idx := range done {
		done[idx] = make(chan struct{})
	}

	var summary Summary

	group, groupCtx := errgroup.WithContext(ctx)
	workers := min(max(opts.Parallel, 1), max(len(picked), 1))

	for worker := range workers {
		first, end := worker*len(picked)/workers, (worker+1)*len(picked)/workers

		group.Go(func() error {
			return sweep.runRange(groupCtx, worker, opts, rangeJob{
				picked: picked[first:end], results: results[first:end], done: done[first:end],
			})
		})
	}

	group.Go(func() error {
		var collectErr error

		summary, collectErr = collectRows(groupCtx, results, done, emit)

		return collectErr
	})

	if err = group.Wait(); err != nil {
		return summary, fmt.Errorf("sweep: %w", err)
	}

	summary.Commits = len(picked)

	return summary, nil
}

type rangeJob struct {
	picked  []Commit
	results []commitResult
	done    []chan struct{}
}

func (sweep Sweep) runRange(ctx context.Context, slot int, opts Options, job rangeJob) error {
	for idx, commit := range job.picked {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("sweep stopped: %w", err)
		}

		result, err := sweep.checkCommit(ctx, slot, commit, opts.Exclude)
		if err != nil {
			return err
		}

		job.results[idx] = result
		close(job.done[idx])
	}

	return nil
}

func collectRows(
	ctx context.Context,
	results []commitResult,
	done []chan struct{},
	emit func(Row) error,
) (Summary, error) {
	var summary Summary

	overheads := make([]time.Duration, 0, len(results))

	for idx := range results {
		select {
		case <-done[idx]:
		case <-ctx.Done():
			return summary, fmt.Errorf("sweep stopped: %w", ctx.Err())
		}

		overheads = append(overheads, results[idx].overhead)

		for _, row := range results[idx].rows {
			summary.add(row)

			if err := emit(row); err != nil {
				return summary, fmt.Errorf("emit row: %w", err)
			}
		}
	}

	slices.Sort(overheads)

	if len(overheads) > 0 {
		summary.OverheadMicros = overheads[len(overheads)/2].Microseconds()
	}

	return summary, nil
}

func (summary *Summary) add(row Row) {
	switch {
	case row.Status == StatusEmpty:
		summary.Empty++

		return
	case row.Status == StatusPass:
		summary.Passed++
	case row.IsWip:
		summary.WipFailed++
	default:
		summary.Failed++
	}

	summary.Checked++

	if row.IsCached {
		summary.Cached++
	}
}
