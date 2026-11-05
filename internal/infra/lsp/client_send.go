package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// cancelNotifyTimeout bounds how long a single $/cancelRequest send may
// block. It is threaded through context.WithoutCancel(ctx), which never
// fires Done on its own, and notify now actually honors ctx cancellation
// (see notify) - without this bound a wedged server would hang cancel
// forever instead of just dropping the best-effort notification.
const cancelNotifyTimeout = 5 * time.Second

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
