package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
)

// client is a JSON-RPC 2.0 client over LSP stdio framing. It correlates
// requests and responses by id, answers server-to-client requests minimally,
// and drops notifications it has no use for. One client serves many
// concurrent in-flight call/notify invocations; run owns the single reader
// goroutine and must be started exactly once.
//
// Residual limitation: every write is serialized onto the single shared
// stdio stream through writeMu (see writeFrame). A write wedged behind an
// unresponsive peer holds writeMu until it either errors out on its own or
// the client's governing ctx ends and run's watcher goroutine force-closes
// the writer. Until then, EVERY OTHER call or notify sharing this client
// blocks trying to acquire writeMu too, no matter how short or generous its
// own ctx is: racing an individual call's ctx only stops that call from
// waiting on the outcome, it cannot free the lock for someone else. This is
// a property of serializing all writes onto one connection, not a per-call
// bound, and no amount of per-call ctx racing changes it. The only way to
// unblock every stuck caller at once is to tear down the whole connection
// (see Session.Close).
type client struct {
	w       io.Writer
	r       io.ReadCloser
	writeMu sync.Mutex // serializes frame writes on w

	logger *slog.Logger

	nextID atomic.Int64

	mu      sync.Mutex // guards pending
	pending map[int64]chan incomingMessage

	// background tracks detached best-effort work this client spawns and
	// does not otherwise join: currently just the async $/cancelRequest
	// send in call, dispatched when the initial send loses its race
	// against ctx while already in flight (see call and waitBackground).
	background sync.WaitGroup
}

// newClient wraps w/r as the LSP stdio transport. r must be an io.ReadCloser
// so run can unblock its blocking read by closing it when ctx is done.
func newClient(w io.Writer, r io.ReadCloser, logger *slog.Logger) *client {
	return &client{
		w:       w,
		r:       r,
		logger:  logger,
		pending: make(map[int64]chan incomingMessage),
	}
}

// call sends method as a request and decodes its result into result (nil
// if unneeded). The initial send races ctx.Done (see sendRacingCtx); if
// ctx wins that race, call fires $/cancelRequest in the background instead
// of waiting on a possibly-wedged write. Once the request is confirmed
// sent, a later ctx.Done sends $/cancelRequest and waits for it.
func (c *client) call(ctx context.Context, method string, params, result any) error {
	id := c.nextID.Add(1)
	ch := make(chan incomingMessage, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	rawID := json.RawMessage(fmt.Appendf(nil, "%d", id))
	req := outgoingMessage{JSONRPC: jsonrpcVersion, ID: rawID, Method: method, Params: params}

	if attempted, err := c.sendRacingCtx(ctx, req); err != nil {
		if attempted {
			// The write was actually started before ctx won the race, so
			// it may still land after this point (see sendRacingCtx): fire
			// the best-effort cancel in the background, tracked via
			// c.background so waitBackground (called from Session.Close)
			// can join it, instead of waiting here on a possibly-wedged
			// writeMu. When ctx was already done before the write was ever
			// attempted, there is nothing the server could have seen, so
			// there is nothing to cancel.
			c.background.Go(func() { c.cancel(ctx, rawID) })
		}

		return fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case msg := <-ch:
		return c.decodeResult(method, msg, result)
	case <-ctx.Done():
		c.cancel(ctx, rawID)

		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// notify sends method as a notification; no response is expected. The
// write races against ctx.Done, but an abandoned write's own I/O keeps
// running and can hold writeMu until it finishes on its own or the
// client's governing ctx (run's ctx, not this one) is cancelled, which
// force-closes the writer (see run's watcher goroutine).
func (c *client) notify(ctx context.Context, method string, params any) error {
	req := outgoingMessage{JSONRPC: jsonrpcVersion, Method: method, Params: params}

	if _, err := c.sendRacingCtx(ctx, req); err != nil {
		return fmt.Errorf("notify %s: %w", method, err)
	}

	return nil
}

// waitBackground blocks until every detached best-effort goroutine this
// client spawned (currently just the async $/cancelRequest send call
// dispatches - see call) has returned. This cannot hang indefinitely on its
// own: each such goroutine runs cancel, which bounds its own notify with
// cancelNotifyTimeout via context.WithTimeout(context.WithoutCancel(ctx),
// ...), independent of whatever ctx the original call was given - so the
// longest any tracked goroutine can still be running is cancelNotifyTimeout
// from when it was spawned, transport torn down or not. Session.Close calls
// this after tearing down the transport (see s.cancel), so in practice a
// wedged write typically errors out and this returns immediately instead of
// waiting out that bound.
func (c *client) waitBackground() {
	c.background.Wait()
}
