package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// defaultPositionEncoding is what this client assumes when a server's
// initialize response leaves capabilities.positionEncoding unset, which the
// LSP spec requires reading as UTF-16 and which gopls always does regardless
// of what general.positionEncodings this client advertised.
const defaultPositionEncoding = "utf-16"

// ErrEmptyCommand reports a ServerConfig with no server command to launch.
var ErrEmptyCommand = errors.New("lsp: server command is empty")

// ErrNoDefinition reports that a definition request found nothing, which
// Session.Definition returns instead of a zero-value entities.Location a
// caller could mistake for a real one.
var ErrNoDefinition = errors.New("lsp: no definition found")

// ServerConfig configures which language server Session starts and how, so
// this package never hardcodes what "Go" means.
type ServerConfig struct {
	// Command and Args launch the server, e.g. "gopls", []string{"-mode=stdio"}.
	Command string
	Args    []string
	// RootPath is the workspace root directory. It is a plain path, not a
	// URI: encoding one is this package's job, not a caller's, and a caller
	// that has to build "file://..." by hand will eventually get a space or a
	// drive letter wrong.
	RootPath string
	// BuildFlags is REQUIRED and explicit on purpose: gopls answers
	// references/implementation/workspace-symbol only for files visible
	// under these flags, silently and without error otherwise. Passing
	// []string{"-tags=integration"} is what makes a file behind
	// //go:build integration visible to those queries; passing nil is a
	// conscious choice to exclude them, not an oversight this type lets
	// happen by accident.
	BuildFlags []string
	// InitializationOptions are extra flat gopls settings merged with
	// BuildFlags under the "buildFlags" key. gopls flattens nested JSON on
	// the last "."-separated segment, so this map must already be flat
	// (e.g. "buildFlags", not "build.buildFlags").
	InitializationOptions map[string]any
	// Logger receives debug-level transport diagnostics. Defaults to
	// slog.Default() when nil.
	Logger *slog.Logger
}

func (c ServerConfig) mergedInitOptions() map[string]any {
	merged := make(map[string]any, len(c.InitializationOptions)+1)
	maps.Copy(merged, c.InitializationOptions)
	merged["buildFlags"] = c.BuildFlags

	return merged
}

// Session starts one language server process and answers indexing.Resolver
// against it. It owns the process and the single reader goroutine that
// drains its stdout; Close stops both.
//
// See client's doc comment for a residual limitation Session inherits: a
// call or notify whose own ctx has already fired can still be stuck behind
// an unrelated wedged write sharing the same connection, until Close (or
// the process context otherwise ending) tears the whole connection down.
type Session struct {
	client *client
	cancel context.CancelFunc
	group  *errgroup.Group
	// wait blocks until the underlying transport is fully gone (the server
	// process has exited); kill is a best-effort forceful stop used only
	// when initialize fails. Both are seams: StartSession wires them to
	// cmd.Wait/cmd.Process.Kill, tests wire them to a fake transport.
	wait func() error
	kill func() error

	// root is the workspace directory. Paths cross this package's boundary
	// repo-relative, the way the rest of the tool names a file, and are
	// absolute inside it, the way the protocol requires. Joining and
	// un-joining here is the adapter absorbing the difference; a caller that
	// had to remember which side it was on would eventually get it wrong.
	root string

	positionEncoding string
}

var _ indexing.Resolver = (*Session)(nil)

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

// newSession wires a client onto the given stdio transport and starts its
// reader goroutine under processCtx. The caller owns processCtx/cancel:
// StartSession derives them from a detached context (see above); cancel is
// what Close calls to stop the reader loop, independently of the process
// itself (see the exec.Command note in StartSession).
func newSession(
	processCtx context.Context,
	cancel context.CancelFunc,
	stdin io.Writer,
	stdout io.ReadCloser,
	logger *slog.Logger,
	wait, kill func() error,
) *Session {
	rpcClient := newClient(stdin, stdout, logger)
	group, gctx := errgroup.WithContext(processCtx)
	group.Go(func() error { return rpcClient.run(gctx) })

	return &Session{
		client:           rpcClient,
		cancel:           cancel,
		group:            group,
		wait:             wait,
		kill:             kill,
		positionEncoding: defaultPositionEncoding,
	}
}

// PositionEncoding reports the code-unit encoding negotiated with the
// server for every Position.Character this Session sends or reads, so a
// caller building a Position from a byte offset knows which conversion in
// this package to apply. It defaults to "utf-16" per spec, which is also
// gopls's actual behavior regardless of what was negotiated.
func (s *Session) PositionEncoding() string {
	return s.positionEncoding
}

// References implements indexing.Resolver.
func (s *Session) References(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	position, err := s.textDocumentPositionParams(at)
	if err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}

	params := ReferenceParams{
		TextDocumentPositionParams: position,
		// The declaration itself is already known from documentSymbol;
		// references answers who calls it, not where it is.
		Context: ReferenceContext{IncludeDeclaration: false},
	}

	var result []Location
	if err := s.client.call(ctx, "textDocument/references", params, &result); err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}

	return s.toEntityLocations(result)
}

// Implementations implements indexing.Resolver.
func (s *Session) Implementations(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	params, err := s.textDocumentPositionParams(at)
	if err != nil {
		return nil, fmt.Errorf("implementations: %w", err)
	}

	var result locations
	if err := s.client.call(ctx, "textDocument/implementation", params, &result); err != nil {
		return nil, fmt.Errorf("implementations: %w", err)
	}

	return s.toEntityLocations(result)
}

