package loadonce

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

var (
	ErrClosed   = errors.New("shared load closed")
	ErrPanicked = errors.New("shared load panicked")
)

type Once[Value any] struct {
	guard  sync.Mutex
	done   chan struct{}
	cancel context.CancelFunc
	closed bool
	value  Value
	err    error
}

func (once *Once[Value]) Do(
	ctx context.Context,
	load func(context.Context) (Value, error),
) (Value, error) {
	var none Value

	done, err := once.start(ctx, load)
	if err != nil {
		return none, err
	}

	// A select picks at random between two ready cases, so a finished load is checked first.
	if !finished(done) {
		select {
		case <-done:
		case <-ctx.Done():
			return none, fmt.Errorf("shared load: %w", ctx.Err())
		}
	}

	if once.err != nil {
		return none, once.err
	}

	return once.value, nil
}

func finished(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (once *Once[Value]) start(
	ctx context.Context,
	load func(context.Context) (Value, error),
) (<-chan struct{}, error) {
	once.guard.Lock()
	defer once.guard.Unlock()

	if once.done == nil {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("shared load: %w", err)
		}

		if once.closed {
			return nil, fmt.Errorf("shared load: %w", ErrClosed)
		}

		// The load serves every caller, so no one caller's cancellation may end it: each caller
		// stops waiting on its own context, and only Close cancels the load itself.
		//nolint:gosec // cancel is kept for Close, and run calls it when the load ends
		loadCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		once.done, once.cancel = make(chan struct{}), cancel

		go once.run(loadCtx, load)
	}

	return once.done, nil
}

func (once *Once[Value]) run(
	loadCtx context.Context,
	load func(context.Context) (Value, error),
) {
	defer close(once.done)
	defer once.cancel()

	// A panic in the load would leave every waiter blocked on a channel nobody closes.
	defer func() {
		if recovered := recover(); recovered != nil {
			var none Value

			once.value = none
			once.err = fmt.Errorf("%w: %v\n%s", ErrPanicked, recovered, debug.Stack())
		}
	}()

	once.value, once.err = load(loadCtx)
}

func (once *Once[Value]) Close(ctx context.Context) error {
	once.guard.Lock()
	once.closed = true
	cancel, done := once.cancel, once.done
	once.guard.Unlock()

	if done == nil {
		return nil
	}

	cancel()

	if finished(done) {
		return nil
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for shared load: %w", ctx.Err())
	}
}
