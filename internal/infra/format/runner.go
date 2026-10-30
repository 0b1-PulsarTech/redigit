package format

import (
	"context"
	"fmt"
	"path/filepath"

	"golang.org/x/sync/errgroup"
)

type (
	job struct {
		index   int
		path    string
		clean   string
		content []byte
		alone   bool
	}

	result struct {
		content []byte
		failure *Failure
	}
)

func (engine *Engine) runStep(ctx context.Context, step tool, all []job) []result {
	results := make([]result, len(all))

	var jobs []job

	var slots []int

	for position, item := range all {
		if step.needs(item.content) {
			jobs, slots = append(jobs, item), append(slots, position)
		} else {
			results[position] = result{content: item.content}
		}
	}

	part := engine.runNeeded(ctx, step, jobs)
	for slot, position := range slots {
		results[position] = part[slot]
	}

	return results
}

func (engine *Engine) runNeeded(ctx context.Context, step tool, jobs []job) []result {
	results := make([]result, len(jobs))

	var group errgroup.Group

	group.SetLimit(engine.workers)

	if step.mode == ModeStdin {
		engine.spawnStdin(ctx, step, jobs, results, &group)
	} else {
		engine.spawnFiles(ctx, step, jobs, results, &group)
	}

	_ = group.Wait()

	return results
}

func (engine *Engine) spawnStdin(
	ctx context.Context,
	step tool,
	jobs []job,
	results []result,
	group *errgroup.Group,
) {
	state := stepState{pilot: min(pilotBlobs, len(jobs))}

	solve := func(position int) {
		part := lane{jobs: jobs[position : position+1], results: results[position : position+1]}
		engine.solve(ctx, step, &state, part)
	}

	// the first blobs run in input order on this goroutine, so a hung tool is called out deterministically
	for position := range state.pilot {
		solve(position)
	}

	state.endPilot()

	for position := pilotBlobs; position < len(jobs); position++ {
		group.Go(func() error {
			solve(position)

			return nil
		})
	}
}

func (engine *Engine) spawnFiles(
	ctx context.Context,
	step tool,
	jobs []job,
	results []result,
	group *errgroup.Group,
) {
	var state stepState

	valid, origin := make([]job, 0, len(jobs)), make([]int, 0, len(jobs))

	for position, item := range jobs {
		// a path outside the repo never reaches a process, and never costs its neighbours one
		if !filepath.IsLocal(item.clean) {
			cause := fmt.Errorf("%w: path leaves the repo", ErrConfig)
			results[position] = result{failure: newFailure(step, item.path, "", cause)}

			continue
		}

		valid, origin = append(valid, item), append(origin, position)
	}

	solve := func(positions []int) {
		part, local := make([]job, len(positions)), make([]result, len(positions))
		for slot, position := range positions {
			part[slot] = valid[position]
		}

		engine.solve(ctx, step, &state, lane{jobs: part, results: local})

		for slot, position := range positions {
			results[origin[position]] = local[slot]
		}
	}

	chunks := engine.chunks(step, valid, argBudget)

	// The first chunks run alone until they cover the pilot blobs, so a lone isolated blob cannot
	// call a hang by itself. A tool that hangs only after a success costs one timeout per blob.
	state.pilot = min(pilotBlobs, len(valid))

	next := 0
	for covered := 0; covered < state.pilot; next++ {
		solve(chunks[next])

		covered += len(chunks[next])
	}

	state.endPilot()

	for _, positions := range chunks[next:] {
		group.Go(func() error {
			solve(positions)

			return nil
		})
	}
}

func (part lane) failAll(step tool, cause *run) {
	for position, item := range part.jobs {
		part.results[position] = result{
			failure: newFailure(step, item.path, cause.stderr, cause.err),
		}
	}
}

func (part lane) failAbort(step tool, cause error) {
	aborted := fmt.Errorf("%w: %w", ErrStepAborted, cause)

	for position, item := range part.jobs {
		part.results[position] = result{failure: newFailure(step, item.path, "", aborted)}
	}
}

func (part lane) at(position int) lane {
	return lane{
		jobs:    part.jobs[position : position+1],
		results: part.results[position : position+1],
	}
}