// Definition implements indexing.Resolver.
func (s *Session) Definition(ctx context.Context, at entities.Location) (entities.Location, error) {
	params, err := s.textDocumentPositionParams(at)
	if err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	var result locations
	if err = s.client.call(ctx, "textDocument/definition", params, &result); err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	if len(result) == 0 {
		return entities.Location{}, fmt.Errorf("definition: %w", ErrNoDefinition)
	}

	found, err := s.toEntityLocation(result[0])
	if err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	return found, nil
}

// DocumentSymbols answers textDocument/documentSymbol for path. It is not
// part of indexing.Resolver: declaration boundaries are Syntax's concern,
// but exposing the raw query lets a Syntax adapter fall back to it for a
// language with no dedicated parser (see docs/PLANO.md 2.1).
func (s *Session) DocumentSymbols(ctx context.Context, path string) ([]DocumentSymbol, error) {
	uri, err := pathToURI(path)
	if err != nil {
		return nil, fmt.Errorf("document symbols: %w", err)
	}

	var result []DocumentSymbol

	params := DocumentSymbolParams{TextDocument: TextDocumentIdentifier{URI: uri}}
	if err := s.client.call(ctx, "textDocument/documentSymbol", params, &result); err != nil {
		return nil, fmt.Errorf("document symbols: %w", err)
	}

	return result, nil
}

// Close runs the shutdown/exit sequence and waits for the server process to
// exit, stopping the reader goroutine this Session owns; ctx past its
// deadline force-kills instead of blocking forever. Close's whole purpose
// is guaranteeing the session's resources stop, so it never simply trusts
// whatever ctx the caller passed: closeTimeout caps the total regardless of
// whether ctx carries its own deadline at all (context.Background() is
// legal here). context.WithTimeout composes correctly with a
// caller-supplied shorter deadline - the earlier of the two always wins -
// so this never weakens a caller's own tighter bound, it only adds a
// ceiling for the case where the caller passed none. A best-effort
// $/cancelRequest can still add up to cancelNotifyTimeout beyond that in
// edge cases (see waitBackground).
func (s *Session) Close(ctx context.Context) error {
	ctx, releaseBound := context.WithTimeout(ctx, closeTimeout)
	defer releaseBound()

	var errs []error

	if err := s.client.call(ctx, "shutdown", nil, nil); err != nil {
		errs = append(errs, fmt.Errorf("shutdown: %w", err))
	}

	if err := s.client.notify(ctx, "exit", nil); err != nil {
		errs = append(errs, fmt.Errorf("exit: %w", err))
	}

	// s.cancel stops the reader and, via run's watcher goroutine,
	// force-closes the writer once its ctx ends - this is what unblocks any
	// write still wedged on writeMu. waitBackground runs right after so a
	// detached cancel-notify goroutine (see client.call) errors out and
	// returns promptly against the now-torn-down transport instead of
	// running its own notify out to cancelNotifyTimeout; it runs before
	// s.group.Wait so a background write queued behind the reader's own
	// writeJSON contention cannot create ordering surprises with the
	// reader's own shutdown.
	s.cancel()
	s.client.waitBackground()

	if err := s.group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		errs = append(errs, fmt.Errorf("reader: %w", err))
	}

	if err := s.waitOrKill(ctx); err != nil {
		errs = append(errs, fmt.Errorf("wait: %w", err))
	}

	return errors.Join(errs...)
}

// waitOrKill waits for the server process to exit, racing s.wait() against
// ctx in its own goroutine so it can return the instant ctx fires without
// leaking. If ctx wins, it force-kills via s.kill, then drains waitDone
// (kill makes cmd.Wait return) so that goroutine still gets a stop
// condition instead of being abandoned.
func (s *Session) waitOrKill(ctx context.Context) error {
	waitDone := make(chan error, 1)
	go func() { waitDone <- s.wait() }()

	select {
	case err := <-waitDone:
		return err
	case <-ctx.Done():
		_ = s.kill()

		return errors.Join(ctx.Err(), <-waitDone)
	}
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

func (s *Session) textDocumentPositionParams(
	at entities.Location,
) (TextDocumentPositionParams, error) {
	uri, err := pathToURI(s.absolute(at.File))
	if err != nil {
		return TextDocumentPositionParams{}, err
	}

	return TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: at.Line, Character: at.Char},
	}, nil
}

func (s *Session) toEntityLocation(l Location) (entities.Location, error) {
	path, err := uriToPath(l.URI)
	if err != nil {
		return entities.Location{}, err
	}

	return entities.Location{
		File: s.relative(path),
		Line: l.Range.Start.Line,
		Char: l.Range.Start.Character,
	}, nil
}

func (s *Session) toEntityLocations(ls []Location) ([]entities.Location, error) {
	out := make([]entities.Location, 0, len(ls))

	for _, l := range ls {
		converted, err := s.toEntityLocation(l)
		if err != nil {
			return nil, err
		}

		out = append(out, converted)
	}

	return out, nil
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

// absolute resolves a repo-relative path against the workspace root. An
// already-absolute path is returned unchanged, so a caller working outside a
// workspace is not forced to invent one.
func (s *Session) absolute(path string) string {
	if s.root == "" || filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(s.root, path)
}

// relative is the inverse, so a location leaves this package named the way the
// rest of the tool names files.
func (s *Session) relative(path string) string {
	if s.root == "" {
		return path
	}

	rel, err := filepath.Rel(s.root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}

	return rel
}
