package format

import (
	"context"
	"errors"
	"sync"
)

const pilotBlobs = 3

type (
	run struct {
		contents [][]byte
		stderr   string
		err      error
	}

	lane struct {
		jobs    []job
		results []result
	}

	stepState struct {
		mu     sync.Mutex
		cause  error
		worked bool
		hangs  int
		pilot  int
	}
)

func (engine *Engine) solve(ctx context.Context, step tool, state *stepState, part lane) {
	if cause := state.aborted(); cause != nil {
		part.failAbort(step, cause)

		return
	}

	if err := ctx.Err(); err != nil {
		part.failAll(step, &run{err: err})

		return
	}

	got := state.attempt(func() run { return engine.exec(ctx, step, part.jobs) })
	engine.settle(ctx, step, state, part, got)
}

func (state *stepState) attempt(start func() run) run {
	got := start()
	if !errors.Is(got.err, errStart) {
		return got
	}

	// A step another lane already ended gets no retry; else one retry covers a transient ETXTBSY
	if state.aborted() != nil {
		return got
	}

	got = start()
	if errors.Is(got.err, errStart) {
		state.abort(got.err)
	}

	return got
}

func (engine *Engine) exec(ctx context.Context, step tool, jobs []job) run {
	if step.mode != ModeStdin {
		return engine.inPlace(ctx, step, jobs)
	}

	out, stderr, err := engine.pipe(ctx, step, jobs[0].content)

	return run{contents: [][]byte{out}, stderr: stderr, err: err}
}

func (engine *Engine) settle(
	ctx context.Context,
	step tool,
	state *stepState,
	part lane,
	got run,
) {
	if got.err == nil {
		state.succeed()

		for position, item := range part.jobs {
			// A neighbour may have emptied it, so only its own solo run can condemn it
			if len(part.jobs) > 1 && emptied(step, item, got.contents[position]) {
				engine.solve(ctx, step, state, part.at(position))

				continue
			}

			part.results[position] = checkEmpty(step, item, got.contents[position])
		}

		return
	}

	// the caller's own deadline is not the tool hanging
	if len(part.jobs) == 1 && ctx.Err() == nil && errors.Is(got.err, context.DeadlineExceeded) {
		state.hang(got.err)
	}

	if err := ctx.Err(); err != nil {
		// the chunk's exit and stderr are the kill's side effects, not a verdict of any blob
		part.failAll(step, &run{err: err})

		return
	}

	if cause := state.aborted(); cause != nil {
		part.failAbort(step, cause)

		return
	}

	if len(part.jobs) == 1 {
		part.failAll(step, &got)

		return
	}

	// a blob fails only on its own solo run, so a failed chunk is split until each blob runs alone
	const bisectParts = 2

	mid := len(part.jobs) / bisectParts
	engine.solve(ctx, step, state, lane{jobs: part.jobs[:mid], results: part.results[:mid]})
	engine.solve(ctx, step, state, lane{jobs: part.jobs[mid:], results: part.results[mid:]})
}

func (state *stepState) aborted() error {
	state.mu.Lock()
	defer state.mu.Unlock()

	return state.cause
}

func (state *stepState) abort(cause error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.cause == nil {
		state.cause = cause
	}
}

func (state *stepState) succeed() {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.worked = true
}

func (state *stepState) endPilot() {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.pilot = 0
}

func (state *stepState) hang(cause error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.hangs++

	// Only the pilot can call a tool hung; a hang after a success is per blob and costs a timeout each
	if state.pilot > 0 && !state.worked && state.hangs >= pilotBlobs &&
		state.cause == nil {
		state.cause = cause
	}
}
