package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/pkg/jsonrpc"
)

// testLogger discards diagnostics so tests stay quiet; failures are asserted
// on behavior, never on log output.
func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// fakeServer drives the server side of an in-memory LSP stdio transport, so
// Client and Session tests exercise the real framing and dispatch code
// without spawning a real language server process.
type fakeServer struct {
	t      testing.TB
	reader *bufio.Reader
	writer io.WriteCloser
}

func newFakeServer(t testing.TB, r io.Reader, w io.WriteCloser) *fakeServer {
	t.Helper()

	return &fakeServer{t: t, reader: bufio.NewReader(r), writer: w}
}

// hangup closes the server's write side, so the client's next read fails
// with EOF - simulating the server process exiting or the pipe breaking.
func (s *fakeServer) hangup() {
	s.t.Helper()
	_ = s.writer.Close()
}

// next reads the next frame the client sent and decodes it generically; it
// does not distinguish requests from notifications, since fakeServer's
// callers already know which one they are waiting for.
func (s *fakeServer) next() incomingMessage {
	s.t.Helper()

	raw, err := jsonrpc.ReadFrame(s.reader)
	if err != nil {
		s.t.Fatalf("fakeServer: read frame: %v", err)
	}

	var msg incomingMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		s.t.Fatalf("fakeServer: decode message: %v", err)
	}

	return msg
}

func (s *fakeServer) respond(id json.RawMessage, result any) {
	s.t.Helper()
	s.send(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{JSONRPC: jsonrpcVersion, ID: id, Result: result})
}

func (s *fakeServer) respondErr(id json.RawMessage, code int, message string) {
	s.t.Helper()
	s.send(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   *responseError  `json:"error"`
	}{JSONRPC: jsonrpcVersion, ID: id, Error: &responseError{Code: code, Message: message}})
}

// sendRequest sends a server-to-client request, for tests exercising
// answerServerRequest.
func (s *fakeServer) sendRequest(id json.RawMessage, method string, params any) {
	s.t.Helper()
	s.send(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  any             `json:"params,omitempty"`
	}{JSONRPC: jsonrpcVersion, ID: id, Method: method, Params: params})
}

func (s *fakeServer) send(v any) {
	s.t.Helper()

	body, err := json.Marshal(v)
	if err != nil {
		s.t.Fatalf("fakeServer: encode: %v", err)
	}

	if err := jsonrpc.WriteFrame(s.writer, body); err != nil {
		s.t.Fatalf("fakeServer: write frame: %v", err)
	}
}

// pipeTransport wires the client side of an in-memory LSP stdio transport
// (two io.Pipes, one per direction) to a fakeServer driving the other side -
// the shape a subprocess's stdin/stdout would have. Both pipePair and
// fakeSession build on this so Client and Session tests exercise the same
// framing without spawning a real language server process.
func pipeTransport(
	t testing.TB,
) (clientWriter io.WriteCloser, clientReader io.ReadCloser, server *fakeServer) {
	t.Helper()

	var serverReader io.ReadCloser

	var serverWriter io.WriteCloser

	serverReader, clientWriter = io.Pipe() // client writes -> server reads
	clientReader, serverWriter = io.Pipe() // server writes -> client reads
	server = newFakeServer(t, serverReader, serverWriter)

	t.Cleanup(func() {
		_ = clientWriter.Close()
		_ = serverWriter.Close()
	})

	return clientWriter, clientReader, server
}

// pipePair wires a client to a fakeServer over pipeTransport.
func pipePair(t testing.TB) (*client, *fakeServer) {
	t.Helper()

	clientWriter, clientReader, server := pipeTransport(t)
	rpcClient := newClient(clientWriter, clientReader, testLogger())

	return rpcClient, server
}

// startSignalWriter wraps w and closes started exactly once, right before
// the first Write reaches w - so a test can know a blocking write has
// genuinely begun without racing on goroutine scheduling or sleeping. It
// forwards Close too, so it can also drive tests of run's watcher
// goroutine, which force-closes c.w through an io.Closer type assertion.
type startSignalWriter struct {
	w       io.WriteCloser
	started chan struct{}
	once    sync.Once
}

func (s *startSignalWriter) Write(p []byte) (int, error) {
	s.once.Do(func() { close(s.started) })

	n, err := s.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("write: %w", err)
	}

	return n, nil
}

func (s *startSignalWriter) Close() error {
	if err := s.w.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

// blockedWriterPair wires a client whose writer blocks inside the pipe
// until something reads it, so a test can prove a write has genuinely
// begun (via the returned started channel) without racing on goroutine
// scheduling or sleeping.
func blockedWriterPair(
	t testing.TB,
) (rpcClient *client, server *fakeServer, started chan struct{}) {
	t.Helper()

	clientWriter, clientReader, server := pipeTransport(t)
	started = make(chan struct{})
	writer := &startSignalWriter{w: clientWriter, started: started}
	rpcClient = newClient(writer, clientReader, testLogger())

	return rpcClient, server, started
}

// startRun runs client.run in its own goroutine, tracked by the returned
// wait function - the one owner discipline this package's reader goroutine
// requires. Callers cancel ctx (or let the test's own defer cancel it) and
// then call wait to observe run's exit.
func startRun(t testing.TB, ctx context.Context, client *client) (wait func() error) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- client.run(ctx) }()

	return func() error { return <-done }
}

// runningClient wires a client to a fakeServer over pipePair and starts its
// reader goroutine under a cancellable ctx, for tests that drive call/notify
// against a live run loop and tear it down explicitly via cancel then wait.
func runningClient(
	t testing.TB,
) (rpcClient *client, server *fakeServer, cancel context.CancelFunc, wait func() error) {
	t.Helper()

	rpcClient, server = pipePair(t)

	var ctx context.Context

	ctx, cancel = context.WithCancel(context.Background())
	wait = startRun(t, ctx, rpcClient)

	return rpcClient, server, cancel, wait
}

// runAsync runs fn in its own goroutine and returns a wait func that blocks
// until fn returns - for a void-returning call a test still wants to join
// against concurrent activity (e.g. draining the frame it blocks on).
func runAsync(fn func()) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	return func() { <-done }
}

// waitBounded runs fn in its own goroutine and reports whether it returned
// before timeout elapses, so a test can assert "this does not block" (e.g.
// a WaitGroup.Wait that should already be satisfied) without a fixed sleep.
func waitBounded(timeout time.Duration, fn func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
