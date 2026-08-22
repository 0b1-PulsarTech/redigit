package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// methodNotFoundCode is the JSON-RPC error code for a server-to-client
// request this client does not implement.
const methodNotFoundCode = -32601

// cancelNotifyTimeout bounds how long a single $/cancelRequest send may
// block. It is threaded through context.WithoutCancel(ctx), which never
// fires Done on its own, and notify now actually honors ctx cancellation
// (see notify) - without this bound a wedged server would hang cancel
// forever instead of just dropping the best-effort notification.
const cancelNotifyTimeout = 5 * time.Second

// closeTimeout bounds how long Session.Close may take even when the caller
// passes a ctx with no deadline of its own (context.Background() is a legal
// value; nothing in this package's exported API forbids it). Close exists
// specifically to guarantee the session's resources stop, so it cannot
// simply inherit an undeadlined ctx and risk blocking forever behind a
// wedged shutdown call (see sendRacingCtx). This value is generous enough
// for a real language server's graceful shutdown/exit round trip - which
// normally completes in a small fraction of a second, even for a large
// workspace, since it is just two small frames over an already-open pipe -
// while still keeping Close's worst case bounded to a small, human-scale
// ceiling instead of an indefinite hang.
const closeTimeout = 5 * time.Second

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

// sendRacingCtx writes msg, returning promptly with ctx.Err() once ctx is
// done, even if the write is still in flight: on a shared stream,
// cancelling ctx cannot interrupt bytes already going out, only stop this
// call from waiting on them. The write's own goroutine runs to completion
// regardless (see notify and run's watcher goroutine). attempted reports
// whether the write was actually started before ctx won the race - call
// uses it to decide whether a best-effort $/cancelRequest could possibly
// still land: ctx firing before the write was ever attempted means the
// server never saw the request, so there is nothing to cancel.
func (c *client) sendRacingCtx(
	ctx context.Context,
	msg outgoingMessage,
) (attempted bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("%w", err)
	}

	done := make(chan error, 1)

	go func() { done <- c.send(msg) }()

	select {
	case err := <-done:
		return true, err
	case <-ctx.Done():
		return true, fmt.Errorf("%w", ctx.Err())
	}
}

// run reads frames until ctx is done or the transport fails, dispatching
// each to a pending caller, a minimal answer, or the drop path. It is the
// single owner of r and returns ctx.Err() once ctx.Done fires; the caller
// starts it in exactly one goroutine (an errgroup.Group entry is the
// intended shape).
func (c *client) run(ctx context.Context) error {
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-ctx.Done():
			_ = c.r.Close() // unblocks the blocking read below

			// Writes race ctx in sendRacingCtx (see notify), but an
			// abandoned write's own I/O keeps running and can hold
			// writeMu until it finishes on its own. Closing c.w here
			// (when it supports it) bounds that instead: it force-fails
			// the write once THIS ctx ends, freeing writeMu for others.
			if wc, ok := c.w.(io.Closer); ok {
				_ = wc.Close()
			}
		case <-stop:
		}
	}()

	reader := bufio.NewReader(c.r)

	for {
		raw, err := readFrame(reader)
		if err != nil {
			c.failPending(err)

			if cause := ctx.Err(); cause != nil {
				return fmt.Errorf("run: %w", cause)
			}

			return fmt.Errorf("read frame: %w", err)
		}

		var msg incomingMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.logger.Debug("lsp: decode message", slog.String("error", err.Error()))

			continue
		}

		c.dispatch(msg)
	}
}

func (c *client) dispatch(msg incomingMessage) {
	switch {
	case msg.Method == "":
		c.deliver(msg)
	case len(msg.ID) > 0:
		c.answerServerRequest(msg)
	// default: a notification from the server (window/logMessage,
	// $/progress, ...). This client's subset has no use for any of them.
	default:
	}
}

