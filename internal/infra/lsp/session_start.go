package lsp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
)

// StartSession launches cfg.Command, performs the initialize/initialized
// handshake, and returns a Session ready to answer Resolver queries. The
// caller must call Close when done to stop the server process.
func StartSession(ctx context.Context, cfg ServerConfig) (*Session, error) {
	if cfg.Command == "" {
		return nil, ErrEmptyCommand
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// The reader goroutine outlives the ctx that started it - it is owned
	// by this Session, not by the caller's request - so it is derived from
	// a detached context, never ctx itself, and stopped only when Close (or
	// the initialize failure path below) cancels it.
	processCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	// Deliberately exec.Command, not exec.CommandContext: this process's
	// graceful shutdown (Close sends "shutdown" then "exit" and only then
	// stops the reader loop below) must run to completion before anything
	// kills it. Binding the command to processCtx would race that - Close
	// cancelling processCtx to stop the reader would also SIGKILL the
	// process before it finishes exiting on its own; kill (below) is the
	// deliberate, explicit path for that instead.
	cmd := exec.Command(cfg.Command, cfg.Args...) //nolint:noctx,gosec // see comment above

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()

		return nil, fmt.Errorf("open stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()

		return nil, fmt.Errorf("open stdout: %w", err)
	}

	cmd.Stderr = &stderrLog{logger: logger}

	if err := cmd.Start(); err != nil {
		cancel()

		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}

	session := newSession(
		processCtx,
		cancel,
		stdin,
		stdout,
		logger,
		cmd.Wait,
		func() error { return cmd.Process.Kill() },
	)

	if err := session.initialize(ctx, cfg); err != nil {
		session.cancel()
		_ = session.kill()
		_ = session.group.Wait()
		_ = session.wait()

		return nil, err
	}

	return session, nil
}

func (s *Session) initialize(ctx context.Context, cfg ServerConfig) error {
	s.root = cfg.RootPath

	// rootUri is nullable in the protocol, so a session with no workspace
	// root is legitimate. Only a non-empty path that is not absolute is a
	// caller mistake worth refusing.
	var rootURI string

	if cfg.RootPath != "" {
		encoded, err := pathToURI(cfg.RootPath)
		if err != nil {
			return fmt.Errorf("initialize: %w", err)
		}

		rootURI = encoded
	}

	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   rootURI,
		Capabilities: ClientCapabilities{
			General: GeneralClientCapabilities{
				PositionEncodings: []string{defaultPositionEncoding},
			},
			TextDocument: TextDocumentClientCapabilities{
				DocumentSymbol: DocumentSymbolClientCapabilities{
					HierarchicalDocumentSymbolSupport: true,
				},
			},
		},
		InitializationOptions: cfg.mergedInitOptions(),
	}

	var result InitializeResult
	if err := s.client.call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	if result.Capabilities.PositionEncoding != "" {
		s.positionEncoding = result.Capabilities.PositionEncoding
	}

	if err := s.client.notify(ctx, "initialized", struct{}{}); err != nil {
		return fmt.Errorf("initialized: %w", err)
	}

	return nil
}

// stderrLog forwards a language server's stderr to slog.Debug so parse
// errors and diagnostics are visible without the caller wiring anything up;
// it is not line-buffered, so one Write may not be one log line.
type stderrLog struct {
	logger *slog.Logger
}

var _ io.Writer = (*stderrLog)(nil)

func (w *stderrLog) Write(p []byte) (int, error) {
	w.logger.Debug("lsp: server stderr", slog.String("message", string(p)))

	return len(p), nil
}
