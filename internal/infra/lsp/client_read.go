package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	"github.com/wrapped-owls/redigit/pkg/jsonrpc"
)

// methodNotFoundCode is the JSON-RPC error code for a server-to-client
// request this client does not implement.
const methodNotFoundCode = -32601

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
		raw, err := jsonrpc.ReadFrame(reader)
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

func cleanExit(waitErr error) error {
	// After a completed shutdown/exit handshake the server decides how it ends: gopls v0.23.0
	// dies of SIGPIPE on its closed stdin, so any exit status, a signal included, is a clean close.
	if _, exited := errors.AsType[*exec.ExitError](waitErr); exited {
		return nil
	}

	return waitErr
}

func readerEndedCleanly(readErr error) bool {
	// A server may end first once exit is sent; a frame cut short is io.ErrUnexpectedEOF, an error.
	return errors.Is(readErr, context.Canceled) || errors.Is(readErr, io.EOF)
}