func (c *client) deliver(msg incomingMessage) {
	var id int64
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		c.logger.Debug("lsp: response with non-numeric id", slog.String("error", err.Error()))

		return
	}

	c.mu.Lock()
	ch, ok := c.pending[id]
	c.mu.Unlock()

	if ok {
		ch <- msg
	}
}

// answerServerRequest answers the small set of server-to-client requests
// gopls (and LSP servers in general) send unsolicited during a session, and
// rejects anything else with "method not found" so the server does not hang
// waiting for a reply this client will never send.
func (c *client) answerServerRequest(msg incomingMessage) {
	switch msg.Method {
	case "workspace/configuration":
		var params struct {
			Items []json.RawMessage `json:"items"`
		}

		_ = json.Unmarshal(msg.Params, &params)
		c.respondResult(msg.ID, make([]any, len(params.Items)))
	case "client/registerCapability",
		"client/unregisterCapability",
		"window/workDoneProgress/create":
		c.respondResult(msg.ID, nil)
	default:
		c.respondError(msg.ID, methodNotFoundCode, "method not found")
	}
}

// respondResult answers a server-to-client request with a success result. A
// JSON-RPC success response must carry a "result" key even when the value
// is null, so Result has no omitempty: dropping the key would make this
// response indistinguishable from one carrying neither result nor error.
func (c *client) respondResult(id json.RawMessage, result any) {
	c.writeJSON(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{JSONRPC: jsonrpcVersion, ID: id, Result: result})
}

func (c *client) respondError(id json.RawMessage, code int, message string) {
	c.writeJSON(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   *responseError  `json:"error"`
	}{JSONRPC: jsonrpcVersion, ID: id, Error: &responseError{Code: code, Message: message}})
}

// writeJSON marshals and writes v, logging (never returning) a failure: it
// backs the two server-request answers above, neither of which has anyone
// waiting on an error to propagate to.
func (c *client) writeJSON(v any) {
	body, err := json.Marshal(v)
	if err != nil {
		c.logger.Debug("lsp: encode message", slog.String("error", err.Error()))

		return
	}

	if err := c.writeFrame(body); err != nil {
		c.logger.Debug("lsp: send message", slog.String("error", err.Error()))
	}
}

// failPending delivers cause to every call still waiting on a response,
// which is what unblocks call once the transport is gone; a call whose own
// ctx is cancelled unblocks through its own select branch instead.
func (c *client) failPending(cause error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[int64]chan incomingMessage)
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- incomingMessage{Error: &responseError{Message: cause.Error()}}
	}
}

func (c *client) decodeResult(method string, msg incomingMessage, result any) error {
	if msg.Error != nil {
		if msg.Error.Code == cancelledCode {
			return fmt.Errorf("%s: %w", method, ErrCancelled)
		}

		return fmt.Errorf(
			"%s: %w",
			method,
			&ResponseError{Code: msg.Error.Code, Message: msg.Error.Message},
		)
	}

	if result != nil && len(msg.Result) > 0 {
		if err := json.Unmarshal(msg.Result, result); err != nil {
			return fmt.Errorf("unmarshal %s result: %w", method, err)
		}
	}

	return nil
}

// cancel notifies the server that rawID is no longer wanted. ctx is the
// just-expired call context: cancel detaches it from cancellation via
// context.WithoutCancel (keeping trace/request values) and bounds the
// result with its own short timeout, so a wedged server cannot hang this
// best-effort send forever.
func (c *client) cancel(ctx context.Context, rawID json.RawMessage) {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancelNotifyTimeout)
	defer cancel()

	err := c.notify(cancelCtx, "$/cancelRequest", cancelParams{ID: rawID})
	if err != nil {
		c.logger.Debug("lsp: send cancel", slog.String("error", err.Error()))
	}
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

func (c *client) send(msg outgoingMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	return c.writeFrame(body)
}

func (c *client) writeFrame(body []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := writeFrame(c.w, body); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}
